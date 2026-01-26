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
	caCertPEM, caKey, err := generateTestCA(trustDomain)
	require.NoError(t, err)

	certPEM, pkPEM, err := generateTestLeafCert(caCertPEM, caKey, name, trustDomain)
	require.NoError(t, err)

	ed := structs.ExtensionData{
		PrivateKeyPEM: pkPEM,
		CertPEM:       certPEM,
		RootCertPEM:   caCertPEM,
		TrustDomain:   trustDomain,
	}

	edJSON, err := json.Marshal(ed)
	require.NoError(t, err)
	return string(edJSON)
}

func generateTestCA(trustDomain string) (certPEM string, caKey *rsa.PrivateKey, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: trustDomain},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", nil, err
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return string(certPEMBytes), key, nil
}

func generateTestLeafCert(caCertPEM string, caKey *rsa.PrivateKey, name, trustDomain string) (certPEM string, keyPEM string, err error) {
	caBlock, _ := pem.Decode([]byte(caCertPEM))
	if caBlock == nil {
		return "", "", fmt.Errorf("failed to decode CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return "", "", err
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    now.Add(-1 * time.Minute),
		NotAfter:     now.Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{fmt.Sprintf("%s.%s", name, trustDomain)},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}

	certPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return string(certPEMBytes), string(keyPEMBytes), nil
}
