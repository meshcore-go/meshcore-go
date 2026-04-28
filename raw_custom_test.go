package meshcore

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestRawCustomFromBytes(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		wantHex string
	}{
		{
			name:    "empty data",
			hex:     "",
			wantHex: "",
		},
		{
			name:    "single byte",
			hex:     "A1",
			wantHex: "A1",
		},
		{
			name:    "multi-byte",
			hex:     "A1B2C3D4E5F6A7B8C9",
			wantHex: "A1B2C3D4E5F6A7B8C9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			rc, err := RawCustomFromBytes(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := hex.EncodeToString(rc.Data)
			if !strings.EqualFold(got, tt.wantHex) {
				t.Errorf("Data = %s, want %s", got, tt.wantHex)
			}
		})
	}
}

func TestRawCustomToBytes(t *testing.T) {
	tests := []struct {
		name    string
		rc      RawCustom
		wantHex string
	}{
		{
			name: "empty data",
			rc: RawCustom{
				Data: []byte{},
			},
			wantHex: "",
		},
		{
			name: "single byte",
			rc: RawCustom{
				Data: []byte{0xA1},
			},
			wantHex: "A1",
		},
		{
			name: "multi-byte",
			rc: RawCustom{
				Data: []byte{0xA1, 0xB2, 0xC3, 0xD4, 0xE5, 0xF6, 0xA7, 0xB8, 0xC9},
			},
			wantHex: "A1B2C3D4E5F6A7B8C9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.rc.ToBytes()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			gotHex := hex.EncodeToString(got)
			if !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("ToBytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestRawCustomRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		hex  string
	}{
		{
			name: "empty",
			hex:  "",
		},
		{
			name: "single byte",
			hex:  "FF",
		},
		{
			name: "multi-byte",
			hex:  "A1B2C3D4E5F6A7B8C9DEADBEEF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			rc, err := RawCustomFromBytes(original)
			if err != nil {
				t.Fatalf("RawCustomFromBytes(): %v", err)
			}

			wire, err := rc.ToBytes()
			if err != nil {
				t.Fatalf("ToBytes(): %v", err)
			}

			if !strings.EqualFold(hex.EncodeToString(wire), hex.EncodeToString(original)) {
				t.Errorf("round trip failed: got %s, want %s", hex.EncodeToString(wire), hex.EncodeToString(original))
			}
		})
	}
}
