package extension

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

	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/proxy"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/structs"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/trace"
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
	service    structs.Service
	proxy      *proxy.Server
	proxyMutex sync.Mutex
	data       structs.ExtensionData
	upstreams  []structs.Service
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

	// Cleanup on return. Cancel the context and close the proxy.
	defer func() {
		ext.proxyMutex.Lock()
		defer ext.proxyMutex.Unlock()
		ext.closeProxy()
	}()
	defer cancel()

	errChan := make(chan error)

	// The first time the proxy starts we need to wait for it to be completely
	// initialized and ready to accept connections before we start event processing.
	// Starting the event processing loop signals to the Lambda runtime that the extension
	// has completed its initialization.
	// The runProxy func will close the readyChan once the proxy is initialized and ready.
	readyChan := make(chan struct{})
	go ext.runProxy(ctx, readyChan, errChan)
	<-readyChan

	go ext.runEvents(ctx, errChan)

	// Run until either the proxy returns or until the event processing loop returns.
	return <-errChan
}

func (ext *Extension) runProxy(ctx context.Context, readyChan chan struct{}, errChan chan error) {
	// Fire up the proxy for the first time.
	pErrChan := make(chan error)
	err := ext.startProxy(ctx, pErrChan)

	// After startProxy returns, all listeners are initialized and the proxy is ready to accept connections.
	close(readyChan)
	if err != nil {
		errChan <- fmt.Errorf("failed to start proxy: %w", err)
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
			// The refresh interval has expired so reconfigure and restart the proxy if necessary.
			err := ext.startProxy(ctx, pErrChan)
			if err != nil {
				errChan <- fmt.Errorf("failed to start proxy: %w", err)
				return
			}
		case err := <-pErrChan:
			if err != nil {
				errChan <- fmt.Errorf("proxy failed with an error: %w", err)
				return
			}
		}
	}
}

func (ext *Extension) runEvents(ctx context.Context, errChan chan error) {
	ext.Logger.Info("processing events")
	err := ext.Events.ProcessEvents(ctx)
	if err != nil {
		errChan <- fmt.Errorf("event processing failed with an error: %w", err)
		return
	}
	ext.Logger.Info("event processing completed")
	errChan <- nil
}

func (ext *Extension) getExtensionData(ctx context.Context) (structs.ExtensionData, error) {
	trace.Enter()
	defer trace.Exit()

	var extData structs.ExtensionData

	// Retrieve the data.
	key := fmt.Sprintf("%s%s", ext.ExtensionDataPrefix, ext.service.ExtensionPath())
	d, err := ext.Store.Get(ctx, key)
	if err != nil {
		return extData, fmt.Errorf("failed to get extension data for %s: %w", key, err)
	}

	// Unmarshal.
	if err := json.Unmarshal([]byte(d), &extData); err != nil {
		return extData, fmt.Errorf("failed to unmarshal extension data for %s: %w", key, err)
	}
	return extData, nil
}

// startProxy starts, or restarts, the extension's proxy server.
// It retrieves the configuration for the proxy and if the configuration has changed
// it closes the existing proxy server and reconfigures a new proxy server.
// It starts the new proxy server in a separate go routine that reports any errors to the
// caller via errChan.
func (ext *Extension) startProxy(ctx context.Context, errChan chan error) error {
	trace.Enter()
	defer trace.Exit()

	const errFmt = "failed to init proxy: %w"

	// Get the lambda extension configuration data for this service.
	extData, err := ext.getExtensionData(ctx)
	if err != nil {
		// TODO: Handle the distinction between transient errors from parameter store and the case where the
		// the function's mTLS material has been deleted because the function was removed from the mesh.
		// For transient errors we should just attempt to use the existing extension data and log any failures.
		// For now we just err out to make sure that the function can't make any more outgoing calls.
		return fmt.Errorf(errFmt, err)
	}

	// If the extension data has not changed then the proxy is already configured.
	if extData.Equals(ext.data) {
		return nil
	}

	// Update the extension data.
	ext.data = extData

	ext.Logger.Info("starting proxy server")

	// Parse the configured list of upstreams. The upstreams are configured as part of the environment
	// so we only do this on the first time through.
	if len(ext.upstreams) == 0 {
		ext.upstreams, err = ext.parseUpstreams()
		if err != nil {
			return fmt.Errorf(errFmt, err)
		}
	}

	// Create a proxy listener configuration for each upstream.
	proxyConfigs := make([]*proxy.Config, len(ext.upstreams))
	for i, upstream := range ext.upstreams {
		// Create the listener config.
		ext.Logger.Debug("configuring upstream", "SNI", upstream.SNI(), "port", upstream.Port)
		cfg, err := ext.proxyConfig(upstream)
		if err != nil {
			return fmt.Errorf(errFmt, err)
		}
		proxyConfigs[i] = cfg
	}

	// Drain and close the existing proxy.
	ext.closeProxy()

	// Create the proxy server.
	ext.proxy = proxy.New(ext.Logger, proxyConfigs...)

	go func(errChan chan error) {
		// Get the lock for the proxy mutex. This go routine will exit and release
		// the lock when the proxy's Close method is called. The lock ensures that
		// we never attempt to start a new proxy server until the old one has exited.
		ext.proxyMutex.Lock()
		defer ext.proxyMutex.Unlock()
		errChan <- ext.proxy.Serve()
	}(errChan)

	// Wait for the proxy to be ready to serve requests before returning
	timeout := time.NewTicker(ext.ProxyTimeout)
	defer timeout.Stop()
	select {
	case <-ext.proxy.Wait():
		ext.Logger.Info("proxy server ready")
	case <-timeout.C:
		return fmt.Errorf("timeout waiting for proxy to start")
	}

	return nil
}

func (ext *Extension) closeProxy() {
	trace.Enter()
	defer trace.Exit()

	if ext.proxy != nil {
		ext.proxy.Close()
	}
}

func (ext *Extension) proxyConfig(upstream structs.Service) (*proxy.Config, error) {
	trace.Enter()
	defer trace.Exit()

	cfg := &proxy.Config{}

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM([]byte(ext.data.RootCertPEM))

	cert, err := tls.X509KeyPair([]byte(ext.data.CertPEM), []byte(ext.data.PrivateKeyPEM))
	if err != nil {
		return cfg, err
	}

	// Listen on the upstream's port on all interfaces.
	cfg.ListenFunc = func() (net.Listener, error) {
		return net.Listen("tcp", fmt.Sprintf(":%d", upstream.Port))
	}

	// Wrap the outgoing request in an mTLS session and dial the mesh gateway.
	cfg.DialFunc = func() (net.Conn, error) {
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

	return cfg, nil
}

func (ext *Extension) parseUpstreams() ([]structs.Service, error) {
	trace.Enter()
	defer trace.Exit()

	u := make([]structs.Service, 0, len(ext.ServiceUpstreams))
	for _, s := range ext.ServiceUpstreams {
		up, err := structs.ParseUpstream(s)
		if err != nil {
			return u, fmt.Errorf("failed to parse upstream: %w", err)
		}
		up.TrustDomain = ext.data.TrustDomain
		u = append(u, up)
	}
	return u, nil
}
