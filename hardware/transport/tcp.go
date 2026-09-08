package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// DefaultTCPKeepAlivePeriod is the keepalive idle/probe interval used when
// TCPConfig.KeepAlivePeriod is zero.
const DefaultTCPKeepAlivePeriod = 15 * time.Second

// DefaultTCPReadIdleTimeout is the per-read deadline used when
// TCPConfig.ReadIdleTimeout is zero.
const DefaultTCPReadIdleTimeout = 60 * time.Second

// DefaultTCPWriteTimeout bounds a Send's wait on the kernel send buffer when
// TCPConfig.WriteTimeout is zero.
const DefaultTCPWriteTimeout = 10 * time.Second

type TCPConfig struct {
	Address string
	// KeepAlivePeriod is the SO_KEEPALIVE idle/probe interval; zero uses the
	// default, negative disables keepalive.
	KeepAlivePeriod time.Duration
	// ReadIdleTimeout is the longest wait for inbound bytes before the
	// connection is dead; zero uses the default, negative disables it.
	ReadIdleTimeout time.Duration
	// WriteTimeout bounds a single Send's wait on the kernel send buffer;
	// zero uses the default, negative disables it.
	WriteTimeout time.Duration
}

type TCPTransport struct {
	base
	config TCPConfig

	// 0 means disabled
	keepAlive    time.Duration
	idle         time.Duration
	writeTimeout time.Duration
}

var _ Transport = (*TCPTransport)(nil)

func NewTCPTransport(config TCPConfig) *TCPTransport {
	return &TCPTransport{
		base:         newBase(),
		config:       config,
		keepAlive:    resolve(config.KeepAlivePeriod, DefaultTCPKeepAlivePeriod),
		idle:         resolve(config.ReadIdleTimeout, DefaultTCPReadIdleTimeout),
		writeTimeout: resolve(config.WriteTimeout, DefaultTCPWriteTimeout),
	}
}

func resolve(v, def time.Duration) time.Duration {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 0
	default:
		return v
	}
}

// Connect dials the modem and starts reading.
func (t *TCPTransport) Connect(ctx context.Context) error {
	dialer := &net.Dialer{KeepAlive: -1}
	if t.keepAlive > 0 {
		dialer.KeepAlive = t.keepAlive
	}

	conn, err := dialer.DialContext(ctx, "tcp", t.config.Address)
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}

	if tcpConn, ok := conn.(*net.TCPConn); ok && t.keepAlive > 0 {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(t.keepAlive)
	}

	cfg := readLoopConfig{
		getFrameHandler: t.frameHandler,
		getErrorHandler: t.makeErrorGetter(t.idle),
	}
	if t.idle > 0 {
		cfg.beforeRead = func() error {
			return conn.SetReadDeadline(time.Now().Add(t.idle))
		}
	}

	t.start(conn, cfg)
	return nil
}

func (t *TCPTransport) Send(data []byte) error {
	w, err := t.writer()
	if err != nil {
		return err
	}
	conn := w.(net.Conn)

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.writeTimeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(t.writeTimeout)); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
		defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	}

	return writeRaw(conn, data)
}

// makeErrorGetter wraps idle-timeout errors, reading the live handler each
// call so SetErrorHandler stays race-free.
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
			if (errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, os.ErrDeadlineExceeded) {
				h(fmt.Errorf("tcp read idle timeout (%s): %w", idle, err))
				return
			}
			h(err)
		}
	}
}
