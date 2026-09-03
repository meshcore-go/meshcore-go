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
}

type SerialTransport struct {
	base
	config SerialConfig
}

var _ Transport = (*SerialTransport)(nil)

func NewSerialTransport(config SerialConfig) *SerialTransport {
	return &SerialTransport{base: newBase(), config: config}
}

// Connect opens the port and starts reading.
func (t *SerialTransport) Connect(_ context.Context) error {
	port, err := serial.Open(t.config.Port, &serial.Mode{BaudRate: t.config.BaudRate})
	if err != nil {
		return fmt.Errorf("open serial port: %w", err)
	}

	if err := port.SetReadTimeout(100 * time.Millisecond); err != nil {
		_ = port.Close()
		return fmt.Errorf("set read timeout: %w", err)
	}

	t.start(port, readLoopConfig{
		getFrameHandler: t.frameHandler,
		getErrorHandler: t.errorHandler,
	})
	return nil
}

func (t *SerialTransport) Send(data []byte) error {
	port, err := t.writer()
	if err != nil {
		return err
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return writeRaw(port, data)
}
