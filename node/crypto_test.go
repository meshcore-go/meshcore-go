package node

import (
	"bytes"
	"sync"
	"testing"

	meshcore "github.com/meshcore-go/meshcore-go"
)

func TestSecretCache_ComputesOnMiss(t *testing.T) {
	self := seedIdentity(0xA0)
	peer := seedIdentity(0xA1)
	sc := newSecretCache(self)

	got, err := sc.get(peer.Identity)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if len(got) != meshcore.PubKeySize {
		t.Fatalf("secret length = %d, want %d", len(got), meshcore.PubKeySize)
	}

	want, err := self.SharedSecret(peer.Identity)
	if err != nil {
		t.Fatalf("SharedSecret error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("cached secret does not match direct SharedSecret computation")
	}
}

func TestSecretCache_ReturnsCachedValue(t *testing.T) {
	self := seedIdentity(0xA2)
	peer := seedIdentity(0xA3)
	sc := newSecretCache(self)

	first, _ := sc.get(peer.Identity)
	second, _ := sc.get(peer.Identity)

	if !bytes.Equal(first, second) {
		t.Error("second call returned different secret")
	}
}

func TestSecretCache_ReturnsCopy(t *testing.T) {
	self := seedIdentity(0xA4)
	peer := seedIdentity(0xA5)
	sc := newSecretCache(self)

	first, _ := sc.get(peer.Identity)
	first[0] ^= 0xFF

	second, _ := sc.get(peer.Identity)
	want, _ := self.SharedSecret(peer.Identity)
	if !bytes.Equal(second, want) {
		t.Error("mutation of returned slice affected cache")
	}
}

func TestSecretCache_DifferentPeersDifferentSecrets(t *testing.T) {
	self := seedIdentity(0xA6)
	peer1 := seedIdentity(0xA7)
	peer2 := seedIdentity(0xA8)
	sc := newSecretCache(self)

	s1, _ := sc.get(peer1.Identity)
	s2, _ := sc.get(peer2.Identity)

	if bytes.Equal(s1, s2) {
		t.Error("different peers should produce different shared secrets")
	}
}

func TestSecretCache_Concurrent(t *testing.T) {
	self := seedIdentity(0xA9)
	peer := seedIdentity(0xAA)
	sc := newSecretCache(self)

	want, _ := self.SharedSecret(peer.Identity)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := sc.get(peer.Identity)
			if err != nil {
				t.Errorf("get error: %v", err)
				return
			}
			if !bytes.Equal(got, want) {
				t.Errorf("concurrent get returned wrong secret")
			}
		}()
	}
	wg.Wait()
}

func TestNode_SharedSecret(t *testing.T) {
	radio := &mockRadio{}
	self := seedIdentity(0xAB)
	peer := seedIdentity(0xAC)
	n := New(self, radio)

	got, err := n.SharedSecret(peer.Identity)
	if err != nil {
		t.Fatalf("SharedSecret error: %v", err)
	}

	want, _ := self.SharedSecret(peer.Identity)
	if !bytes.Equal(got, want) {
		t.Error("Node.SharedSecret does not match direct computation")
	}
}

func TestNode_SharedSecretSymmetric(t *testing.T) {
	radio1 := &mockRadio{}
	radio2 := &mockRadio{}
	id1 := seedIdentity(0xAD)
	id2 := seedIdentity(0xAE)
	n1 := New(id1, radio1)
	n2 := New(id2, radio2)

	s1, _ := n1.SharedSecret(id2.Identity)
	s2, _ := n2.SharedSecret(id1.Identity)

	if !bytes.Equal(s1, s2) {
		t.Error("shared secrets should be symmetric between two nodes")
	}
}
