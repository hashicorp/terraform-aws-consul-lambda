package structs_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/structs"
)

const (
	svc  = "test-service"
	port = 1234
	ns   = "ns1"
	ap   = "ap1"
	dc   = "dc2"
	td   = "ba471007-78d1-3261-2e02-24258f2cb341.consul"

	internal        = "internal"
	version         = "v1"
	internalVersion = internal + "-" + version
)

func TestService(t *testing.T) {

	cases := map[string]struct {
		str  string
		up   structs.Service
		sni  string
		sid  string
		path string
		err  string
	}{
		"service only": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port},
			str:  "test-service:1234",
			sni:  "test-service.default.dc1.internal.ba471007-78d1-3261-2e02-24258f2cb341.consul",
			sid:  "spiffe://ba471007-78d1-3261-2e02-24258f2cb341.consul/ns/default/dc/dc1/svc/test-service",
			path: "/default/default/test-service",
		},
		"service, ns": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, Namespace: ns},
			str:  "test-service.ns1:1234",
			sni:  "test-service.ns1.dc1.internal.ba471007-78d1-3261-2e02-24258f2cb341.consul",
			sid:  "spiffe://ba471007-78d1-3261-2e02-24258f2cb341.consul/ns/ns1/dc/dc1/svc/test-service",
			path: "/default/ns1/test-service",
		},
		"service, ns, ap": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, Namespace: ns, Partition: ap},
			str:  "test-service.ns1.ap1:1234",
			sni:  "test-service.ns1.ap1.dc1.internal-v1.ba471007-78d1-3261-2e02-24258f2cb341.consul",
			sid:  "spiffe://ba471007-78d1-3261-2e02-24258f2cb341.consul/ap/ap1/ns/ns1/dc/dc1/svc/test-service",
			path: "/ap1/ns1/test-service",
		},
		"service, ns, ap, dc": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, Namespace: ns, Partition: ap, Datacenter: dc},
			str:  "test-service.ns1.ap1:1234:dc2",
			sni:  "test-service.ns1.ap1.dc2.internal-v1.ba471007-78d1-3261-2e02-24258f2cb341.consul",
			sid:  "spiffe://ba471007-78d1-3261-2e02-24258f2cb341.consul/ap/ap1/ns/ns1/dc/dc2/svc/test-service",
			path: "/ap1/ns1/test-service",
		},
		"invalid service format": {
			up:  structs.Service{},
			str: svc,
			err: "invalid service format",
		},
		"invalid port": {
			up:  structs.Service{},
			str: "invalid:port",
			err: "invalid service port",
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			c := c
			t.Parallel()
			obs, err := structs.ParseUpstream(c.str)
			obs.TrustDomain = td
			if len(c.err) == 0 {
				require.NoError(t, err)
				require.True(t, cmp.Equal(c.up, obs), cmp.Diff(c.up, obs))
				require.Equal(t, c.sni, obs.SNI())
				require.Equal(t, c.sid, obs.SpiffeID())
				require.Equal(t, c.path, obs.ExtensionPath())
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.err)
			}
		})
	}
}
