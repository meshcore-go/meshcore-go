package node

import (
	"encoding/binary"
	"sync"

	meshcore "github.com/meshcore-go/meshcore-go"
)

const (
	maxPacketHashes = 128
	maxACKEntries   = 64
)

type dedupCache struct {
	mu       sync.Mutex
	hashes   [maxPacketHashes][meshcore.PacketHashSize]byte
	hashNext int
	acks     [maxACKEntries]uint32
	ackNext  int
}

// hasSeen reports whether this packet has been seen before. If not, it records
// the packet so future calls with the same packet return true.
// Matches MeshCore's SimpleMeshTables::hasSeen.
func (d *dedupCache) hasSeen(pkt *meshcore.Packet) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if pkt.PayloadType() == meshcore.PayloadTypeAck {
		return d.hasSeenACK(pkt)
	}
	return d.hasSeenHash(pkt)
}

func (d *dedupCache) hasSeenACK(pkt *meshcore.Packet) bool {
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
	d.ackNext = (d.ackNext + 1) % maxACKEntries
	return false
}

func (d *dedupCache) hasSeenHash(pkt *meshcore.Packet) bool {
	h := pkt.PacketHash()
	for i := range d.hashes {
		if d.hashes[i] == h {
			return true
		}
	}
	d.hashes[d.hashNext] = h
	d.hashNext = (d.hashNext + 1) % maxPacketHashes
	return false
}

// markSeen records a packet as seen without checking. Used for self-originated
// packets to prevent relaying our own transmissions back.
func (d *dedupCache) markSeen(pkt *meshcore.Packet) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if pkt.PayloadType() == meshcore.PayloadTypeAck {
		if len(pkt.Payload) < 4 {
			return
		}
		crc := binary.LittleEndian.Uint32(pkt.Payload[:4])
		d.acks[d.ackNext] = crc
		d.ackNext = (d.ackNext + 1) % maxACKEntries
		return
	}

	h := pkt.PacketHash()
	d.hashes[d.hashNext] = h
	d.hashNext = (d.hashNext + 1) % maxPacketHashes
}
