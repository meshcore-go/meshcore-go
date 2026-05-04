package node

import (
	"sync"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

const queuedRadioTickInterval = 50 * time.Millisecond

// QueuedRadio wraps a plain Radio with a serializing transmit queue.
// All sends are funneled through a single goroutine, preventing
// concurrent writes to the underlying radio.
type QueuedRadio struct {
	inner Radio
	mu    sync.Mutex
	queue *txQueue
	done  chan struct{}
}

func NewQueuedRadio(inner Radio, maxQueue int, done chan struct{}) *QueuedRadio {
	q := &QueuedRadio{
		inner: inner,
		queue: newTxQueue(maxQueue),
		done:  done,
	}
	go q.loop()
	return q
}

func (q *QueuedRadio) Enqueue(data []byte, priority uint8, delay time.Duration) bool {
	q.mu.Lock()
	ok := q.queue.add(data, priority, time.Now().Add(delay))
	q.mu.Unlock()
	return ok
}

func (q *QueuedRadio) SendData(data []byte) error {
	if !q.Enqueue(data, PrioritySend, 0) {
		return ErrTxQueueFull
	}
	return nil
}

func (q *QueuedRadio) TxQueueLen() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queue.len()
}

func (q *QueuedRadio) SetDataHandler(h func(*meshcore.Packet))      { q.inner.SetDataHandler(h) }
func (q *QueuedRadio) SetRawDataHandler(h func([]byte, int8, int8)) { q.inner.SetRawDataHandler(h) }
func (q *QueuedRadio) AddOutboundHandler(h func([]byte))            { q.inner.AddOutboundHandler(h) }
func (q *QueuedRadio) Close() error                                 { return q.inner.Close() }

func (q *QueuedRadio) loop() {
	ticker := time.NewTicker(queuedRadioTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-q.done:
			return
		case <-ticker.C:
			q.drain()
		}
	}
}

func (q *QueuedRadio) drain() {
	for {
		q.mu.Lock()
		entry := q.queue.pop(time.Now())
		q.mu.Unlock()
		if entry == nil {
			return
		}
		_ = q.inner.SendData(entry.data)
	}
}

var _ TxRadio = (*QueuedRadio)(nil)
