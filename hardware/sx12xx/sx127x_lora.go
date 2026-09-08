package sx12xx

import (
	"errors"
	"fmt"
	"time"
)

// --- Register-mapping helpers (SX127x-specific) ------------------------------

// sx127xBwBits maps a Bandwidth to its RegModemConfig1 value, pre-shifted into
// bits 7:4.
func sx127xBwBits(bw Bandwidth) (byte, bool) {
	switch bw {
	case BW7800:
		return mc1Bw7800, true
	case BW10400:
		return mc1Bw10400, true
	case BW15600:
		return mc1Bw15600, true
	case BW20800:
		return mc1Bw20800, true
	case BW31250:
		return mc1Bw31250, true
	case BW41700:
		return mc1Bw41700, true
	case BW62500:
		return mc1Bw62500, true
	case BW125000:
		return mc1Bw125000, true
	case BW250000:
		return mc1Bw250000, true
	case BW500000:
		return mc1Bw500000, true
	default:
		return 0, false
	}
}

// sx127xCrBits maps a CodingRate to its RegModemConfig1 value, pre-shifted
// into bits 3:1.
func sx127xCrBits(cr CodingRate) (byte, bool) {
	switch cr {
	case CR4_5:
		return mc1Cr4_5, true
	case CR4_6:
		return mc1Cr4_6, true
	case CR4_7:
		return mc1Cr4_7, true
	case CR4_8:
		return mc1Cr4_8, true
	default:
		return 0, false
	}
}

// sx127xSfBits maps a spreading factor (6..12) to its RegModemConfig2 value,
// pre-shifted into bits 7:4.
func sx127xSfBits(sf int) (byte, bool) {
	switch sf {
	case 6:
		return mc2Sf6, true
	case 7:
		return mc2Sf7, true
	case 8:
		return mc2Sf8, true
	case 9:
		return mc2Sf9, true
	case 10:
		return mc2Sf10, true
	case 11:
		return mc2Sf11, true
	case 12:
		return mc2Sf12, true
	default:
		return 0, false
	}
}

// --- Configuration ----------------------------------------------------------

// SetFrequency programs the RF carrier frequency in Hz and leaves the chip in
// standby.
func (d *SX127x) SetFrequency(hz uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recvArmed {
		return errors.New("sx127x: busy receiving continuously")
	}
	if err := d.setFrequency(hz); err != nil {
		return err
	}
	return d.setMode(modeStandby)
}

// SetTxPower sets the output power in dBm, clamped to the range of the
// configured PA, and programs over-current protection.
func (d *SX127x) SetTxPower(dBm int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if dBm > 20 {
		dBm = 20
	}

	if !d.opts.PaBoost {
		// RFO: Pout = Pmax - (15 - OutputPower).
		if dBm > 14 {
			dBm = 14
		}
		if dBm < -1 {
			dBm = -1
		}
		var paConfig byte
		var outputPower int
		if dBm == 14 {
			// MaxPower 7 here would overshoot the RFO +14 dBm limit.
			paConfig = paMaxPower6
			outputPower = dBm + 1
		} else {
			paConfig = 0x40 // MaxPower = 4 (Pmax = 13.2 dBm)
			outputPower = dBm + 2
		}
		if outputPower < 0 {
			outputPower = 0
		}
		if err := d.writeReg(regPaConfig, paSelectRfo|paConfig|byte(outputPower&0x0F)); err != nil {
			return err
		}
		if err := d.writeReg(regPaDac, paDacDefault); err != nil {
			return err
		}
		return d.setOcp(d.opts.OcpMilliamps)
	}

	// PA_BOOST: Pout = 17 - (15 - OutputPower).
	paDac := byte(paDacDefault)
	var outputPower int
	// The PA_BOOST PA draws ~120 mA at +20 dBm, so floor OCP at 140 mA.
	ocp := d.opts.OcpMilliamps
	if ocp < 140 {
		ocp = 140
	}
	// The high-power DAC is +20 dBm or nothing: RadioLib allows 2..17 or exactly
	// 20 here, so 18 and 19 clamp down rather than overshooting by 2-3 dB.
	if dBm >= 20 {
		outputPower = 15
		paDac = paDacBoost20
	} else {
		if dBm > 17 {
			dBm = 17
		}
		if dBm < 2 {
			dBm = 2
		}
		outputPower = dBm - 2
	}
	if err := d.writeReg(regPaDac, paDac); err != nil {
		return err
	}
	if err := d.writeReg(regPaConfig, paSelectBoost|paMaxPower7|byte(outputPower&0x0F)); err != nil {
		return err
	}
	return d.setOcp(ocp)
}

