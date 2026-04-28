package meshcore

import (
	"bytes"
	"fmt"
)

type GroupData struct {
	ChannelHash      byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func GroupDataFromBytes(data []byte) (*GroupData, error) {
	buffer := bytes.NewBuffer(data)

	channelHash, err := buffer.ReadByte()
	if err != nil {
		return nil, err
	}

	var mac [2]byte
	n, macErr := buffer.Read(mac[:])
	if macErr != nil || n != 2 {
		if macErr != nil {
			return nil, macErr
		}
		return nil, fmt.Errorf("MAC read: expected 2 bytes, got %d", n)
	}

	encryptedPayload := buffer.Bytes()

	return &GroupData{
		ChannelHash:      channelHash,
		MAC:              mac,
		EncryptedPayload: encryptedPayload,
	}, nil
}

func (g *GroupData) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	err := buffer.WriteByte(g.ChannelHash)
	if err != nil {
		return nil, err
	}

	_, macErr := buffer.Write(g.MAC[:])
	if macErr != nil {
		return nil, macErr
	}

	_, payloadErr := buffer.Write(g.EncryptedPayload)
	if payloadErr != nil {
		return nil, payloadErr
	}

	return buffer.Bytes(), nil
}

func (g *GroupData) VerifyMAC(channelKey []byte) bool {
	src := make([]byte, cipherMACSize+len(g.EncryptedPayload))
	copy(src[:cipherMACSize], g.MAC[:])
	copy(src[cipherMACSize:], g.EncryptedPayload)

	result, _ := MACThenDecrypt(channelKey, src)
	return result != nil
}

func (g *GroupData) Decrypt(channelKey []byte) []byte {
	src := make([]byte, cipherMACSize+len(g.EncryptedPayload))
	copy(src[:cipherMACSize], g.MAC[:])
	copy(src[cipherMACSize:], g.EncryptedPayload)

	result, _ := MACThenDecrypt(channelKey, src)
	return result
}
