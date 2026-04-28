package meshcore

import "bytes"

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
