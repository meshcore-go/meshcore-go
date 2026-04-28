package node

import (
	"crypto/ed25519"
	"testing"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

func makeSignedAdvert(id meshcore.LocalIdentity, timestamp uint32, name string) *meshcore.Advert {
	appData := &meshcore.AdvertAppData{Type: "CHAT", Name: name}
	raw, err := appData.ToBytes()
	if err != nil {
		panic(err)
	}
	adv := &meshcore.Advert{
		PublicKey:  id.Identity,
		Timestamp:  timestamp,
		RawAppData: raw,
	}
	adv.Sign(id.PrivateKey())

	b, err := adv.ToBytes()
	if err != nil {
		panic(err)
	}
	parsed, err := meshcore.AdvertFromBytes(b)
	if err != nil {
		panic(err)
	}
	return parsed
}

func peerIdentity(seedByte byte) meshcore.LocalIdentity {
	var seed [ed25519.SeedSize]byte
	seed[0] = seedByte
	return meshcore.NewLocalIdentityFromSeed(seed)
}

func TestPeerTable_UpdateAndLookup(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x01)
	adv := makeSignedAdvert(id, 100, "alice")

	if ok := pt.Update(adv, -5, -80, nil); !ok {
		t.Fatal("Update returned false for new peer")
	}
	if pt.Count() != 1 {
		t.Fatalf("Count = %d, want 1", pt.Count())
	}

	p := pt.Lookup(id.PublicKey())
	if p == nil {
		t.Fatal("Lookup returned nil")
	}
	if p.Name != "alice" {
		t.Errorf("Name = %q, want %q", p.Name, "alice")
	}
	if p.Type != "CHAT" {
		t.Errorf("Type = %q, want %q", p.Type, "CHAT")
	}
	if p.SNR != -5 {
		t.Errorf("SNR = %d, want -5", p.SNR)
	}
	if p.RSSI != -80 {
		t.Errorf("RSSI = %d, want -80", p.RSSI)
	}
	if p.LastAdvertTimestamp != 100 {
		t.Errorf("LastAdvertTimestamp = %d, want 100", p.LastAdvertTimestamp)
	}
}

func TestPeerTable_UpdateExisting(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x02)

	pt.Update(makeSignedAdvert(id, 100, "bob"), -5, -80, nil)
	pt.Update(makeSignedAdvert(id, 200, "bob2"), -3, -70, nil)

	if pt.Count() != 1 {
		t.Fatalf("Count = %d, want 1", pt.Count())
	}
	p := pt.Lookup(id.PublicKey())
	if p.Name != "bob2" {
		t.Errorf("Name = %q, want %q", p.Name, "bob2")
	}
	if p.LastAdvertTimestamp != 200 {
		t.Errorf("LastAdvertTimestamp = %d, want 200", p.LastAdvertTimestamp)
	}
	if p.SNR != -3 {
		t.Errorf("SNR = %d, want -3", p.SNR)
	}
}

func TestPeerTable_ReplayProtection(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x03)

	pt.Update(makeSignedAdvert(id, 100, "carol"), 0, 0, nil)

	if ok := pt.Update(makeSignedAdvert(id, 100, "carol-replay"), 0, 0, nil); ok {
		t.Fatal("Update should reject equal timestamp (replay)")
	}
	if ok := pt.Update(makeSignedAdvert(id, 50, "carol-old"), 0, 0, nil); ok {
		t.Fatal("Update should reject older timestamp")
	}

	p := pt.Lookup(id.PublicKey())
	if p.Name != "carol" {
		t.Errorf("Name = %q, want %q (unchanged)", p.Name, "carol")
	}
}

func TestPeerTable_LookupByHash(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x04)
	pt.Update(makeSignedAdvert(id, 100, "dave"), 0, 0, nil)

	hash := id.Hash()
	peers := pt.LookupByHash(hash)
	if len(peers) == 0 {
		t.Fatal("LookupByHash returned no results")
	}
	found := false
	for _, p := range peers {
		if p.Identity.Matches(id.Identity) {
			found = true
		}
	}
	if !found {
		t.Error("expected peer not found in LookupByHash results")
	}
}

