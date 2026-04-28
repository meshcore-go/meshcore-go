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
}

func readLoop(conn io.Reader, done <-chan struct{}, onFrame FrameHandler, onError ErrorHandler) {
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
			frames, rem := hardware.ExtractFrames(data)
			remainder = rem

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
