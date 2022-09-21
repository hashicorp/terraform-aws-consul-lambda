package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/proxy"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/trace"
)

type Config struct {
	ServiceName         string        `ignored:"true"`
	ServiceNamespace    string        `envconfig:"CONSUL_SERVICE_NAMESPACE"`
	ServicePartition    string        `envconfig:"CONSUL_SERVICE_PARTITION"`
	ServiceUpstreams    []string      `envconfig:"CONSUL_SERVICE_UPSTREAMS"`
	MeshGatewayURI      string        `envconfig:"CONSUL_MESH_GATEWAY_URI" required:"true"`
	ExtensionDataPrefix string        `envconfig:"CONSUL_EXTENSION_DATA_PREFIX" required:"true"`
	RefreshFrequency    time.Duration `envconfig:"CONSUL_REFRESH_FREQUENCY" default:"5m"`
	ProxyTimeout        time.Duration `envconfig:"CONSUL_EXTENSION_PROXY_TIMEOUT" default:"3s"`

	Store  ParamGetter
	Events EventProcessor
	Logger hclog.Logger
}

type ParamGetter interface {
	// Get the value for the given key.
	Get(ctx context.Context, key string) (string, error)
}

type EventProcessor interface {
	// Register the event processor.
	Register(ctx context.Context, i interface{}) error
	// ProcessEvents handles events until the provided context is cancelled or an error occurs.
	ProcessEvents(ctx context.Context) error
}

type Extension struct {
	*Config
	service   structs.Service
	proxy     *proxy.Server
	dataMutex sync.RWMutex
	data      structs.ExtensionData
	dataInit  bool
	upstreams []*structs.Service
}

// NewExtension returns an instance of the Extension from the given configuration.
func NewExtension(cfg *Config) *Extension {
	return &Extension{
		Config: cfg,
		service: structs.Service{
			Name:           cfg.ServiceName,
			EnterpriseMeta: structs.NewEnterpriseMeta(cfg.ServicePartition, cfg.ServiceNamespace),
		},
	}
}

// Start executes the main processing loop for the extension.
// It initializes and starts the proxy server and starts monitoring for incoming
// events from the Lambda runtime.
// It periodically retrieves the extension data from the parameter store and updates
// the proxy configuration for the configured upstreams as necessary.
func (ext *Extension) Start(ctx context.Context) error {
	trace.Enter()
	defer trace.Exit()

	ctx, cancel := context.WithCancel(ctx)

	// Cancel the context when this func returns to trigger resource cleanup.
	defer cancel()

	// Parse the upstreams configuration.
	err := ext.parseUpstreams()
	if err != nil {
		return err
	}

	errChan := make(chan error)

	// Start the proxy server and initialize all the upstream listeners so that the extension
	// is ready to accept connections from the Lambda function as soon as it starts.
	// The proxy server runs in a go-routine and reports any asynchronous errors via the
	// errChan.
	err = ext.startProxy(ctx, errChan)
	if err != nil {
		return err
	}

	go ext.refreshExtensionData(ctx, errChan)
	go ext.runEvents(ctx, errChan)

	// Run until either the proxy returns, the event processing loop returns, or
	// the extension data refresh loop returns.
	return <-errChan
}

func (ext *Extension) refreshExtensionData(ctx context.Context, errChan chan error) {
	trace.Enter()
	defer trace.Exit()

	// Fetch the initial extension data.
	err := ext.getExtensionData(ctx)
	if err != nil {
		errChan <- err
		return
	}

	refresh := time.NewTicker(ext.RefreshFrequency)
	defer refresh.Stop()

	for {
		select {
		case <-ctx.Done():
			errChan <- nil
			return
		case <-refresh.C:
			// The refresh interval has expired so update the extension data.
			err := ext.getExtensionData(ctx)
			if err != nil {
				errChan <- err
				return
			}
		}
	}
}

func (ext *Extension) runEvents(ctx context.Context, errChan chan error) {
	trace.Enter()
	defer trace.Exit()

	ext.Logger.Info("processing events")
	err := ext.Events.ProcessEvents(ctx)
	if err != nil {
		errChan <- fmt.Errorf("event processing failed with an error: %w", err)
		return
	}
	ext.Logger.Info("event processing completed")
	errChan <- nil
}

