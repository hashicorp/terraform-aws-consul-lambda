package extension_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/tlsutil"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	ext "github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/extension"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/structs"
)

func TestExtension(t *testing.T) {
	const trustDomain = "1e6de438-c2bd-e632-0ce1-c0fa58607a45.consul"
	const name = "test"
	var wg sync.WaitGroup
	cfg := &ext.Config{
		MeshGatewayURI:      "mesh.gateway.consul:8443",
		ExtensionDataPrefix: "test",
		ServiceName:         "lambda-function",
		ServiceUpstreams:    []string{"upstream-1:1234", "upstream-2:1235"},
		Events:              MockEventProcessor{Wait: &wg},
		Logger:              hclog.NewNullLogger(),
		RefreshFrequency:    10 * time.Millisecond,
		ProxyTimeout:        time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())

	extData1 := generateExtensionData(t, name, trustDomain)
	extData2 := generateExtensionData(t, name, trustDomain)

	mpg := &MockParamGetter{
		t:             t,
		cancel:        cancel,
		wg:            &wg,
		path:          "test/default/default/lambda-function",
		extensionData: []string{extData1, extData1, extData2},
	}
	cfg.Store = mpg
	wg.Add(1)

	e := ext.NewExtension(cfg)

	go func() {
		err := e.Start(ctx)
		if err != nil {
			// If serve failed with an error, then we need to explicitly call wg.Done
			// to end the test.
			wg.Done()
		}
		require.NoError(t, err)
	}()

	wg.Wait()
}

type MockParamGetter struct {
	t             *testing.T
	cancel        context.CancelFunc
	wg            *sync.WaitGroup
	path          string
	extensionData []string
	idx           int
}

func (m *MockParamGetter) Get(_ context.Context, key string) (string, error) {
	require.True(m.t, m.idx < len(m.extensionData))
	require.Equal(m.t, m.path, key)

	ed := m.extensionData[m.idx]

	// If the expected number of calls has been reached end the test.
	m.idx++
	if m.idx >= len(m.extensionData) {
		defer m.cancel()
		defer m.wg.Done()
	}

	return ed, nil
}

type MockEventProcessor struct {
	Wait *sync.WaitGroup
}

func (m MockEventProcessor) Register(_ context.Context, _ interface{}) error {
	return nil
}

func (m MockEventProcessor) ProcessEvents(ctx context.Context) error {
	m.Wait.Add(1)
	defer m.Wait.Done()
	<-ctx.Done()
	return nil
}

func generateExtensionData(t *testing.T, name, trustDomain string) string {
	ca, caKey, err := tlsutil.GenerateCA(tlsutil.CAOpts{Domain: trustDomain})
	require.NoError(t, err)

	signer, err := tlsutil.ParseSigner(caKey)
	require.NoError(t, err)

	cert, pk, err := tlsutil.GenerateCert(tlsutil.CertOpts{
		CA:          ca,
		Signer:      signer,
		Name:        name,
		DNSNames:    []string{fmt.Sprintf("%s.%s", name, trustDomain)},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	require.NoError(t, err)

	ed := structs.ExtensionData{
		PrivateKeyPEM: pk,
		CertPEM:       cert,
		RootCertPEM:   ca,
		TrustDomain:   trustDomain,
	}

	edJSON, err := json.Marshal(ed)
	require.NoError(t, err)
	return string(edJSON)
}
