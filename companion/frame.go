package companion

import (
	"encoding/binary"
	"fmt"
)

type Frame struct {
	Type byte
	Data []byte
}

func FrameEncode(frameType byte, data []byte) ([]byte, error) {
	if frameType != FrameTypeIncoming && frameType != FrameTypeOutgoing {
		return nil, fmt.Errorf("invalid frame type: 0x%02x", frameType)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("frame data cannot be empty")
	}
	if len(data) > MaxFrameSize {
		return nil, fmt.Errorf("frame payload too large: %d bytes, max %d", len(data), MaxFrameSize)
	}

	buf := make([]byte, FrameHeaderSize+len(data))
	buf[0] = frameType
	binary.LittleEndian.PutUint16(buf[1:], uint16(len(data)))
	copy(buf[FrameHeaderSize:], data)

	return buf, nil
}

func FrameDecode(raw []byte) (Frame, error) {
	if len(raw) < FrameHeaderSize {
		return Frame{}, fmt.Errorf("frame too short: expected at least %d bytes, got %d", FrameHeaderSize, len(raw))
	}

	frameType := raw[0]
	if frameType != FrameTypeIncoming && frameType != FrameTypeOutgoing {
		return Frame{}, fmt.Errorf("invalid frame type: 0x%02x", frameType)
	}

	length := binary.LittleEndian.Uint16(raw[1:FrameHeaderSize])
	if length == 0 {
		return Frame{}, fmt.Errorf("invalid frame length: 0")
	}

	totalLength := FrameHeaderSize + int(length)
	if len(raw) < totalLength {
		return Frame{}, fmt.Errorf("truncated frame: need %d bytes, got %d", totalLength, len(raw))
	}

	data := make([]byte, int(length))
	copy(data, raw[FrameHeaderSize:totalLength])

	return Frame{Type: frameType, Data: data}, nil
}

type FrameParser struct {
	buf []byte
}

func NewFrameParser() *FrameParser {
	return &FrameParser{}
}

func (p *FrameParser) Feed(data []byte) []Frame {
	p.buf = append(p.buf, data...)

	var frames []Frame
	for len(p.buf) >= FrameHeaderSize {
		frameType := p.buf[0]
		if frameType != FrameTypeIncoming && frameType != FrameTypeOutgoing {
			p.buf = p.buf[1:]
			continue
		}

		length := binary.LittleEndian.Uint16(p.buf[1:FrameHeaderSize])
		if length == 0 {
			p.buf = p.buf[1:]
			continue
		}

		totalLength := FrameHeaderSize + int(length)
		if len(p.buf) < totalLength {
			break
		}

		payload := make([]byte, int(length))
		copy(payload, p.buf[FrameHeaderSize:totalLength])
		frames = append(frames, Frame{Type: frameType, Data: payload})
		p.buf = p.buf[totalLength:]
	}

	return frames
}

func (p *FrameParser) Reset() {
	p.buf = nil
}
