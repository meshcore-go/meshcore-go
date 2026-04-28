package transport

import (
	"context"
	"fmt"
	"net"
)

type TCPConfig struct {
	Address string
	BaseConfig
}

type TCPTransport struct {
	*baseTransport
}

func NewTCPTransport(config TCPConfig) *TCPTransport {
	dial := func(ctx context.Context) (Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", config.Address)
		if err != nil {
			return nil, fmt.Errorf("dial tcp: %w", err)
		}
		return conn, nil
	}

	return &TCPTransport{
		baseTransport: newBaseTransport(dial, config.BaseConfig),
	}
}

func (t *TCPTransport) Connect(ctx context.Context) error {
	return t.baseTransport.connect(ctx)
}

func (t *TCPTransport) Close() error {
	return t.baseTransport.close()
}

func (t *TCPTransport) Send(command []byte) error {
	return t.baseTransport.send(command)
}

func (t *TCPTransport) SetResponseHandler(h ResponseHandler) {
	t.mu.Lock()
	t.onResponse = h
	t.mu.Unlock()
}

func (t *TCPTransport) SetErrorHandler(h ErrorHandler) {
	t.mu.Lock()
	t.onError = h
	t.mu.Unlock()
}

func (t *TCPTransport) SetDisconnectHandler(h func()) {
	t.mu.Lock()
	t.onDisconnect = h
	t.mu.Unlock()
}

func (t *TCPTransport) SetReconnectHandler(h func()) {
	t.mu.Lock()
	t.onReconnect = h
	t.mu.Unlock()
}
