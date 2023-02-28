// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxy_test

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/proxy"
)

// TestProxyHTTP tests that the proxy can successfully and correctly proxy L7 HTTP requests.
func TestProxyHTTP(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(b)
	}))
	u, err := url.Parse(httpServer.URL)
	require.NoError(t, err)

	listenFunc, addr := makeListenFunc(t)
	dialFunc := func() (net.Conn, error) {
		return net.Dial("tcp", u.Host)
	}
	cfg := []*proxy.Config{{ListenFunc: listenFunc, DialFunc: dialFunc}}

	// Create and start the proxy
	p := proxy.New(hclog.NewNullLogger(), cfg...)
	t.Cleanup(func() { p.Close() })
	go p.Serve()

	// Wait for the proxy to be ready before sending it requests.
	<-p.Wait()

	msg := "hello"
	httpRequest(t, http.MethodGet, fmt.Sprintf("http://%s", addr), msg, http.StatusOK, msg)
}

// TestProxyTCP tests that the proxy can successfully and correctly proxy L4 TCP traffic.
func TestProxyTCP(t *testing.T) {
	cases := map[string]struct {
		tls bool
	}{
		"insecure": {},
		"secure": {
			tls: true,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {

			var clientTLS, serverTLS *tls.Config
			if c.tls {
				clientCert, err := tls.LoadX509KeyPair("testdata/client-cert.pem", "testdata/client-key.pem")
				require.NoError(t, err)
				clientTLS = &tls.Config{Certificates: []tls.Certificate{clientCert}, InsecureSkipVerify: true}

				serverCert, err := tls.LoadX509KeyPair("testdata/server-cert.pem", "testdata/server-key.pem")
				require.NoError(t, err)
				serverTLS = &tls.Config{Certificates: []tls.Certificate{serverCert}}
			}

			server, err := NewTCPServer(serverTLS)
			require.NoError(t, err)

			listenFunc, addr := makeListenFunc(t)
			var dialFunc func() (net.Conn, error)

			if clientTLS != nil {
				dialFunc = func() (net.Conn, error) {
					return tls.Dial("tcp", server.Listener.Addr().String(), clientTLS)
				}

			} else {
				dialFunc = func() (net.Conn, error) {
					return net.Dial("tcp", server.Listener.Addr().String())
				}
			}

			cfg := []*proxy.Config{{ListenFunc: listenFunc, DialFunc: dialFunc}}

			// Create and start the proxy
			p := proxy.New(hclog.NewNullLogger(), cfg...)

			go func() {
				err := p.Serve()
				if err != nil {
					panic(err)
				}
			}()

			// Close the test server and the proxy on cleanup
			t.Cleanup(func() {
				server.Close()
				p.Close()
			})

			// Wait for the proxy to be ready before sending it requests.
			<-p.Wait()

			c := tcpClient{}

			msg := "hello"
			s, err := c.request(addr, msg)
			require.NoError(t, err)
			require.Equal(t, msg, s)
		})
	}
}

// TestProxyListenError tests that the proxy fails and everything gets cleaned up if an error occurs
// on the Listener's listenFunc.
func TestProxyListenError(t *testing.T) {
	listenFunc := func() (net.Listener, error) {
		return nil, errors.New("error")
	}
	dialFunc := func() (net.Conn, error) {
		return net.Dial("tcp", "localhost:1234")
	}
	cfg := []*proxy.Config{{ListenFunc: listenFunc, DialFunc: dialFunc}}

	// Create and start the proxy
	p := proxy.New(hclog.NewNullLogger(), cfg...)
	t.Cleanup(func() { p.Close() })
	require.Error(t, p.Serve())
}

// TestProxyDialError tests the case where the proxy is unable to connect to the upstream.
func TestProxyDialError(t *testing.T) {
	listenFunc, addr := makeListenFunc(t)
	dialFunc := func() (net.Conn, error) {
		return nil, errors.New("dial error")
	}
	cfg := []*proxy.Config{{ListenFunc: listenFunc, DialFunc: dialFunc}}

	// Create and start the proxy
	p := proxy.New(hclog.NewNullLogger(), cfg...)
	t.Cleanup(func() { p.Close() })

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		// We expect no error from the Serve func because the proxy listener is accepting connections.
		err := p.Serve()
		require.NoError(t, err)
		wg.Done()
	}(wg)

	// Wait for the proxy to be ready before sending it requests.
	<-p.Wait()

	// Make a request to the proxy. We expect the call to error because the proxy can't dial the upstream.
	c := tcpClient{}
	_, err := c.request(addr, "error")
	require.Error(t, err)

	// Close the proxy and wait for it to exit gracefully.
	p.Close()
	wg.Wait()
}

func makeListenFunc(t *testing.T) (func() (net.Listener, error), string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })

	lf := func() (net.Listener, error) {
		return l, nil
	}
	return lf, l.Addr().String()
}

func httpRequest(t *testing.T, method, url, reqBody string, statusCode int, resBody string) {
	hc := http.Client{}
	var reader io.Reader
	if len(reqBody) > 0 {
		reader = bytes.NewBufferString(reqBody)
	}
	request, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	resp, err := hc.Do(request)
	require.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, statusCode, resp.StatusCode)
	if len(resBody) > 0 {
		require.Equal(t, resBody, string(b))
	}
}

type tcpClient struct {
	Delay   time.Duration
	Timeout time.Duration
}

func (c *tcpClient) request(addr, body string) (string, error) {
	if c.Timeout == 0 {
		c.Timeout = time.Millisecond * 500
	}

	time.Sleep(c.Delay)

	var r string
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return r, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(c.Timeout))

	if len(body) > 0 {
		nw, err := conn.Write([]byte(body))
		if err != nil {
			return r, err
		}
		if nw != len(body) {
			return r, fmt.Errorf("invalid write: expected %d, wrote %d", len(body), nw)
		}
		b := make([]byte, 512)
		nr, err := conn.Read(b)
		if err != nil {
			return r, err
		}
		if nr != nw {
			return r, fmt.Errorf("failed to receive data, expected: %s, got: %s", body, string(b))
		}
		r = string(b[:nr])
	}
	return r, nil
}

type tcpServer struct {
	Listener net.Listener
	done     int32
}

func NewTCPServer(tlsConfig *tls.Config) (*tcpServer, error) {
	var err error
	s := &tcpServer{}
	if tlsConfig != nil {
		s.Listener, err = tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	} else {
		s.Listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return s, err
	}
	go s.Listen()
	return s, nil
}

func (s *tcpServer) Listen() error {
	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			if atomic.LoadInt32(&s.done) == 1 && errors.Is(err, net.ErrClosed) {
				// The server closed expectedly
				return nil
			}
			return err
		}
		go func(conn net.Conn) {
			defer conn.Close()
			io.Copy(conn, conn)
		}(conn)

		if atomic.LoadInt32(&s.done) == 1 {
			return nil
		}
	}
}

func (s *tcpServer) Close() {
	done := atomic.SwapInt32(&s.done, 1)
	if done == 0 {
		s.Listener.Close()
	}
}
