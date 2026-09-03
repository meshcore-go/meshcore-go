package node

import (
	"bytes"
	"testing"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

func mustPacketFromBytes(t *testing.T, data []byte) *meshcore.Packet {
	t.Helper()
	pkt, err := meshcore.PacketFromBytes(data)
	if err != nil {
		t.Fatalf("PacketFromBytes() error = %v", err)
	}
	return pkt
}

func makeDirectPacket(payloadType byte, path []byte, payload []byte) []byte {
	out := []byte{meshcore.MakeHeader(meshcore.RouteTypeDirect, payloadType, 0), byte(len(path))}
	out = append(out, path...)
	out = append(out, payload...)
	return out
}

func makePacketWithPath(routeType, payloadType, pathLength byte, path []byte, payload []byte) []byte {
	out := []byte{meshcore.MakeHeader(routeType, payloadType, 0), pathLength}
	out = append(out, path...)
	out = append(out, payload...)
	return out
}

type testRouterOpts struct {
	identity     meshcore.LocalIdentity
	allowForward func(*meshcore.Packet) bool
	allowPacket  func(*meshcore.Packet) bool
	send         func([]byte, uint8) error
	sendDirect   func([]byte) error
}

func newTestRouter(opts testRouterOpts) *router {
	n := &Node{
		identity: opts.identity,
	}
	n.allowForward = opts.allowForward
	n.allowPacket = opts.allowPacket
	n.router.node = n
	n.router.send = opts.send
	n.router.sendDirect = opts.sendDirect
	return &n.router
}

func TestRouter_FloodDedup(t *testing.T) {
	r := newTestRouter(testRouterOpts{identity: seedIdentity(0x01)})
	data := makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0x01, 0x02})

	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDeliver {
		t.Fatalf("first route() = %v, want %v", got, RouteActionDeliver)
	}
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDrop {
		t.Fatalf("second route() = %v, want %v", got, RouteActionDrop)
	}
}

func TestRouter_FloodForward(t *testing.T) {
	identity := seedIdentity(0x01)
	var sent [][]byte
	r := newTestRouter(testRouterOpts{
		identity:     identity,
		allowForward: func(*meshcore.Packet) bool { return true },
		send: func(data []byte, _ uint8) error {
			sent = append(sent, append([]byte(nil), data...))
			return nil
		},
	})

	pkt := mustPacketFromBytes(t, makeFloodPacket(meshcore.PayloadTypeGrpTxt, []byte{0xAA, 0xBB}))
	if got := r.route(pkt); got != RouteActionDeliver {
		t.Fatalf("route() = %v, want %v", got, RouteActionDeliver)
	}
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if pkt.PathHashCount() != 0 {
		t.Fatalf("original PathHashCount() = %d, want 0", pkt.PathHashCount())
	}

	forwarded := mustPacketFromBytes(t, sent[0])
	if got := forwarded.PathHashCount(); got != 1 {
		t.Fatalf("forwarded PathHashCount() = %d, want 1", got)
	}
	hashes := forwarded.PathHashes()
	if len(hashes) != 1 || !bytes.Equal(hashes[0], identity.Hash()) {
		t.Fatalf("forwarded path hash = %x, want %x", forwarded.Path, identity.Hash())
	}
}

func TestRouter_FloodNoForwardByDefault(t *testing.T) {
	called := false
	r := newTestRouter(testRouterOpts{
		identity: seedIdentity(0x01),
		send: func([]byte, uint8) error {
			called = true
			return nil
		},
	})

	pkt := mustPacketFromBytes(t, makeFloodPacket(meshcore.PayloadTypeGrpTxt, []byte{0x01}))
	if got := r.route(pkt); got != RouteActionDeliver {
		t.Fatalf("route() = %v, want %v", got, RouteActionDeliver)
	}
	if called {
		t.Fatal("send() called, want no forward")
	}
}

func TestRouter_FloodPathFull(t *testing.T) {
	called := false
	path := bytes.Repeat([]byte{0xAA, 0xBB}, meshcore.MaxPathSize/2)
	pkt := mustPacketFromBytes(t, makePacketWithPath(meshcore.RouteTypeFlood, meshcore.PayloadTypeGrpTxt, 0x60, path, []byte{0x01}))
	r := newTestRouter(testRouterOpts{
		identity:     seedIdentity(0x01),
		allowForward: func(*meshcore.Packet) bool { return true },
		send: func([]byte, uint8) error {
			called = true
			return nil
		},
	})

	if got := r.route(pkt); got != RouteActionDeliver {
		t.Fatalf("route() = %v, want %v", got, RouteActionDeliver)
	}
	if called {
		t.Fatal("send() called for full path")
	}
}

