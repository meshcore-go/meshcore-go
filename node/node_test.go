package node

import (
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type mockRadio struct {
	mu       sync.Mutex
	sent     [][]byte
	dataH    func(*meshcore.Packet)
	rawDataH func([]byte, int8, int8, bool)
}

func (m *mockRadio) SendData(data []byte) error {
	m.mu.Lock()
	m.sent = append(m.sent, data)
	m.mu.Unlock()
	return nil
}

func (m *mockRadio) SetDataHandler(h func(*meshcore.Packet))                { m.dataH = h }
func (m *mockRadio) SetRawDataHandler(h func([]byte, int8, int8, bool))     { m.rawDataH = h }
func (m *mockRadio) AddOutboundHandler(h func([]byte))                      {}
func (m *mockRadio) Close() error                                           { return nil }

func (m *mockRadio) inject(data []byte) {
	pkt, err := meshcore.PacketFromBytes(data)
	if err != nil {
		if m.rawDataH != nil {
			m.rawDataH(data, 0, 0, false)
		}
		return
	}
	if m.dataH != nil {
		m.dataH(pkt)
	}
	if m.rawDataH != nil {
		m.rawDataH(data, 0, 0, false)
	}
}

func (m *mockRadio) injectWithSignal(data []byte, snr int8, rssi int8) {
	pkt, err := meshcore.PacketFromBytes(data)
	if err != nil {
		if m.rawDataH != nil {
			m.rawDataH(data, snr, rssi, true)
		}
		return
	}
	pkt.SNR = snr
	pkt.RSSI = rssi
	pkt.HasSignalInfo = true
	if m.dataH != nil {
		m.dataH(pkt)
	}
	if m.rawDataH != nil {
		m.rawDataH(data, snr, rssi, true)
	}
}

func (m *mockRadio) sentData() [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(m.sent))
	copy(out, m.sent)
	return out
}

var _ Radio = (*mockRadio)(nil)

// pathLen=0x00 means hashCount=0, hashSize=1 → 0 path bytes.
func makeFloodPacket(payloadType byte, payload []byte) []byte {
	header := meshcore.MakeHeader(meshcore.RouteTypeFlood, payloadType, 0)
	out := []byte{header, 0x00}
	out = append(out, payload...)
	return out
}

func seedIdentity(seedByte byte) meshcore.LocalIdentity {
	var seed [ed25519.SeedSize]byte
	seed[0] = seedByte
	return meshcore.NewLocalIdentityFromSeed(seed)
}

func TestNode_HandlerDispatch(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x01), radio)

	var received []*meshcore.Packet
	n.OnPacket(meshcore.PayloadTypeAdvert, func(pkt *meshcore.Packet) {
		received = append(received, pkt)
	})

	radio.inject(makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0x01, 0x02}))

	if len(received) != 1 {
		t.Fatalf("expected 1 packet, got %d", len(received))
	}
	if received[0].PayloadType() != meshcore.PayloadTypeAdvert {
		t.Errorf("payload type = %d, want ADVERT", received[0].PayloadType())
	}
}

func TestNode_MultipleHandlersSameType(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x02), radio)

	callCount := 0
	n.OnPacket(meshcore.PayloadTypeAdvert, func(_ *meshcore.Packet) { callCount++ })
	n.OnPacket(meshcore.PayloadTypeAdvert, func(_ *meshcore.Packet) { callCount++ })

	radio.inject(makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0x01}))

	if callCount != 2 {
		t.Errorf("expected 2 handler calls, got %d", callCount)
	}
}

func TestNode_UnregisteredTypeDropped(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x03), radio)

	called := false
	n.OnPacket(meshcore.PayloadTypeAdvert, func(_ *meshcore.Packet) { called = true })

	// TXT_MSG: dest(1) + src(1) + mac(2) + encrypted
	radio.inject(makeFloodPacket(meshcore.PayloadTypeTxtMsg, []byte{0xCC, 0x01, 0x00, 0x00, 0xFF}))

	if called {
		t.Error("advert handler should not fire for TXT_MSG packet")
	}
}

