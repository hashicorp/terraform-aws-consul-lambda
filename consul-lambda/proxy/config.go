package proxy

import (
	"net"
)

// Config holds the configuration for a single proxy connection between a source and destination.
type Config struct {
	// ListenFunc returns a net.Listener that listens for incoming source connections.
	ListenFunc func() (net.Listener, error)
	// DialFunc dials a remote and returns a net.Conn for the destination.
	DialFunc func() (net.Conn, error)
}
