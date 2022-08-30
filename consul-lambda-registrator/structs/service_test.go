package structs_test

import (
	"fmt"
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
			str:  fmt.Sprintf("%s:%d", svc, port),
			sni:  fmt.Sprintf("%s.default.dc1.%s.%s", svc, internal, td),
			sid:  fmt.Sprintf("spiffe://%s/ns/default/dc/dc1/svc/%s", td, svc),
			path: fmt.Sprintf("/default/default/%s", svc),
		},
		"service, ns": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, EnterpriseMeta: &structs.EnterpriseMeta{Namespace: ns, Partition: "default"}},
			str:  fmt.Sprintf("%s.%s:%d", svc, ns, port),
			sni:  fmt.Sprintf("%s.%s.dc1.%s.%s", svc, ns, internal, td),
			sid:  fmt.Sprintf("spiffe://%s/ns/%s/dc/dc1/svc/%s", td, ns, svc),
			path: fmt.Sprintf("/default/%s/%s", ns, svc),
		},
		"service, ns, ap": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, EnterpriseMeta: &structs.EnterpriseMeta{Namespace: ns, Partition: ap}},
			str:  fmt.Sprintf("%s.%s.%s:%d", svc, ns, ap, port),
			sni:  fmt.Sprintf("%s.%s.%s.dc1.%s.%s", svc, ns, ap, internalVersion, td),
			sid:  fmt.Sprintf("spiffe://%s/ap/%s/ns/%s/dc/dc1/svc/%s", td, ap, ns, svc),
			path: fmt.Sprintf("/%s/%s/%s", ap, ns, svc),
		},
		"service, ns, ap, dc": {
			up:   structs.Service{TrustDomain: td, Name: svc, Port: port, Datacenter: dc, EnterpriseMeta: &structs.EnterpriseMeta{Namespace: ns, Partition: ap}},
			str:  fmt.Sprintf("%s.%s.%s:%d:%s", svc, ns, ap, port, dc),
			sni:  fmt.Sprintf("%s.%s.%s.%s.%s.%s", svc, ns, ap, dc, internalVersion, td),
			sid:  fmt.Sprintf("spiffe://%s/ap/%s/ns/%s/dc/%s/svc/%s", td, ap, ns, dc, svc),
			path: fmt.Sprintf("/%s/%s/%s", ap, ns, svc),
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
			obs, err := structs.ParseService(c.str)
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
