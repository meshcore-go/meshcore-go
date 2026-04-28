package meshcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type Ack struct {
	AckCRC uint32
}

func AckFromBytes(data []byte) (*Ack, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("ack too short: expected at least 4 bytes, got %d", len(data))
	}

	buffer := bytes.NewBuffer(data)
	ack := &Ack{}

	if err := binary.Read(buffer, binary.LittleEndian, &ack.AckCRC); err != nil {
		return nil, fmt.Errorf("reading ack crc: %w", err)
	}

	return ack, nil
}

func (a *Ack) ToBytes() ([]byte, error) {
	buffer := bytes.Buffer{}

	if err := binary.Write(&buffer, binary.LittleEndian, a.AckCRC); err != nil {
		return nil, fmt.Errorf("writing ack crc: %w", err)
	}

	return buffer.Bytes(), nil
}
