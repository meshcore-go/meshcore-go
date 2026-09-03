package meshcore

import "fmt"

// anonReqHeaderSize is dest hash + ephemeral pubkey + MAC.
const anonReqHeaderSize = 1 + PubKeySize + cipherMACSize

type AnonReq struct {
	Destination      byte
	EphemeralPubKey  [32]byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func AnonReqFromBytes(data []byte) (*AnonReq, error) {
	if len(data) < anonReqHeaderSize {
		return nil, fmt.Errorf("%w: anon req needs %d header bytes, have %d", ErrTooShort, anonReqHeaderSize, len(data))
	}
	a := &AnonReq{
		Destination:      data[0],
		MAC:              [2]byte{data[33], data[34]},
		EncryptedPayload: data[anonReqHeaderSize:],
	}
	copy(a.EphemeralPubKey[:], data[1:33])
	return a, nil
}

func (a *AnonReq) ToBytes() ([]byte, error) {
	out := make([]byte, 0, anonReqHeaderSize+len(a.EncryptedPayload))
	out = append(out, a.Destination)
	out = append(out, a.EphemeralPubKey[:]...)
	out = append(out, a.MAC[:]...)
	return append(out, a.EncryptedPayload...), nil
}

func (a *AnonReq) VerifyMAC(sharedSecret []byte) bool {
	return a.Decrypt(sharedSecret) != nil
}

// Decrypt returns the plaintext, or nil if the MAC does not verify.
func (a *AnonReq) Decrypt(sharedSecret []byte) []byte {
	return macDecrypt(sharedSecret, a.MAC, a.EncryptedPayload)
}
