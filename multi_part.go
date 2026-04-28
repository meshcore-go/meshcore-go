package meshcore

import (
	"bytes"
	"fmt"
)

type MultiPart struct {
	Remaining      uint8
	WrappedType    byte
	WrappedPayload []byte
}

func MultiPartFromBytes(data []byte) (*MultiPart, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input: %w", fmt.Errorf("data required"))
	}

	buffer := bytes.NewBuffer(data)

	headerByte, err := buffer.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}

	remaining := uint8(headerByte >> 4)
	wrappedType := headerByte & 0x0F
	wrappedPayload := buffer.Bytes()

	return &MultiPart{
		Remaining:      remaining,
		WrappedType:    wrappedType,
		WrappedPayload: wrappedPayload,
	}, nil
}

func (m *MultiPart) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	headerByte := (m.Remaining << 4) | (m.WrappedType & 0x0F)
	err := buffer.WriteByte(headerByte)
	if err != nil {
		return nil, fmt.Errorf("writing header: %w", err)
	}

	_, err = buffer.Write(m.WrappedPayload)
	if err != nil {
		return nil, fmt.Errorf("writing payload: %w", err)
	}

	return buffer.Bytes(), nil
}
