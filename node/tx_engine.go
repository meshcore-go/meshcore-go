package node

import (
	"log/slog"
	"sync"
	"time"
)

const queuedRadioTickInterval = 50 * time.Millisecond

const txBusyBackoff = 200 * time.Millisecond

type txEngine struct {
	mu         sync.Mutex
	queue      *txQueue
	budget     *airtimeBudget
	nextTxTime time.Time
	sendFn     func([]byte) error
	retryable  func(error) bool
	log        *slog.Logger
	errH       func(error)
	done       chan struct{}
}

type txEngineConfig struct {
	maxQueue  int
	log       *slog.Logger
	errH      func(error)
	budget    *airtimeBudget
	retryable func(error) bool
}

type txEngineOption func(*txEngineConfig)

func withTxMaxQueue(max int) txEngineOption {
	return func(c *txEngineConfig) {
		c.maxQueue = max
	}
}

func withTxLogger(l *slog.Logger) txEngineOption {
	return func(c *txEngineConfig) {
		c.log = l
	}
}

func withTxErrorHandler(h func(error)) txEngineOption {
	return func(c *txEngineConfig) {
		c.errH = h
	}
}

func withTxAirtimeBudget(budget *airtimeBudget) txEngineOption {
	return func(c *txEngineConfig) {
		c.budget = budget
	}
}

func withTxRetryable(fn func(error) bool) txEngineOption {
	return func(c *txEngineConfig) {
		c.retryable = fn
	}
}

func newTxEngine(sendFn func([]byte) error, done chan struct{}, opts ...txEngineOption) *txEngine {
	cfg := txEngineConfig{
		maxQueue: DefaultMaxTxQueue,
		log:      slog.Default(),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	e := &txEngine{
		queue:     newTxQueue(cfg.maxQueue),
		budget:    cfg.budget,
		sendFn:    sendFn,
		retryable: cfg.retryable,
		log:       cfg.log,
		errH:      cfg.errH,
		done:      done,
	}
	go e.loop()
	return e
}

func (e *txEngine) enqueue(data []byte, priority uint8, delay time.Duration) bool {
	e.mu.Lock()
	ok := e.queue.add(data, priority, time.Now().Add(delay))
	e.mu.Unlock()
	return ok
}

func (e *txEngine) queueLen() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.queue.len()
}

func (e *txEngine) loop() {
	ticker := time.NewTicker(queuedRadioTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			e.drain()
		}
	}
}

func (e *txEngine) drain() {
	for {
		e.mu.Lock()
		now := time.Now()

		if e.budget != nil {
			e.budget.refill(now)
		}

		entry := e.queue.peek(now)
		if entry == nil {
			e.mu.Unlock()
			return
		}

		if e.budget != nil {
			estAirtime := e.budget.estimator(len(entry.data))
			ok, waitMs := e.budget.canSend(estAirtime)
			if !ok {
				e.nextTxTime = now.Add(time.Duration(waitMs * float64(time.Millisecond)))
				e.mu.Unlock()
				return
			}
			if now.Before(e.nextTxTime) {
				e.mu.Unlock()
				return
			}
		}

		popped := e.queue.pop(now)
		e.mu.Unlock()

		if popped == nil {
			return
		}

		sendStart := time.Now()
		err := e.sendFn(popped.data)

		if err != nil {
			if e.retryable != nil && e.retryable(err) {
				e.mu.Lock()
				ok := e.queue.add(popped.data, popped.priority, time.Now().Add(txBusyBackoff))
				e.mu.Unlock()
				if ok {
					e.log.Debug("tx busy, re-enqueued packet", "error", err, "data_len", len(popped.data))
				} else {
					e.log.Warn("tx busy, queue full, dropping packet", "error", err, "data_len", len(popped.data))
				}
			} else {
				e.log.Warn("tx failed, dropping packet", "error", err, "data_len", len(popped.data))
			}
			if e.errH != nil {
				e.errH(err)
			}
			return
		}

		if e.budget != nil {
			actualMs := uint64(time.Since(sendStart).Milliseconds())
			estAirtime := e.budget.estimator(len(popped.data))
			if actualMs < 1 && estAirtime > 0 {
				actualMs = uint64(estAirtime)
			}
			e.mu.Lock()
			e.budget.deduct(actualMs)
			delay := e.budget.nextTxDelay()
			if delay > 0 {
				e.nextTxTime = time.Now().Add(delay)
			}
			e.mu.Unlock()
		}
	}
}
