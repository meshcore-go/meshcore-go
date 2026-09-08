package sx12xx

// FSK/OOK configuration only; the TX/RX flow is shared with the LoRa method
// set on *SX127x.

import "fmt"

// --- Modem selection --------------------------------------------------------

// SetModemFSK switches the active modem to FSK/OOK and leaves the chip in
// sleep; call Standby before transmitting or receiving.
func (d *SX127x) SetModemFSK() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.switchModem(false); err != nil {
		return fmt.Errorf("sx127x: set fsk modem: %w", err)
	}
	// Modulation type, RegOpMode bits 6:5.
	if err := d.writeBits(regOpMode, modeModulationFsk>>5, 5, 2); err != nil {
		return fmt.Errorf("sx127x: set fsk modem: %w", err)
	}
	return nil
}

// --- Modulation parameters --------------------------------------------------

// SetFskBitRate programs the on-air bit rate in bits/second (RegBitrate =
// Fxosc / bps).
func (d *SX127x) SetFskBitRate(bps uint32) error {
	if bps == 0 {
		return fmt.Errorf("sx127x: set fsk bit rate: bps must be non-zero")
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	br := sx127xXtalFreqHz / bps
	if br > 0xFFFF {
		return fmt.Errorf("sx127x: set fsk bit rate: %d bps too low (RegBitrate overflow)", bps)
	}
	if err := d.writeReg(regBitrateMsbFsk, byte(br>>8)); err != nil {
		return fmt.Errorf("sx127x: set fsk bit rate: %w", err)
	}
	if err := d.writeReg(regBitrateLsbFsk, byte(br)); err != nil {
		return fmt.Errorf("sx127x: set fsk bit rate: %w", err)
	}
	return nil
}

// SetFskFdev programs the single-sided frequency deviation in Hz (RegFdev =
// hz / Fstep).
func (d *SX127x) SetFskFdev(hz uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	fdev := uint32(uint64(hz) * sx127xFstepDiv / sx127xXtalFreqHz)
	if fdev > 0x3FFF {
		return fmt.Errorf("sx127x: set fsk fdev: %d Hz too large (RegFdev overflow)", hz)
	}
	if err := d.writeReg(regFdevMsbFsk, byte(fdev>>8)&0x3F); err != nil {
		return fmt.Errorf("sx127x: set fsk fdev: %w", err)
	}
	if err := d.writeReg(regFdevLsbFsk, byte(fdev)); err != nil {
		return fmt.Errorf("sx127x: set fsk fdev: %w", err)
	}
	return nil
}

// SetFskRxBandwidth programs the FSK receiver channel-filter bandwidth to the
// narrowest standard step that covers hz.
func (d *SX127x) SetFskRxBandwidth(hz uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	reg, _ := sx127xRxBwReg(hz)
	if err := d.writeReg(regRxBwFsk, reg); err != nil {
		return fmt.Errorf("sx127x: set fsk rx bandwidth: %w", err)
	}
	return nil
}

// sx127xRxBwReg returns the RegRxBw byte (mantissa in bits 4:3, exponent in
// bits 2:0) and the bandwidth it selects: the narrowest standard step covering
// hz, or the widest step when hz exceeds all of them.
func sx127xRxBwReg(hz uint32) (reg byte, bwHz uint32) {
	mantissas := [...]struct {
		field byte
		m     uint32
	}{{0, 16}, {1, 20}, {2, 24}}

	var bestReg, widestReg byte
	var bestBw, widestBw uint32
	for e := byte(1); e <= 7; e++ {
		for _, mn := range mantissas {
			bw := uint32(sx127xXtalFreqHz / (mn.m * (1 << (uint(e) + 2))))
			r := (mn.field << 3) | e
			if bw > widestBw {
				widestBw, widestReg = bw, r
			}
			if bw >= hz && (bestBw == 0 || bw < bestBw) {
				bestBw, bestReg = bw, r
			}
		}
	}
	if bestBw == 0 {
		return widestReg, widestBw
	}
	return bestReg, bestBw
}

// --- Sync word --------------------------------------------------------------

// SetFskSyncWord programs 1..8 sync-word bytes and enables detection; an empty
// slice disables it.
func (d *SX127x) SetFskSyncWord(sw []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(sw) > 8 {
		return fmt.Errorf("sx127x: set fsk sync word: length %d out of range (0..8)", len(sw))
	}
	if len(sw) == 0 {
		if err := d.writeBits(regSyncConfigFsk, 0, 4, 1); err != nil {
			return fmt.Errorf("sx127x: set fsk sync word: %w", err)
		}
		return nil
	}

	cfg := fskSyncOn | (byte(len(sw)-1) & fskSyncSizeMask)
	if err := d.writeBits(regSyncConfigFsk, cfg, 0, 5); err != nil {
		return fmt.Errorf("sx127x: set fsk sync word: %w", err)
	}
	// RegSyncValue1..n auto-increment over a burst.
	if err := d.writeBurst(regSyncValueBaseFsk, sw); err != nil {
		return fmt.Errorf("sx127x: set fsk sync word: %w", err)
	}
	return nil
}

// --- Packet format ----------------------------------------------------------

// SetFskPacket configures the FSK packet engine: variable or fixed length,
// DC-free encoding (one of the fskDcFree* values), CRC, preamble length in
// bytes and payload length.
func (d *SX127x) SetFskPacket(variableLength bool, dcFree byte, crcOn bool, preambleLen uint16, payloadLen byte) error {
	switch dcFree {
	case fskDcFreeOff, fskDcFreeManchester, fskDcFreeWhitening:
	default:
		return fmt.Errorf("sx127x: set fsk packet: invalid dcFree %#02x", dcFree)
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.writeReg(regPreambleMsbFsk, byte(preambleLen>>8)); err != nil {
		return fmt.Errorf("sx127x: set fsk packet: %w", err)
	}
	if err := d.writeReg(regPreambleLsbFsk, byte(preambleLen)); err != nil {
		return fmt.Errorf("sx127x: set fsk packet: %w", err)
	}

	// Address filtering off; CrcWhiteningType left at CCITT.
	cfg1 := dcFree | fskAddrFilterOff
	if variableLength {
		cfg1 |= fskPacketFormatVariable
	} else {
		cfg1 |= fskPacketFormatFixed
	}
	if crcOn {
		cfg1 |= fskCrcOn
	}
	if err := d.writeReg(regPacketConfig1Fsk, cfg1); err != nil {
		return fmt.Errorf("sx127x: set fsk packet: %w", err)
	}

	// DataMode = packet, with the upper payload-length bits left 0.
	if err := d.writeBits(regPacketConfig2Fsk, fskDataModePacket, 0, 7); err != nil {
		return fmt.Errorf("sx127x: set fsk packet: %w", err)
	}

	if err := d.writeReg(regPayloadLengthFsk, payloadLen); err != nil {
		return fmt.Errorf("sx127x: set fsk packet: %w", err)
	}
	return nil
}
