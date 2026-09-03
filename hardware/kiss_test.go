package hardware

import (
	"bytes"
	"errors"
	"testing"
)

func TestEscapeData(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		expect []byte
	}{
		{
			name:   "no escaping needed",
			input:  []byte{0x01, 0x02, 0x03},
			expect: []byte{0x01, 0x02, 0x03},
		},
		{
			name:   "empty data",
			input:  []byte{},
			expect: []byte{},
		},
		{
			name:   "escape FEND",
			input:  []byte{0xAA, KISS_FEND, 0xBB},
			expect: []byte{0xAA, KISS_FESC, KISS_TFEND, 0xBB},
		},
		{
			name:   "escape FESC",
			input:  []byte{0xAA, KISS_FESC, 0xBB},
			expect: []byte{0xAA, KISS_FESC, KISS_TFESC, 0xBB},
		},
		{
			name:   "escape both FEND and FESC",
			input:  []byte{KISS_FEND, 0x01, KISS_FESC},
			expect: []byte{KISS_FESC, KISS_TFEND, 0x01, KISS_FESC, KISS_TFESC},
		},
		{
			name:   "consecutive FEND bytes",
			input:  []byte{KISS_FEND, KISS_FEND},
			expect: []byte{KISS_FESC, KISS_TFEND, KISS_FESC, KISS_TFEND},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapeData(tc.input)
			if !bytes.Equal(got, tc.expect) {
				t.Errorf("EscapeData(%X) = %X, want %X", tc.input, got, tc.expect)
			}
		})
	}
}

func TestUnescapeData(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		expect    []byte
		expectErr error
	}{
		{
			name:   "no unescaping needed",
			input:  []byte{0x01, 0x02, 0x03},
			expect: []byte{0x01, 0x02, 0x03},
		},
		{
			name:   "empty data",
			input:  []byte{},
			expect: []byte{},
		},
		{
			name:   "unescape TFEND",
			input:  []byte{0xAA, KISS_FESC, KISS_TFEND, 0xBB},
			expect: []byte{0xAA, KISS_FEND, 0xBB},
		},
		{
			name:   "unescape TFESC",
			input:  []byte{0xAA, KISS_FESC, KISS_TFESC, 0xBB},
			expect: []byte{0xAA, KISS_FESC, 0xBB},
		},
		{
			name:      "incomplete escape at end",
			input:     []byte{0x01, KISS_FESC},
			expectErr: ErrIncompleteEscape,
		},
		{
			name:      "invalid escape sequence",
			input:     []byte{0x01, KISS_FESC, 0x01},
			expectErr: ErrInvalidEscape,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UnescapeData(tc.input)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Errorf("UnescapeData(%X) error = %v, want %v", tc.input, err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnescapeData(%X) unexpected error: %v", tc.input, err)
			}
			if !bytes.Equal(got, tc.expect) {
				t.Errorf("UnescapeData(%X) = %X, want %X", tc.input, got, tc.expect)
			}
		})
	}
}

func TestEscapeUnescapeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"plain bytes", []byte{0x01, 0x02, 0x03}},
		{"FEND and FESC mixed", []byte{KISS_FEND, KISS_FESC, 0x00, KISS_FEND}},
		{"consecutive FESC", []byte{KISS_FESC, KISS_FESC, KISS_FESC}},
		{"all special values", []byte{0xC0, 0xDB, 0xDC, 0xDD}},
		{"empty", []byte{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			escaped := EscapeData(tc.input)
			unescaped, err := UnescapeData(escaped)
			if err != nil {
				t.Fatalf("round trip error for %X: %v", tc.input, err)
			}
			if !bytes.Equal(unescaped, tc.input) {
				t.Errorf("round trip failed: input=%X, escaped=%X, unescaped=%X", tc.input, escaped, unescaped)
			}
		})
	}
}

