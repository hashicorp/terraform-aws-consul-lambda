package extension_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"

	ext "github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/extension"
)

func TestExtension(t *testing.T) {
	cfg := &ext.Config{
		MeshGatewayURI:      "mesh.gateway.consul:8443",
		ExtensionDataPrefix: "test",
		ServiceName:         "lambda-function",
		ServiceUpstreams:    []string{"upstream-1:1234", "upstream-2:1235"},
		Store:               MockParamGetter{},
		Events:              MockEventProcessor{},
		Logger:              hclog.Default(),
	}

	e := ext.NewExtension(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go e.Serve(ctx)

	time.Sleep(time.Second)
	cancel()
	time.Sleep(time.Second)
}

type MockParamGetter struct{}

func (m MockParamGetter) Get(_ context.Context, _ string) (string, error) {
	return extensionData, nil
}

type MockEventProcessor struct{}

func (m MockEventProcessor) Register(_ context.Context, _ interface{}) error {
	return nil
}

func (m MockEventProcessor) ProcessEvents(ctx context.Context) error {
	fmt.Println("MOCK PROCESSING EVENTS")
	<-ctx.Done()
	fmt.Println("MOCK PROCESSING COMPLETED")
	return nil
}

const extensionData = `{"privateKey":"-----BEGIN EC PRIVATE KEY-----\nMHcCAQEEIBOESs619TDc4W+v2pT1B4HcIm5YsOpdWGcrUr8CaAxXoAoGCCqGSM49\nAwEHoUQDQgAEFIbwX0KkjD2z247PF5QDM6KV0oMtJYJvqT7tvE7aJNvcVI3UqHt9\nPVDyObnIw8ezr49WBCYAwIZsbsBWFs+kUQ==\n-----END EC PRIVATE KEY-----\n","cert":"-----BEGIN CERTIFICATE-----\nMIICJDCCAcmgAwIBAgIBDTAKBggqhkjOPQQDAjAxMS8wLQYDVQQDEyZwcmktMWgz\naXZqcXIuY29uc3VsLmNhLjFlNmRlNDM4LmNvbnN1bDAeFw0yMjA4MTUxOTExMzVa\nFw0yMjA4MTgxOTExMzVaMAAwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAQUhvBf\nQqSMPbPbjs8XlAMzopXSgy0lgm+pPu28Ttok29xUjdSoe309UPI5ucjDx7Ovj1YE\nJgDAhmxuwFYWz6RRo4IBATCB/jAOBgNVHQ8BAf8EBAMCA7gwHQYDVR0lBBYwFAYI\nKwYBBQUHAwIGCCsGAQUFBwMBMAwGA1UdEwEB/wQCMAAwKQYDVR0OBCIEIJaBEXNd\nFjRFqu84YweH5HbE5i2LnRKS6jrjjKIt05QqMCsGA1UdIwQkMCKAICke+V39fWi7\ni7sSSu3z61tg5zHelIPeHo7EF/ODP6aKMGcGA1UdEQEB/wRdMFuGWXNwaWZmZTov\nLzFlNmRlNDM4LWMyYmQtZTYzMi0wY2UxLWMwZmE1ODYwN2E0NS5jb25zdWwvbnMv\nZGVmYXVsdC9kYy9kYzEvc3ZjL2xhbWJkYS1zZXJ2aWNlMAoGCCqGSM49BAMCA0kA\nMEYCIQC8jGhSMn3H9ME9tamBwwOLH1l78S0AjaJN9plQgLdtOwIhANVnYkOaBQdK\n4GoRF96mtwqJ2X0fosLYBG2pUKgDuw0l\n-----END CERTIFICATE-----\n","rootCert":"-----BEGIN CERTIFICATE-----\nMIICEDCCAbWgAwIBAgIBBzAKBggqhkjOPQQDAjAxMS8wLQYDVQQDEyZwcmktMWgz\naXZqcXIuY29uc3VsLmNhLjFlNmRlNDM4LmNvbnN1bDAeFw0yMjA4MTUxNDQyNTZa\nFw0zMjA4MTIxNDQyNTZaMDExLzAtBgNVBAMTJnByaS0xaDNpdmpxci5jb25zdWwu\nY2EuMWU2ZGU0MzguY29uc3VsMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEPg8b\n92nHCFRObrt/A2ZlyIpDa7pfUJsMkeom04RqFiLDta9sgPx63qTBwyRLAvmCmBoC\nD9nUAJ0lHVN4jlRC9aOBvTCBujAOBgNVHQ8BAf8EBAMCAYYwDwYDVR0TAQH/BAUw\nAwEB/zApBgNVHQ4EIgQgKR75Xf19aLuLuxJK7fPrW2DnMd6Ug94ejsQX84M/poow\nKwYDVR0jBCQwIoAgKR75Xf19aLuLuxJK7fPrW2DnMd6Ug94ejsQX84M/poowPwYD\nVR0RBDgwNoY0c3BpZmZlOi8vMWU2ZGU0MzgtYzJiZC1lNjMyLTBjZTEtYzBmYTU4\nNjA3YTQ1LmNvbnN1bDAKBggqhkjOPQQDAgNJADBGAiEA3QMknYjieerzcpMZhPXA\n/nI/HVoTnh/E95WdVDwcMYgCIQDjdESSjPoikuRaN5IMZoZwO6/aHmI/WH3I7xFS\nqS3Zjw==\n-----END CERTIFICATE-----\n","trustDomain":"1e6de438-c2bd-e632-0ce1-c0fa58607a45.consul"}`
