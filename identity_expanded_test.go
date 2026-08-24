package meshcore

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha512"
	"testing"
)

// Known-good vector from MeshCore's LocalIdentity::validatePrivateKey.
const (
	fwTestPrv = "7065e18fd9fabb70c1ed90dca19907de698c88b709ea146eafd93d9b830c7b60c4681193c79bbc39945ba8064104bb618f8fd7a84a0af6f57033d6e8ddcd6471"
	fwTestPub = "1ec77175b0918ed206f9ae04ec136d6d5d4315bb26305427f645b492e9350c10"
)

// expandSeedForTest derives the expanded key for a seed: SHA-512, clamped.
func expandSeedForTest(seed []byte) []byte {
	h := sha512.Sum512(seed)
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	return h[:]
}

func TestExpandedKey_DerivesFirmwarePubKey(t *testing.T) {
	li, err := NewLocalIdentityFromExpandedKey(hexDecode(t, fwTestPrv))
	if err != nil {
		t.Fatalf("NewLocalIdentityFromExpandedKey: %v", err)
	}
	if !bytes.Equal(li.PublicKeyBytes(), hexDecode(t, fwTestPub)) {
		t.Fatalf("derived pubkey mismatch:\n got  %x\n want %s", li.PublicKeyBytes(), fwTestPub)
	}
}

func TestExpandedKey_SignVerifiesWithStdEd25519(t *testing.T) {
	li, err := NewLocalIdentityFromExpandedKey(hexDecode(t, fwTestPrv))
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("meshcore advert payload")
	sig := li.Sign(msg)
	if len(sig) != SignatureSize {
		t.Fatalf("signature length = %d, want %d", len(sig), SignatureSize)
	}
	// A peer verifies with the ordinary public key via standard ed25519.
	if !ed25519.Verify(hexDecode(t, fwTestPub), msg, sig) {
		t.Fatal("expanded-key signature failed standard ed25519 verification")
	}
}

// A seed-based identity and the expanded key derived from that same seed must be
// interchangeable: same pubkey, and identical (deterministic) signatures.
func TestExpandedKey_MatchesSeedIdentity(t *testing.T) {
	var seed [ed25519.SeedSize]byte
	copy(seed[:], hexDecode(t, "1122334455667788112233445566778811223344556677881122334455667788"))
	seedID := NewLocalIdentityFromSeed(seed)

	// The expanded key for this seed is exactly what crypto/ed25519 uses internally.
	expID, err := NewLocalIdentityFromExpandedKey(expandSeedForTest(seed[:]))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(seedID.PublicKeyBytes(), expID.PublicKeyBytes()) {
		t.Fatalf("pubkey mismatch: seed %x vs expanded %x", seedID.PublicKeyBytes(), expID.PublicKeyBytes())
	}

	msg := []byte("determinism check")
	if !bytes.Equal(seedID.Sign(msg), expID.Sign(msg)) {
		t.Fatal("seed and expanded-key signatures differ for the same identity")
	}
}

// ECDH between two expanded-key identities must agree from both sides (mirrors
// the firmware's validatePrivateKey key-exchange consistency check).
func TestExpandedKey_SharedSecretAgrees(t *testing.T) {
	a, err := NewLocalIdentityFromExpandedKey(hexDecode(t, fwTestPrv))
	if err != nil {
		t.Fatal(err)
	}
	var bs [ed25519.SeedSize]byte
	copy(bs[:], hexDecode(t, "99aabbcc99aabbcc99aabbcc99aabbcc99aabbcc99aabbcc99aabbcc99aabbcc"))
	bID, err := NewLocalIdentityFromExpandedKey(expandSeedForTest(bs[:]))
	if err != nil {
		t.Fatal(err)
	}

	ssAB, err := a.SharedSecret(bID.Identity)
	if err != nil {
		t.Fatal(err)
	}
	ssBA, err := bID.SharedSecret(a.Identity)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ssAB, ssBA) {
		t.Fatalf("shared secrets differ:\n a->b %x\n b->a %x", ssAB, ssBA)
	}
	if bytes.Equal(ssAB, make([]byte, len(ssAB))) {
		t.Fatal("shared secret is all zero")
	}
}
