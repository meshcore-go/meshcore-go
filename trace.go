package meshcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Trace struct {
	Tag        uint32
	AuthCode   uint32
	Flags      byte
	PathHashes []byte
}

func TraceFromBytes(data []byte) (*Trace, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("trace data too short: need 9 bytes, have %d", len(data))
	}

	buffer := bytes.NewBuffer(data)

	var tag uint32
	if err := binary.Read(buffer, binary.LittleEndian, &tag); err != nil {
		return nil, fmt.Errorf("reading tag: %w", err)
	}

	var authCode uint32
	if err := binary.Read(buffer, binary.LittleEndian, &authCode); err != nil {
		return nil, fmt.Errorf("reading auth code: %w", err)
	}

	flags, err := buffer.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading flags: %w", err)
	}

	pathHashes := buffer.Bytes()

	return &Trace{
		Tag:        tag,
		AuthCode:   authCode,
		Flags:      flags,
		PathHashes: pathHashes,
	}, nil
}

func (t *Trace) ToBytes() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	if err := binary.Write(buffer, binary.LittleEndian, t.Tag); err != nil {
		return nil, fmt.Errorf("writing tag: %w", err)
	}

	if err := binary.Write(buffer, binary.LittleEndian, t.AuthCode); err != nil {
		return nil, fmt.Errorf("writing auth code: %w", err)
	}

	if err := buffer.WriteByte(t.Flags); err != nil {
		return nil, fmt.Errorf("writing flags: %w", err)
	}

	if err := binary.Write(buffer, binary.LittleEndian, t.PathHashes); err != nil {
		return nil, fmt.Errorf("writing path hashes: %w", err)
	}

	return buffer.Bytes(), nil
}

// PathHashSize returns the per-hop hash size in bytes, derived from bits 0-1
// of the Flags field. Matches C++ MeshCore: 1 << (flags & 0x03), giving
// possible sizes of 1, 2, 4, or 8 bytes.
func (t *Trace) PathHashSize() uint8 {
	return 1 << (t.Flags & 0x03)
}