// setOcp programs RegOcp from a current limit in milliamps; caller holds d.mu.
func (d *SX127x) setOcp(mA int) error {
	return d.writeReg(regOcp, ocpOn|ocpTrim(mA))
}

// ocpTrim converts an over-current limit in milliamps to the RegOcp OcpTrim
// field, clamped to the ~240 mA cap.
func ocpTrim(mA int) byte {
	if mA <= 0 {
		mA = 100
	}
	var trim int
	switch {
	case mA <= 120:
		trim = (mA - 45) / 5
	case mA <= 240:
		trim = (mA + 30) / 10
	default:
		trim = 27 // ~240 mA cap
	}
	if trim < 0 {
		trim = 0
	}
	if trim > int(ocpTrimMask) {
		trim = int(ocpTrimMask)
	}
	return byte(trim)
}

// SetModulationParams configures the LoRa spreading factor (6..12), bandwidth
// and coding rate; ldroAuto enables LowDataRateOptimize once the symbol time
// reaches 16 ms.
func (d *SX127x) SetModulationParams(sf int, bw Bandwidth, cr CodingRate, ldroAuto bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recvArmed {
		return errors.New("sx127x: busy receiving continuously")
	}

	sfBits, ok := sx127xSfBits(sf)
	if !ok {
		return fmt.Errorf("sx127x: invalid spreading factor %d (valid 6..12)", sf)
	}
	if sf == 6 && !d.implicitHeader {
		return errors.New("sx127x: spreading factor 6 requires implicit-header mode (call SetPacketParams with explicitHeader=false first)")
	}
	bwBits, ok := sx127xBwBits(bw)
	if !ok {
		return fmt.Errorf("sx127x: invalid bandwidth %d Hz", uint32(bw))
	}
	crBits, ok := sx127xCrBits(cr)
	if !ok {
		return fmt.Errorf("sx127x: invalid coding rate 4/%d", uint8(cr))
	}

	// Preserve the cached implicit-header bit so setter order does not matter.
	mc1 := bwBits | crBits
	if d.implicitHeader {
		mc1 |= mc1ImplicitHeaderOn
	}
	if err := d.writeReg(regModemConfig1, mc1); err != nil {
		return err
	}

	// sfBits is pre-shifted into bits 7:4; writeBits wants the field's low bits.
	if err := d.writeBits(regModemConfig2, sfBits>>4, 4, 4); err != nil {
		return err
	}

	optimize := byte(detectOptimizeSf7)
	threshold := byte(detectThresholdSf7)
	if sf == 6 {
		optimize = detectOptimizeSf6
		threshold = detectThresholdSf6
	}
	if err := d.writeBits(regDetectOptimize, optimize, 0, 3); err != nil {
		return err
	}
	if err := d.writeReg(regDetectionThreshold, threshold); err != nil {
		return err
	}

	ldro := byte(0x00)
	if ldroAuto {
		symbolMs := float64(uint64(1)<<uint(sf)) * 1000.0 / float64(uint32(bw))
		if symbolMs >= 16.0 {
			ldro = mc3LowDataRateOptimize >> 3
		}
	}
	if err := d.writeBits(regModemConfig3, ldro, 3, 1); err != nil {
		return err
	}
	if err := d.writeBits(regModemConfig3, mc3AgcAutoOn>>2, 2, 1); err != nil {
		return err
	}

	// LIMITATION: the SX1276 BW500 errata writes to regHighBwOptimize1/2 are not
	// applied.
	return nil
}

