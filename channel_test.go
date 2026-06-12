package meshcore

import "testing"

func TestNewChannelFromPSK(t *testing.T) {
	psk := make([]byte, 16)
	psk[0] = 0xAA
	ch, err := NewChannelFromPSK("test", psk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch.Name != "test" {
		t.Errorf("Name = %q, want %q", ch.Name, "test")
	}
	if ch.PSK[0] != 0xAA {
		t.Errorf("PSK[0] = 0x%02x, want 0xAA", ch.PSK[0])
	}
	if ch.Hash == 0 && ch.PSK == [16]byte{} {
		t.Error("Hash should be derived from PSK")
	}
}

func TestNewChannelFromPSK_InvalidLength(t *testing.T) {
	_, err := NewChannelFromPSK("bad", []byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for wrong PSK length")
	}
}

func TestNewChannelFromBase64(t *testing.T) {
	// 16 zero bytes in base64
	ch, err := NewChannelFromBase64("b64test", "AAAAAAAAAAAAAAAAAAAAAA==")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch.Name != "b64test" {
		t.Errorf("Name = %q, want %q", ch.Name, "b64test")
	}
	if ch.PSK != [16]byte{} {
		t.Errorf("expected zero PSK, got %x", ch.PSK)
	}
}

func TestNewChannelFromBase64_Invalid(t *testing.T) {
	_, err := NewChannelFromBase64("bad", "not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestNewChannelFromHashtag(t *testing.T) {
	ch := NewChannelFromHashtag("general")
	if ch.Name != "#general" {
		t.Errorf("Name = %q, want %q", ch.Name, "#general")
	}
	// Already normalized
	ch2 := NewChannelFromHashtag("#general")
	if ch2.Name != "#general" {
		t.Errorf("Name = %q, want %q", ch2.Name, "#general")
	}
	if ch.PSK != ch2.PSK {
		t.Error("same hashtag should produce same PSK")
	}
	if ch.Hash != ch2.Hash {
		t.Error("same hashtag should produce same Hash")
	}
}

func TestNormalizeHashtag(t *testing.T) {
	if got := NormalizeHashtag("foo"); got != "#foo" {
		t.Errorf("NormalizeHashtag(foo) = %q, want #foo", got)
	}
	if got := NormalizeHashtag("#bar"); got != "#bar" {
		t.Errorf("NormalizeHashtag(#bar) = %q, want #bar", got)
	}
}

func TestDeriveHashtagPSK(t *testing.T) {
	psk := DeriveHashtagPSK("test")
	psk2 := DeriveHashtagPSK("#test")
	if psk != psk2 {
		t.Error("normalized and explicit should produce same PSK")
	}
	if psk == [16]byte{} {
		t.Error("PSK should not be all zeros")
	}
}

func TestDeriveChannelHash(t *testing.T) {
	psk := DeriveHashtagPSK("#hello")
	hash := DeriveChannelHash(psk)
	ch := NewChannelFromHashtag("hello")
	if hash != ch.Hash {
		t.Errorf("DeriveChannelHash = 0x%02x, want 0x%02x from NewChannelFromHashtag", hash, ch.Hash)
	}
}