func TestEncodeFrame(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		command byte
		data    []byte
		expect  []byte
	}{
		{
			name:    "simple data frame port 0",
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{0x01, 0x02, 0x03},
			expect:  []byte{KISS_FEND, 0x00, 0x01, 0x02, 0x03, KISS_FEND},
		},
		{
			name:    "data frame port 1",
			port:    1,
			command: KISS_CMD_DATA,
			data:    []byte{0x01},
			expect:  []byte{KISS_FEND, 0x10, 0x01, KISS_FEND},
		},
		{
			name:    "txdelay command",
			port:    0,
			command: KISS_CMD_TXDELAY,
			data:    []byte{0x28},
			expect:  []byte{KISS_FEND, 0x01, 0x28, KISS_FEND},
		},
		{
			name:    "data with FEND byte is escaped",
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{0xAA, KISS_FEND, 0xBB},
			expect:  []byte{KISS_FEND, 0x00, 0xAA, KISS_FESC, KISS_TFEND, 0xBB, KISS_FEND},
		},
		{
			name:    "empty data",
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{},
			expect:  []byte{KISS_FEND, 0x00, KISS_FEND},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodeFrame(tc.port, tc.command, tc.data)
			if !bytes.Equal(got, tc.expect) {
				t.Errorf("EncodeFrame(%d, 0x%02X, %X) = %X, want %X", tc.port, tc.command, tc.data, got, tc.expect)
			}
		})
	}
}

func TestEncodeDataFrame(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	got := EncodeDataFrame(data)
	expect := []byte{KISS_FEND, 0x00, 0x01, 0x02, 0x03, KISS_FEND}
	if !bytes.Equal(got, expect) {
		t.Errorf("EncodeDataFrame(%X) = %X, want %X", data, got, expect)
	}
}