func TestRouter_DirectForward(t *testing.T) {
	identity := seedIdentity(0x01)
	other := seedIdentity(0x02)
	var sent [][]byte
	r := newTestRouter(testRouterOpts{
		identity:     identity,
		allowForward: func(*meshcore.Packet) bool { return true },
		sendDirect: func(data []byte) error {
			sent = append(sent, append([]byte(nil), data...))
			return nil
		},
	})

	path := []byte{identity.Hash()[0], other.Hash()[0]}
	pkt := mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeAdvert, path, []byte{0x99}))
	if got := r.route(pkt); got != RouteActionForward {
		t.Fatalf("route() = %v, want %v", got, RouteActionForward)
	}
	if len(sent) != 1 {
		t.Fatalf("send count = %d, want 1", len(sent))
	}
	if got := pkt.PathHashCount(); got != 1 {
		t.Fatalf("PathHashCount() = %d, want 1", got)
	}
	if !bytes.Equal(pkt.Path, []byte{other.Hash()[0]}) {
		t.Fatalf("Path = %x, want %x", pkt.Path, []byte{other.Hash()[0]})
	}

	forwarded := mustPacketFromBytes(t, sent[0])
	if got := forwarded.PathHashCount(); got != 1 {
		t.Fatalf("forwarded PathHashCount() = %d, want 1", got)
	}
	if !bytes.Equal(forwarded.Path, []byte{other.Hash()[0]}) {
		t.Fatalf("forwarded Path = %x, want %x", forwarded.Path, []byte{other.Hash()[0]})
	}
}

func TestRouter_DirectNotNextHop(t *testing.T) {
	identity := seedIdentity(0x01)
	next := seedIdentity(0x02)
	called := false
	r := newTestRouter(testRouterOpts{
		identity:     identity,
		allowForward: func(*meshcore.Packet) bool { return true },
		send: func([]byte, uint8) error {
			called = true
			return nil
		},
	})

	pkt := mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeAdvert, []byte{next.Hash()[0]}, []byte{0x01}))
	if got := r.route(pkt); got != RouteActionDrop {
		t.Fatalf("route() = %v, want %v", got, RouteActionDrop)
	}
	if called {
		t.Fatal("send() called for wrong next hop")
	}
}

func TestRouter_DirectDedup(t *testing.T) {
	identity := seedIdentity(0x01)
	other := seedIdentity(0x02)
	var sends int
	r := newTestRouter(testRouterOpts{
		identity:     identity,
		allowForward: func(*meshcore.Packet) bool { return true },
		sendDirect: func([]byte) error {
			sends++
			return nil
		},
	})

	data := makeDirectPacket(meshcore.PayloadTypeAdvert, []byte{identity.Hash()[0], other.Hash()[0]}, []byte{0x44})
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionForward {
		t.Fatalf("first route() = %v, want %v", got, RouteActionForward)
	}
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDrop {
		t.Fatalf("second route() = %v, want %v", got, RouteActionDrop)
	}
	if sends != 1 {
		t.Fatalf("send count = %d, want 1", sends)
	}
}

func TestRouter_SendPacketMarksSeen(t *testing.T) {
	radio := &mockRadio{}
	n := New(seedIdentity(0x03), radio)
	called := false
	n.OnPacket(meshcore.PayloadTypeAdvert, func(*meshcore.Packet) {
		called = true
	})

	pkt := mustPacketFromBytes(t, makeFloodPacket(meshcore.PayloadTypeAdvert, []byte{0xDE, 0xAD}))
	if err := n.SendPacket(pkt); err != nil {
		t.Fatalf("SendPacket() error = %v", err)
	}

	data, err := pkt.ToBytes()
	if err != nil {
		t.Fatalf("ToBytes() error = %v", err)
	}
	radio.inject(data)

	if called {
		t.Fatal("handler called for self-originated packet")
	}
}

func TestRouter_NonFloodNonDirect_Dedup(t *testing.T) {
	r := newTestRouter(testRouterOpts{identity: seedIdentity(0x01)})
	data := makeDirectPacket(meshcore.PayloadTypeAdvert, nil, []byte{0x33})

	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDeliver {
		t.Fatalf("first route() = %v, want %v", got, RouteActionDeliver)
	}
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDrop {
		t.Fatalf("second route() = %v, want %v", got, RouteActionDrop)
	}
}

