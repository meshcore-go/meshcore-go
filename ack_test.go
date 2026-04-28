package meshcore

import (
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
	}{
		{
			name:    "valid 4 bytes",
			hex:     "01020304",
			wantErr: false,
			wantCRC: 0x04030201,
		},
		{
			name:    "valid with extra trailing bytes",
			hex:     "05060708AABBCCDD",
			wantErr: false,
			wantCRC: 0x08070605,
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
		},
		{
			name:    "max uint32",
			hex:     "FFFFFFFF",
			wantErr: false,
			wantCRC: 0xFFFFFFFF,
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

			if ack.AckCRC != tt.wantCRC {
				t.Errorf("AckCRC = 0x%08x, want 0x%08x", ack.AckCRC, tt.wantCRC)
			}
		})
	}
}

func TestAckToBytes(t *testing.T) {
	tests := []struct {
		name    string
		ack     Ack
		wantHex string
	}{
		{
			name:    "zero crc",
			ack:     Ack{AckCRC: 0x00000000},
			wantHex: "00000000",
		},
		{
			name:    "max uint32",
			ack:     Ack{AckCRC: 0xFFFFFFFF},
			wantHex: "FFFFFFFF",
		},
		{
			name:    "typical value",
			ack:     Ack{AckCRC: 0x04030201},
			wantHex: "01020304",
		},
		{
			name:    "another typical value",
			ack:     Ack{AckCRC: 0xDEADBEEF},
			wantHex: "EFBEADDE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.ack.ToBytes()
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
		name string
		crc  uint32
	}{
		{
			name: "zero crc",
			crc:  0x00000000,
		},
		{
			name: "max uint32",
			crc:  0xFFFFFFFF,
		},
		{
			name: "typical value",
			crc:  0x12345678,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &Ack{AckCRC: tt.crc}

			bytes, err := original.ToBytes()
			if err != nil {
				t.Fatalf("ToBytes(): %v", err)
			}

			decoded, err := AckFromBytes(bytes)
			if err != nil {
				t.Fatalf("AckFromBytes(): %v", err)
			}

			if decoded.AckCRC != original.AckCRC {
				t.Errorf("round trip AckCRC = 0x%08x, want 0x%08x", decoded.AckCRC, original.AckCRC)
			}
		})
	}
}