func TestDecodeFrame(t *testing.T) {
	tests := []struct {
		name      string
		raw       []byte
		expectErr error
		port      int
		command   byte
		data      []byte
	}{
		{
			name:    "simple data frame",
			raw:     []byte{KISS_FEND, 0x00, 0x01, 0x02, 0x03, KISS_FEND},
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{0x01, 0x02, 0x03},
		},
		{
			name:    "frame with port 2",
			raw:     []byte{KISS_FEND, 0x20, 0xAA, KISS_FEND},
			port:    2,
			command: KISS_CMD_DATA,
			data:    []byte{0xAA},
		},
		{
			name:    "frame with escaped data",
			raw:     []byte{KISS_FEND, 0x00, KISS_FESC, KISS_TFEND, KISS_FEND},
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{KISS_FEND},
		},
		{
			name:    "txdelay command",
			raw:     []byte{KISS_FEND, 0x01, 0x28, KISS_FEND},
			port:    0,
			command: KISS_CMD_TXDELAY,
			data:    []byte{0x28},
		},
		{
			name:      "too short",
			raw:       []byte{KISS_FEND, KISS_FEND},
			expectErr: ErrFrameTooShort,
		},
		{
			name:      "empty",
			raw:       []byte{},
			expectErr: ErrFrameTooShort,
		},
		{
			name:    "no leading FEND",
			raw:     []byte{0x00, 0x01, 0x02, KISS_FEND},
			port:    0,
			command: KISS_CMD_DATA,
			data:    []byte{0x01, 0x02},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame, err := DecodeFrame(tc.raw)
			if tc.expectErr != nil {
				if !errors.Is(err, tc.expectErr) {
					t.Errorf("DecodeFrame(%X) error = %v, want %v", tc.raw, err, tc.expectErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeFrame(%X) unexpected error: %v", tc.raw, err)
			}
			if frame.Port != tc.port {
				t.Errorf("port = %d, want %d", frame.Port, tc.port)
			}
			if frame.Command != tc.command {
				t.Errorf("command = 0x%02X, want 0x%02X", frame.Command, tc.command)
			}
			if !bytes.Equal(frame.Data, tc.data) {
				t.Errorf("data = %X, want %X", frame.Data, tc.data)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		port    int
		command byte
		data    []byte
	}{
		{0, KISS_CMD_DATA, []byte{0x01, 0x02, 0x03}},
		{1, KISS_CMD_DATA, []byte{KISS_FEND, KISS_FESC, 0x00}},
		{0, KISS_CMD_TXDELAY, []byte{0x28}},
		{3, KISS_CMD_DATA, []byte{}},
	}

	for _, tc := range tests {
		encoded := EncodeFrame(tc.port, tc.command, tc.data)
		frame, err := DecodeFrame(encoded)
		if err != nil {
			t.Fatalf("round trip error for port=%d cmd=0x%02X data=%X: %v", tc.port, tc.command, tc.data, err)
		}
		if frame.Port != tc.port || frame.Command != tc.command || !bytes.Equal(frame.Data, tc.data) {
			t.Errorf("round trip mismatch: got port=%d cmd=0x%02X data=%X, want port=%d cmd=0x%02X data=%X",
				frame.Port, frame.Command, frame.Data, tc.port, tc.command, tc.data)
		}
	}
}

func TestExtractFrames(t *testing.T) {
	tests := []struct {
		name       string
		stream     []byte
		frameCount int
		remainder  []byte
	}{
		{
			name:       "single complete frame",
			stream:     []byte{KISS_FEND, 0x00, 0x01, 0x02, KISS_FEND},
			frameCount: 1,
			remainder:  []byte{KISS_FEND},
		},
		{
			name: "two frames",
			stream: append(
				[]byte{KISS_FEND, 0x00, 0x01, KISS_FEND},
				[]byte{KISS_FEND, 0x00, 0x02, KISS_FEND}...,
			),
			frameCount: 2,
			remainder:  []byte{KISS_FEND},
		},
		{
			name:       "incomplete frame",
			stream:     []byte{KISS_FEND, 0x00, 0x01, 0x02},
			frameCount: 0,
			remainder:  []byte{KISS_FEND, 0x00, 0x01, 0x02},
		},
		{
			name:       "garbage before frame",
			stream:     []byte{0x55, 0x66, KISS_FEND, 0x00, 0x01, KISS_FEND},
			frameCount: 1,
			remainder:  []byte{KISS_FEND},
		},
		{
			name:       "frame followed by inter-frame FEND fill",
			stream:     []byte{KISS_FEND, 0x00, 0x01, KISS_FEND, KISS_FEND, KISS_FEND},
			frameCount: 1,
			remainder:  []byte{KISS_FEND},
		},
		{
			name:       "lone FEND",
			stream:     []byte{KISS_FEND},
			frameCount: 0,
			remainder:  []byte{KISS_FEND},
		},
		{
			name:       "empty stream",
			stream:     []byte{},
			frameCount: 0,
			remainder:  nil,
		},
		{
			name:       "no FEND markers",
			stream:     []byte{0x01, 0x02, 0x03},
			frameCount: 0,
			remainder:  nil,
		},
		{
			name:       "complete frame then incomplete",
			stream:     []byte{KISS_FEND, 0x00, 0xAA, KISS_FEND, KISS_FEND, 0x00, 0xBB},
			frameCount: 1,
			remainder:  []byte{KISS_FEND, 0x00, 0xBB},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frames, remainder, _ := ExtractFrames(tc.stream)
			if len(frames) != tc.frameCount {
				t.Errorf("frame count = %d, want %d", len(frames), tc.frameCount)
			}
			if !bytes.Equal(remainder, tc.remainder) {
				t.Errorf("remainder = %X, want %X", remainder, tc.remainder)
			}
		})
	}
}

func TestExtractFrames_SplitAfterOpeningFEND(t *testing.T) {
	chunks := [][]byte{
		{KISS_FEND, 0x00, 0x01, 0x02, 0x03, KISS_FEND, KISS_FEND},
		{0x00, 0x04, 0x05, 0x06, KISS_FEND},
	}
	var got []*KissFrame
	var remainder []byte
	for _, chunk := range chunks {
		frames, rem, errs := ExtractFrames(append(remainder, chunk...))
		if len(errs) != 0 {
			t.Fatalf("unexpected decode errors: %v", errs)
		}
		got = append(got, frames...)
		remainder = rem
	}
	if len(got) != 2 {
		t.Fatalf("frame count = %d, want 2", len(got))
	}
	if !bytes.Equal(got[0].Data, []byte{0x01, 0x02, 0x03}) || !bytes.Equal(got[1].Data, []byte{0x04, 0x05, 0x06}) {
		t.Errorf("frames = %X / %X, want 010203 / 040506", got[0].Data, got[1].Data)
	}
}

func TestHwResp(t *testing.T) {
	tests := []struct {
		cmd    byte
		expect byte
	}{
		{HW_CMD_GET_IDENTITY, 0x81},
		{HW_CMD_GET_RANDOM, 0x82},
		{HW_CMD_SET_RADIO, 0x89},
		{HW_CMD_GET_VERSION, 0x91},
		{HW_CMD_PING, 0x97},
	}
	for _, tc := range tests {
		got := HwResp(tc.cmd)
		if got != tc.expect {
			t.Errorf("HwResp(0x%02X) = 0x%02X, want 0x%02X", tc.cmd, got, tc.expect)
		}
	}
}

func TestEncodeHardwareFrame(t *testing.T) {
	data := []byte{0xAA, 0xBB}
	got := EncodeHardwareFrame(0, HW_CMD_GET_RANDOM, data)

	// Should be: FEND, 0x06 (SETHARDWARE), subcmd, data..., FEND
	// The subcmd and data get escaped inside the frame payload
	frame, err := DecodeFrame(got)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frame.Command != KISS_CMD_SETHARDWARE {
		t.Errorf("command = 0x%02X, want 0x%02X", frame.Command, KISS_CMD_SETHARDWARE)
	}
	// frame.Data should be [subcmd, 0xAA, 0xBB]
	if len(frame.Data) != 3 {
		t.Fatalf("data len = %d, want 3", len(frame.Data))
	}
	if frame.Data[0] != HW_CMD_GET_RANDOM {
		t.Errorf("sub-command = 0x%02X, want 0x%02X", frame.Data[0], HW_CMD_GET_RANDOM)
	}
	if !bytes.Equal(frame.Data[1:], data) {
		t.Errorf("payload = %X, want %X", frame.Data[1:], data)
	}
}

func TestDecodeHardwareFrame(t *testing.T) {
	// Build a hardware frame, decode it, then extract sub-command
	original := []byte{0x01, 0x02, 0x03}
	raw := EncodeHardwareFrame(0, HW_CMD_HASH, original)
	frame, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}

	subCmd, data, err := DecodeHardwareFrame(frame)
	if err != nil {
		t.Fatalf("DecodeHardwareFrame error: %v", err)
	}
	if subCmd != HW_CMD_HASH {
		t.Errorf("sub-command = 0x%02X, want 0x%02X", subCmd, HW_CMD_HASH)
	}
	if !bytes.Equal(data, original) {
		t.Errorf("data = %X, want %X", data, original)
	}
}

func TestDecodeHardwareFrame_NotHardware(t *testing.T) {
	frame := &KissFrame{Command: KISS_CMD_DATA, Data: []byte{0x01}}
	_, _, err := DecodeHardwareFrame(frame)
	if err == nil {
		t.Error("expected error for non-hardware frame")
	}
}

func TestDecodeHardwareFrame_Empty(t *testing.T) {
	frame := &KissFrame{Command: KISS_CMD_SETHARDWARE, Data: []byte{}}
	_, _, err := DecodeHardwareFrame(frame)
	if !errors.Is(err, ErrFrameTooShort) {
		t.Errorf("expected ErrFrameTooShort, got %v", err)
	}
}

func TestEncodeHardwareFrame_WithSpecialBytes(t *testing.T) {
	// Data containing FEND and FESC bytes should survive round-trip
	original := []byte{KISS_FEND, KISS_FESC, 0x42}
	raw := EncodeHardwareFrame(0, HW_CMD_ENCRYPT_DATA, original)
	frame, err := DecodeFrame(raw)
	if err != nil {
		t.Fatalf("DecodeFrame error: %v", err)
	}
	subCmd, data, err := DecodeHardwareFrame(frame)
	if err != nil {
		t.Fatalf("DecodeHardwareFrame error: %v", err)
	}
	if subCmd != HW_CMD_ENCRYPT_DATA {
		t.Errorf("sub-command = 0x%02X, want 0x%02X", subCmd, HW_CMD_ENCRYPT_DATA)
	}
	if !bytes.Equal(data, original) {
		t.Errorf("data = %X, want %X", data, original)
	}
}

func TestRadioConfigRoundTrip(t *testing.T) {
	config := &RadioConfig{
		FreqHz: 917375000,
		BwHz:   62500,
		SF:     7,
		CR:     5,
	}

	encoded := config.ToBytes()
	if len(encoded) != 10 {
		t.Fatalf("encoded length = %d, want 10", len(encoded))
	}

	decoded, err := RadioConfigFromBytes(encoded)
	if err != nil {
		t.Fatalf("RadioConfigFromBytes error: %v", err)
	}

	if decoded.FreqHz != config.FreqHz {
		t.Errorf("FreqHz = %d, want %d", decoded.FreqHz, config.FreqHz)
	}
	if decoded.BwHz != config.BwHz {
		t.Errorf("BwHz = %d, want %d", decoded.BwHz, config.BwHz)
	}
	if decoded.SF != config.SF {
		t.Errorf("SF = %d, want %d", decoded.SF, config.SF)
	}
	if decoded.CR != config.CR {
		t.Errorf("CR = %d, want %d", decoded.CR, config.CR)
	}
}

func TestRadioConfigFromBytes_TooShort(t *testing.T) {
	_, err := RadioConfigFromBytes([]byte{0x01, 0x02})
	if err == nil {
		t.Error("expected error for short data")
	}
}