func TestRouter_DirectNextHopNoForward_Dropped(t *testing.T) {
	identity := seedIdentity(0x01)
	other := seedIdentity(0x02)
	r := newTestRouter(testRouterOpts{identity: identity})

	path := []byte{identity.Hash()[0], other.Hash()[0]}
	pkt := mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeTxtMsg, path, []byte{0x55}))
	if got := r.route(pkt); got != RouteActionDrop {
		t.Fatalf("route() = %v, want %v (hops remaining, no forward → drop)", got, RouteActionDrop)
	}

	r = newTestRouter(testRouterOpts{identity: identity, allowPacket: func(*meshcore.Packet) bool { return true }})
	if got := r.route(mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeTxtMsg, path, []byte{0x55}))); got != RouteActionDeliver {
		t.Fatalf("route() = %v, want %v (allowPacket opts in)", got, RouteActionDeliver)
	}
}

func TestRouter_FloodForwardPolicy(t *testing.T) {
	cases := []struct {
		name    string
		typ     byte
		forward bool
	}{
		{"ACK", meshcore.PayloadTypeAck, true},
		{"PATH", meshcore.PayloadTypePath, true},
		{"REQ", meshcore.PayloadTypeReq, true},
		{"RESPONSE", meshcore.PayloadTypeResponse, true},
		{"TXT_MSG", meshcore.PayloadTypeTxtMsg, true},
		{"ANON_REQ", meshcore.PayloadTypeAnonReq, true},
		{"GRP_TXT", meshcore.PayloadTypeGrpTxt, true},
		{"GRP_DATA", meshcore.PayloadTypeGrpData, true},
		{"ADVERT_garbage", meshcore.PayloadTypeAdvert, false},
		{"TRACE", meshcore.PayloadTypeTrace, false},
		{"MULTIPART", meshcore.PayloadTypeMultiPart, false},
		{"CONTROL", meshcore.PayloadTypeControl, false},
		{"RAW_CUSTOM", meshcore.PayloadTypeRawCustom, false},
		{"unknown_0x0C", 0x0C, false},
		{"unknown_0x0E", 0x0E, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			r := newTestRouter(testRouterOpts{
				identity:     seedIdentity(0x01),
				allowForward: func(*meshcore.Packet) bool { return true },
				send:         func([]byte, uint8) error { sends++; return nil },
			})
			pkt := mustPacketFromBytes(t, makeFloodPacket(tc.typ, []byte{0x01, 0x02, 0x03, 0x04, 0x05}))
			if got := r.route(pkt); got != RouteActionDeliver {
				t.Fatalf("route() = %v, want %v (flood packets are always delivered)", got, RouteActionDeliver)
			}
			if (sends == 1) != tc.forward {
				t.Fatalf("sends = %d, want forward=%v", sends, tc.forward)
			}
		})
	}
}

func TestRouter_FloodAdvertForwardOnlyWhenVerified(t *testing.T) {
	self := seedIdentity(0x01)
	other := seedIdentity(0x02)
	newRouter := func(sends *int) *router {
		return newTestRouter(testRouterOpts{
			identity:     self,
			allowForward: func(*meshcore.Packet) bool { return true },
			send:         func([]byte, uint8) error { *sends++; return nil },
		})
	}
	advertPacket := func(t *testing.T, adv *meshcore.Advert) *meshcore.Packet {
		payload, err := adv.ToBytes()
		if err != nil {
			t.Fatalf("Advert.ToBytes() error = %v", err)
		}
		return mustPacketFromBytes(t, makeFloodPacket(meshcore.PayloadTypeAdvert, payload))
	}

	var sends int
	newRouter(&sends).route(advertPacket(t, makeSignedAdvert(other, 100, "peer")))
	if sends != 1 {
		t.Fatalf("valid advert: sends = %d, want 1", sends)
	}

	sends = 0
	forged := makeSignedAdvert(other, 100, "peer")
	forged.Signature[0] ^= 0xFF
	newRouter(&sends).route(advertPacket(t, forged))
	if sends != 0 {
		t.Fatalf("forged advert: sends = %d, want 0", sends)
	}

	sends = 0
	newRouter(&sends).route(advertPacket(t, makeSignedAdvert(self, 100, "me")))
	if sends != 0 {
		t.Fatalf("own advert: sends = %d, want 0", sends)
	}
}

