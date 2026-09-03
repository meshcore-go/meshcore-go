package node

import (
	"time"
)

const DefaultMaxTxQueue = 64

type txEntry struct {
	data        []byte
	priority    uint8
	scheduledAt time.Time
	seq         uint64
}

// txQueue is a bounded priority queue for outbound packets.
// Lower priority number = higher priority. Within the same priority level,
// entries are dequeued in FIFO order (by insertion sequence number).
type txQueue struct {
	entries []txEntry
	max     int
	nextSeq uint64
}

func newTxQueue(size int) *txQueue {
	return &txQueue{
		entries: make([]txEntry, 0, size),
		max:     size,
	}
}

func (q *txQueue) add(data []byte, priority uint8, scheduledAt time.Time) bool {
	if len(q.entries) >= q.max {
		return false
	}
	q.entries = append(q.entries, txEntry{
		data:        data,
		priority:    priority,
		scheduledAt: scheduledAt,
		seq:         q.nextSeq,
	})
	q.nextSeq++
	return true
}

// readyIdx returns the index of the highest-priority ready entry (FIFO within a priority), or -1.
func (q *txQueue) readyIdx(now time.Time) int {
	best := -1
	for i := range q.entries {
		e := &q.entries[i]
		if e.scheduledAt.After(now) {
			continue
		}
		if best < 0 || e.priority < q.entries[best].priority ||
			(e.priority == q.entries[best].priority && e.seq < q.entries[best].seq) {
			best = i
		}
	}
	return best
}

// peek returns the highest-priority ready entry, or nil.
func (q *txQueue) peek(now time.Time) *txEntry {
	i := q.readyIdx(now)
	if i < 0 {
		return nil
	}
	return &q.entries[i]
}

func (q *txQueue) pop(now time.Time) *txEntry {
	bestIdx := q.readyIdx(now)
	if bestIdx < 0 {
		return nil
	}
	out := q.entries[bestIdx]
	q.entries[bestIdx] = q.entries[len(q.entries)-1]
	q.entries = q.entries[:len(q.entries)-1]
	return &out
}

func (q *txQueue) len() int {
	return len(q.entries)
}

func (q *txQueue) count(now time.Time) int {
	n := 0
	for i := range q.entries {
		if !q.entries[i].scheduledAt.After(now) {
			n++
		}
	}
	return n
}
