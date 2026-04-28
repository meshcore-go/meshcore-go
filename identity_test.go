package meshcore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestNewIdentity(t *testing.T) {
	var pub [PubKeySize]byte
	for i := range pub {
		pub[i] = byte(i)
	}

	id := NewIdentity(pub)
	if id.PublicKey() != pub {
		t.Errorf("PublicKey() = %x, want %x", id.PublicKey(), pub)
	}
}

func TestNewIdentityFromBytes(t *testing.T) {
	t.Run("valid 32 bytes", func(t *testing.T) {
		pub := make([]byte, PubKeySize)
		pub[0] = 0xAB
		id, err := NewIdentityFromBytes(pub)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id.PublicKey()[0] != 0xAB {
			t.Errorf("PublicKey()[0] = 0x%02x, want 0xAB", id.PublicKey()[0])
		}
	})

	t.Run("wrong length", func(t *testing.T) {
		_, err := NewIdentityFromBytes([]byte{0x01, 0x02})
		if err == nil {
			t.Fatal("expected error for short slice, got nil")
		}
	})
}

func TestNewIdentityFromHex(t *testing.T) {
	pubHex := "d7b4b39ccf9f10d3b0d0597c0e5f0d5886b8df23c7f8ad0cc702f932b9a3ba86"
	id, err := NewIdentityFromHex(pubHex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != pubHex {
		t.Errorf("String() = %s, want %s", id.String(), pubHex)
	}

	_, err = NewIdentityFromHex("not-hex")
	if err == nil {
		t.Fatal("expected error for invalid hex, got nil")
	}

	_, err = NewIdentityFromHex("aabb")
	if err == nil {
		t.Fatal("expected error for wrong length hex, got nil")
	}
}

func TestIdentityPublicKeyBytes(t *testing.T) {
	var pub [PubKeySize]byte
	pub[31] = 0xFF
	id := NewIdentity(pub)

	b := id.PublicKeyBytes()
	if len(b) != PubKeySize {
		t.Fatalf("PublicKeyBytes() len = %d, want %d", len(b), PubKeySize)
	}
	if b[31] != 0xFF {
		t.Errorf("PublicKeyBytes()[31] = 0x%02x, want 0xFF", b[31])
	}

	b[31] = 0x00
	if id.PublicKey()[31] != 0xFF {
		t.Error("PublicKeyBytes() should return a copy, not a reference")
	}
}

func TestIdentityPrefix(t *testing.T) {
	var pub [PubKeySize]byte
	for i := range pub {
		pub[i] = byte(i + 0xA0)
	}
	id := NewIdentity(pub)
	prefix := id.Prefix()
	for i := 0; i < 6; i++ {
		if prefix[i] != pub[i] {
			t.Errorf("Prefix()[%d] = 0x%02x, want 0x%02x", i, prefix[i], pub[i])
		}
	}
}

func TestIdentityHash(t *testing.T) {
	var pub [PubKeySize]byte
	pub[0] = 0x42
	id := NewIdentity(pub)
	h := id.Hash()
	if len(h) != PathHashSize {
		t.Fatalf("Hash() len = %d, want %d", len(h), PathHashSize)
	}
	if h[0] != 0x42 {
		t.Errorf("Hash()[0] = 0x%02x, want 0x42", h[0])
	}
}

func TestIdentityIsHashMatch(t *testing.T) {
	var pub [PubKeySize]byte
	for i := range pub {
		pub[i] = byte(i)
	}
	id := NewIdentity(pub)

	t.Run("single byte match", func(t *testing.T) {
		if !id.IsHashMatch([]byte{0x00}) {
			t.Error("expected match for first byte")
		}
	})

	t.Run("multi byte match", func(t *testing.T) {
		if !id.IsHashMatch([]byte{0x00, 0x01, 0x02}) {
			t.Error("expected match for first 3 bytes")
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		if id.IsHashMatch([]byte{0xFF}) {
			t.Error("expected no match for 0xFF")
		}
	})

	t.Run("empty hash", func(t *testing.T) {
		if id.IsHashMatch([]byte{}) {
			t.Error("expected no match for empty hash")
		}
	})

	t.Run("hash longer than key", func(t *testing.T) {
		long := make([]byte, PubKeySize+1)
		if id.IsHashMatch(long) {
			t.Error("expected no match for oversized hash")
		}
	})
}

func TestIdentityMatches(t *testing.T) {
	var pub [PubKeySize]byte
	pub[0] = 0x01
	a := NewIdentity(pub)
	b := NewIdentity(pub)

	if !a.Matches(b) {
		t.Error("identical keys should match")
	}

	pub[0] = 0x02
	c := NewIdentity(pub)
	if a.Matches(c) {
		t.Error("different keys should not match")
	}
}

func TestIdentityIsZero(t *testing.T) {
	zero := Identity{}
	if !zero.IsZero() {
		t.Error("zero-value Identity should be zero")
	}

	var pub [PubKeySize]byte
	pub[0] = 0x01
	nonZero := NewIdentity(pub)
	if nonZero.IsZero() {
		t.Error("non-zero Identity should not be zero")
	}
}

func TestIdentityVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var pubArr [PubKeySize]byte
	copy(pubArr[:], pub)
	id := NewIdentity(pubArr)

	msg := []byte("test message")
	sig := ed25519.Sign(priv, msg)

	if !id.Verify(msg, sig) {
		t.Error("valid signature should verify")
	}

	if id.Verify([]byte("wrong message"), sig) {
		t.Error("wrong message should not verify")
	}

	badSig := make([]byte, len(sig))
	copy(badSig, sig)
	badSig[0] ^= 0xFF
	if id.Verify(msg, badSig) {
		t.Error("tampered signature should not verify")
	}
}

func TestIdentityX25519PublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var pubArr [PubKeySize]byte
	copy(pubArr[:], pub)
	id := NewIdentity(pubArr)

	x25519Pub, err := id.X25519PublicKey()
	if err != nil {
		t.Fatalf("X25519PublicKey: %v", err)
	}
	if len(x25519Pub) != 32 {
		t.Errorf("X25519 key length = %d, want 32", len(x25519Pub))
	}
}

