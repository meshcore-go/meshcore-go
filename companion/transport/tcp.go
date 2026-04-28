package transport

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/meshcore-go/meshcore-go/companion"
)

type TCPConfig struct {
	Address string
}

type TCPTransport struct {
	config     TCPConfig
	conn       net.Conn
	parser     *companion.FrameParser
	onResponse ResponseHandler
	onError    ErrorHandler
	done       chan struct{}
	closeOnce  sync.Once
}

func NewTCPTransport(config TCPConfig) *TCPTransport {
	return &TCPTransport{
		config: config,
		parser: companion.NewFrameParser(),
		done:   make(chan struct{}),
	}
}

func (t *TCPTransport) Connect(ctx context.Context) error {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", t.config.Address)
	if err != nil {
		return fmt.Errorf("dial tcp: %w", err)
	}

	t.conn = conn
	go readLoop(t.conn, t.done, t.parser, t.onResponse, t.onError)

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

func (t *TCPTransport) Send(command []byte) error {
	if t.conn == nil {
		return fmt.Errorf("tcp transport not connected")
	}

	return writeCommand(t.conn, command)
}

func (t *TCPTransport) SetResponseHandler(h ResponseHandler) {
	t.onResponse = h
}

func (t *TCPTransport) SetErrorHandler(h ErrorHandler) {
	t.onError = h
}