func TestRouter_DirectControlZeroHopOnly(t *testing.T) {
	identity := seedIdentity(0x01)
	sends := 0
	r := newTestRouter(testRouterOpts{
		identity:     identity,
		allowForward: func(*meshcore.Packet) bool { return true },
		sendDirect:   func([]byte) error { sends++; return nil },
	})

	withHops := makeDirectPacket(meshcore.PayloadTypeControl, []byte{identity.Hash()[0]}, []byte{0x80, 0x01})
	if got := r.route(mustPacketFromBytes(t, withHops)); got != RouteActionDrop {
		t.Fatalf("route(control, 1 hop) = %v, want %v", got, RouteActionDrop)
	}
	if sends != 0 {
		t.Fatalf("sends = %d, want 0", sends)
	}

	zeroHop := makeDirectPacket(meshcore.PayloadTypeControl, nil, []byte{0x80, 0x01})
	if got := r.route(mustPacketFromBytes(t, zeroHop)); got != RouteActionDeliver {
		t.Fatalf("route(control, 0 hops) = %v, want %v", got, RouteActionDeliver)
	}
}

// A relayed direct packet is re-transmitted but never handed to local handlers.
func TestNode_DirectRelayNotDelivered(t *testing.T) {
	identity := seedIdentity(0x01)
	other := seedIdentity(0x02)
	radio := &mockRadio{}
	n := New(identity, radio, WithAllowForwardHandler(func(*meshcore.Packet) bool { return true }))
	defer n.Stop()

	delivered := false
	n.OnPacket(meshcore.PayloadTypeTxtMsg, func(*meshcore.Packet) { delivered = true })

	radio.inject(makeDirectPacket(meshcore.PayloadTypeTxtMsg, []byte{identity.Hash()[0], other.Hash()[0]}, []byte{0x01, 0x02, 0x03}))
	time.Sleep(100 * time.Millisecond)

	if delivered {
		t.Fatal("relayed direct packet was delivered to local handlers")
	}
	if got := len(radio.sentData()); got != 1 {
		t.Fatalf("relayed sends = %d, want 1", got)
	}

	// Path exhausted: delivered, not relayed.
	radio.inject(makeDirectPacket(meshcore.PayloadTypeTxtMsg, nil, []byte{0x04, 0x05, 0x06}))
	if !delivered {
		t.Fatal("zero-hop direct packet was not delivered")
	}
}

func TestRouter_ForwardErrorReported(t *testing.T) {
	var got error
	r := newTestRouter(testRouterOpts{
		identity:     seedIdentity(0x01),
		allowForward: func(*meshcore.Packet) bool { return true },
		send:         func([]byte, uint8) error { return ErrTxQueueFull },
	})
	r.node.errH = func(err error) { got = err }

	r.route(mustPacketFromBytes(t, makeFloodPacket(meshcore.PayloadTypeGrpTxt, []byte{0x01})))
	if got != ErrTxQueueFull {
		t.Fatalf("error handler got %v, want %v", got, ErrTxQueueFull)
	}
}

func TestRouter_DirectNotNextHop_AllowPacketTrue(t *testing.T) {
	identity := seedIdentity(0x01)
	next := seedIdentity(0x02)
	r := newTestRouter(testRouterOpts{
		identity:    identity,
		allowPacket: func(*meshcore.Packet) bool { return true },
	})

	pkt := mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeAdvert, []byte{next.Hash()[0]}, []byte{0x01}))
	if got := r.route(pkt); got != RouteActionDeliver {
		t.Fatalf("route() = %v, want %v (allowPacket=true)", got, RouteActionDeliver)
	}
}

func TestRouter_DirectNotNextHop_AllowPacketFalse(t *testing.T) {
	identity := seedIdentity(0x01)
	next := seedIdentity(0x02)
	r := newTestRouter(testRouterOpts{
		identity:    identity,
		allowPacket: func(*meshcore.Packet) bool { return false },
	})

	pkt := mustPacketFromBytes(t, makeDirectPacket(meshcore.PayloadTypeAdvert, []byte{next.Hash()[0]}, []byte{0x01}))
	if got := r.route(pkt); got != RouteActionDrop {
		t.Fatalf("route() = %v, want %v (allowPacket=false)", got, RouteActionDrop)
	}
}

func TestRouter_DirectNotNextHop_AllowPacketDedup(t *testing.T) {
	identity := seedIdentity(0x01)
	next := seedIdentity(0x02)
	r := newTestRouter(testRouterOpts{
		identity:    identity,
		allowPacket: func(*meshcore.Packet) bool { return true },
	})

	data := makeDirectPacket(meshcore.PayloadTypeAdvert, []byte{next.Hash()[0]}, []byte{0x77})
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDeliver {
		t.Fatalf("first route() = %v, want %v", got, RouteActionDeliver)
	}
	if got := r.route(mustPacketFromBytes(t, data)); got != RouteActionDrop {
		t.Fatalf("second route() = %v, want %v (dedup)", got, RouteActionDrop)
	}
}