func TestIdentityString(t *testing.T) {
	var pub [PubKeySize]byte
	for i := range pub {
		pub[i] = byte(i)
	}
	id := NewIdentity(pub)
	want := hex.EncodeToString(pub[:])
	if id.String() != want {
		t.Errorf("String() = %s, want %s", id.String(), want)
	}
}

func TestGenerateLocalIdentity(t *testing.T) {
	li, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity: %v", err)
	}
	if li.IsZero() {
		t.Error("generated identity should not be zero")
	}
	if len(li.PrivateKey()) != ed25519.PrivateKeySize {
		t.Errorf("PrivateKey length = %d, want %d", len(li.PrivateKey()), ed25519.PrivateKeySize)
	}
}

func TestNewLocalIdentityFromSeed(t *testing.T) {
	var seed [ed25519.SeedSize]byte
	for i := range seed {
		seed[i] = byte(i + 1)
	}

	li1 := NewLocalIdentityFromSeed(seed)
	li2 := NewLocalIdentityFromSeed(seed)

	if !li1.Matches(li2.Identity) {
		t.Error("same seed should produce same identity")
	}
	if li1.Seed() != seed {
		t.Error("Seed() should return the original seed")
	}
}

func TestNewLocalIdentityFromPrivateKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	li, err := NewLocalIdentityFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pub := priv.Public().(ed25519.PublicKey)
	if !bytes.Equal(li.PublicKeyBytes(), pub) {
		t.Error("public key should match the one derived from private key")
	}

	_, err = NewLocalIdentityFromPrivateKey([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for short private key, got nil")
	}
}

func TestLocalIdentitySignAndVerify(t *testing.T) {
	li, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity: %v", err)
	}

	msg := []byte("hello meshcore")
	sig := li.Sign(msg)

	if !li.Verify(msg, sig) {
		t.Error("identity should verify its own signature")
	}

	peer := li.Identity
	if !peer.Verify(msg, sig) {
		t.Error("peer Identity should verify signature from LocalIdentity")
	}
}

