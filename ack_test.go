package meshcore

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func TestAckFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		wantErr bool
		wantCRC uint32
		wantLen int
	}{
		{
			name:    "valid 4 bytes",
			hex:     "01020304",
			wantErr: false,
			wantCRC: 0x04030201,
			wantLen: 4,
		},
		{
			name:    "valid 5 bytes (extended attempt)",
			hex:     "0506070809",
			wantErr: false,
			wantCRC: 0x08070605,
			wantLen: 5,
		},
		{
			name:    "valid 6 bytes (extended + random)",
			hex:     "05060708AABB",
			wantErr: false,
			wantCRC: 0x08070605,
			wantLen: 6,
		},
		{
			name:    "truncated 3 bytes",
			hex:     "010203",
			wantErr: true,
		},
		{
			name:    "empty input",
			hex:     "",
			wantErr: true,
		},
		{
			name:    "zero crc",
			hex:     "00000000",
			wantErr: false,
			wantCRC: 0x00000000,
			wantLen: 4,
		},
		{
			name:    "max uint32",
			hex:     "FFFFFFFF",
			wantErr: false,
			wantCRC: 0xFFFFFFFF,
			wantLen: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			ack, err := AckFromBytes(data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if ack.CRC() != tt.wantCRC {
				t.Errorf("CRC() = 0x%08x, want 0x%08x", ack.CRC(), tt.wantCRC)
			}
			if len(ack.Payload) != tt.wantLen {
				t.Errorf("Payload len = %d, want %d", len(ack.Payload), tt.wantLen)
			}
		})
	}
}

func TestAckToBytes(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		wantHex string
	}{
		{
			name:    "4-byte ack",
			payload: []byte{0x00, 0x00, 0x00, 0x00},
			wantHex: "00000000",
		},
		{
			name:    "max uint32",
			payload: []byte{0xFF, 0xFF, 0xFF, 0xFF},
			wantHex: "FFFFFFFF",
		},
		{
			name:    "typical 4-byte value",
			payload: []byte{0x01, 0x02, 0x03, 0x04},
			wantHex: "01020304",
		},
		{
			name:    "6-byte extended",
			payload: []byte{0xEF, 0xBE, 0xAD, 0xDE, 0x03, 0x7F},
			wantHex: "EFBEADDE037F",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ack := &Ack{Payload: tt.payload}
			got, err := ack.ToBytes()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotHex := hex.EncodeToString(got); !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("ToBytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestAckRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{
			name:    "4-byte zero",
			payload: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:    "4-byte max",
			payload: []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
		{
			name:    "6-byte extended",
			payload: []byte{0x12, 0x34, 0x56, 0x78, 0x03, 0xAB},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &Ack{Payload: tt.payload}

			data, err := original.ToBytes()
			if err != nil {
				t.Fatalf("ToBytes(): %v", err)
			}

			decoded, err := AckFromBytes(data)
			if err != nil {
				t.Fatalf("AckFromBytes(): %v", err)
			}

			if decoded.CRC() != original.CRC() {
				t.Errorf("round trip CRC = 0x%08x, want 0x%08x", decoded.CRC(), original.CRC())
			}
			if len(decoded.Payload) != len(original.Payload) {
				t.Errorf("round trip payload len = %d, want %d", len(decoded.Payload), len(original.Payload))
			}
		})
	}
}

func TestBuildAckPayload(t *testing.T) {
	plaintext := []byte{0x01, 0x02, 0x03, 0x04, 0x00, 0x48, 0x65, 0x6c, 0x6c, 0x6f}
	pubKey := make([]byte, 32)
	pubKey[0] = 0xAA

	payload := BuildAckPayload(plaintext, pubKey, 0x02, 0x7F)

	if len(payload) != 6 {
		t.Fatalf("expected 6 bytes, got %d", len(payload))
	}

	expectedCRC := CalcAckHash(plaintext, pubKey)
	gotCRC := binary.LittleEndian.Uint32(payload[:4])
	if gotCRC != expectedCRC {
		t.Errorf("CRC = 0x%08x, want 0x%08x", gotCRC, expectedCRC)
	}
	if payload[4] != 0x02 {
		t.Errorf("attempt byte = 0x%02x, want 0x02", payload[4])
	}
	if payload[5] != 0x7F {
		t.Errorf("random byte = 0x%02x, want 0x7F", payload[5])
	}
}
