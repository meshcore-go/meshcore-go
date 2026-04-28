package node

import (
	"encoding/binary"
	"testing"

	meshcore "github.com/meshcore-go/meshcore-go"
)

func dedupPacket(payload []byte) *meshcore.Packet {
	return &meshcore.Packet{
		Header:  meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		Payload: append([]byte(nil), payload...),
	}
}

func dedupACKPacket(crc uint32) *meshcore.Packet {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, crc)
	return &meshcore.Packet{
		Header:  meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAck, 0),
		Payload: payload,
	}
}

func TestDedup_FirstSeenReturnsFalse(t *testing.T) {
	var d dedupCache
	if got := d.hasSeen(dedupPacket([]byte{0x01})); got {
		t.Fatal("hasSeen() = true, want false")
	}
}

func TestDedup_SecondSeenReturnsTrue(t *testing.T) {
	var d dedupCache
	pkt := dedupPacket([]byte{0x01})
	if got := d.hasSeen(pkt); got {
		t.Fatal("first hasSeen() = true, want false")
	}
	if got := d.hasSeen(pkt); !got {
		t.Fatal("second hasSeen() = false, want true")
	}
}

func TestDedup_DifferentPacketsNotDuplicate(t *testing.T) {
	var d dedupCache
	if got := d.hasSeen(dedupPacket([]byte{0x01})); got {
		t.Fatal("first packet hasSeen() = true, want false")
	}
	if got := d.hasSeen(dedupPacket([]byte{0x02})); got {
		t.Fatal("second packet hasSeen() = true, want false")
	}
}

func TestDedup_ACK_FirstSeenReturnsFalse(t *testing.T) {
	var d dedupCache
	if got := d.hasSeen(dedupACKPacket(0x11223344)); got {
		t.Fatal("hasSeen() = true, want false")
	}
}

func TestDedup_ACK_DuplicateCRC(t *testing.T) {
	var d dedupCache
	pkt := dedupACKPacket(0x11223344)
	if got := d.hasSeen(pkt); got {
		t.Fatal("first hasSeen() = true, want false")
	}
	if got := d.hasSeen(pkt); !got {
		t.Fatal("second hasSeen() = false, want true")
	}
}

func TestDedup_RingBufferEviction(t *testing.T) {
	var d dedupCache
	first := dedupPacket([]byte{0x00, 0x00})
	if got := d.hasSeen(first); got {
		t.Fatal("first hasSeen() = true, want false")
	}

	for i := 1; i <= maxPacketHashes; i++ {
		pkt := dedupPacket([]byte{byte(i), byte(i >> 8)})
		if got := d.hasSeen(pkt); got {
			t.Fatalf("hasSeen() = true for unique packet %d", i)
		}
	}

	if got := d.hasSeen(first); got {
		t.Fatal("hasSeen() = true after eviction, want false")
	}
}

func TestDedup_ACK_RingBufferEviction(t *testing.T) {
	var d dedupCache
	first := dedupACKPacket(1)
	if got := d.hasSeen(first); got {
		t.Fatal("first hasSeen() = true, want false")
	}

	for i := uint32(2); i <= maxACKEntries+1; i++ {
		pkt := dedupACKPacket(i)
		if got := d.hasSeen(pkt); got {
			t.Fatalf("hasSeen() = true for unique ACK %d", i)
		}
	}

	if got := d.hasSeen(first); got {
		t.Fatal("hasSeen() = true after ACK eviction, want false")
	}
}

func TestDedup_MarkSeen(t *testing.T) {
	var d dedupCache
	pkt := dedupPacket([]byte{0xAA, 0xBB})
	d.markSeen(pkt)
	if got := d.hasSeen(pkt); !got {
		t.Fatal("hasSeen() = false after markSeen(), want true")
	}
}

func TestDedup_MarkSeen_ACK(t *testing.T) {
	var d dedupCache
	pkt := dedupACKPacket(0xAABBCCDD)
	d.markSeen(pkt)
	if got := d.hasSeen(pkt); !got {
		t.Fatal("hasSeen() = false after markSeen(), want true")
	}
}
