package meshcore

import (
	"encoding/binary"
	"fmt"
	"time"
)

type TextMessage struct {
	Destination      byte
	Source           byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func TextMessageFromBytes(data []byte) (*TextMessage, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: text message needs 4 header bytes, have %d", ErrTooShort, len(data))
	}
	return &TextMessage{
		Destination:      data[0],
		Source:           data[1],
		MAC:              [2]byte{data[2], data[3]},
		EncryptedPayload: data[4:],
	}, nil
}

func (t *TextMessage) ToBytes() ([]byte, error) {
	return append([]byte{t.Destination, t.Source, t.MAC[0], t.MAC[1]}, t.EncryptedPayload...), nil
}

func (t *TextMessage) VerifyMAC(sharedSecret []byte) bool {
	return t.Decrypt(sharedSecret) != nil
}

// Decrypt returns the plaintext, or nil if the MAC does not verify.
func (t *TextMessage) Decrypt(sharedSecret []byte) []byte {
	return macDecrypt(sharedSecret, t.MAC, t.EncryptedPayload)
}

func BuildTextPlaintext(timestamp time.Time, flags byte, text []byte) []byte {
	buf := make([]byte, 4+1+len(text))
	binary.LittleEndian.PutUint32(buf[:4], uint32(timestamp.Unix()))
	buf[4] = flags
	copy(buf[5:], text)
	return buf
}

// BuildTextPlaintextWithAttempt builds the decrypted TXT_MSG payload for a given
// send attempt, matching firmware BaseChatMesh::composeMsgPacket. The attempt
// number is encoded into the low 2 bits of the flags byte (the upper bits, which
// carry the TXT_TYPE_*, are preserved). When attempt > 3 a [0x00][attempt] tail
// is appended: the low 2 bits wrap every 4 attempts, so the explicit attempt
// byte keeps each retransmission's packet hash unique past that point.
//
// The expected ACK hash covers the flags byte, so callers must recompute
// CalcAckHash from the payload returned here for each attempt.
func BuildTextPlaintextWithAttempt(timestamp time.Time, flags byte, text []byte, attempt int) []byte {
	f := (flags &^ 0x03) | byte(attempt&0x03)
	buf := make([]byte, 0, 5+len(text)+2)
	var hdr [5]byte
	binary.LittleEndian.PutUint32(hdr[:4], uint32(timestamp.Unix()))
	hdr[4] = f
	buf = append(buf, hdr[:]...)
	buf = append(buf, text...)
	if attempt > 3 {
		buf = append(buf, 0x00, byte(attempt))
	}
	return buf
}

func NewTextMessage(self LocalIdentity, peer Identity, plaintext []byte, sharedSecret []byte) (*TextMessage, error) {
	encrypted, err := EncryptThenMAC(sharedSecret, plaintext)
	if err != nil {
		return nil, err
	}

	var mac [2]byte
	copy(mac[:], encrypted[:cipherMACSize])

	return &TextMessage{
		Destination:      peer.PublicKey()[0],
		Source:           self.PublicKey()[0],
		MAC:              mac,
		EncryptedPayload: encrypted[cipherMACSize:],
	}, nil
}
