package proxy

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

const errBufSize = 5

// Listener is the implementation of a specific proxy listener. It has pluggable
// Listen and Dial methods to suit public mTLS vs upstream semantics. It handles
// the lifecycle of the listener and all connections opened through it
type Listener struct {
	listenFunc func() (net.Listener, error)
	dialFunc   func() (net.Conn, error)
	errChan    chan error

	stopFlag int32
	stopChan chan struct{}

	// listeningChan is closed when listener is opened successfully.
	listeningChan chan struct{}

	// listenerLock guards access to the listener field
	listenerLock sync.Mutex
	listener     net.Listener

	connWG sync.WaitGroup
}

// NewListener returns a Listener setup to listen for public mTLS
// connections and proxy them to the configured local application over TCP.
func NewListener(cfg *Config) *Listener {
	return &Listener{
		listenFunc:    cfg.ListenFunc,
		dialFunc:      cfg.DialFunc,
		stopChan:      make(chan struct{}),
		listeningChan: make(chan struct{}),
		errChan:       make(chan error, errBufSize),
	}
}

// Serve runs the listener until it is stopped. It is an error to call Serve
// more than once for any given Listener instance.
func (l *Listener) Serve() error {
	// Ensure we mark state closed if we fail before Close is called externally.
	defer l.Close()

	if atomic.LoadInt32(&l.stopFlag) != 0 {
		return errors.New("serve called on a closed listener")
	}

	listener, err := l.listenFunc()
	if err != nil {
		return err
	}

	l.setListener(listener)

	close(l.listeningChan)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if atomic.LoadInt32(&l.stopFlag) == 1 {
				return nil
			}
			return err
		}

		l.connWG.Add(1)
		go l.handleConn(conn)
	}
}

// Listening returns a channel that is closed when the Listener is ready to accept incoming connections.
func (l *Listener) Listening() <-chan struct{} {
	return l.listeningChan
}

// handleConn is the internal connection handler goroutine.
func (l *Listener) handleConn(src net.Conn) {
	defer func() {
		// Make sure Listener.Close waits for this conn to be cleaned up.
		src.Close()
		l.connWG.Done()
	}()

	dst, err := l.dialFunc()
	if err != nil {
		l.sendError(fmt.Errorf("failed to dial destination: %w", err))
		return
	}

	// Note no need to defer dst.Close() since conn handles that for us.
	conn := NewConn(src, dst)
	defer conn.Close()

	connStop := make(chan struct{})

	// Run another goroutine to copy the bytes.
	go func() {
		err = conn.CopyBytes()
		if err != nil {
			l.sendError(fmt.Errorf("connection failed: %w", err))
		}
		close(connStop)
	}()

	// Wait for conn to close or the listener's Close method to be called.
	for {
		select {
		case <-connStop:
			return
		case <-l.stopChan:
			return
		}
	}
}

// Close terminates the listener and all active connections.
func (l *Listener) Close() {
	// Prevent the listener from being started.
	oldFlag := atomic.SwapInt32(&l.stopFlag, 1)
	if oldFlag != 0 {
		return
	}

	// Stop the current listener and stop accepting new requests.
	if listener := l.getListener(); listener != nil {
		listener.Close()
	}

	// Stop outstanding requests.
	close(l.stopChan)

	// Stop sending errors.
	close(l.errChan)

	// Wait for all conns to close
	l.connWG.Wait()
}

// Errors returns a channel that the listener writes errors to.
// The channel is closed when the listener is closed.
func (l *Listener) Errors() <-chan error {
	return l.errChan
}

// Wait for the listener to be ready to accept connections.
func (l *Listener) Wait() {
	<-l.Listening()
}

func (l *Listener) setListener(listener net.Listener) {
	l.listenerLock.Lock()
	defer l.listenerLock.Unlock()
	l.listener = listener
}

func (l *Listener) getListener() net.Listener {
	l.listenerLock.Lock()
	defer l.listenerLock.Unlock()
	return l.listener
}

func (l *Listener) sendError(err error) {
	// Don't send errors if stopped because the error channel is closed.
	if atomic.LoadInt32(&l.stopFlag) == 1 {
		return
	}

	// Drop the oldest error from the channel so that this call doesn't block if the buffer is full.
	for len(l.errChan) >= errBufSize {
		<-l.errChan
	}
	l.errChan <- err
}
