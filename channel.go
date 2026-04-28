package meshcore

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// ChannelEntry represents a group channel with its name, pre-shared key, and derived hash.
type ChannelEntry struct {
	Name string
	PSK  [16]byte
	Hash byte // SHA256(PSK)[0]
}

// NewChannelFromPSK creates a ChannelEntry from a name and raw 16-byte PSK.
func NewChannelFromPSK(name string, psk []byte) (*ChannelEntry, error) {
	if len(psk) != 16 {
		return nil, fmt.Errorf("PSK must be 16 bytes, got %d", len(psk))
	}
	e := &ChannelEntry{Name: name}
	copy(e.PSK[:], psk)
	h := sha256.Sum256(e.PSK[:])
	e.Hash = h[0]
	return e, nil
}

// NewChannelFromBase64 creates a ChannelEntry from a name and base64-encoded PSK.
func NewChannelFromBase64(name string, pskBase64 string) (*ChannelEntry, error) {
	psk, err := base64.StdEncoding.DecodeString(pskBase64)
	if err != nil {
		return nil, fmt.Errorf("decoding base64 PSK: %w", err)
	}
	return NewChannelFromPSK(name, psk)
}

// NewChannelFromHashtag creates a ChannelEntry for a hashtag channel.
// The name is normalized to include a leading '#' if missing.
// PSK is derived as SHA256("#name")[:16].
func NewChannelFromHashtag(name string) *ChannelEntry {
	name = NormalizeHashtag(name)
	pskHash := sha256.Sum256([]byte(name))
	e := &ChannelEntry{Name: name}
	copy(e.PSK[:], pskHash[:16])
	chHash := sha256.Sum256(e.PSK[:])
	e.Hash = chHash[0]
	return e
}

// NormalizeHashtag ensures the name has a leading '#'.
func NormalizeHashtag(name string) string {
	if !strings.HasPrefix(name, "#") {
		return "#" + name
	}
	return name
}

// DeriveHashtagPSK returns the 16-byte PSK for a hashtag channel name.
// The name is normalized to include a leading '#' if missing.
func DeriveHashtagPSK(name string) [16]byte {
	name = NormalizeHashtag(name)
	h := sha256.Sum256([]byte(name))
	var psk [16]byte
	copy(psk[:], h[:16])
	return psk
}

// DeriveChannelHash returns the single-byte channel hash for a 16-byte PSK.
func DeriveChannelHash(psk [16]byte) byte {
	h := sha256.Sum256(psk[:])
	return h[0]
}
