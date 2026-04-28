package node

import (
	"math/rand/v2"
	"sync"
	"time"
)

const schedulerTickInterval = 50 * time.Millisecond

type txScheduler struct {
	mu     sync.Mutex
	budget *airtimeBudget
	queue  *txQueue
	send   func([]byte) error
	errH   func(error)

	nextTxTime time.Time
	done       chan struct{}
}

func newTxScheduler(budget *airtimeBudget, maxQueue int, send func([]byte) error, done chan struct{}) *txScheduler {
	s := &txScheduler{
		budget: budget,
		queue:  newTxQueue(maxQueue),
		send:   send,
		done:   done,
	}
	go s.loop()
	return s
}

func (s *txScheduler) setErrorHandler(h func(error)) {
	s.mu.Lock()
	s.errH = h
	s.mu.Unlock()
}

// enqueue adds serialised packet data to the transmit queue at the given
// priority and scheduled time. Returns false if the queue is full.
func (s *txScheduler) enqueue(data []byte, priority uint8, delay time.Duration) bool {
	s.mu.Lock()
	ok := s.queue.add(data, priority, time.Now().Add(delay))
	s.mu.Unlock()
	return ok
}

func (s *txScheduler) loop() {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.checkSend()
		}
	}
}

func (s *txScheduler) checkSend() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	if s.queue.count(now) == 0 {
		return
	}

	s.budget.refill(now)

	entry := s.queue.peek(now)
	if entry == nil {
		return
	}

	estAirtime := uint32(0)
	if s.budget.estimator != nil {
		estAirtime = s.budget.estimator(len(entry.data))
	}

	ok, waitMs := s.budget.canSend(estAirtime)
	if !ok {
		s.nextTxTime = now.Add(time.Duration(waitMs) * time.Millisecond)
		return
	}

	if now.Before(s.nextTxTime) {
		return
	}

	popped := s.queue.pop(now)
	if popped == nil {
		return
	}

	sendStart := time.Now()

	err := s.send(popped.data)
	if err != nil {
		if s.errH != nil {
			s.errH(err)
		}
		return
	}

	actualMs := uint64(time.Since(sendStart).Milliseconds())
	if actualMs < 1 && estAirtime > 0 {
		actualMs = uint64(estAirtime)
	}
	s.budget.deduct(actualMs)

	delay := s.budget.nextTxDelay()
	if delay > 0 {
		s.nextTxTime = time.Now().Add(delay)
	} else {
		s.nextTxTime = time.Now()
	}
}

func (s *txScheduler) queueLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.len()
}

// FloodRetransmitDelay computes a random retransmission delay for flood relay,
// matching MeshCore C++: random(0,5) × (airtime × 1.04 / 2).
func FloodRetransmitDelay(estAirtimeMs uint32) time.Duration {
	t := float64(estAirtimeMs) * 52.0 / 50.0 / 2.0
	n := rand.IntN(6)
	return time.Duration(float64(n)*t) * time.Millisecond
}