func TestNode_SendPacket(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x05), radio)

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeAdvert, 0),
		PathLength: 0x00,
		Path:       []byte{},
		Payload:    []byte{0xDE, 0xAD},
	}

	if err := n.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket error: %v", err)
	}

	// Send goes through QueuedRadio queue
	time.Sleep(200 * time.Millisecond)

	sent := radio.sentData()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent packet, got %d", len(sent))
	}
}

func TestNode_StopPreventsDispatch(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x06), radio)

	called := false
	n.OnPacket(meshcore.PayloadTypeAdvert, func(_ *meshcore.Packet) { called = true })

	n.Stop()
	radio.inject(makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0x01}))

	if called {
		t.Error("handler should not fire after Stop")
	}
}

func TestNode_StopIdempotent(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x07), radio)
	n.Stop()
	n.Stop()
}

func TestNode_Identity(t *testing.T) {
	id := seedIdentity(0x08)
	radio := &mockRadio{}
	n := New(id, radio)

	if n.Identity().PublicKey() != id.PublicKey() {
		t.Error("identity mismatch")
	}
}

func TestNode_SetIdentity(t *testing.T) {
	idA := seedIdentity(0x08)
	idB := seedIdentity(0x09)
	radio := &mockRadio{}
	n := New(idA, radio)

	if n.Identity().PublicKey() != idA.PublicKey() {
		t.Fatal("initial identity mismatch")
	}

	n.SetIdentity(idB)

	if n.Identity().PublicKey() != idB.PublicKey() {
		t.Error("identity not updated after SetIdentity")
	}
	if n.Identity().PublicKey() == idA.PublicKey() {
		t.Error("identity still matches old identity")
	}
}

func TestNode_DataCopied(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x09), radio)

	var receivedPayload []byte
	n.OnPacket(meshcore.PayloadTypeAdvert, func(pkt *meshcore.Packet) {
		receivedPayload = append([]byte{}, pkt.Payload...)
	})

	data := makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0xAA, 0xBB})
	radio.inject(data)

	// Mutate original after dispatch.
	data[len(data)-1] = 0x00

	if len(receivedPayload) < 1 {
		t.Fatal("no payload received")
	}
	if receivedPayload[len(receivedPayload)-1] != 0xBB {
		t.Error("received payload was not copied — mutation leaked through")
	}
}

func TestNode_SignalMetadataOnPacket(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x0B), radio)

	var got *meshcore.Packet
	n.OnPacket(meshcore.PayloadTypeAdvert, func(pkt *meshcore.Packet) {
		got = pkt
	})

	radio.injectWithSignal(makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0x01, 0x02}), -7, -85)

	if got == nil {
		t.Fatal("expected packet")
	}
	if got.SNR != -7 {
		t.Errorf("SNR = %d, want -7", got.SNR)
	}
	if got.RSSI != -85 {
		t.Errorf("RSSI = %d, want -85", got.RSSI)
	}
}

func TestNode_AllPayloadTypes(t *testing.T) {
	types := []struct {
		name       string
		payloadTyp byte
	}{
		{"REQ", meshcore.PayloadTypeReq},
		{"RESPONSE", meshcore.PayloadTypeResponse},
		{"TXT_MSG", meshcore.PayloadTypeTxtMsg},
		{"ACK", meshcore.PayloadTypeAck},
		{"ADVERT", meshcore.PayloadTypeAdvert},
		{"GRP_TXT", meshcore.PayloadTypeGrpTxt},
		{"GRP_DATA", meshcore.PayloadTypeGrpData},
		{"ANON_REQ", meshcore.PayloadTypeAnonReq},
		{"PATH", meshcore.PayloadTypePath},
		{"TRACE", meshcore.PayloadTypeTrace},
		{"MULTI_PART", meshcore.PayloadTypeMultiPart},
		{"CONTROL", meshcore.PayloadTypeControl},
		{"RAW_CUSTOM", meshcore.PayloadTypeRawCustom},
	}

	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			radio := &mockRadio{}
			n := New(seedIdentity(0x0A), radio)

			called := false
			n.OnPacket(tc.payloadTyp, func(_ *meshcore.Packet) { called = true })

			radio.inject(makeFloodPacket(tc.payloadTyp, []byte{0x01, 0x02, 0x03, 0x04}))

			if !called {
				t.Errorf("handler not called for payload type %s (0x%02X)", tc.name, tc.payloadTyp)
			}
		})
	}
}
