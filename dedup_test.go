package meshcore

import (
	"encoding/binary"
	"testing"
)

func dedupPacket(payload []byte) *Packet {
	return &Packet{
		Header:  MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0),
		Payload: append([]byte(nil), payload...),
	}
}

func dedupACKPacket(crc uint32) *Packet {
	payload := make([]byte, 4)
	binary.LittleEndian.PutUint32(payload, crc)
	return &Packet{
		Header:  MakeHeader(RouteTypeFlood, PayloadTypeAck, 0),
		Payload: payload,
	}
}

func TestDedup_FirstSeenReturnsFalse(t *testing.T) {
	var d DedupCache
	if got := d.HasSeen(dedupPacket([]byte{0x01})); got {
		t.Fatal("HasSeen() = true, want false")
	}
}

func TestDedup_SecondSeenReturnsTrue(t *testing.T) {
	var d DedupCache
	pkt := dedupPacket([]byte{0x01})
	if got := d.HasSeen(pkt); got {
		t.Fatal("first HasSeen() = true, want false")
	}
	if got := d.HasSeen(pkt); !got {
		t.Fatal("second HasSeen() = false, want true")
	}
}

func TestDedup_DifferentPacketsNotDuplicate(t *testing.T) {
	var d DedupCache
	if got := d.HasSeen(dedupPacket([]byte{0x01})); got {
		t.Fatal("first packet HasSeen() = true, want false")
	}
	if got := d.HasSeen(dedupPacket([]byte{0x02})); got {
		t.Fatal("second packet HasSeen() = true, want false")
	}
}

func TestDedup_ACK_FirstSeenReturnsFalse(t *testing.T) {
	var d DedupCache
	if got := d.HasSeen(dedupACKPacket(0x11223344)); got {
		t.Fatal("HasSeen() = true, want false")
	}
}

func TestDedup_ACK_DuplicateCRC(t *testing.T) {
	var d DedupCache
	pkt := dedupACKPacket(0x11223344)
	if got := d.HasSeen(pkt); got {
		t.Fatal("first HasSeen() = true, want false")
	}
	if got := d.HasSeen(pkt); !got {
		t.Fatal("second HasSeen() = false, want true")
	}
}

func TestDedup_RingBufferEviction(t *testing.T) {
	var d DedupCache
	first := dedupPacket([]byte{0x00, 0x00})
	if got := d.HasSeen(first); got {
		t.Fatal("first HasSeen() = true, want false")
	}

	for i := 1; i <= MaxPacketHashes; i++ {
		pkt := dedupPacket([]byte{byte(i), byte(i >> 8)})
		if got := d.HasSeen(pkt); got {
			t.Fatalf("HasSeen() = true for unique packet %d", i)
		}
	}

	if got := d.HasSeen(first); got {
		t.Fatal("HasSeen() = true after eviction, want false")
	}
}

func TestDedup_ACK_RingBufferEviction(t *testing.T) {
	var d DedupCache
	first := dedupACKPacket(1)
	if got := d.HasSeen(first); got {
		t.Fatal("first HasSeen() = true, want false")
	}

	for i := uint32(2); i <= MaxACKEntries+1; i++ {
		pkt := dedupACKPacket(i)
		if got := d.HasSeen(pkt); got {
			t.Fatalf("HasSeen() = true for unique ACK %d", i)
		}
	}

	if got := d.HasSeen(first); got {
		t.Fatal("HasSeen() = true after ACK eviction, want false")
	}
}

func TestDedup_MarkSeen(t *testing.T) {
	var d DedupCache
	pkt := dedupPacket([]byte{0xAA, 0xBB})
	d.MarkSeen(pkt)
	if got := d.HasSeen(pkt); !got {
		t.Fatal("HasSeen() = false after MarkSeen(), want true")
	}
}

func TestDedup_MarkSeen_ACK(t *testing.T) {
	var d DedupCache
	pkt := dedupACKPacket(0xAABBCCDD)
	d.MarkSeen(pkt)
	if got := d.HasSeen(pkt); !got {
		t.Fatal("HasSeen() = false after MarkSeen(), want true")
	}
}
