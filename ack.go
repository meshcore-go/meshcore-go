package meshcore

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Ack is an ACK payload: a 4-byte CRC, optionally followed by an attempt byte
// and a random byte that keep retransmissions out of the dedup table.
type Ack struct {
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

// CalcAckHash computes the 4-byte ACK CRC: SHA256(plaintext ‖ senderPubKey)
// truncated to a little-endian uint32.
func CalcAckHash(plaintextData []byte, senderPubKey []byte) uint32 {
	h := sha256.New()
	h.Write(plaintextData)
	h.Write(senderPubKey)
	sum := h.Sum(nil)
	return binary.LittleEndian.Uint32(sum[:4])
}

// BuildAckPayload builds the 6-byte ACK payload: CRC, attempt byte, random byte.
func BuildAckPayload(plaintextData []byte, senderPubKey []byte, attemptByte byte, randomByte byte) []byte {
	crc := CalcAckHash(plaintextData, senderPubKey)
	payload := make([]byte, 6)
	binary.LittleEndian.PutUint32(payload[:4], crc)
	payload[4] = attemptByte
	payload[5] = randomByte
	return payload
}