// getExtensionData asynchronously retrieves the extension data and updates the local cache.
func (ext *Extension) getExtensionData(ctx context.Context) error {
	trace.Enter()
	defer trace.Exit()

	// If the extension data has not yet been initialized then we need to lock the
	// extension data mutex so that the proxy's dial func will wait for the initial
	// fetch to complete before attempting to dial out.
	if !ext.dataInit {
		ext.dataMutex.Lock()
		defer ext.dataMutex.Unlock()

		// Defer setting ext.dataInit to true until after the func exits so we don't
		// deadlock when updating the extension data below.
		defer func() {
			ext.dataInit = true
		}()
	}

	ext.Logger.Info("retrieving extension data")

	// Retrieve the data.
	key := fmt.Sprintf("%s%s", ext.ExtensionDataPrefix, ext.service.ExtensionPath())
	d, err := ext.Store.Get(ctx, key)
	if err != nil {
		// TODO: Handle the distinction between transient errors from parameter store and the case where the
		// the function's mTLS material has been deleted because the function was removed from the mesh.
		// For transient errors we should just attempt to use the existing extension data and log any failures.
		// For now we just err out to make sure that the function can't make any more outgoing calls.
		return fmt.Errorf("failed to get extension data for %s: %w", key, err)
	}

	// Unmarshal.
	var extData structs.ExtensionData
	if err := json.Unmarshal([]byte(d), &extData); err != nil {
		return fmt.Errorf("failed to unmarshal extension data for %s: %w", key, err)
	}

	// If the extension data has changed then update the cached copy.
	if !extData.Equals(ext.data) {
		if ext.dataInit {
			ext.dataMutex.Lock()
			defer ext.dataMutex.Unlock()
		}
		ext.data = extData

		// We get the trust domain from the extension data so update the trust domain for each upstream.
		for idx := range ext.upstreams {
			ext.upstreams[idx].TrustDomain = ext.data.TrustDomain
		}
	}

	return nil
}

// startProxy starts, or restarts, the extension's proxy server.
// It retrieves the configuration for the proxy and if the configuration has changed
// it closes the existing proxy server and reconfigures a new proxy server.
// It starts the new proxy server in a separate go routine that reports any errors to the
// caller via errChan.
func (ext *Extension) startProxy(ctx context.Context, errChan chan error) error {
	trace.Enter()
	defer trace.Exit()

	ext.Logger.Info("starting proxy server")

	// Create a proxy listener configuration for each upstream.
	proxyConfigs := make([]*proxy.Config, len(ext.upstreams))
	for i, upstream := range ext.upstreams {
		// Create the listener config.
		ext.Logger.Debug("configuring upstream", "name", upstream.Name, "port", upstream.Port)
		proxyConfigs[i] = ext.proxyConfig(upstream)
	}

	// Create and start the proxy server.
	ext.proxy = proxy.New(ext.Logger, proxyConfigs...)
	go func(errChan chan error) {
		defer ext.proxy.Close()
		errChan <- ext.proxy.Serve()
	}(errChan)

	// Wait for the proxy to be ready to serve requests before returning.
	timeout := time.NewTicker(ext.ProxyTimeout)
	defer timeout.Stop()
	select {
	case <-ctx.Done():
		ext.Logger.Info("context closed while waiting for proxy to start.")
	case <-ext.proxy.Wait():
		ext.Logger.Info("proxy server ready")
	case <-timeout.C:
		return fmt.Errorf("timeout waiting for proxy to start")
	}

	return nil
}

func (ext *Extension) proxyConfig(upstream *structs.Service) *proxy.Config {
	trace.Enter()
	defer trace.Exit()

	cfg := &proxy.Config{}

	// Listen on the upstream's port on all interfaces.
	cfg.ListenFunc = func() (net.Listener, error) {
		return net.Listen("tcp", fmt.Sprintf(":%d", upstream.Port))
	}

	// Wrap the outgoing request in an mTLS session and dial the mesh gateway.
	cfg.DialFunc = func() (net.Conn, error) {
		// Get the lock for the extension data to ensure that this func picks up the
		// latest config. This also ensures that the extension data doesn't get updated
		// while we are dialing out.
		ext.dataMutex.RLock()
		defer ext.dataMutex.RUnlock()

		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM([]byte(ext.data.RootCertPEM))

		cert, err := tls.X509KeyPair([]byte(ext.data.CertPEM), []byte(ext.data.PrivateKeyPEM))
		if err != nil {
			return nil, err
		}

		ext.Logger.Debug("dialing upstream", "sni", upstream.SNI(), "port", upstream.Port)

		return tls.Dial("tcp", ext.MeshGatewayURI, &tls.Config{
			RootCAs:            roots,
			Certificates:       []tls.Certificate{cert},
			ServerName:         upstream.SNI(),
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				certs := make([]*x509.Certificate, len(rawCerts))
				for i, asn1Data := range rawCerts {
					cert, err := x509.ParseCertificate(asn1Data)
					if err != nil {
						return fmt.Errorf("failed to parse tls certificate from peer: %w", err)
					}
					certs[i] = cert
				}

				opts := x509.VerifyOptions{
					Roots:         roots,
					Intermediates: x509.NewCertPool(),
				}

				// All but the first cert are intermediates.
				for _, cert := range certs[1:] {
					opts.Intermediates.AddCert(cert)
				}

				_, err := certs[0].Verify(opts)
				if err != nil {
					return err
				}

				// Match the SPIFFE ID.
				if !strings.EqualFold(certs[0].URIs[0].String(), upstream.SpiffeID()) {
					return fmt.Errorf("invalid SPIFFE ID for upstream %s", upstream.Name)
				}
				return nil
			},
		})
	}

	return cfg
}

func (ext *Extension) parseUpstreams() error {
	trace.Enter()
	defer trace.Exit()

	ext.upstreams = make([]*structs.Service, 0, len(ext.ServiceUpstreams))
	for _, s := range ext.ServiceUpstreams {
		up, err := structs.ParseUpstream(s)
		if err != nil {
			return fmt.Errorf("failed to parse upstream: %w", err)
		}
		ext.upstreams = append(ext.upstreams, &up)
	}
	return nil
}