// SetPacketParams configures the LoRa preamble length in symbols, header mode,
// CRC and IQ inversion.
func (d *SX127x) SetPacketParams(preambleLen uint16, explicitHeader bool, crc bool, invertIq bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.recvArmed {
		return errors.New("sx127x: busy receiving continuously")
	}

	if err := d.writeReg(regPreambleMsb, byte(preambleLen>>8)); err != nil {
		return err
	}
	if err := d.writeReg(regPreambleLsb, byte(preambleLen)); err != nil {
		return err
	}

	header := byte(0x00)
	if !explicitHeader {
		header = 0x01
	}
	if err := d.writeBits(regModemConfig1, header, 0, 1); err != nil {
		return err
	}
	d.implicitHeader = !explicitHeader

	crcBit := byte(0x00)
	if crc {
		crcBit = mc2RxPayloadCrcOn >> 2
	}
	if err := d.writeBits(regModemConfig2, crcBit, 2, 1); err != nil {
		return err
	}

	// RegInvertIQ bit0 = TX, bit6 = RX. The TX bit reads inverted from the
	// datasheet on real silicon, so normal IQ leaves it set (RadioLib #778).
	iqBit := byte(0x00)
	txBit := byte(0x01)
	iq2 := byte(invertIq2Off)
	if invertIq {
		iqBit = 0x01
		txBit = 0x00
		iq2 = invertIq2On
	}
	if err := d.writeBits(regInvertIQ, txBit, 0, 1); err != nil {
		return err
	}
	if err := d.writeBits(regInvertIQ, iqBit, 6, 1); err != nil {
		return err
	}
	if err := d.writeReg(regInvertIQ2, iq2); err != nil {
		return err
	}
	return nil
}

// SetPayloadLength sets RegPayloadLength, the fixed length used in
// implicit-header mode.
func (d *SX127x) SetPayloadLength(length byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeReg(regPayloadLength, length)
}

// SetSyncWord writes the 1-byte LoRa sync word (0x12 private, 0x34 public).
func (d *SX127x) SetSyncWord(b byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writeReg(regSyncWordLoRa, b)
}

// SetPublicNetwork selects the public/LoRaWAN sync word when public is true,
// otherwise the private one.
func (d *SX127x) SetPublicNetwork(public bool) error {
	sw := byte(loraSyncWordPrivate)
	if public {
		sw = loraSyncWordPublic
	}
	return d.SetSyncWord(sw)
}

// --- Transmit ----------------------------------------------------------------

// Transmit sends one LoRa payload and blocks until it completes or timeout
// elapses (zero waits indefinitely).
func (d *SX127x) Transmit(payload []byte, timeout time.Duration) error {
	if len(payload) == 0 {
		return errors.New("sx127x: empty payload")
	}
	if len(payload) > 255 {
		return errors.New("sx127x: payload exceeds 255 bytes")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stop != nil {
		// Standby and the re-arm below would discard an already-received packet.
		d.drainPending()
		defer func() {
			if err := d.resumeRx(); err != nil {
				d.recvErr = err
			}
		}()
	}
	return d.transmit(payload, timeout)
}

// transmit performs one transmission; caller holds d.mu.
func (d *SX127x) transmit(payload []byte, timeout time.Duration) error {
	if err := d.setMode(modeStandby); err != nil {
		return err
	}
	if err := d.writeReg(regFifoTxBaseAddr, 0x00); err != nil {
		return err
	}
	if err := d.writeReg(regFifoAddrPtr, 0x00); err != nil {
		return err
	}
	if err := d.writeBurst(regFifo, payload); err != nil {
		return err
	}
	if err := d.writeReg(regPayloadLength, byte(len(payload))); err != nil {
		return err
	}

	if err := d.writeReg(regIrqFlags, lrIrqAll); err != nil {
		return err
	}
	if err := d.writeBits(regDioMapping1, dio0TxDone>>6, 6, 2); err != nil {
		return err
	}

	d.rfTx()
	if err := d.setMode(modeTx); err != nil {
		d.rfIdle()
		return err
	}

	_, err := d.waitIrq(lrIrqTxDone, 0, timeout)
	d.rfIdle()
	_ = d.writeReg(regIrqFlags, lrIrqAll)
	if err != nil {
		_ = d.setMode(modeStandby)
		return err
	}
	d.nSent.Add(1)
	d.txLed.blink()
	return d.setMode(modeStandby)
}

// resumeRx returns the receiver to continuous mode; caller holds d.mu.
func (d *SX127x) resumeRx() error {
	d.recvArmed = false
	if err := d.armRx(); err != nil {
		return err
	}
	d.rfRx()
	if err := d.setMode(modeRxContinuous); err != nil {
		d.rfIdle()
		return err
	}
	d.recvArmed = true
	d.recvErr = nil
	return nil
}

// drainPending reads out a completed packet the RX goroutine has not yet picked
// up; caller holds d.mu.
func (d *SX127x) drainPending() {
	if d.stop == nil {
		return
	}
	flags, err := d.readReg(regIrqFlags)
	if err != nil || flags&lrIrqRxDone == 0 {
		return
	}
	d.rxLed.blink()
	if flags&lrIrqPayloadCrcErr != 0 {
		d.nCRCErr.Add(1)
		_ = d.writeReg(regIrqFlags, lrIrqAll)
		return
	}
	pkt, err := d.readPacket()
	_ = d.writeReg(regIrqFlags, lrIrqAll)
	if err == nil {
		d.nRecv.Add(1)
		d.enqueue(*pkt)
	} else {
		d.nRecvErr.Add(1)
	}
}

// enqueue hands pkt to the consumer without blocking, dropping the oldest when
// the buffer is full; caller holds d.mu.
func (d *SX127x) enqueue(pkt Packet) {
	select {
	case d.packets <- pkt:
		return
	default:
	}
	select {
	case <-d.packets:
		d.dropped.Add(1)
	default:
	}
	select {
	case d.packets <- pkt:
	default:
		d.dropped.Add(1)
	}
}

// InRecvMode reports whether continuous receive is running and armed.
func (d *SX127x) InRecvMode() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stop != nil && d.recvArmed
}

