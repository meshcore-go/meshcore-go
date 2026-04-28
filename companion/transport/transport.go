package transport

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

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

type Conn interface {
	io.ReadWriter
	Close() error
}

type DialFunc func(ctx context.Context) (Conn, error)

type BaseConfig struct {
	// ReconnectInterval is the initial backoff delay. Doubles each failure up to MaxReconnectInterval. Default: 1s.
	ReconnectInterval time.Duration

	// MaxReconnectInterval caps the backoff. Default: 30s.
	MaxReconnectInterval time.Duration

	// TxQueueSize is the max pending commands while disconnected. Default: 64. Oldest dropped when full.
	TxQueueSize int
}

type baseTransport struct {
	dial   DialFunc
	config BaseConfig
	parser *companion.FrameParser
	conn   Conn

	onResponse   ResponseHandler
	onError      ErrorHandler
	onDisconnect func()
	onReconnect  func()

	mu        sync.Mutex
	connected bool
	txQueue   chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func newBaseTransport(dial DialFunc, config BaseConfig) *baseTransport {
	if config.ReconnectInterval <= 0 {
		config.ReconnectInterval = 1 * time.Second
	}
	if config.MaxReconnectInterval <= 0 {
		config.MaxReconnectInterval = 30 * time.Second
	}
	if config.TxQueueSize <= 0 {
		config.TxQueueSize = 64
	}

	return &baseTransport{
		dial:    dial,
		config:  config,
		parser:  companion.NewFrameParser(),
		txQueue: make(chan []byte, config.TxQueueSize),
		done:    make(chan struct{}),
	}
}

func (t *baseTransport) connect(ctx context.Context) error {
	conn, err := t.dial(ctx)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.conn = conn
	t.connected = true
	t.mu.Unlock()

	go t.readLoopWithReconnect()
	go t.writeLoop()

	return nil
}

func (t *baseTransport) close() error {
	var closeErr error

	t.closeOnce.Do(func() {
		close(t.done)

		t.mu.Lock()
		t.connected = false
		if t.conn != nil {
			closeErr = t.conn.Close()
		}
		t.mu.Unlock()
	})

	return closeErr
}

func (t *baseTransport) send(command []byte) error {
	select {
	case <-t.done:
		return fmt.Errorf("transport closed")
	default:
	}

	cmd := make([]byte, len(command))
	copy(cmd, command)

	select {
	case t.txQueue <- cmd:
		return nil
	default:
		<-t.txQueue
		t.txQueue <- cmd
		return nil
	}
}

func (t *baseTransport) writeLoop() {
	for {
		select {
		case <-t.done:
			return
		case cmd := <-t.txQueue:
			t.drainCommand(cmd)
		}
	}
}

func (t *baseTransport) drainCommand(cmd []byte) {
	for {
		t.mu.Lock()
		connected := t.connected
		conn := t.conn
		t.mu.Unlock()

		if !connected {
			select {
			case <-t.done:
				return
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}

		if err := writeCommand(conn, cmd); err != nil {
			t.mu.Lock()
			t.connected = false
			if t.conn != nil {
				_ = t.conn.Close()
			}
			t.mu.Unlock()
			continue
		}

		return
	}
}

func (t *baseTransport) readLoopWithReconnect() {
	for {
		t.mu.Lock()
		conn := t.conn
		responseFn := t.onResponse
		errorFn := t.onError
		t.mu.Unlock()

		readLoop(conn, t.done, t.parser, responseFn, errorFn)

		select {
		case <-t.done:
			return
		default:
		}

		t.mu.Lock()
		t.connected = false
		if t.conn != nil {
			_ = t.conn.Close()
		}
		t.mu.Unlock()

		t.mu.Lock()
		disconnectFn := t.onDisconnect
		t.mu.Unlock()
		if disconnectFn != nil {
			disconnectFn()
		}

		if !t.reconnect() {
			return
		}
	}
}

func (t *baseTransport) reconnect() bool {
	interval := t.config.ReconnectInterval

	for {
		select {
		case <-t.done:
			return false
		case <-time.After(interval):
		}

		conn, err := t.dial(context.Background())
		if err != nil {
			t.mu.Lock()
			errFn := t.onError
			t.mu.Unlock()
			if errFn != nil {
				errFn(fmt.Errorf("reconnect: %w", err))
			}

			interval *= 2
			if interval > t.config.MaxReconnectInterval {
				interval = t.config.MaxReconnectInterval
			}
			continue
		}

		t.parser.Reset()

		t.mu.Lock()
		t.conn = conn
		t.connected = true
		reconnectFn := t.onReconnect
		t.mu.Unlock()

		if reconnectFn != nil {
			reconnectFn()
		}

		return true
	}
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
