package meshcore

import (
	"bytes"
	"testing"
)

func TestPacket_IsRouteFlood(t *testing.T) {
	tests := []struct {
		name   string
		header byte
		want   bool
	}{
		{"flood", MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), true},
		{"transport flood", MakeHeader(RouteTypeTransportFlood, PayloadTypeAdvert, 0), true},
		{"direct", MakeHeader(RouteTypeDirect, PayloadTypeAdvert, 0), false},
		{"transport direct", MakeHeader(RouteTypeTransportDirect, PayloadTypeAdvert, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{Header: tt.header}
			if got := pkt.IsRouteFlood(); got != tt.want {
				t.Fatalf("IsRouteFlood() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPacket_IsRouteDirect(t *testing.T) {
	tests := []struct {
		name   string
		header byte
		want   bool
	}{
		{"direct", MakeHeader(RouteTypeDirect, PayloadTypeAdvert, 0), true},
		{"transport direct", MakeHeader(RouteTypeTransportDirect, PayloadTypeAdvert, 0), true},
		{"flood", MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), false},
		{"transport flood", MakeHeader(RouteTypeTransportFlood, PayloadTypeAdvert, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{Header: tt.header}
			if got := pkt.IsRouteDirect(); got != tt.want {
				t.Fatalf("IsRouteDirect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPacket_IsTransport(t *testing.T) {
	tests := []struct {
		name   string
		header byte
		want   bool
	}{
		{"transport flood", MakeHeader(RouteTypeTransportFlood, PayloadTypeAdvert, 0), true},
		{"transport direct", MakeHeader(RouteTypeTransportDirect, PayloadTypeAdvert, 0), true},
		{"flood", MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), false},
		{"direct", MakeHeader(RouteTypeDirect, PayloadTypeAdvert, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkt := &Packet{Header: tt.header}
			if got := pkt.IsTransport(); got != tt.want {
				t.Fatalf("IsTransport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPacket_AppendPathHash(t *testing.T) {
	pkt := &Packet{pathHashSize: 1}
	hash := []byte{0xAB}

	if ok := pkt.AppendPathHash(hash); !ok {
		t.Fatal("AppendPathHash() = false, want true")
	}
	if got := pkt.PathHashCount(); got != 1 {
		t.Fatalf("PathHashCount() = %d, want 1", got)
	}
	if !bytes.Equal(pkt.Path, hash) {
		t.Fatalf("Path = %x, want %x", pkt.Path, hash)
	}
	if pkt.PathLength != 0x01 {
		t.Fatalf("PathLength = 0x%02x, want 0x01", pkt.PathLength)
	}
}

func TestPacket_AppendPathHash_Full(t *testing.T) {
	pkt := &Packet{
		PathLength:    0x60,
		Path:          bytes.Repeat([]byte{0xAA, 0xBB}, MaxPathSize/2),
		pathHashSize:  2,
		pathHashCount: MaxPathSize / 2,
	}

	if ok := pkt.AppendPathHash([]byte{0xCC, 0xDD}); ok {
		t.Fatal("AppendPathHash() = true, want false")
	}
	if got := pkt.PathHashCount(); got != MaxPathSize/2 {
		t.Fatalf("PathHashCount() = %d, want %d", got, MaxPathSize/2)
	}
}

func TestPacket_RemoveFirstPathHash(t *testing.T) {
	pkt := &Packet{pathHashSize: 1}
	for _, hash := range []byte{0x01, 0x02, 0x03} {
		if ok := pkt.AppendPathHash([]byte{hash}); !ok {
			t.Fatalf("AppendPathHash(%x) = false, want true", hash)
		}
	}

	if ok := pkt.RemoveFirstPathHash(); !ok {
		t.Fatal("RemoveFirstPathHash() = false, want true")
	}
	if got := pkt.PathHashCount(); got != 2 {
		t.Fatalf("PathHashCount() = %d, want 2", got)
	}
	if !bytes.Equal(pkt.Path, []byte{0x02, 0x03}) {
		t.Fatalf("Path = %x, want 0203", pkt.Path)
	}
	if pkt.PathLength != 0x02 {
		t.Fatalf("PathLength = 0x%02x, want 0x02", pkt.PathLength)
	}
}

func TestPacket_RemoveFirstPathHash_Empty(t *testing.T) {
	pkt := &Packet{pathHashSize: 1}
	if ok := pkt.RemoveFirstPathHash(); ok {
		t.Fatal("RemoveFirstPathHash() = true, want false")
	}
}

func TestPacket_PacketHash_Deterministic(t *testing.T) {
	pkt := &Packet{
		Header:  MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0),
		Payload: []byte{0x01, 0x02, 0x03},
	}

	h1 := pkt.PacketHash()
	h2 := pkt.PacketHash()
	if h1 != h2 {
		t.Fatalf("PacketHash() mismatch: %x != %x", h1, h2)
	}
}

func TestPacket_PacketHash_DifferentPayload(t *testing.T) {
	pkt1 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), Payload: []byte{0x01}}
	pkt2 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), Payload: []byte{0x02}}

	if pkt1.PacketHash() == pkt2.PacketHash() {
		t.Fatal("PacketHash() matched for different payloads")
	}
}

func TestPacket_PacketHash_DifferentType(t *testing.T) {
	payload := []byte{0x10, 0x20, 0x30}
	pkt1 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), Payload: payload}
	pkt2 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeTrace, 0), Payload: payload}

	if pkt1.PacketHash() == pkt2.PacketHash() {
		t.Fatal("PacketHash() matched for different payload types")
	}
}

func TestPacket_PacketHash_TraceIncludesPathLen(t *testing.T) {
	payload := []byte{0xAA, 0xBB}
	pkt1 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeTrace, 0), PathLength: 0x00, Payload: payload}
	pkt2 := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeTrace, 0), PathLength: 0x01, Payload: payload}

	if pkt1.PacketHash() == pkt2.PacketHash() {
		t.Fatal("PacketHash() matched for TRACE packets with different PathLength")
	}
}

func TestPacket_PacketHash_Size(t *testing.T) {
	pkt := &Packet{Header: MakeHeader(RouteTypeFlood, PayloadTypeAdvert, 0), Payload: []byte{0x01, 0x02}}
	h := pkt.PacketHash()
	if got := len(h[:]); got != PacketHashSize {
		t.Fatalf("len(PacketHash()) = %d, want %d", got, PacketHashSize)
	}
}
