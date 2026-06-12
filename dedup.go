package meshcore

import (
	"sync"
)

const (
	MaxPacketHashes = 160 // matches C++ SimpleMeshTables (128+32)
)

// DedupCache is a fixed-size circular buffer that tracks recently seen packets
// to prevent duplicate processing. It matches the behaviour of MeshCore's
// SimpleMeshTables. Safe for concurrent use.
type DedupCache struct {
	mu       sync.Mutex
	hashes   [MaxPacketHashes][PacketHashSize]byte
	hashNext int
}

// HasSeen reports whether this packet has been seen before. If not, it records
// the packet so future calls with the same packet return true.
func (d *DedupCache) HasSeen(pkt *Packet) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.hasSeenHash(pkt)
}

func (d *DedupCache) hasSeenHash(pkt *Packet) bool {
	h := pkt.PacketHash()
	for i := range d.hashes {
		if d.hashes[i] == h {
			return true
		}
	}
	d.hashes[d.hashNext] = h
	d.hashNext = (d.hashNext + 1) % MaxPacketHashes
	return false
}

// MarkSeen records a packet as seen without checking. Used for self-originated
// packets to prevent relaying our own transmissions back.
func (d *DedupCache) MarkSeen(pkt *Packet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	h := pkt.PacketHash()
	d.hashes[d.hashNext] = h
	d.hashNext = (d.hashNext + 1) % MaxPacketHashes
}
