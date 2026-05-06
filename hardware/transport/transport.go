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

func readLoop(conn io.Reader, done <-chan struct{}, dead chan struct{}, onFrame FrameHandler, onError ErrorHandler) {
	defer close(dead)

	buf := make([]byte, 1024)
	var remainder []byte

	for {
		select {
		case <-done:
			return
		default:
		}

		n, err := conn.Read(buf)

		if n > 0 {
			data := append(remainder, buf[:n]...)
			frames, rem, decodeErrs := hardware.ExtractFrames(data)
			remainder = rem

			if len(remainder) > hardware.KISS_MAX_FRAME_SIZE {
				if onError != nil {
					onError(fmt.Errorf("kiss: remainder exceeded max frame size, resyncing"))
				}
				remainder = nil
			}

			for _, err := range decodeErrs {
				if onError != nil {
					onError(err)
				}
			}

			for _, frame := range frames {
				if onFrame != nil {
					onFrame(frame)
				}
			}
		}

		if err != nil {
			select {
			case <-done:
				return
			default:
			}

			if onError != nil {
				onError(err)
			}
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
