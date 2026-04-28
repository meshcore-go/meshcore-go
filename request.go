package meshcore

import "bytes"

type Request struct {
	Destination      byte
	Source           byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func RequestFromBytes(data []byte) (*Request, error) {
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

	encryptedPayload := buffer.Bytes()

	return &Request{
		Destination:      dest,
		Source:           src,
		MAC:              mac,
		EncryptedPayload: encryptedPayload,
	}, nil
}

func (r *Request) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	destErr := buffer.WriteByte(r.Destination)
	if destErr != nil {
		return nil, destErr
	}

	srcErr := buffer.WriteByte(r.Source)
	if srcErr != nil {
		return nil, srcErr
	}

	_, macErr := buffer.Write(r.MAC[:])
	if macErr != nil {
		return nil, macErr
	}

	_, payloadErr := buffer.Write(r.EncryptedPayload)
	if payloadErr != nil {
		return nil, payloadErr
	}

	return buffer.Bytes(), nil
}

func (r *Request) VerifyMAC(sharedSecret []byte) bool {
	src := make([]byte, cipherMACSize+len(r.EncryptedPayload))
	copy(src[:cipherMACSize], r.MAC[:])
	copy(src[cipherMACSize:], r.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result != nil
}

func (r *Request) Decrypt(sharedSecret []byte) []byte {
	src := make([]byte, cipherMACSize+len(r.EncryptedPayload))
	copy(src[:cipherMACSize], r.MAC[:])
	copy(src[cipherMACSize:], r.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result
}
