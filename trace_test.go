package meshcore

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestTraceFromBytes(t *testing.T) {
	tests := []struct {
		name           string
		hex            string
		wantErr        bool
		wantTag        uint32
		wantAuthCode   uint32
		wantFlags      byte
		wantPathHashes string
	}{
		{
			name:           "valid 9 bytes no path hashes",
			hex:            "01020304050607080A",
			wantTag:        0x04030201,
			wantAuthCode:   0x08070605,
			wantFlags:      0x0A,
			wantPathHashes: "",
		},
		{
			name:           "valid with single path hash",
			hex:            "01020304050607080ABC",
			wantTag:        0x04030201,
			wantAuthCode:   0x08070605,
			wantFlags:      0x0A,
			wantPathHashes: "BC",
		},
		{
			name:           "valid with multiple path hashes",
			hex:            "01020304050607080ABCDEF0A1B2",
			wantTag:        0x04030201,
			wantAuthCode:   0x08070605,
			wantFlags:      0x0A,
			wantPathHashes: "BCDEF0A1B2",
		},
		{
			name:    "truncated 8 bytes missing flags",
			hex:     "0102030405060708",
			wantErr: true,
		},
		{
			name:    "empty input",
			hex:     "",
			wantErr: true,
		},
		{
			name:    "too short 4 bytes",
			hex:     "01020304",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			trace, err := TraceFromBytes(data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if trace.Tag != tt.wantTag {
				t.Errorf("Tag = 0x%08x, want 0x%08x", trace.Tag, tt.wantTag)
			}
			if trace.AuthCode != tt.wantAuthCode {
				t.Errorf("AuthCode = 0x%08x, want 0x%08x", trace.AuthCode, tt.wantAuthCode)
			}
			if trace.Flags != tt.wantFlags {
				t.Errorf("Flags = 0x%02x, want 0x%02x", trace.Flags, tt.wantFlags)
			}
			if tt.wantPathHashes != "" {
				if got := hex.EncodeToString(trace.PathHashes); !strings.EqualFold(got, tt.wantPathHashes) {
					t.Errorf("PathHashes = %s, want %s", got, tt.wantPathHashes)
				}
			}
		})
	}
}

func TestTraceToBytes(t *testing.T) {
	tests := []struct {
		name    string
		trace   Trace
		wantHex string
	}{
		{
			name: "minimal no path hashes",
			trace: Trace{
				Tag:        0x04030201,
				AuthCode:   0x08070605,
				Flags:      0x0A,
				PathHashes: []byte{},
			},
			wantHex: "01020304050607080A",
		},
		{
			name: "with single path hash",
			trace: Trace{
				Tag:        0x04030201,
				AuthCode:   0x08070605,
				Flags:      0x0A,
				PathHashes: []byte{0xBC},
			},
			wantHex: "01020304050607080ABC",
		},
		{
			name: "with multiple path hashes",
			trace: Trace{
				Tag:        0x04030201,
				AuthCode:   0x08070605,
				Flags:      0x0A,
				PathHashes: []byte{0xBC, 0xDE, 0xF0, 0xA1, 0xB2},
			},
			wantHex: "01020304050607080ABCDEF0A1B2",
		},
		{
			name: "zero tag and auth code",
			trace: Trace{
				Tag:        0x00000000,
				AuthCode:   0x00000000,
				Flags:      0xFF,
				PathHashes: []byte{0xAA},
			},
			wantHex: "0000000000000000FFAA",
		},
		{
			name: "max uint32 values",
			trace: Trace{
				Tag:        0xFFFFFFFF,
				AuthCode:   0xFFFFFFFF,
				Flags:      0x00,
				PathHashes: []byte{},
			},
			wantHex: "FFFFFFFFFFFFFFFF00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.trace.ToBytes()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotHex := hex.EncodeToString(got); !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("ToBytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestTracePathHashSize(t *testing.T) {
	tests := []struct {
		name     string
		flags    byte
		wantSize uint8
	}{
		{
			name:     "flags 0x00 returns 1",
			flags:    0x00,
			wantSize: 1,
		},
		{
			name:     "flags 0x01 returns 2",
			flags:    0x01,
			wantSize: 2,
		},
		{
			name:     "flags 0x02 returns 4",
			flags:    0x02,
			wantSize: 4,
		},
		{
			name:     "flags 0x03 returns 8",
			flags:    0x03,
			wantSize: 8,
		},
		{
			name:     "flags 0x04 (upper bits set) returns 1",
			flags:    0x04,
			wantSize: 1,
		},
		{
			name:     "flags 0xFF returns 8",
			flags:    0xFF,
			wantSize: 8,
		},
		{
			name:     "flags 0xFE returns 4",
			flags:    0xFE,
			wantSize: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := &Trace{Flags: tt.flags}
			if got := trace.PathHashSize(); got != tt.wantSize {
				t.Errorf("PathHashSize() = %d, want %d", got, tt.wantSize)
			}
		})
	}
}

func TestTraceRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		trace Trace
	}{
		{
			name: "no path hashes",
			trace: Trace{
				Tag:        0x12345678,
				AuthCode:   0x9ABCDEF0,
				Flags:      0x01,
				PathHashes: []byte{},
			},
		},
		{
			name: "with path hashes",
			trace: Trace{
				Tag:        0xDEADBEEF,
				AuthCode:   0xCAFEBABE,
				Flags:      0x02,
				PathHashes: []byte{0x11, 0x22, 0x33, 0x44, 0x55},
			},
		},
		{
			name: "max values",
			trace: Trace{
				Tag:        0xFFFFFFFF,
				AuthCode:   0xFFFFFFFF,
				Flags:      0xFF,
				PathHashes: []byte{0xFF, 0xFF},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := tt.trace.ToBytes()
			if err != nil {
				t.Fatalf("ToBytes(): %v", err)
			}

			decoded, err := TraceFromBytes(wire)
			if err != nil {
				t.Fatalf("TraceFromBytes(): %v", err)
			}

			if decoded.Tag != tt.trace.Tag {
				t.Errorf("Tag = 0x%08x, want 0x%08x", decoded.Tag, tt.trace.Tag)
			}
			if decoded.AuthCode != tt.trace.AuthCode {
				t.Errorf("AuthCode = 0x%08x, want 0x%08x", decoded.AuthCode, tt.trace.AuthCode)
			}
			if decoded.Flags != tt.trace.Flags {
				t.Errorf("Flags = 0x%02x, want 0x%02x", decoded.Flags, tt.trace.Flags)
			}
			if got := hex.EncodeToString(decoded.PathHashes); got != hex.EncodeToString(tt.trace.PathHashes) {
				t.Errorf("PathHashes = %s, want %s", got, hex.EncodeToString(tt.trace.PathHashes))
			}
		})
	}
}