func TestLocalIdentitySharedSecret(t *testing.T) {
	alice, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity alice: %v", err)
	}
	bob, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity bob: %v", err)
	}

	secretAB, err := alice.SharedSecret(bob.Identity)
	if err != nil {
		t.Fatalf("alice.SharedSecret(bob): %v", err)
	}
	secretBA, err := bob.SharedSecret(alice.Identity)
	if err != nil {
		t.Fatalf("bob.SharedSecret(alice): %v", err)
	}

	if !bytes.Equal(secretAB, secretBA) {
		t.Errorf("shared secrets differ:\n  A->B: %x\n  B->A: %x", secretAB, secretBA)
	}
	if len(secretAB) != 32 {
		t.Errorf("shared secret length = %d, want 32", len(secretAB))
	}
}

func TestLocalIdentitySharedSecretMatchesDeriveSharedSecret(t *testing.T) {
	li, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity: %v", err)
	}
	peer, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity peer: %v", err)
	}

	viaIdentity, err := li.SharedSecret(peer.Identity)
	if err != nil {
		t.Fatalf("SharedSecret: %v", err)
	}

	seed := li.Seed()
	viaDirect, err := DeriveSharedSecret(seed[:], peer.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedSecret: %v", err)
	}

	if !bytes.Equal(viaIdentity, viaDirect) {
		t.Errorf("SharedSecret and DeriveSharedSecret disagree:\n  identity: %x\n  direct:   %x", viaIdentity, viaDirect)
	}
}

func TestLocalIdentityX25519PrivateKey(t *testing.T) {
	li, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity: %v", err)
	}

	x25519Priv := li.X25519PrivateKey()
	if len(x25519Priv) != 32 {
		t.Errorf("X25519PrivateKey length = %d, want 32", len(x25519Priv))
	}

	seed := li.Seed()
	direct := edPrivateToX25519(seed[:])
	if !bytes.Equal(x25519Priv, direct) {
		t.Error("X25519PrivateKey should match edPrivateToX25519")
	}
}

func TestLocalIdentityPrivateKeyIsCopy(t *testing.T) {
	li, err := GenerateLocalIdentity(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateLocalIdentity: %v", err)
	}

	priv1 := li.PrivateKey()
	priv2 := li.PrivateKey()

	if !bytes.Equal(priv1, priv2) {
		t.Error("consecutive PrivateKey() calls should return equal keys")
	}

	priv1[0] ^= 0xFF
	priv2Again := li.PrivateKey()
	if priv1[0] == priv2Again[0] {
		t.Error("PrivateKey() should return a copy, not a reference")
	}
}

func TestLocalIdentityEncryptDecryptRoundTrip(t *testing.T) {
	alice, _ := GenerateLocalIdentity(rand.Reader)
	bob, _ := GenerateLocalIdentity(rand.Reader)

	secret, err := alice.SharedSecret(bob.Identity)
	if err != nil {
		t.Fatalf("SharedSecret: %v", err)
	}

	plaintext := []byte("secret mesh data")
	encrypted, err := EncryptThenMAC(secret, plaintext)
	if err != nil {
		t.Fatalf("EncryptThenMAC: %v", err)
	}

	bobSecret, err := bob.SharedSecret(alice.Identity)
	if err != nil {
		t.Fatalf("bob SharedSecret: %v", err)
	}

	decrypted, err := MACThenDecrypt(bobSecret, encrypted)
	if err != nil {
		t.Fatalf("MACThenDecrypt: %v", err)
	}
	if decrypted == nil {
		t.Fatal("MACThenDecrypt returned nil (MAC rejected)")
	}

	trimmed := bytes.TrimRight(decrypted, "\x00")
	if !bytes.Equal(trimmed, plaintext) {
		t.Errorf("decrypted = %q, want %q", trimmed, plaintext)
	}
}