// ResumeReceive re-arms continuous receive after a failed attempt to restore it.
func (d *SX127x) ResumeReceive() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop == nil {
		return errors.New("sx127x: not receiving continuously")
	}
	if err := d.resumeRx(); err != nil {
		d.recvErr = err
		return err
	}
	return nil
}

// Stats returns the radio's lifetime counters.
func (d *SX127x) Stats() RadioStats {
	return RadioStats{
		PacketsRecv:       d.nRecv.Load(),
		PacketsSent:       d.nSent.Load(),
		PacketsRecvErrors: d.nRecvErr.Load(),
		PacketsCRCErrors:  d.nCRCErr.Load(),
		PacketsDropped:    d.dropped.Load(),
	}
}

// waitIrq blocks until a bit in want or fail is set in RegIrqFlags or timeout
// elapses (zero waits indefinitely), preferring the DIO0 edge over polling;
// caller holds d.mu.
func (d *SX127x) waitIrq(want, fail byte, timeout time.Duration) (byte, error) {
	deadline := time.Time{}
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if d.dio0 != nil {
			step := 100 * time.Millisecond
			if !deadline.IsZero() {
				if remain := time.Until(deadline); remain < step {
					step = remain
				}
			}
			if step > 0 {
				d.dio0.WaitForEdge(step)
			}
		} else {
			time.Sleep(2 * time.Millisecond)
		}

		flags, err := d.readReg(regIrqFlags)
		if err != nil {
			return 0, err
		}
		if flags&(want|fail) != 0 {
			return flags, nil
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return flags, fmt.Errorf("sx127x: waiting for irq: %w", ErrTimeout)
		}
	}
}

// --- Receive -----------------------------------------------------------------

// Receive performs a single LoRa reception, blocking until a packet arrives or
// timeout elapses (zero waits indefinitely).
func (d *SX127x) Receive(timeout time.Duration) (*Packet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stop != nil {
		return nil, errors.New("sx127x: already receiving continuously")
	}

	if err := d.armRx(); err != nil {
		return nil, err
	}
	d.rfRx()
	if err := d.setMode(modeRxContinuous); err != nil {
		d.rfIdle()
		return nil, err
	}

	// Continuous RX never raises the hardware RxTimeout IRQ; waitIrq enforces the
	// deadline in software.
	flags, err := d.waitIrq(lrIrqRxDone, 0, timeout)
	d.rfIdle()
	if err != nil {
		_ = d.setMode(modeStandby)
		_ = d.writeReg(regIrqFlags, lrIrqAll)
		return nil, err
	}
	_ = d.setMode(modeStandby)

	if flags&lrIrqPayloadCrcErr != 0 {
		_ = d.writeReg(regIrqFlags, lrIrqAll)
		return nil, fmt.Errorf("sx127x: receive: %w", ErrCRC)
	}
	pkt, err := d.readPacket()
	_ = d.writeReg(regIrqFlags, lrIrqAll)
	return pkt, err
}

