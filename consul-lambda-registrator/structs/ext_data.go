package structs

import "reflect"

// ExtensionData holds the information that a Lambda function needs to call services in the Consul service mesh.
type ExtensionData struct {
	// PrivateKeyPEM is the TLS certificate private key in PEM format.
	PrivateKeyPEM string `json:"privateKeyPEM"`
	// CertPEM is the TLS certificate in PEM format.
	CertPEM string `json:"certPEM"`
	// RootCertPEM is the TLS root CA certificate in PEM format.
	RootCertPEM string `json:"rootCertPEM"`
	// TrustDomain is the trusted domain that the service belongs to.
	TrustDomain string `json:"trustDomain"`
	// Peers is the list of peers.
	Peers []Peer `json:"peers,omitempty"`
}

// Peer holds the information for a Consul peer.
type Peer struct {
	// Name of the peer.
	Name string `json:"name"`
	// TrustDomain is the trusted domain of the peer.
	TrustDomain string `json:"trustDomain"`
}

func (x ExtensionData) Equals(y ExtensionData) bool {
	return reflect.DeepEqual(x, y)
}
