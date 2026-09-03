package meshcore

import "fmt"

type Path struct {
	Destination      byte
	Source           byte
	MAC              [2]byte
	EncryptedPayload []byte
}

func PathFromBytes(data []byte) (*Path, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: path needs 4 header bytes, have %d", ErrTooShort, len(data))
	}
	return &Path{
		Destination:      data[0],
		Source:           data[1],
		MAC:              [2]byte{data[2], data[3]},
		EncryptedPayload: data[4:],
	}, nil
}

func (p *Path) ToBytes() ([]byte, error) {
	return append([]byte{p.Destination, p.Source, p.MAC[0], p.MAC[1]}, p.EncryptedPayload...), nil
}

func (p *Path) VerifyMAC(sharedSecret []byte) bool {
	return p.Decrypt(sharedSecret) != nil
}

// Decrypt returns the plaintext, or nil if the MAC does not verify.
func (p *Path) Decrypt(sharedSecret []byte) []byte {
	return macDecrypt(sharedSecret, p.MAC, p.EncryptedPayload)
}

// PathPayload is the decrypted body of a PATH (returned path) packet.
type PathPayload struct {
	PathLength byte   // raw path_len byte: bits 6-7 = hash size - 1, bits 0-5 = hop count
	Path       []byte // hop hashes, PathHashCount()*PathHashSize() bytes
	ExtraType  byte   // bundled payload type (lower 4 bits), e.g. PayloadTypeAck
	Extra      []byte // bundled payload; may be zero-padded to the cipher block
}

func (p *PathPayload) PathHashSize() uint8  { s, _ := pathLenFields(p.PathLength); return s }
func (p *PathPayload) PathHashCount() uint8 { _, c := pathLenFields(p.PathLength); return c }
func (p *PathPayload) PathHashes() [][]byte {
	return splitPathHashes(p.Path, p.PathHashSize(), p.PathHashCount())
}

// DecryptStruct decrypts and parses the PATH body.
func (p *Path) DecryptStruct(sharedSecret []byte) (*PathPayload, error) {
	plain, err := macDecryptErr(sharedSecret, p.MAC, p.EncryptedPayload)
	if err != nil {
		return nil, fmt.Errorf("decrypting path: %w", err)
	}
	return ParsePathPayload(plain)
}

// ParsePathPayload decodes the plaintext returned by Path.Decrypt.
func ParsePathPayload(plain []byte) (*PathPayload, error) {
	if len(plain) < 2 {
		return nil, fmt.Errorf("path payload too short: %d bytes", len(plain))
	}
	p := &PathPayload{PathLength: plain[0]}
	if !IsValidPathLen(p.PathLength) {
		return nil, fmt.Errorf("path payload: bad path_len 0x%02x", p.PathLength)
	}
	n := int(p.PathHashCount()) * int(p.PathHashSize())
	if len(plain) < 1+n+1 {
		return nil, fmt.Errorf("path payload: %d hop bytes exceed %d available", n, len(plain)-2)
	}
	p.Path = plain[1 : 1+n]
	p.ExtraType = plain[1+n] & PacketTypeMask
	p.Extra = plain[2+n:]
	return p, nil
}
