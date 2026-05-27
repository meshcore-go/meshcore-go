package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/meshcore-go/meshcore-go/hardware"
)

// DefaultTCPKeepAlivePeriod is the default keepalive idle/probe interval
// when TCPConfig.KeepAlivePeriod is zero. Chosen to fail half-open
// connections within roughly a couple of minutes on most stacks while
// remaining gentle on the link.
const DefaultTCPKeepAlivePeriod = 15 * time.Second

// DefaultTCPReadIdleTimeout is the default per-read deadline. If no bytes
// arrive within this window, the read loop reports an error and exits,
// closing Dead() so callers can reconnect. Set TCPConfig.ReadIdleTimeout
// to a non-zero value to override; set to -1 to disable.
const DefaultTCPReadIdleTimeout = 60 * time.Second

type TCPConfig struct {
	Address string
	// KeepAlivePeriod controls SO_KEEPALIVE idle/probe interval. Zero uses
	// DefaultTCPKeepAlivePeriod. Negative disables keepalive entirely.
	KeepAlivePeriod time.Duration
	// ReadIdleTimeout is the maximum time the read loop will wait for
	// inbound bytes before treating the connection as dead. Zero uses
	// DefaultTCPReadIdleTimeout. Negative disables the idle timeout.
	ReadIdleTimeout time.Duration
}

type TCPTransport struct {
	config    TCPConfig
	conn      net.Conn
	onFrame   FrameHandler
	onError   ErrorHandler
	done      chan struct{}
	dead      chan struct{}
	closeOnce sync.Once
}

func NewTCPTransport(config TCPConfig) *TCPTransport {
	return &TCPTransport{
		config: config,
		done:   make(chan struct{}),
		dead:   make(chan struct{}),
	}
}

func (t *TCPTransport) Connect(ctx context.Context) error {
	keepAlive := t.config.KeepAlivePeriod
	if keepAlive == 0 {
		keepAlive = DefaultTCPKeepAlivePeriod
	}

	dialer := &net.Dialer{}
	if keepAlive > 0 {
		dialer.KeepAlive = keepAlive
	} else {
		dialer.KeepAlive = -1
	}

	conn, err := dialer.DialContext(ctx, "tcp", t.config.Address)
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok && keepAlive > 0 {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(keepAlive)
	}

	t.conn = conn

	idle := t.config.ReadIdleTimeout
	if idle == 0 {
		idle = DefaultTCPReadIdleTimeout
	}

	var beforeRead beforeReadFunc
	if idle > 0 {
		beforeRead = func() error {
			return t.conn.SetReadDeadline(time.Now().Add(idle))
		}
	}

	onError := t.onError
	wrappedErr := onError
	if idle > 0 && onError != nil {
		wrappedErr = func(err error) {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				onError(fmt.Errorf("tcp read idle timeout (%s): %w", idle, err))
				return
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				onError(fmt.Errorf("tcp read idle timeout (%s): %w", idle, err))
				return
			}
			onError(err)
		}
	}

	go readLoop(t.conn, t.done, t.dead, t.onFrame, wrappedErr, beforeRead)

	return nil
}

func (t *TCPTransport) Close() error {
	var closeErr error

	t.closeOnce.Do(func() {
		close(t.done)
		if t.conn != nil {
			closeErr = t.conn.Close()
		}
	})

	return closeErr
}

func (t *TCPTransport) Send(data []byte) error {
	if t.conn == nil {
		return fmt.Errorf("tcp transport not connected")
	}

	return writeRaw(t.conn, data)
}

func (t *TCPTransport) SetFrameHandler(h func(*hardware.KissFrame)) {
	t.onFrame = h
}

func (t *TCPTransport) SetErrorHandler(h func(error)) {
	t.onError = h
}

func (t *TCPTransport) Dead() <-chan struct{} {
	return t.dead
}
