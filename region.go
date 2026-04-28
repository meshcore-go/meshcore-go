package meshcore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"strings"
)

const (
	RegionKeySize          = 16
	MaxRegionName          = 30
	MaxRegions             = 32
	RegionDenyFlood  uint8 = 0x01
	RegionDenyDirect uint8 = 0x02
)

type RegionKey [RegionKeySize]byte

type Region struct {
	ID     uint16
	Parent uint16
	Flags  uint8
	Name   string
	Key    RegionKey
}

func NewRegionFromHashtag(name string) *Region {
	name = normalizeRegionName(name)
	key := DeriveRegionKey(name)
	return &Region{
		Name: name,
		Key:  key,
	}
}

func NewRegionFromKey(name string, key RegionKey) *Region {
	return &Region{
		Name: name,
		Key:  key,
	}
}

func normalizeRegionName(name string) string {
	if strings.HasPrefix(name, "#") || strings.HasPrefix(name, "$") {
		return name
	}
	return "#" + name
}

// DeriveRegionKey computes the 16-byte region key from a region name.
// For hashtag regions: SHA256(name)[0:16]. The name should already
// include the '#' prefix.
func DeriveRegionKey(name string) RegionKey {
	h := sha256.Sum256([]byte(name))
	var key RegionKey
	copy(key[:], h[:RegionKeySize])
	return key
}

func (k RegionKey) IsZero() bool {
	for _, b := range k {
		if b != 0 {
			return false
		}
	}
	return true
}

// CalcTransportCode computes the per-packet transport code for this region key.
// Result is HMAC-SHA256(key, payloadType || payload) truncated to uint16.
// Codes 0x0000 and 0xFFFF are reserved and shifted to 0x0001/0xFFFE.
func (k RegionKey) CalcTransportCode(payloadType byte, payload []byte) uint16 {
	mac := hmac.New(sha256.New, k[:])
	mac.Write([]byte{payloadType})
	mac.Write(payload)
	sum := mac.Sum(nil)
	code := binary.LittleEndian.Uint16(sum[:2])
	switch code {
	case 0x0000:
		code = 0x0001
	case 0xFFFF:
		code = 0xFFFE
	}
	return code
}

func (r *Region) CalcTransportCode(pkt *Packet) uint16 {
	return r.Key.CalcTransportCode(pkt.PayloadType(), pkt.Payload)
}

func (r *Region) MatchesPacket(pkt *Packet) bool {
	return pkt.IsTransport() && pkt.TransportCode1 == r.CalcTransportCode(pkt)
}

func (r *Region) DenyFlood() bool {
	return r.Flags&RegionDenyFlood != 0
}

func (r *Region) DenyDirect() bool {
	return r.Flags&RegionDenyDirect != 0
}

// IsValidRegionNameChar matches C++ RegionMap::is_name_char:
// accepts alphanumeric, accented chars (>=0x41), '-', '$', '#'.
func IsValidRegionNameChar(c byte) bool {
	return c == '-' || c == '$' || c == '#' || (c >= '0' && c <= '9') || c >= 'A'
}
