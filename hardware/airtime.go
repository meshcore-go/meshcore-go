package hardware

import "math"

// LoRaAirtimeEstimator returns an airtime estimator function for the given
// radio configuration. The returned function computes the LoRa time-on-air
// in milliseconds for a packet of the given byte length.
//
// config.CR may be 1..4 or 5..8 (raw denominator).
func LoRaAirtimeEstimator(config *RadioConfig) func(packetLen int) uint32 {
	sf := float64(config.SF)
	bw := float64(config.BwHz)
	cr := float64(config.CR)
	if cr < 5 {
		cr += 4
	}

	symbolTimeMs := math.Pow(2, sf) / bw * 1000.0

	sfCoeff1, sfCoeff2 := 4.25, 8.0
	if config.SF == 5 || config.SF == 6 {
		sfCoeff1, sfCoeff2 = 6.25, 0
	}

	sfDivisor := 4 * sf
	if symbolTimeMs >= 16.0 {
		sfDivisor = 4 * (sf - 2) // low data rate optimisation
	}
	if sfDivisor <= 0 {
		sfDivisor = 1
	}

	const bitsPerCrc, headerBits = 16.0, 20.0
	preambleSymbols := 8 + 8 + sfCoeff1

	return func(packetLen int) uint32 {
		bitCount := 8*float64(packetLen) + bitsPerCrc - 4*sf + sfCoeff2 + headerBits
		if bitCount < 0 {
			bitCount = 0
		}
		symbolCount := preambleSymbols + math.Ceil(bitCount/sfDivisor)*cr
		return uint32(math.Ceil(symbolCount * symbolTimeMs))
	}
}
