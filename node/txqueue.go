package node

import (
	"time"
)

const DefaultMaxTxQueue = 32

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

func newTxQueue(max int) *txQueue {
	return &txQueue{
		entries: make([]txEntry, 0, max),
		max:     max,
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

// peek returns the highest-priority entry whose scheduled time has passed.
// Returns nil if nothing is ready.
func (q *txQueue) peek(now time.Time) *txEntry {
	var best *txEntry
	bestIdx := -1
	for i := range q.entries {
		e := &q.entries[i]
		if e.scheduledAt.After(now) {
			continue
		}
		if best == nil || e.priority < best.priority || (e.priority == best.priority && e.seq < best.seq) {
			best = e
			bestIdx = i
		}
	}
	_ = bestIdx
	return best
}

func (q *txQueue) pop(now time.Time) *txEntry {
	bestIdx := -1
	var best *txEntry
	for i := range q.entries {
		e := &q.entries[i]
		if e.scheduledAt.After(now) {
			continue
		}
		if best == nil || e.priority < best.priority || (e.priority == best.priority && e.seq < best.seq) {
			best = e
			bestIdx = i
		}
	}
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
