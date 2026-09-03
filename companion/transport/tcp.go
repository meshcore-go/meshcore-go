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

// TCPTransport is a Transport over TCP.
type TCPTransport struct {
	*baseTransport
}

var _ Transport = (*TCPTransport)(nil)

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
