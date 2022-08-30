package extension

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/proxy"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/structs"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/trace"
)

type Config struct {
	MeshGatewayURI      string        `envconfig:"MESH_GATEWAY_URI" required:"true"`
	ExtensionDataPrefix string        `envconfig:"EXTENSION_DATA_PREFIX" required:"true"`
	Datacenter          string        `envconfig:"DATACENTER"`
	ServiceName         string        `envconfig:"SERVICE_NAME"`
	ServiceNamespace    string        `envconfig:"SERVICE_NAMESPACE"`
	ServicePartition    string        `envconfig:"SERVICE_PARTITION"`
	ServiceUpstreams    []string      `envconfig:"SERVICE_UPSTREAMS"`
	RefreshFrequency    time.Duration `envconfig:"REFRESH_FREQUENCY" default:"5m"`
	Timeout             time.Duration `envconfig:"PROXY_TIMEOUT" default:"10s"`
	LogLevel            string        `envconfig:"LOG_LEVEL" default:"info"`
	TraceEnabled        bool          `envconfig:"TRACE_ENABLED" default:"false"`

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
	service structs.Service
	proxy   *proxy.Server
}

func NewExtension(cfg *Config) *Extension {
	return &Extension{
		Config: cfg,
		service: structs.Service{
			Datacenter:     cfg.Datacenter,
			Name:           cfg.ServiceName,
			EnterpriseMeta: structs.NewEnterpriseMeta(cfg.ServicePartition, cfg.ServiceNamespace),
		},
	}
}

func (ext *Extension) Serve(ctx context.Context) error {
	trace.Enter()
	defer trace.Exit()

	// Initialize the proxy server.
	err := ext.initProxy(ctx)
	if err != nil {
		return err
	}

	// Start the proxy
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		err := ext.proxy.Serve()
		if err != nil {
			ext.Logger.Error("proxy failed with an error", "error", err)
		}
		// Once the proxy exits we cannot handle any more events so cancel our context.
		cancel()
	}()
	defer ext.proxy.Close()

	// Start the event processor and block until it returns.
	// It will return when there are no more events to process or its context is cancelled,
	// whichever happens first.
	ext.Logger.Info("processing events")
	err = ext.Events.ProcessEvents(ctx)
	if err != nil {
		return fmt.Errorf("event processing failed with an error: %w", err)
	}

	ext.Logger.Info("processing events finished")
	return nil
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

func (ext *Extension) initProxy(ctx context.Context) error {
	trace.Enter()
	defer trace.Exit()

	// Get the lambda extension configuration data for this service.
	extData, err := ext.getExtensionData(ctx)
	if err != nil {
		return fmt.Errorf("failed to init proxy: %w", err)
	}

	// Parse the configured list of upstreams.
	upstreams, err := ext.parseUpstreams(extData)
	if err != nil {
		return fmt.Errorf("failed to init proxy: %w", err)
	}

	// Create a proxy listener configuration for each upstream.
	proxyConfigs := make([]*proxy.Config, len(upstreams))
	for i, upstream := range upstreams {
		// Create the listener config.
		cfg, err := ext.proxyConfig(upstream, extData)
		if err != nil {
			return fmt.Errorf("failed to init proxy: %w", err)
		}
		proxyConfigs[i] = cfg
	}

	// Create the proxy server.
	ext.proxy = proxy.New(proxyConfigs...)

	return nil
}

func (ext *Extension) proxyConfig(upstream structs.Service, extData structs.ExtensionData) (*proxy.Config, error) {
	trace.Enter()
	defer trace.Exit()

	cfg := &proxy.Config{}

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM([]byte(extData.RootCertPEM))

	cert, err := tls.X509KeyPair([]byte(extData.CertPEM), []byte(extData.PrivateKeyPEM))
	if err != nil {
		return cfg, err
	}

	// Listen on the upstream's port on all interfaces.
	cfg.ListenFunc = func() (net.Listener, error) {
		return net.Listen("tcp", fmt.Sprintf(":%d", upstream.Port))
	}

	// TODO: How do we want to handle cert rotation?
	// Considerations:
	// - Lambda rgy will keep the mTLS material up-to-date in parameter store.
	// - The extension will likely live between invocations and the mTLS material may go stale
	// - We shouldn't ever need to recreate the ListenerFunc because it isn't TLS
	// - We could recreate the DialFunc with updated mTLS on every request.. but that may be overkill?
	// - Could keep a record (hash) of the mTLS material and update DialFunc only if it has changed.
	// - In either case, I think this means that the proxy must support updating the DialFunc in flight.
	//
	// De-registration is another case to consider:
	// - When a Lambda service is de-registered we want it to no longer be able to call into the mesh.
	// - Lambda registrator deletes the mTLS data from parameter store.
	// - If the Lambda RT and extension stays alive - this is realistic, I've seen it stay up for >15m -
	//   then the DialFunc for each upstream still contains the mTLS info needed to call in to the mesh (not good).

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
					return errors.New("spiffe id mismatch")
				}
				return nil
			},
		})
	}

	return cfg, nil
}

func (ext *Extension) parseUpstreams(extData structs.ExtensionData) ([]structs.Service, error) {
	trace.Enter()
	defer trace.Exit()

	u := make([]structs.Service, 0, len(ext.ServiceUpstreams))
	for _, s := range ext.ServiceUpstreams {
		up, err := structs.ParseService(s)
		if err != nil {
			return u, fmt.Errorf("failed to parse upstream: %w", err)
		}
		up.TrustDomain = extData.TrustDomain
		u = append(u, up)
	}
	return u, nil
}
