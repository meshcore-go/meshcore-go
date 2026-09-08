package sx12xx

import "time"

// Default activity windows, replaced by SetActivityWindows.
const (
	defaultPreambleWindow = 66 * time.Millisecond
	defaultPayloadWindow  = 3934 * time.Millisecond

	// Far longer than a four-symbol scan: expiring means the chip did not answer.
	cadTimeout = 250 * time.Millisecond
)

const activityIrqs = IRQPreambleDetected | IRQSyncWordValid | IRQHeaderValid | IRQHeaderErr

// SetActivityWindows bounds how long IsReceivingPacket trusts a stale
// preamble-detected or header-valid flag; derive them with PacketWindows.
func (d *SX126x) SetActivityWindows(preamble, payload time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.preambleWindow, d.payloadWindow = preamble, payload
}

// IsReceivingPacket reports whether a packet is currently arriving, treating a
// flag set for longer than its activity window as stale.
func (d *SX126x) IsReceivingPacket() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	irq, err := d.getIrqStatus()
	if err != nil {
		return false, err
	}
	now := time.Now()
	preamble := irq&IRQPreambleDetected != 0
	header := irq&IRQHeaderValid != 0

	if irq&IRQHeaderErr != 0 {
		return false, d.clearActivity()
	}
	if !header && d.headerSeen {
		d.activityAt, d.headerSeen = time.Time{}, false
		return false, nil
	}
	if header {
		if !d.headerSeen {
			d.headerSeen, d.activityAt = true, now
		}
		if now.Sub(d.activityAt) > d.payloadWindow {
			return false, d.clearActivity()
		}
		return true, nil
	}
	if preamble {
		if d.activityAt.IsZero() {
			d.activityAt = now
		}
		if now.Sub(d.activityAt) > d.preambleWindow {
			d.activityAt = time.Time{}
			return false, d.clearIrqStatus(IRQPreambleDetected)
		}
		return true, nil
	}
	d.activityAt, d.headerSeen = time.Time{}, false
	return false, nil
}

// clearActivity drops the latch and the chip's activity flags. Holds d.mu.
func (d *SX126x) clearActivity() error {
	d.activityAt, d.headerSeen = time.Time{}, false
	return d.clearIrqStatus(activityIrqs)
}

// SetRxBoostedGain selects the receiver's boosted-gain mode (REG_RX_GAIN).
func (d *SX126x) SetRxBoostedGain(on bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.setRxBoostedGain(on)
}

// setRxBoostedGain writes the gain mode. Holds d.mu.
func (d *SX126x) setRxBoostedGain(on bool) error {
	v := byte(RxGainPowerSaving)
	if on {
		v = RxGainBoosted
	}
	return d.writeRegister(regRxGain, []byte{v})
}

// RxBoostedGain reports whether boosted gain is active, read from the chip
// rather than cached.
func (d *SX126x) RxBoostedGain() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rxBoostedGain()
}

// rxBoostedGain reads the gain mode. Holds d.mu.
func (d *SX126x) rxBoostedGain() (bool, error) {
	v, err := d.readRegister(regRxGain, 1)
	if err != nil {
		return false, err
	}
	return v[0] == RxGainBoosted, nil
}

// ResetAGC recovers a receiver whose AGC has latched onto an interferer.
func (d *SX126x) ResetAGC() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	receiving := d.stop != nil
	d.drainPending()

	// Every path out short of the re-arm below leaves the receiver down.
	d.recvArmed = false

	// Read the gain back before calibration clears it.
	boosted, err := d.rxBoostedGain()
	if err != nil {
		return err
	}

	if err := d.setStandby(StandbyRC); err != nil {
		return err
	}
	if err := d.setSleep(SleepWarmStart); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	if err := d.wake(); err != nil {
		return err
	}
	if err := d.calibrate(CalibAll); err != nil {
		return err
	}
	// Must follow the calibration above, which reset the image band to 902-928.
	f1, f2 := imageCalBand(d.frequency)
	if err := d.calibrateImage(f1, f2); err != nil {
		return err
	}
	if d.opts.UseDIO2AsRfSwitch {
		if err := d.setDIO2AsRfSwitchCtrl(true); err != nil {
			return err
		}
	}
	if err := d.setRxBoostedGain(boosted); err != nil {
		return err
	}
	// Calibration clears it, exactly as it clears the boosted gain above.
	if err := d.applyFEMRxPatch(); err != nil {
		return err
	}
	// Warm-start sleep retains registers selectively, so do not assume the
	// clamp survived; it is also the marker ConfigurationLost reads.
	if err := d.applyTxClamp(); err != nil {
		return err
	}

	if receiving {
		if err := d.resumeRx(); err != nil {
			d.recvErr = err
			return err
		}
	}
	return nil
}

// IsReceivingPacket reports whether a packet is currently arriving, from the
// live modem status.
func (d *SX127x) IsReceivingPacket() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	st, err := d.readReg(regModemStat)
	if err != nil {
		return false, err
	}
	return st&(modemStatSignalDetected|modemStatSignalSync|modemStatHeaderValid) != 0, nil
}

// RSSIInst returns the instantaneous RSSI in dBm, valid while in receive mode.
func (d *SX127x) RSSIInst() (float64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	raw, err := d.readReg(regRssiValue)
	if err != nil {
		return 0, err
	}
	return float64(d.rssiOffset()) + float64(raw), nil
}

// ResetAGC resets the analog front-end with a sleep and re-arms the receiver.
func (d *SX127x) ResetAGC() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	receiving := d.stop != nil
	d.drainPending()
	if err := d.setMode(modeSleep); err != nil {
		return err
	}
	time.Sleep(time.Millisecond)
	if err := d.setMode(modeStandby); err != nil {
		return err
	}
	if receiving {
		if err := d.resumeRx(); err != nil {
			d.recvErr = err
			return err
		}
	}
	return nil
}

// setCadParams programs SetCadParams (0x88). Holds d.mu.
func (d *SX126x) setCadParams(symNum, detPeak, detMin, exitMode byte, timeout uint32) error {
	return d.command(opSetCadParams, symNum, detPeak, detMin, exitMode,
		byte(timeout>>16), byte(timeout>>8), byte(timeout))
}

// ScanChannel runs one hardware channel-activity detection and reports whether
// the channel is busy.
func (d *SX126x) ScanChannel() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// A packet already in the FIFO would not survive the standby CAD needs.
	d.drainPending()
	receiving := d.stop != nil
	d.recvArmed = false

	if err := d.setStandby(StandbyRC); err != nil {
		return false, err
	}
	if err := d.clearIrqStatus(IRQAll); err != nil {
		return false, err
	}
	mask := uint16(IRQCadDone | IRQCadDetected)
	if err := d.setDioIrqParams(mask, mask, 0, 0); err != nil {
		return false, err
	}
	if err := d.setCadParams(CadOn4Symb, d.sf+cadPeakForSF, CadDetMin, CadExitStandbyRC, 0); err != nil {
		return false, err
	}
	if err := d.command(opSetCad); err != nil {
		return false, err
	}

	irq, err := d.waitIrq(IRQCadDone, cadTimeout)
	busy := err == nil && irq&IRQCadDetected != 0

	// Clear before re-arming: a leftover CAD flag reads as a received packet.
	_ = d.clearIrqStatus(IRQAll)
	if receiving {
		if rerr := d.resumeRx(); rerr != nil {
			d.recvErr = rerr
			return busy, rerr
		}
	}
	if err != nil {
		return false, nil // scan did not complete; do not hold up the caller
	}
	return busy, nil
}
