package transport

import (
	"context"
	"fmt"
	"time"

	"go.bug.st/serial"
)

type SerialConfig struct {
	Port     string
	BaudRate int
	BaseConfig
}

type serialConn struct {
	serial.Port
}

func (c *serialConn) Close() error {
	return c.Port.Close()
}

// SerialTransport is a Transport over a serial port.
type SerialTransport struct {
	*baseTransport
}

var _ Transport = (*SerialTransport)(nil)

func NewSerialTransport(config SerialConfig) *SerialTransport {
	dial := func(_ context.Context) (Conn, error) {
		port, err := serial.Open(config.Port, &serial.Mode{BaudRate: config.BaudRate})
		if err != nil {
			return nil, fmt.Errorf("open serial port: %w", err)
		}

		if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
			_ = port.Close()
			return nil, fmt.Errorf("set read timeout: %w", err)
		}

		return &serialConn{port}, nil
	}

	return &SerialTransport{
		baseTransport: newBaseTransport(dial, config.BaseConfig),
	}
}
