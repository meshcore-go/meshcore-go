package meshcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type Ack struct {
	AckCRC uint32
}

func AckFromBytes(data []byte) (*Ack, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("ack too short: expected at least 4 bytes, got %d", len(data))
	}

	buffer := bytes.NewBuffer(data)
	ack := &Ack{}

	if err := binary.Read(buffer, binary.LittleEndian, &ack.AckCRC); err != nil {
		return nil, fmt.Errorf("reading ack crc: %w", err)
	}

	return ack, nil
}

func (a *Ack) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	if err := binary.Write(&buffer, binary.LittleEndian, a.AckCRC); err != nil {
		return nil, fmt.Errorf("writing ack crc: %w", err)
	}

	return buffer.Bytes(), nil
}

// CalcAckHash computes the ACK CRC for a text message.
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
