package hardware

import (
	"testing"
)

func TestLoRaAirtimeEstimator(t *testing.T) {
	tests := []struct {
		name   string
		config RadioConfig
		pktLen int
		minMs  uint32
		maxMs  uint32
	}{
		{
			name:   "SF7/125kHz/CR5 small packet",
			config: RadioConfig{FreqHz: 915000000, BwHz: 125000, SF: 7, CR: 1},
			pktLen: 20,
			minMs:  40,
			maxMs:  80,
		},
		{
			name:   "SF12/125kHz/CR5 small packet",
			config: RadioConfig{FreqHz: 915000000, BwHz: 125000, SF: 12, CR: 1},
			pktLen: 20,
			minMs:  1000,
			maxMs:  2000,
		},
		{
			name:   "SF12/125kHz/CR5 max packet",
			config: RadioConfig{FreqHz: 915000000, BwHz: 125000, SF: 12, CR: 1},
			pktLen: 255,
			minMs:  5000,
			maxMs:  15000,
		},
		{
			name:   "SF9/250kHz/CR5 medium packet",
			config: RadioConfig{FreqHz: 915000000, BwHz: 250000, SF: 9, CR: 1},
			pktLen: 50,
			minMs:  50,
			maxMs:  200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			est := LoRaAirtimeEstimator(&tc.config)
			got := est(tc.pktLen)
			if got < tc.minMs || got > tc.maxMs {
				t.Errorf("got %d ms, want between %d and %d ms", got, tc.minMs, tc.maxMs)
			}
		})
	}
}
