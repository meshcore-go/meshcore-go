package hardware

import (
	"bytes"
	"errors"
	"testing"
)

func TestKissEscapedType(t *testing.T) {
	for _, tt := range []struct {
		name        string
		port        int
		cmd         byte
		wire        []byte
		decodedPort int
	}{
		{"FEND type", 12, 0, []byte{KISS_FESC, KISS_TFEND}, 12},
		{"FESC type", 13, 11, []byte{KISS_FESC, KISS_TFESC}, 13},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := EncodeFrame(tt.port, tt.cmd, []byte{1})
			want := append([]byte{KISS_FEND}, tt.wire...)
			want = append(want, 1, KISS_FEND)
			if !bytes.Equal(raw, want) {
				t.Fatalf("wire=%x want%x", raw, want)
			}
			f, err := DecodeFrame(raw)
			if err != nil {
				t.Fatal(err)
			}
			if f.Port != tt.decodedPort || f.Command != tt.cmd || !bytes.Equal(f.Data, []byte{1}) {
				t.Fatalf("decoded=%+v", f)
			}
		})
	}
	for _, raw := range [][]byte{{KISS_FEND, KISS_FESC, KISS_FEND}, {KISS_FEND, KISS_FESC, 1, KISS_FEND}} {
		if _, err := DecodeFrame(raw); err == nil {
			t.Fatalf("accepted invalid type escape %x", raw)
		}
	}
}

func TestKissFrameBoundsAndResync(t *testing.T) {
	for _, fill := range []byte{0x55, KISS_FEND} {
		payload := bytes.Repeat([]byte{fill}, KISS_MAX_DECODED_FRAME_SIZE-1)
		raw := EncodeFrame(12, 0, payload)
		if _, err := DecodeFrame(raw); err != nil {
			t.Fatalf("maximum valid frame rejected: %v", err)
		}
		raw = EncodeFrame(12, 0, append(payload, fill))
		if _, err := DecodeFrame(raw); !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("oversize=%v", err)
		}
		raw = append(raw, EncodeDataFrame([]byte{9})...)
		frames, _, errs := ExtractFrames(raw)
		if len(frames) != 1 || len(errs) != 1 || frames[0].Data[0] != 9 {
			t.Fatalf("resync frames=%v errors=%v", frames, errs)
		}
	}
}
