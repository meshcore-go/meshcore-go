package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/meshcore-go/meshcore-go/hardware"
	"go.bug.st/serial"
)

type SerialConfig struct {
	Port     string
	BaudRate int
}

type SerialTransport struct {
	config SerialConfig
	port   serial.Port

	hMu     sync.RWMutex
	onFrame FrameHandler
	onError ErrorHandler

	writeMu sync.Mutex

	done      chan struct{}
	dead      chan struct{}
	closeOnce sync.Once
}

func NewSerialTransport(config SerialConfig) *SerialTransport {
	return &SerialTransport{
		config: config,
		done:   make(chan struct{}),
		dead:   make(chan struct{}),
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
	go readLoop(t.port, t.done, t.dead, readLoopConfig{
		getFrameHandler: t.frameHandler,
		getErrorHandler: t.errorHandler,
	})

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

func (t *SerialTransport) Send(data []byte) error {
	if t.port == nil {
		return fmt.Errorf("serial transport not connected")
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return writeRaw(t.port, data)
}

func (t *SerialTransport) SetFrameHandler(h func(*hardware.KissFrame)) {
	t.hMu.Lock()
	t.onFrame = h
	t.hMu.Unlock()
}

func (t *SerialTransport) SetErrorHandler(h func(error)) {
	t.hMu.Lock()
	t.onError = h
	t.hMu.Unlock()
}

func (t *SerialTransport) frameHandler() FrameHandler {
	t.hMu.RLock()
	defer t.hMu.RUnlock()
	return t.onFrame
}

func (t *SerialTransport) errorHandler() ErrorHandler {
	t.hMu.RLock()
	defer t.hMu.RUnlock()
	return t.onError
}

func (t *SerialTransport) Dead() <-chan struct{} {
	return t.dead
}
