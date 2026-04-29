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

type SerialTransport struct {
	*baseTransport
}

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

func (t *SerialTransport) Connect(ctx context.Context) error {
	return t.baseTransport.connect(ctx)
}

func (t *SerialTransport) Close() error {
	return t.baseTransport.close()
}

func (t *SerialTransport) Send(command []byte) error {
	return t.baseTransport.send(command)
}

func (t *SerialTransport) SetResponseHandler(h ResponseHandler) {
	t.mu.Lock()
	t.onResponse = h
	t.mu.Unlock()
}

func (t *SerialTransport) SetErrorHandler(h ErrorHandler) {
	t.mu.Lock()
	t.onError = h
	t.mu.Unlock()
}

func (t *SerialTransport) SetDisconnectHandler(h func()) {
	t.mu.Lock()
	t.onDisconnect = h
	t.mu.Unlock()
}

func (t *SerialTransport) SetReconnectHandler(h func()) {
	t.mu.Lock()
	t.onReconnect = h
	t.mu.Unlock()
}
