// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	ext "github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/consul-lambda-extension"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
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
	ca, caKey := generateTestCA(t, trustDomain)
	cert, pk := generateTestCert(t, ca, caKey, name, trustDomain, []string{fmt.Sprintf("%s.%s", name, trustDomain)}, []net.IP{net.ParseIP("127.0.0.1")})

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

func generateTestCA(t *testing.T, trustDomain string) (caPEM string, caKey *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: trustDomain,
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NotEmpty(t, certPEMBytes)

	return string(certPEMBytes), key
}

func generateTestCert(
	t *testing.T,
	caPEM string,
	caKey *rsa.PrivateKey,
	name string,
	trustDomain string,
	dnsNames []string,
	ipAddresses []net.IP,
) (certPEM string, keyPEM string) {
	t.Helper()

	caBlock, _ := pem.Decode([]byte(caPEM))
	require.NotNil(t, caBlock)

	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: name,
		},
		NotBefore:    now.Add(-1 * time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
		Issuer:       caCert.Subject,
		SubjectKeyId: []byte(trustDomain),
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NotEmpty(t, certPEMBytes)

	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	require.NotEmpty(t, keyPEMBytes)

	return string(certPEMBytes), string(keyPEMBytes)
}
