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

// CR 5..8 (wire form) must estimate identically to 1..4.
func TestLoRaAirtimeEstimator_CRConventions(t *testing.T) {
	for _, sf := range []uint8{5, 7, 9, 12} {
		for cr := uint8(1); cr <= 4; cr++ {
			lo := LoRaAirtimeEstimator(&RadioConfig{BwHz: 125000, SF: sf, CR: cr})
			hi := LoRaAirtimeEstimator(&RadioConfig{BwHz: 125000, SF: sf, CR: cr + 4})
			for _, n := range []int{1, 20, 255} {
				if lo(n) != hi(n) {
					t.Errorf("SF%d len %d: CR%d=%d ms, CR%d=%d ms", sf, n, cr, lo(n), cr+4, hi(n))
				}
			}
		}
	}
}

func TestLoRaAirtimeEstimator_SF5(t *testing.T) {
	est := LoRaAirtimeEstimator(&RadioConfig{BwHz: 125000, SF: 5, CR: 5})
	if got := est(255); got != 138 {
		t.Errorf("SF5 airtime = %d ms, want 138", got)
	}
}
