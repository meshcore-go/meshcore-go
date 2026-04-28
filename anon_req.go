package meshcore

import "bytes"

type AnonReq struct {
	Destination      byte
	EphemeralPubKey  [32]byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func AnonReqFromBytes(data []byte) (*AnonReq, error) {
	if len(data) < 35 {
		return nil, bytes.ErrTooLarge
	}

	buffer := bytes.NewBuffer(data)

	dest, destErr := buffer.ReadByte()
	if destErr != nil {
		return nil, destErr
	}

	var ephemeralPubKey [32]byte
	_, ephErr := buffer.Read(ephemeralPubKey[:])
	if ephErr != nil {
		return nil, ephErr
	}

	var mac [2]byte
	_, macErr := buffer.Read(mac[:])
	if macErr != nil {
		return nil, macErr
	}

	encryptedPayload := buffer.Bytes()

	return &AnonReq{
		Destination:      dest,
		EphemeralPubKey:  ephemeralPubKey,
		MAC:              mac,
		EncryptedPayload: encryptedPayload,
	}, nil
}

func (a *AnonReq) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	destErr := buffer.WriteByte(a.Destination)
	if destErr != nil {
		return nil, destErr
	}

	_, ephErr := buffer.Write(a.EphemeralPubKey[:])
	if ephErr != nil {
		return nil, ephErr
	}

	_, macErr := buffer.Write(a.MAC[:])
	if macErr != nil {
		return nil, macErr
	}

	_, payloadErr := buffer.Write(a.EncryptedPayload)
	if payloadErr != nil {
		return nil, payloadErr
	}

	return buffer.Bytes(), nil
}

func (a *AnonReq) VerifyMAC(sharedSecret []byte) bool {
	src := make([]byte, cipherMACSize+len(a.EncryptedPayload))
	copy(src[:cipherMACSize], a.MAC[:])
	copy(src[cipherMACSize:], a.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result != nil
}

func (a *AnonReq) Decrypt(sharedSecret []byte) []byte {
	src := make([]byte, cipherMACSize+len(a.EncryptedPayload))
	copy(src[:cipherMACSize], a.MAC[:])
	copy(src[cipherMACSize:], a.EncryptedPayload)

	result, _ := MACThenDecrypt(sharedSecret, src)
	return result
}
