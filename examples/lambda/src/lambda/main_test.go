// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestHandleRequest(t *testing.T) {
	respHello1 := map[string]interface{}{"msg": "hello 1"}
	respHello2 := map[string]interface{}{"msg": "hello 2"}
	cases := map[string]struct {
		request  string
		response map[string]interface{}
		env      map[string]string
		handlers map[string]handler
	}{
		"mesh to lambda: primitive": {
			request: `{"body":"hello"}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body":       "hello",
			},
		},
		"mesh to lambda: object": {
			request: `{"body":{"msg":"hello"}}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body":       map[string]interface{}{"msg": "hello"},
			},
		},
		"mesh to lambda: payload passthrough primitive": {
			request: `"hello"`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body":       "hello",
			},
		},
		"mesh to lambda: payload passthrough object": {
			request: `{"msg":"hello"}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body":       map[string]interface{}{"msg": "hello"},
			},
		},
		"mesh to lambda: nil body": {
			request: `{"body":null}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
			},
		},
		"mesh to lambda: payload passthrough nil body": {
			request: "",
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
			},
		},
		"lambda to mesh": {
			env: map[string]string{"UPSTREAMS": "http://localhost:1234,http://localhost:1235"},
			handlers: map[string]handler{
				"http://localhost:1234": {statusCode: http.StatusOK, response: respHello1},
				"http://localhost:1235": {statusCode: http.StatusOK, response: respHello2},
			},
			request: `{"body":{"lambda-to-mesh":true}}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body": UpstreamResponseList{
					{Name: "http://localhost:1234", Code: http.StatusOK, Body: respHello1},
					{Name: "http://localhost:1235", Code: http.StatusOK, Body: respHello2},
				},
			},
		},
		"lambda to mesh payload passthrough": {
			env: map[string]string{"UPSTREAMS": "http://localhost:1234,http://localhost:1235"},
			handlers: map[string]handler{
				"http://localhost:1234": {statusCode: http.StatusOK, response: respHello1},
				"http://localhost:1235": {statusCode: http.StatusOK, response: respHello2},
			},
			request: `{"lambda-to-mesh":true}`,
			response: map[string]interface{}{
				"statusCode": http.StatusOK,
				"body": UpstreamResponseList{
					{Name: "http://localhost:1234", Code: http.StatusOK, Body: respHello1},
					{Name: "http://localhost:1235", Code: http.StatusOK, Body: respHello2},
				},
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range c.env {
				require.NoError(t, os.Setenv(k, v))
			}
			t.Cleanup(func() {
				for k := range c.env {
					require.NoError(t, os.Unsetenv(k))
				}
			})

			// Create an HTTP server for each upstream in the test.
			// This step is skipped for the mesh to lambda cases.
			var upstreams []string
			if rawUpstreams := os.Getenv("UPSTREAMS"); rawUpstreams != "" {
				upstreams = strings.Split(rawUpstreams, ",")
			}
			for _, upstream := range upstreams {
				handler, ok := c.handlers[upstream]
				require.True(t, ok, fmt.Sprintf("test setup error: missing handler for %s", upstream))
				startServer(t, upstream, handler)
			}

			var request interface{}
			if c.request != "" {
				require.NoError(t, json.Unmarshal([]byte(c.request), &request))
			}
			response, err := HandleRequest(request)
			require.NoError(t, err)
			require.True(t, cmp.Equal(c.response, response), cmp.Diff(c.response, response))
		})
	}
}

func startServer(t *testing.T, upstream string, h handler) {
	u, err := url.Parse(upstream)
	require.NoError(t, err)

	s := &http.Server{
		Addr:    u.Host,
		Handler: h,
	}
	go func() {
		err := s.ListenAndServe()
		require.Equal(t, err, http.ErrServerClosed)
	}()
	t.Cleanup(func() { s.Close() })

	// wait for the server to start
	require.NoError(t, retryFunc(time.Millisecond, time.Second, func() error {
		_, err := http.Get(upstream)
		return err
	}))
}

func retryFunc(i, t time.Duration, f func() error) error {
	var err error
	stop := time.Now().Add(t)
	for time.Now().Before(stop) {
		err = f()
		if err == nil {
			return nil
		}
		time.Sleep(i)
	}
	return err
}

type handler struct {
	statusCode int
	response   interface{}
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.statusCode == 0 {
		h.statusCode = http.StatusOK
	}
	w.WriteHeader(h.statusCode)
	if h.response != nil {
		if b, err := json.Marshal(h.response); err == nil {
			w.Write(b)
		}
	}
}
