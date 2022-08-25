package proxy

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// Server implements a proxy server that manages TCP listeners for a configurable set of upstreams.
type Server struct {
	cfgs      []*Config
	listeners []*Listener

	// waitChan is closed once the server is up and running. It can be used by
	// callers to wait until the server is initialized and ready to handle connections.
	waitChan chan struct{}

	stopFlag int32
	stopChan chan struct{}
}

// New returns a new, unstarted proxy server from the given proxy configurations.
// The proxy can be started by calling Serve.
func New(cfgs ...*Config) *Server {
	return &Server{
		waitChan: make(chan struct{}),
		stopChan: make(chan struct{}),
		cfgs:     cfgs,
	}
}

// Serve starts the proxy and initializes all of the configured listeners.
// It blocks until Close is called or an error occurs.
func (s *Server) Serve() error {
	if atomic.LoadInt32(&s.stopFlag) != 0 {
		return errors.New("serve called on a closed server")
	}

	errChan := make(chan error)
	lwg := &sync.WaitGroup{}

	s.listeners = make([]*Listener, 0, len(s.cfgs))
	for _, lc := range s.cfgs {
		l := NewListener(lc)
		s.listeners = append(s.listeners, l)

		// Start the listener. If Serve returns an error it is written to the errChan and handled below.
		go func(l *Listener) {
			if err := l.Serve(); err != nil {
				errChan <- fmt.Errorf("failed to serve listener: %w", err)
				return
			}
		}(l)

		// Add a wait for this listener to start.
		lwg.Add(1)
		go func(wg *sync.WaitGroup, l *Listener) {
			defer wg.Done()
			l.Wait()
		}(lwg, l)

		// Watch for errors on this listener.
		go func(l *Listener) {
			for le := range l.Errors() {
				errChan <- le
			}
		}(l)
	}

	// Wait for all listeners to start. Once they have, close the waitChan to indicate
	// that the proxy is ready to serve requests.
	go func() {
		lwg.Wait()
		close(s.waitChan)
	}()

	// Block until a stop event is received or until one of the listeners errs.
	// We do not currently attempt to recover a failed listener so an error is fatal.
	select {
	case err := <-errChan:
		return err
	case <-s.stopChan:
		return nil
	}
}

// Wait returns a channel that is closed once the proxy is ready to serve requests on all listeners.
func (s *Server) Wait() <-chan struct{} {
	return s.waitChan
}

// Close shuts down the proxy and closes all active connections and listeners.
func (s *Server) Close() {
	stopFlag := atomic.SwapInt32(&s.stopFlag, 1)
	if stopFlag != 0 {
		return
	}

	defer close(s.stopChan)

	// close all active listeners
	wg := &sync.WaitGroup{}
	for _, l := range s.listeners {
		wg.Add(1)
		go func(l *Listener, wg *sync.WaitGroup) {
			defer wg.Done()
			l.Close()
		}(l, wg)
	}
	wg.Wait()
}
