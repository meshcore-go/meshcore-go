package meshcore

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Ack represents an ACK payload. The wire format is variable-length:
//   - Bytes 0-3: CRC (SHA256 truncated to 4 bytes)
//   - Byte 4 (optional): extended attempt byte (makes packet hash unique per retry)
//   - Byte 5 (optional): random byte (further dedup uniqueness)
//
// Receivers match on the first 4 bytes (CRC) only. The extended bytes exist
// to prevent the dedup table from dropping retransmitted ACKs.
type Ack struct {
	// Payload is the raw ACK bytes (4-6 bytes). The first 4 bytes are the
	// CRC used for matching; remaining bytes are dedup salt.
	Payload []byte
}

// CRC returns the 4-byte ACK identifier used for matching.
func (a *Ack) CRC() uint32 {
	if len(a.Payload) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(a.Payload[:4])
}

func AckFromBytes(data []byte) (*Ack, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("ack too short: expected at least 4 bytes, got %d", len(data))
	}
	payload := make([]byte, len(data))
	copy(payload, data)
	return &Ack{Payload: payload}, nil
}

func (a *Ack) ToBytes() ([]byte, error) {
	if len(a.Payload) < 4 {
		return nil, fmt.Errorf("ack payload too short: %d bytes", len(a.Payload))
	}
	out := make([]byte, len(a.Payload))
	copy(out, a.Payload)
	return out, nil
}

// CalcAckHash computes the 4-byte ACK CRC for a text message.
// It matches MeshCore C++ firmware: SHA256(plaintext_data || sender_pub_key)
// truncated to 4 bytes (little-endian uint32).
//
// plaintext_data is the decrypted message content (timestamp + flags + text).
// senderPubKey is the 32-byte Ed25519 public key of the sender.
func CalcAckHash(plaintextData []byte, senderPubKey []byte) uint32 {
	h := sha256.New()
	h.Write(plaintextData)
	h.Write(senderPubKey)
	sum := h.Sum(nil)
	return binary.LittleEndian.Uint32(sum[:4])
}

// BuildAckPayload constructs the full ACK payload (up to 6 bytes) matching
// the current C++ firmware format:
//   - Bytes 0-3: SHA256(plaintext || senderPubKey)[0:4]
//   - Byte 4: attemptByte (last byte of decrypted payload — makes retries unique)
//   - Byte 5: randomByte (caller should supply a random byte)
func BuildAckPayload(plaintextData []byte, senderPubKey []byte, attemptByte byte, randomByte byte) []byte {
	crc := CalcAckHash(plaintextData, senderPubKey)
	payload := make([]byte, 6)
	binary.LittleEndian.PutUint32(payload[:4], crc)
	payload[4] = attemptByte
	payload[5] = randomByte
	return payload
}
