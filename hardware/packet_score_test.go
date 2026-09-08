package hardware

import "testing"

func TestPacketScore(t *testing.T) {
	tests := []struct {
		name      string
		snr       float64
		sf        uint8
		packetLen int
		want      float64
	}{
		{"below threshold", -8, 7, 32, 0},
		{"at threshold", -7.5, 7, 32, 0},
		{"unsupported sf", 10, 6, 32, 0},
		{"sf12 floor", -20, 12, 32, 0},
		// (0 - -7.5)/10 * (1 - 32/256) = 0.75 * 0.875
		{"mid range", 0, 7, 32, 0.65625},
		{"clamped to one", 20, 12, 0, 1},
		{"full length scores zero", 20, 7, 256, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PacketScore(tt.snr, tt.sf, tt.packetLen); got != tt.want {
				t.Errorf("PacketScore(%v, %d, %d) = %v, want %v", tt.snr, tt.sf, tt.packetLen, got, tt.want)
			}
		})
	}
}

func TestPacketScoreMonotonic(t *testing.T) {
	prev := -1.0
	for snr := -20.0; snr <= 15; snr += 0.5 {
		got := PacketScore(snr, 9, 64)
		if got < prev {
			t.Fatalf("score fell from %v to %v at snr %v", prev, got, snr)
		}
		prev = got
	}
}
