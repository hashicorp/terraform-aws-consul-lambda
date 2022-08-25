package proxy_test

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/proxy"
)

func TestCloseErrors(t *testing.T) {
	l := proxy.NewListener(&proxy.Config{
		ListenFunc: func() (net.Listener, error) {
			return net.Listen("tcp", "localhost:0")
		},
		DialFunc: func() (net.Conn, error) {
			return nil, errors.New("error")
		},
	})
	l.Close()
	require.Error(t, l.Serve())
}
