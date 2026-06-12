package transport

import (
	"context"
	"fmt"
	"io"

	"github.com/meshcore-go/meshcore-go/hardware"
)

type FrameHandler = func(*hardware.KissFrame)

type ErrorHandler = func(error)

type Transport interface {
	Connect(ctx context.Context) error
	Close() error
	Send(data []byte) error
	SetFrameHandler(func(*hardware.KissFrame))
	SetErrorHandler(func(error))
	// Dead returns a channel that is closed when the read loop has exited
	// (due to I/O error or connection loss). Callers can select on this to
	// detect that the transport is no longer receiving.
	Dead() <-chan struct{}
}

// beforeRead is an optional hook invoked before each conn.Read. Transports
// use it to refresh per-read deadlines (e.g. TCP idle timeouts). A non-nil
// error from the hook is reported via the error getter and terminates the
// loop.
type beforeReadFunc = func() error

// readLoopConfig wires runtime-mutable handlers and an optional pre-read
// hook into the shared read loop. Handler getters are called per iteration
// so transports can swap handlers safely under their own lock.
type readLoopConfig struct {
	getFrameHandler func() FrameHandler
	getErrorHandler func() ErrorHandler
	beforeRead      beforeReadFunc
}

func readLoop(conn io.Reader, done <-chan struct{}, dead chan struct{}, cfg readLoopConfig) {
	defer close(dead)

	buf := make([]byte, 1024)
	var remainder []byte

	dispatchErr := func(err error) {
		if cfg.getErrorHandler == nil {
			return
		}
		if h := cfg.getErrorHandler(); h != nil {
			h(err)
		}
	}

	dispatchFrame := func(frame *hardware.KissFrame) {
		if cfg.getFrameHandler == nil {
			return
		}
		if h := cfg.getFrameHandler(); h != nil {
			h(frame)
		}
	}

	for {
		select {
		case <-done:
			return
		default:
		}

		if cfg.beforeRead != nil {
			if err := cfg.beforeRead(); err != nil {
				dispatchErr(fmt.Errorf("set read deadline: %w", err))
				return
			}
		}

		n, err := conn.Read(buf)

		if n > 0 {
			data := append(remainder, buf[:n]...)
			frames, rem, decodeErrs := hardware.ExtractFrames(data)
			remainder = rem

			if len(remainder) > hardware.KISS_MAX_FRAME_SIZE {
				dispatchErr(fmt.Errorf("kiss: remainder exceeded max frame size, resyncing"))
				remainder = nil
			}

			for _, derr := range decodeErrs {
				dispatchErr(derr)
			}

			for _, frame := range frames {
				dispatchFrame(frame)
			}
		}

		if err != nil {
			select {
			case <-done:
				return
			default:
			}

			dispatchErr(err)
			return
		}

		if n == 0 {
			continue
		}
	}
}

func writeRaw(conn io.Writer, data []byte) error {
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}
