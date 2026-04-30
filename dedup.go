package meshcore

import (
	"encoding/binary"
	"sync"
)

const (
	MaxPacketHashes = 128
	MaxACKEntries   = 64
)

// DedupCache is a fixed-size circular buffer that tracks recently seen packets
// to prevent duplicate processing. It matches the behaviour of MeshCore's
// SimpleMeshTables. Safe for concurrent use.
type DedupCache struct {
	mu       sync.Mutex
	hashes   [MaxPacketHashes][PacketHashSize]byte
	hashNext int
	acks     [MaxACKEntries]uint32
	ackNext  int
}

// HasSeen reports whether this packet has been seen before. If not, it records
// the packet so future calls with the same packet return true.
func (d *DedupCache) HasSeen(pkt *Packet) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if pkt.PayloadType() == PayloadTypeAck {
		return d.hasSeenACK(pkt)
	}
	return d.hasSeenHash(pkt)
}

func (d *DedupCache) hasSeenACK(pkt *Packet) bool {
	if len(pkt.Payload) < 4 {
		return false
	}
	crc := binary.LittleEndian.Uint32(pkt.Payload[:4])
	for i := range d.acks {
		if d.acks[i] == crc {
			return true
		}
	}
	d.acks[d.ackNext] = crc
	d.ackNext = (d.ackNext + 1) % MaxACKEntries
	return false
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

	if pkt.PayloadType() == PayloadTypeAck {
		if len(pkt.Payload) < 4 {
			return
		}
		crc := binary.LittleEndian.Uint32(pkt.Payload[:4])
		d.acks[d.ackNext] = crc
		d.ackNext = (d.ackNext + 1) % MaxACKEntries
		return
	}

	h := pkt.PacketHash()
	d.hashes[d.hashNext] = h
	d.hashNext = (d.hashNext + 1) % MaxPacketHashes
}
