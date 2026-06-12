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
// to a non-zero value to override; set to a negative value to disable.
const DefaultTCPReadIdleTimeout = 60 * time.Second

// DefaultTCPWriteTimeout bounds how long Send will wait for the kernel
// send buffer to drain before treating the peer as stalled. Set
// TCPConfig.WriteTimeout to a non-zero value to override; set to a
// negative value to disable.
const DefaultTCPWriteTimeout = 10 * time.Second

type TCPConfig struct {
	Address string
	// KeepAlivePeriod controls SO_KEEPALIVE idle/probe interval. Zero uses
	// DefaultTCPKeepAlivePeriod. Negative disables keepalive entirely.
	KeepAlivePeriod time.Duration
	// ReadIdleTimeout is the maximum time the read loop will wait for
	// inbound bytes before treating the connection as dead. Zero uses
	// DefaultTCPReadIdleTimeout. Negative disables the idle timeout.
	ReadIdleTimeout time.Duration
	// WriteTimeout bounds how long a single Send may wait for the kernel
	// send buffer. Zero uses DefaultTCPWriteTimeout. Negative disables.
	WriteTimeout time.Duration
}

type TCPTransport struct {
	config TCPConfig
	conn   net.Conn

	hMu     sync.RWMutex
	onFrame FrameHandler
	onError ErrorHandler

	writeMu      sync.Mutex
	writeTimeout time.Duration

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

	writeTimeout := t.config.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = DefaultTCPWriteTimeout
	}
	if writeTimeout < 0 {
		writeTimeout = 0
	}
	t.writeTimeout = writeTimeout

	idle := t.config.ReadIdleTimeout
	if idle == 0 {
		idle = DefaultTCPReadIdleTimeout
	}

	cfg := readLoopConfig{
		getFrameHandler: t.frameHandler,
		getErrorHandler: t.makeErrorGetter(idle),
	}
	if idle > 0 {
		cfg.beforeRead = func() error {
			return t.conn.SetReadDeadline(time.Now().Add(idle))
		}
	}

	go readLoop(t.conn, t.done, t.dead, cfg)

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

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.writeTimeout > 0 {
		if err := t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout)); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
		defer func() { _ = t.conn.SetWriteDeadline(time.Time{}) }()
	}

	return writeRaw(t.conn, data)
}

func (t *TCPTransport) SetFrameHandler(h func(*hardware.KissFrame)) {
	t.hMu.Lock()
	t.onFrame = h
	t.hMu.Unlock()
}

func (t *TCPTransport) SetErrorHandler(h func(error)) {
	t.hMu.Lock()
	t.onError = h
	t.hMu.Unlock()
}

func (t *TCPTransport) frameHandler() FrameHandler {
	t.hMu.RLock()
	defer t.hMu.RUnlock()
	return t.onFrame
}

func (t *TCPTransport) errorHandler() ErrorHandler {
	t.hMu.RLock()
	defer t.hMu.RUnlock()
	return t.onError
}

// makeErrorGetter returns an error-handler getter that wraps idle-timeout
// errors with a clearer message. The wrapping happens inside the getter
// so the live handler is read each time (race-safe with SetErrorHandler).
func (t *TCPTransport) makeErrorGetter(idle time.Duration) func() ErrorHandler {
	if idle <= 0 {
		return t.errorHandler
	}
	return func() ErrorHandler {
		h := t.errorHandler()
		if h == nil {
			return nil
		}
		return func(err error) {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				h(fmt.Errorf("tcp read idle timeout (%s): %w", idle, err))
				return
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				h(fmt.Errorf("tcp read idle timeout (%s): %w", idle, err))
				return
			}
			h(err)
		}
	}
}

func (t *TCPTransport) Dead() <-chan struct{} {
	return t.dead
}
