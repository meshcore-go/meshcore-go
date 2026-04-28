package transport

import (
	"context"
	"fmt"
	"io"

	"github.com/meshcore-go/meshcore-go/companion"
)

type ResponseHandler = func(companion.Response)

type ErrorHandler = func(error)

type Transport interface {
	Connect(ctx context.Context) error
	Close() error
	Send(command []byte) error
	SetResponseHandler(func(companion.Response))
	SetErrorHandler(func(error))
}

func readLoop(conn io.Reader, done <-chan struct{}, parser *companion.FrameParser, onResponse ResponseHandler, onError ErrorHandler) {
	buf := make([]byte, 256)

	for {
		select {
		case <-done:
			return
		default:
		}

		n, err := conn.Read(buf)

		if n > 0 {
			frames := parser.Feed(buf[:n])
			for _, frame := range frames {
				resp, parseErr := companion.ParseResponse(frame.Data)
				if parseErr != nil {
					if onError != nil {
						onError(parseErr)
					}
					continue
				}

				if onResponse != nil {
					onResponse(resp)
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

func writeCommand(conn io.Writer, command []byte) error {
	frame, err := companion.FrameEncode(companion.FrameTypeOutgoing, command)
	if err != nil {
		return err
	}

	if _, err := conn.Write(frame); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}
