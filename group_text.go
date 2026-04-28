package meshcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// GroupTextPayload represents the decrypted content of a group text message.
type GroupTextPayload struct {
	Timestamp uint32
	Flags     byte
	Sender    string
	Text      string
}

type GroupText struct {
	ChannelHash      byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func GroupTextFromBytes(data []byte) (*GroupText, error) {
	buffer := bytes.NewBuffer(data)

	channelHash, err := buffer.ReadByte()
	if err != nil {
		return nil, err
	}

	var mac [2]byte
	_, macErr := buffer.Read(mac[:])
	if macErr != nil {
		return nil, macErr
	}

	encryptedPayload := buffer.Bytes()

	return &GroupText{
		ChannelHash:      channelHash,
		MAC:              mac,
		EncryptedPayload: encryptedPayload,
	}, nil
}

func (g *GroupText) ToBytes() ([]byte, error) {
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

func (g *GroupText) VerifyMAC(channelKey []byte) bool {
	src := make([]byte, cipherMACSize+len(g.EncryptedPayload))
	copy(src[:cipherMACSize], g.MAC[:])
	copy(src[cipherMACSize:], g.EncryptedPayload)

	result, _ := MACThenDecrypt(channelKey, src)
	return result != nil
}

func (g *GroupText) Decrypt(channelKey []byte) []byte {
	src := make([]byte, cipherMACSize+len(g.EncryptedPayload))
	copy(src[:cipherMACSize], g.MAC[:])
	copy(src[cipherMACSize:], g.EncryptedPayload)

	result, _ := MACThenDecrypt(channelKey, src)
	return result
}

func (p *GroupTextPayload) Encrypt(channelHash byte, psk []byte) (*GroupText, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, p.Timestamp); err != nil {
		return nil, err
	}
	buf.WriteByte(p.Flags)
	if p.Sender != "" {
		buf.WriteString(p.Sender)
		buf.WriteString(": ")
	}
	buf.WriteString(p.Text)

	encrypted, err := EncryptThenMAC(psk, buf.Bytes())
	if err != nil {
		return nil, err
	}

	return &GroupText{
		ChannelHash:      channelHash,
		MAC:              [2]byte{encrypted[0], encrypted[1]},
		EncryptedPayload: encrypted[2:],
	}, nil
}

// DecryptStruct decrypts the payload and parses the result into a GroupTextPayload.
func (g *GroupText) DecryptStruct(channelKey []byte) (*GroupTextPayload, error) {
	plaintext := g.Decrypt(channelKey)
	if plaintext == nil {
		return nil, fmt.Errorf("decryption failed (invalid MAC or key)")
	}
	if len(plaintext) < 5 {
		return nil, fmt.Errorf("decrypted payload too short: %d bytes", len(plaintext))
	}

	msg := string(bytes.TrimRight(plaintext[5:], "\x00"))
	sender, text, _ := strings.Cut(msg, ": ")
	if text == "" {
		// No separator found; treat entire string as text
		text = sender
		sender = ""
	}

	return &GroupTextPayload{
		Timestamp: binary.LittleEndian.Uint32(plaintext[:4]),
		Flags:     plaintext[4],
		Sender:    sender,
		Text:      text,
	}, nil
}