func TestPeerTable_LookupByHash_NoMatch(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x05)
	pt.Update(makeSignedAdvert(id, 100, "eve"), 0, 0, nil)

	peers := pt.LookupByHash([]byte{0xFF})
	for _, p := range peers {
		if p.Identity.Matches(id.Identity) {
			t.Error("should not match unrelated hash")
		}
	}
}

func TestPeerTable_Remove(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x06)
	pt.Update(makeSignedAdvert(id, 100, "frank"), 0, 0, nil)

	if !pt.Remove(id.PublicKey()) {
		t.Fatal("Remove returned false for existing peer")
	}
	if pt.Count() != 0 {
		t.Fatalf("Count = %d, want 0", pt.Count())
	}
	if pt.Remove(id.PublicKey()) {
		t.Error("Remove returned true for already-removed peer")
	}
}

func TestPeerTable_Peers(t *testing.T) {
	pt := NewPeerTable(10)
	for i := range byte(5) {
		id := peerIdentity(0x10 + i)
		pt.Update(makeSignedAdvert(id, uint32(100+i), "peer"), 0, 0, nil)
	}

	peers := pt.Peers()
	if len(peers) != 5 {
		t.Fatalf("Peers() returned %d, want 5", len(peers))
	}
}

func TestPeerTable_LRUEviction(t *testing.T) {
	pt := NewPeerTable(3)

	ids := make([]meshcore.LocalIdentity, 4)
	for i := range ids {
		ids[i] = peerIdentity(0x20 + byte(i))
	}

	for i := range 3 {
		pt.Update(makeSignedAdvert(ids[i], uint32(100+i), "p"), 0, 0, nil)
		time.Sleep(time.Millisecond)
	}

	if pt.Count() != 3 {
		t.Fatalf("Count = %d, want 3", pt.Count())
	}

	pt.Update(makeSignedAdvert(ids[3], 200, "new"), 0, 0, nil)

	if pt.Count() != 3 {
		t.Fatalf("Count after eviction = %d, want 3", pt.Count())
	}

	if p := pt.Lookup(ids[0].PublicKey()); p != nil {
		t.Error("oldest peer should have been evicted")
	}
	if p := pt.Lookup(ids[3].PublicKey()); p == nil {
		t.Error("new peer should exist")
	}
}

func TestPeerTable_LookupReturnsACopy(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x30)
	pt.Update(makeSignedAdvert(id, 100, "original"), 0, 0, nil)

	p := pt.Lookup(id.PublicKey())
	p.Name = "mutated"

	p2 := pt.Lookup(id.PublicKey())
	if p2.Name != "original" {
		t.Errorf("Name = %q, want %q — Lookup should return a copy", p2.Name, "original")
	}
}

func TestPeerTable_UpdateStoresPath(t *testing.T) {
	pt := NewPeerTable(10)
	id := peerIdentity(0x31)
	path := []byte{0xAA, 0xBB, 0xCC}
	pt.Update(makeSignedAdvert(id, 100, "pathtest"), 0, 0, path)

	p := pt.Lookup(id.PublicKey())
	if len(p.OutPath) != 3 || p.OutPath[0] != 0xAA || p.OutPath[1] != 0xBB || p.OutPath[2] != 0xCC {
		t.Errorf("OutPath = %v, want [0xAA 0xBB 0xCC]", p.OutPath)
	}
}

func TestPeerTable_DefaultMaxPeers(t *testing.T) {
	pt := NewPeerTable(0)
	if pt.maxPeers != DefaultMaxPeers {
		t.Errorf("maxPeers = %d, want %d", pt.maxPeers, DefaultMaxPeers)
	}
}
