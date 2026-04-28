package node

import (
	"testing"
	"time"
)

func TestTxQueue_AddAndPop(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()
	if !q.add([]byte{0x01}, 1, now) {
		t.Fatal("add returned false")
	}
	if q.len() != 1 {
		t.Fatalf("len = %d, want 1", q.len())
	}
	e := q.pop(now)
	if e == nil {
		t.Fatal("pop returned nil")
	}
	if e.data[0] != 0x01 {
		t.Errorf("data = %x, want 01", e.data)
	}
	if q.len() != 0 {
		t.Errorf("len after pop = %d, want 0", q.len())
	}
}

func TestTxQueue_Full(t *testing.T) {
	q := newTxQueue(2)
	now := time.Now()
	q.add([]byte{1}, 0, now)
	q.add([]byte{2}, 0, now)
	if q.add([]byte{3}, 0, now) {
		t.Error("add should return false when full")
	}
}

func TestTxQueue_PriorityOrder(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()
	q.add([]byte{0x03}, 3, now)
	q.add([]byte{0x01}, 1, now)
	q.add([]byte{0x02}, 2, now)
	q.add([]byte{0x00}, 0, now)

	expected := []byte{0x00, 0x01, 0x02, 0x03}
	for i, want := range expected {
		e := q.pop(now)
		if e == nil {
			t.Fatalf("pop %d returned nil", i)
		}
		if e.data[0] != want {
			t.Errorf("pop %d: data = %x, want %x", i, e.data[0], want)
		}
	}
}

func TestTxQueue_FIFOWithinPriority(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()
	q.add([]byte{0xA0}, 1, now)
	q.add([]byte{0xA1}, 1, now)
	q.add([]byte{0xA2}, 1, now)

	for _, want := range []byte{0xA0, 0xA1, 0xA2} {
		e := q.pop(now)
		if e == nil {
			t.Fatal("pop returned nil")
		}
		if e.data[0] != want {
			t.Errorf("data = %x, want %x", e.data[0], want)
		}
	}
}

func TestTxQueue_ScheduledInFuture(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()
	q.add([]byte{0x01}, 0, now.Add(time.Hour))

	if e := q.pop(now); e != nil {
		t.Error("should not pop future-scheduled entry")
	}
	if q.count(now) != 0 {
		t.Error("count should be 0 for future entries")
	}
	if q.count(now.Add(2*time.Hour)) != 1 {
		t.Error("count should be 1 when scheduled time passed")
	}
}

func TestTxQueue_PeekDoesNotRemove(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()
	q.add([]byte{0x01}, 0, now)

	e := q.peek(now)
	if e == nil {
		t.Fatal("peek returned nil")
	}
	if q.len() != 1 {
		t.Error("peek should not remove entry")
	}
}

func TestTxQueue_PopEmpty(t *testing.T) {
	q := newTxQueue(8)
	if q.pop(time.Now()) != nil {
		t.Error("pop on empty should return nil")
	}
}

func TestTxQueue_MixedPriorityAndSchedule(t *testing.T) {
	q := newTxQueue(8)
	now := time.Now()

	q.add([]byte{0x01}, 0, now.Add(time.Hour))
	q.add([]byte{0x02}, 2, now)
	q.add([]byte{0x03}, 1, now)

	e := q.pop(now)
	if e == nil || e.data[0] != 0x03 {
		t.Errorf("should pop priority 1 (ready), got %v", e)
	}
	e = q.pop(now)
	if e == nil || e.data[0] != 0x02 {
		t.Errorf("should pop priority 2 (ready), got %v", e)
	}
	e = q.pop(now)
	if e != nil {
		t.Error("priority 0 is future-scheduled, should not pop")
	}
}
