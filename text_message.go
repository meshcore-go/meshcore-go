package meshcore

import (
	"bytes"
	"encoding/binary"
	"time"
)

type TextMessage struct {
	Destination      byte
	Source           byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func TextMessageFromBytes(data []byte) (*TextMessage, error) {
	buffer := bytes.NewBuffer(data)

	// Read Dest
	dest, destErr := buffer.ReadByte()
	if destErr != nil {
		return nil, destErr
	}

	// Read Source
	src, srcErr := buffer.ReadByte()
	if srcErr != nil {
		return nil, srcErr
	}

	// Read Encryption MAC
	var mac [2]byte
	_, macErr := buffer.Read(mac[:])
	if macErr != nil {
		return nil, macErr
	}

	cihperBytes := buffer.Bytes()

	return &TextMessage{
		Destination:      dest,
		Source:           src,
		MAC:              mac,
		EncryptedPayload: cihperBytes,
	}, nil
}

func (t *TextMessage) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	destErr := buffer.WriteByte(t.Destination)
	if destErr != nil {
		return nil, destErr
	}

	srcErr := buffer.WriteByte(t.Source)
	if srcErr != nil {
		return nil, srcErr
	}

	_, macErr := buffer.Write(t.MAC[:])
	if macErr != nil {
		return nil, macErr
	}

	_, payloadErr := buffer.Write(t.EncryptedPayload)
	if payloadErr != nil {
		return nil, payloadErr
	}

	return buffer.Bytes(), nil
}

func (t *TextMessage) VerifyMAC(sharedSecret []byte) bool {
	// Reconstruct the wire format: [MAC][EncryptedPayload]
	src := make([]byte, cipherMACSize+len(t.EncryptedPayload))
	copy(src[:cipherMACSize], t.MAC[:])
	copy(src[cipherMACSize:], t.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result != nil
}

func (t *TextMessage) Decrypt(sharedSecret []byte) []byte {
	// Reconstruct the wire format: [MAC][EncryptedPayload]
	src := make([]byte, cipherMACSize+len(t.EncryptedPayload))
	copy(src[:cipherMACSize], t.MAC[:])
	copy(src[cipherMACSize:], t.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result
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
