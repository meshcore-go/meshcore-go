package meshcore

import "bytes"

type Path struct {
	Destination      byte
	Source           byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func PathFromBytes(data []byte) (*Path, error) {
	buffer := bytes.NewBuffer(data)

	dest, destErr := buffer.ReadByte()
	if destErr != nil {
		return nil, destErr
	}

	src, srcErr := buffer.ReadByte()
	if srcErr != nil {
		return nil, srcErr
	}

	var mac [2]byte
	_, macErr := buffer.Read(mac[:])
	if macErr != nil {
		return nil, macErr
	}

	cipherBytes := buffer.Bytes()

	return &Path{
		Destination:      dest,
		Source:           src,
		MAC:              mac,
		EncryptedPayload: cipherBytes,
	}, nil
}

func (p *Path) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	destErr := buffer.WriteByte(p.Destination)
	if destErr != nil {
		return nil, destErr
	}

	srcErr := buffer.WriteByte(p.Source)
	if srcErr != nil {
		return nil, srcErr
	}

	_, macErr := buffer.Write(p.MAC[:])
	if macErr != nil {
		return nil, macErr
	}

	_, payloadErr := buffer.Write(p.EncryptedPayload)
	if payloadErr != nil {
		return nil, payloadErr
	}

	return buffer.Bytes(), nil
}

func (p *Path) VerifyMAC(sharedSecret []byte) bool {
	src := make([]byte, cipherMACSize+len(p.EncryptedPayload))
	copy(src[:cipherMACSize], p.MAC[:])
	copy(src[cipherMACSize:], p.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result != nil
}

func (p *Path) Decrypt(sharedSecret []byte) []byte {
	src := make([]byte, cipherMACSize+len(p.EncryptedPayload))
	copy(src[:cipherMACSize], p.MAC[:])
	copy(src[cipherMACSize:], p.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result
}
