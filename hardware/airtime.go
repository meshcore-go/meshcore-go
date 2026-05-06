package hardware

import "math"

// LoRaAirtimeEstimator returns an airtime estimator function for the given
// radio configuration. The returned function computes the LoRa time-on-air
// in milliseconds for a packet of the given byte length.
//
// Uses the standard LoRa modem formula with explicit header, CRC enabled,
// and low data rate optimization applied when symbol time exceeds 16ms.
func LoRaAirtimeEstimator(config *RadioConfig) func(packetLen int) uint32 {
	sf := float64(config.SF)
	bw := float64(config.BwHz)
	cr := int(config.CR) // 1..4 → 4/5..4/8

	symbolTimeMs := math.Pow(2, sf) / bw * 1000.0
	preambleTimeMs := (8 + 4.25) * symbolTimeMs

	lowDataRateOpt := 0
	if symbolTimeMs >= 16.0 {
		lowDataRateOpt = 1
	}

	return func(packetLen int) uint32 {
		// Explicit header, CRC enabled
		numerator := 8*float64(packetLen) - 4*sf + 28 + 16
		denominator := 4 * (sf - float64(2*lowDataRateOpt))
		if denominator <= 0 {
			denominator = 1
		}
		symbolCount := 8 + math.Max(math.Ceil(numerator/denominator)*float64(cr+4), 0)
		payloadTimeMs := symbolCount * symbolTimeMs
		return uint32(math.Ceil(preambleTimeMs + payloadTimeMs))
	}
}