// armRx clears stale IRQ flags and maps DIO0 to RxDone; caller holds d.mu.
func (d *SX127x) armRx() error {
	if err := d.setMode(modeStandby); err != nil {
		return err
	}
	if err := d.writeReg(regFifoRxBaseAddr, 0x00); err != nil {
		return err
	}
	if err := d.writeReg(regFifoAddrPtr, 0x00); err != nil {
		return err
	}
	if err := d.writeReg(regIrqFlags, lrIrqAll); err != nil {
		return err
	}
	return d.writeBits(regDioMapping1, dio0RxDone>>6, 6, 2)
}

// readPacket reads the most recently received frame from the FIFO; caller holds
// d.mu.
func (d *SX127x) readPacket() (*Packet, error) {
	cur, err := d.readReg(regFifoRxCurrentAddr)
	if err != nil {
		return nil, err
	}
	if err := d.writeReg(regFifoAddrPtr, cur); err != nil {
		return nil, err
	}
	n, err := d.readReg(regRxNbBytes)
	if err != nil {
		return nil, err
	}
	data, err := d.readBurst(regFifo, int(n))
	if err != nil {
		return nil, err
	}
	rssi, snr, err := d.packetStatus()
	if err != nil {
		return nil, err
	}
	return &Packet{Payload: data, RSSI: rssi, SNR: snr}, nil
}

// ReceiveContinuous starts continuous receive and returns a channel of received
// packets, closed by Halt.
func (d *SX127x) ReceiveContinuous() (<-chan Packet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stop != nil {
		return nil, errors.New("sx127x: already receiving continuously")
	}
	if err := d.armRx(); err != nil {
		return nil, err
	}
	d.rfRx()
	if err := d.setMode(modeRxContinuous); err != nil {
		d.rfIdle()
		return nil, err
	}
	d.recvArmed = true

	packets := make(chan Packet, inboundBuffer)
	stop := make(chan struct{})
	d.packets = packets
	d.stop = stop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer close(packets)
		for {
			select {
			case <-stop:
				return
			default:
			}

			d.mu.Lock()
			flags, err := d.waitIrq(lrIrqRxDone, 0, rxPollStep)
			if err != nil {
				// Timed out with no packet; loop to re-check stop.
				d.mu.Unlock()
				continue
			}
			d.rxLed.blink()
			if flags&lrIrqPayloadCrcErr != 0 {
				d.nCRCErr.Add(1)
				_ = d.writeReg(regIrqFlags, lrIrqAll)
				d.mu.Unlock()
				continue
			}
			// Buffer under the lock so the receiver keeps being serviced.
			pkt, err := d.readPacket()
			_ = d.writeReg(regIrqFlags, lrIrqAll)
			if err == nil {
				d.nRecv.Add(1)
				d.enqueue(*pkt)
			} else {
				d.nRecvErr.Add(1)
			}
			d.mu.Unlock()
		}
	}()
	return packets, nil
}

// --- Link metrics ------------------------------------------------------------

// PacketStatus returns the RSSI (dBm) and SNR (dB) of the last received packet.
func (d *SX127x) PacketStatus() (rssi, snr float64, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.packetStatus()
}

// packetStatus converts RegPktSnrValue and RegPktRssiValue into dBm and dB;
// caller holds d.mu.
func (d *SX127x) packetStatus() (rssi, snr float64, err error) {
	snrRaw, err := d.readReg(regPktSnrValue)
	if err != nil {
		return 0, 0, err
	}
	rssiRaw, err := d.readReg(regPktRssiValue)
	if err != nil {
		return 0, 0, err
	}
	snr = float64(int8(snrRaw)) / 4.0

	rssi = float64(d.rssiOffset()) + float64(rssiRaw)
	if snr < 0 {
		rssi += snr
	}
	return rssi, snr, nil
}

// rssiOffset is the dBm offset for the raw RSSI registers; caller holds d.mu.
func (d *SX127x) rssiOffset() int {
	switch {
	case d.version == versionSX1272:
		return rssiOffset72
	case d.lowFrequency:
		return rssiOffsetLF
	default:
		return rssiOffsetHF
	}
}
