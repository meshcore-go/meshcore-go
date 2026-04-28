package node

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

func fixedEstimator(ms uint32) AirtimeEstimator {
	return func(int) uint32 { return ms }
}

func TestScheduler_SendsQueuedPacket(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	var sent atomic.Int32
	sendFn := func([]byte) error {
		sent.Add(1)
		return nil
	}

	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(10))
	s := newTxScheduler(budget, 8, sendFn, done)

	s.enqueue([]byte{0x01}, 0, 0)
	time.Sleep(200 * time.Millisecond)

	if sent.Load() < 1 {
		t.Error("scheduler did not send queued packet")
	}
}

func TestScheduler_RespectsScheduleDelay(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	var sent atomic.Int32
	sendFn := func([]byte) error {
		sent.Add(1)
		return nil
	}

	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(10))
	s := newTxScheduler(budget, 8, sendFn, done)

	s.enqueue([]byte{0x01}, 0, 500*time.Millisecond)

	time.Sleep(100 * time.Millisecond)
	if sent.Load() > 0 {
		t.Error("sent too early — delay not respected")
	}

	time.Sleep(600 * time.Millisecond)
	if sent.Load() < 1 {
		t.Error("should have sent after delay")
	}
}

func TestScheduler_QueueFullReturnsFalse(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(10))
	s := newTxScheduler(budget, 2, func([]byte) error { return nil }, done)

	s.enqueue([]byte{1}, 0, time.Hour)
	s.enqueue([]byte{2}, 0, time.Hour)
	if s.enqueue([]byte{3}, 0, time.Hour) {
		t.Error("enqueue should return false when full")
	}
}

func TestScheduler_PriorityOrdering(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	var mu sync.Mutex
	var order []byte
	sendFn := func(data []byte) error {
		mu.Lock()
		order = append(order, data[0])
		mu.Unlock()
		return nil
	}

	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(1))
	s := newTxScheduler(budget, 8, sendFn, done)

	s.mu.Lock()
	s.queue.add([]byte{0x04}, PrioritySend, time.Now())
	s.queue.add([]byte{0x00}, PriorityDirectRelay, time.Now())
	s.queue.add([]byte{0x01}, PriorityFloodRelay, time.Now())
	s.mu.Unlock()

	time.Sleep(400 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(order) < 3 {
		t.Fatalf("only %d packets sent, want 3", len(order))
	}
	if order[0] != 0x00 {
		t.Errorf("first sent = %x, want 0x00 (PriorityDirectRelay)", order[0])
	}
	if order[1] != 0x01 {
		t.Errorf("second sent = %x, want 0x01 (PriorityFloodRelay)", order[1])
	}
	if order[2] != 0x04 {
		t.Errorf("third sent = %x, want 0x04 (PrioritySend)", order[2])
	}
}

func TestScheduler_StopsOnDone(t *testing.T) {
	done := make(chan struct{})
	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(10))
	_ = newTxScheduler(budget, 8, func([]byte) error { return nil }, done)
	close(done)
	time.Sleep(100 * time.Millisecond)
}

func TestScheduler_QueueLen(t *testing.T) {
	done := make(chan struct{})
	defer close(done)

	budget := newAirtimeBudget(1.0, time.Hour, fixedEstimator(10))
	s := newTxScheduler(budget, 8, func([]byte) error { return nil }, done)

	s.enqueue([]byte{1}, 0, time.Hour)
	s.enqueue([]byte{2}, 0, time.Hour)
	if s.queueLen() != 2 {
		t.Errorf("queueLen = %d, want 2", s.queueLen())
	}
}

func TestNode_SendPacketWithScheduler(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD0), radio, WithAirtimeEstimator(fixedEstimator(10)))
	defer n.Stop()

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		PathLength: 0x00,
		Path:       []byte{},
		Payload:    []byte{0xDE, 0xAD},
	}

	if err := n.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket error: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	sent := radio.sentData()
	if len(sent) < 1 {
		t.Error("packet should have been sent through scheduler")
	}
}

func TestNode_SendPacketWithoutScheduler(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD1), radio)
	defer n.Stop()

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		PathLength: 0x00,
		Path:       []byte{},
		Payload:    []byte{0xBE, 0xEF},
	}

	if err := n.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket error: %v", err)
	}

	sent := radio.sentData()
	if len(sent) != 1 {
		t.Fatalf("expected immediate send, got %d", len(sent))
	}
}

func TestNode_TxQueueLen(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD2), radio, WithAirtimeEstimator(fixedEstimator(10)))
	defer n.Stop()

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		PathLength: 0x00,
		Path:       []byte{},
		Payload:    []byte{0x01},
	}
	n.SendPacket(pkt)

	if n.TxQueueLen() < 0 {
		t.Error("TxQueueLen should not be negative")
	}
}

func TestNode_TxQueueLenWithoutScheduler(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD3), radio)
	defer n.Stop()

	if n.TxQueueLen() != 0 {
		t.Errorf("TxQueueLen = %d, want 0 without scheduler", n.TxQueueLen())
	}
}

func TestFloodRetransmitDelay_Range(t *testing.T) {
	for range 100 {
		d := FloodRetransmitDelay(100)
		maxDelay := time.Duration(5*100*52/50/2) * time.Millisecond
		if d < 0 || d > maxDelay {
			t.Errorf("FloodRetransmitDelay(100) = %v, want 0..%v", d, maxDelay)
		}
	}
}

func TestFloodRetransmitDelay_ZeroAirtime(t *testing.T) {
	d := FloodRetransmitDelay(0)
	if d != 0 {
		t.Errorf("FloodRetransmitDelay(0) = %v, want 0", d)
	}
}

func TestNode_WithAirtimeFactor(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD4), radio,
		WithAirtimeEstimator(fixedEstimator(10)),
		WithAirtimeFactor(9.0),
	)
	defer n.Stop()

	if n.scheduler == nil {
		t.Fatal("scheduler should be created with estimator")
	}
	dc := n.scheduler.budget.dutyCycle()
	want := 0.1
	if dc < want-0.01 || dc > want+0.01 {
		t.Errorf("duty cycle = %f, want ~%f for factor 9.0", dc, want)
	}
}

func TestNode_WithDutyCycleWindow(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD5), radio,
		WithAirtimeEstimator(fixedEstimator(10)),
		WithDutyCycleWindow(30*time.Minute),
	)
	defer n.Stop()

	if n.scheduler.budget.windowMs != 30*60*1000 {
		t.Errorf("windowMs = %f, want %d", n.scheduler.budget.windowMs, 30*60*1000)
	}
}

func TestNode_WithMaxTxQueue(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0xD6), radio,
		WithAirtimeEstimator(fixedEstimator(10)),
		WithMaxTxQueue(4),
	)
	defer n.Stop()

	if n.scheduler.queue.max != 4 {
		t.Errorf("queue max = %d, want 4", n.scheduler.queue.max)
	}
}
