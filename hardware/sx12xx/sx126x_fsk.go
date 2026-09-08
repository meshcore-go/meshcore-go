package sx12xx

import "fmt"

// fskBitrateReg converts bits/second to the 24-bit BR register value:
// BR = 32 * Fxtal / bitrate.
func fskBitrateReg(bitrate uint32) uint32 {
	if bitrate == 0 {
		return 0
	}
	return uint32(uint64(32) * xtalFreqHz / uint64(bitrate))
}

// fskFdevReg converts a frequency deviation in Hz to the 24-bit FDEV register
// value: FDEV = fdev * 2^25 / Fxtal.
func fskFdevReg(fdev uint32) uint32 {
	return uint32(uint64(fdev) * (1 << 25) / xtalFreqHz)
}

// SetModemFSK switches the active modem to GFSK/FSK (SetPacketType, 0x8A),
// ahead of the other SetFsk* methods.
func (d *SX126x) SetModemFSK() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.setPacketType(PacketTypeFSK)
}

// SetFskModulation configures the GFSK on-air bit rate in bits/second, pulse
// shape (FskPulse*), receiver bandwidth (FskBW*) and frequency deviation in Hz.
func (d *SX126x) SetFskModulation(bitrate uint32, pulseShape, bandwidth byte, fdev uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	br := fskBitrateReg(bitrate)
	fd := fskFdevReg(fdev)
	// Datasheet layout: BR[23:0], PulseShape, Bandwidth, FDEV[23:0].
	if err := d.command(opSetModulationParams,
		byte(br>>16), byte(br>>8), byte(br),
		pulseShape,
		bandwidth,
		byte(fd>>16), byte(fd>>8), byte(fd),
	); err != nil {
		return fmt.Errorf("sx126x: set fsk modulation: %w", err)
	}
	return nil
}

// SetFskPacket configures the GFSK packet format: preamble length and detector,
// sync-word length in bits, address comparison, fixed or variable length,
// payload length, CRC type and whitening.
func (d *SX126x) SetFskPacket(preambleLen uint16, preambleDetector, syncWordLen, addrComp, packetType, payloadLen, crcType, whitening byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.command(opSetPacketParams,
		byte(preambleLen>>8), byte(preambleLen),
		preambleDetector,
		syncWordLen,
		addrComp,
		packetType,
		payloadLen,
		crcType,
		whitening,
	); err != nil {
		return fmt.Errorf("sx126x: set fsk packet: %w", err)
	}
	return nil
}

// SetFskSyncWord programs up to 8 GFSK sync-word bytes; the syncWordLen passed
// to SetFskPacket should match 8*len(sw).
func (d *SX126x) SetFskSyncWord(sw []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(sw) == 0 || len(sw) > 8 {
		return fmt.Errorf("sx126x: set fsk sync word: length %d out of range (1..8)", len(sw))
	}
	if err := d.writeRegister(regSyncWord0Fsk, sw); err != nil {
		return fmt.Errorf("sx126x: set fsk sync word: %w", err)
	}
	return nil
}

// SetFskAddress programs the GFSK node and broadcast addresses used by the
// address-comparison filter.
func (d *SX126x) SetFskAddress(node, broadcast byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.writeRegister(regNodeAddressFsk, []byte{node, broadcast}); err != nil {
		return fmt.Errorf("sx126x: set fsk address: %w", err)
	}
	return nil
}

// SetFskCRC programs the GFSK CRC seed and generator polynomial.
func (d *SX126x) SetFskCRC(crcInit, crcPolynomial uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.writeRegister(regCRCInitialMSBFsk, []byte{
		byte(crcInit >> 8), byte(crcInit),
		byte(crcPolynomial >> 8), byte(crcPolynomial),
	}); err != nil {
		return fmt.Errorf("sx126x: set fsk crc: %w", err)
	}
	return nil
}

// SetFskWhitening programs the GFSK whitening LFSR seed.
func (d *SX126x) SetFskWhitening(seed uint16) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := d.writeRegister(regWhiteningInitialMSBFsk, []byte{
		byte(seed >> 8), byte(seed),
	}); err != nil {
		return fmt.Errorf("sx126x: set fsk whitening: %w", err)
	}
	return nil
}
