package hardware

// snrThresholds is the approximate SNR floor for successful reception at
// SF7..SF12, from the Semtech datasheets.
var snrThresholds = [6]float64{-7.5, -10, -12.5, -15, -17.5, -20}

// PacketScore rates a received packet from 0 (no chance of success) to 1.
func PacketScore(snr float64, sf uint8, packetLen int) float64 {
	if sf < 7 || sf > 12 {
		return 0
	}
	threshold := snrThresholds[sf-7]
	if snr < threshold {
		return 0
	}
	return min(1, max(0, (snr-threshold)/10*(1-float64(packetLen)/256)))
}
