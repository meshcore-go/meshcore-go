package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meshcore-go/meshcore-go/companion"
	"go.bug.st/serial"
)

type SerialConfig struct {
	Port     string
	BaudRate int
}

type SerialTransport struct {
	config     SerialConfig
	port       serial.Port
	parser     *companion.FrameParser
	onResponse ResponseHandler
	onError    ErrorHandler
	done       chan struct{}
	closeOnce  sync.Once
}

func NewSerialTransport(config SerialConfig) *SerialTransport {
	return &SerialTransport{
		config: config,
		parser: companion.NewFrameParser(),
		done:   make(chan struct{}),
	}
}

func (t *SerialTransport) Connect(_ context.Context) error {
	port, err := serial.Open(t.config.Port, &serial.Mode{BaudRate: t.config.BaudRate})
	if err != nil {
		return fmt.Errorf("open serial port: %w", err)
	}

	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		_ = port.Close()
		return fmt.Errorf("set read timeout: %w", err)
	}

	t.port = port
	go readLoop(t.port, t.done, t.parser, t.onResponse, t.onError)

	return nil
}

func (t *SerialTransport) Close() error {
	var closeErr error

	t.closeOnce.Do(func() {
		close(t.done)
		if t.port != nil {
			closeErr = t.port.Close()
		}
	})

	return closeErr
}

func (t *SerialTransport) Send(command []byte) error {
	if t.port == nil {
		return fmt.Errorf("serial transport not connected")
	}

	return writeCommand(t.port, command)
}

func (t *SerialTransport) SetResponseHandler(h ResponseHandler) {
	t.onResponse = h
}

func (t *SerialTransport) SetErrorHandler(h ErrorHandler) {
	t.onError = h
}
