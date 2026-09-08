package sx12xx

import (
	"errors"
	"testing"
	"time"

	"periph.io/x/conn/v3"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpiotest"
	"periph.io/x/conn/v3/spi"
)

var errSPIFailed = errors.New("spi failed")

type fakeChip struct {
	irq     uint16
	payload []byte
	cleared []uint16
	reads   int
	// steadyRx re-raises RxDone after every clear: packets arriving back to back.
	steadyRx bool
	// failOp makes that one opcode fail.
	failOp byte

	ops               []byte   // opcode order
	calls             [][]byte // full command bytes
	rxGain            byte
	femPatch          byte
	clamp             byte
	clearClampOnSleep bool
	// cmdStatus is the byte GetStatus returns; the failure codes are in bits 3:1.
	cmdStatus            byte
	asleep               bool
	devErrors            uint16
	cadDetect            bool
	clearGainOnCalibrate bool
}

func (c *fakeChip) Tx(w, r []byte) error {
	if c.failOp != 0 && w[0] == c.failOp {
		return errSPIFailed
	}
	// Any transaction drives NSS, which wakes a sleeping SX126x.
	c.asleep = false
	c.ops = append(c.ops, w[0])
	c.calls = append(c.calls, append([]byte(nil), w...))
	switch w[0] {
	case opWriteRegister:
		switch uint16(w[1])<<8 | uint16(w[2]) {
		case regRxGain:
			c.rxGain = w[3]
		case regFEMRxPatch:
			c.femPatch = w[3]
		case regTxClampConfig:
			c.clamp = w[3]
		}
	case opReadRegister:
		switch uint16(w[1])<<8 | uint16(w[2]) {
		case regRxGain:
			r[4] = c.rxGain
		case regFEMRxPatch:
			r[4] = c.femPatch
		case regTxClampConfig:
			r[4] = c.clamp
		}
	case opCalibrate:
		if c.clearGainOnCalibrate {
			c.rxGain = RxGainPowerSaving // calibration drops the setting
			c.femPatch = 0
		}
	case opSetSleep:
		c.asleep = true
		if c.clearClampOnSleep {
			c.clamp = 0 // warm-start retention is selective
		}
		// Restarting the oscillator latches XOSC_START_ERR; errors are sticky.
		c.devErrors |= DeviceErrXoscStart
	case opGetDeviceErrors:
		r[2], r[3] = byte(c.devErrors>>8), byte(c.devErrors)
	case opClearDeviceErrors:
		c.devErrors = 0
	case opSetCad:
		c.irq |= IRQCadDone
		if c.cadDetect {
			c.irq |= IRQCadDetected
		}
	case opGetIrqStatus: // w = op + 3 response bytes
		r[2], r[3] = byte(c.irq>>8), byte(c.irq)
	case opClearIrqStatus:
		mask := uint16(w[1])<<8 | uint16(w[2])
		c.cleared = append(c.cleared, mask)
		c.irq &^= mask
		if c.steadyRx {
			c.irq |= IRQRxDone
		}
	case opGetRxBufferStatus:
		r[2], r[3] = byte(len(c.payload)), 0
	case opReadBuffer: // w = op + offset + status byte + payload
		c.reads++
		copy(r[3:], c.payload)
	case opGetPacketStatus: // status, rssiPkt, snrPkt, signalRssiPkt
		r[2], r[3], r[4] = 100, 20, 100
	case opSetTx:
		c.irq |= IRQTxDone
	case opGetStatus:
		r[1] = c.cmdStatus
	}
	return nil
}

func (c *fakeChip) TxPackets([]spi.Packet) error { return nil }
func (c *fakeChip) Duplex() conn.Duplex          { return conn.Full }
func (c *fakeChip) String() string               { return "fakeChip" }

// busyPin models BUSY: held high while asleep, cleared by an NSS edge.
type busyPin struct {
	*gpiotest.Pin
	chip *fakeChip
}

func (p *busyPin) Read() gpio.Level {
	if p.chip.asleep {
		return gpio.High
	}
	return gpio.Low
}

// newTestSX126x builds a driver already in continuous receive.
func newTestSX126x(chip *fakeChip, buffer int) *SX126x {
	busy := &busyPin{Pin: &gpiotest.Pin{N: "BUSY", L: gpio.Low}, chip: chip}
	d := &SX126x{c: chip, busy: busy, packetType: PacketTypeLoRa}
	d.opts.BusyTimeout = time.Millisecond
	d.preambleLen, d.explicitHeader, d.crcOn, d.payloadLen = 8, true, true, 0xFF
	d.packets = make(chan Packet, buffer)
	d.stop = make(chan struct{})
	d.recvArmed = true
	return d
}

func TestDrainPendingRescuesACompletedPacket(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone, payload: []byte{0xDE, 0xAD, 0xBE, 0xEF}}
	d := newTestSX126x(chip, 4)

	d.drainPending()

	select {
	case pkt := <-d.packets:
		if len(pkt.Payload) != 4 || pkt.Payload[0] != 0xDE {
			t.Errorf("payload = %x, want deadbeef", pkt.Payload)
		}
	default:
		t.Fatal("a completed packet was not drained before the receiver was torn down")
	}
	if chip.irq&IRQRxDone != 0 {
		t.Error("RxDone was left set after the drain")
	}
}

func TestDrainPendingIgnoresAnEmptyReceiver(t *testing.T) {
	chip := &fakeChip{irq: 0}
	d := newTestSX126x(chip, 4)

	d.drainPending()

	if chip.reads != 0 {
		t.Errorf("read the FIFO %d times with no packet pending, want 0", chip.reads)
	}
	if len(d.packets) != 0 {
		t.Errorf("queued %d packets with none pending", len(d.packets))
	}
}

func TestDrainPendingDropsACorruptPacket(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone | IRQCRCErr, payload: []byte{1, 2, 3}}
	d := newTestSX126x(chip, 4)

	d.drainPending()

	if len(d.packets) != 0 {
		t.Errorf("delivered %d packets that failed CRC, want 0", len(d.packets))
	}
}

func TestDrainPendingIsANoOpOutsideContinuousReceive(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone, payload: []byte{1, 2, 3}}
	d := newTestSX126x(chip, 4)
	d.stop = nil

	d.drainPending()

	if chip.reads != 0 {
		t.Errorf("read the FIFO %d times outside continuous receive, want 0", chip.reads)
	}
}

func TestEnqueueDropsOldestRatherThanBlocking(t *testing.T) {
	d := newTestSX126x(&fakeChip{}, 2)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 5 {
			d.enqueue(Packet{Payload: []byte{byte(i)}})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue blocked on a full buffer; the receiver would stall behind a slow handler")
	}

	if got := d.Stats().PacketsDropped; got != 3 {
		t.Errorf("dropped %d packets, want 3 (5 offered into a buffer of 2)", got)
	}
	first := <-d.packets
	if first.Payload[0] != 3 {
		t.Errorf("oldest surviving packet = %d, want 3", first.Payload[0])
	}
}

func TestTransmitDrainsAPendingPacketFirst(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone, payload: []byte{0xAA, 0xBB}}
	d := newTestSX126x(chip, 4)

	if err := d.Transmit([]byte{1, 2, 3}, time.Second); err != nil {
		t.Fatalf("Transmit: %v", err)
	}

	select {
	case pkt := <-d.packets:
		if len(pkt.Payload) != 2 || pkt.Payload[0] != 0xAA {
			t.Errorf("payload = %x, want aabb", pkt.Payload)
		}
	default:
		t.Fatal("transmitting destroyed a packet that had already been received")
	}
	if !d.InRecvMode() {
		t.Error("receiver was not re-armed after the transmission")
	}
}

func TestFailedReArmIsVisible(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	if !d.InRecvMode() {
		t.Fatal("test setup should start in receive mode")
	}

	chip.failOp = opSetRx
	_ = d.Transmit([]byte{1, 2, 3}, 200*time.Millisecond)

	if d.InRecvMode() {
		t.Error("driver reports it is receiving after the re-arm failed")
	}
	chip.failOp = 0
	if err := d.ResumeReceive(); err != nil {
		t.Errorf("ResumeReceive on a healthy radio: %v", err)
	}
	if !d.InRecvMode() {
		t.Error("receiver not armed after a successful ResumeReceive")
	}
}

func TestReceiveContinuousBuffersForABusyConsumer(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone, payload: []byte{0x42}, steadyRx: true}
	d := newTestSX126x(chip, 0)
	d.stop, d.packets, d.recvArmed = nil, nil, false

	ch, err := d.ReceiveContinuous()
	if err != nil {
		t.Fatalf("ReceiveContinuous: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := d.Halt(); err != nil {
		t.Fatalf("Halt: %v", err)
	}

	var got int
	for range ch {
		got++
	}
	if got == 0 {
		t.Fatal("no packets retained while the consumer was busy; the receive path is unbuffered")
	}
	if got < 8 {
		t.Errorf("retained only %d packets while the consumer was busy", got)
	}
}

// sawOrder reports whether the opcodes appeared in this order.
func (c *fakeChip) sawOrder(ops ...byte) bool {
	i := 0
	for _, op := range c.ops {
		if i < len(ops) && op == ops[i] {
			i++
		}
	}
	return i == len(ops)
}

func TestResetAGCCalibrates(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.frequency = 869_525_000

	if err := d.ResetAGC(); err != nil {
		t.Fatalf("ResetAGC: %v", err)
	}
	if !chip.sawOrder(opSetSleep, opCalibrate, opCalibrateImage) {
		t.Errorf("reset sequence was %#v, want a warm sleep then Calibrate then CalibrateImage", chip.ops)
	}
}

func TestResetAGCRecalibratesTheOperatingBand(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.frequency = 869_525_000

	if err := d.ResetAGC(); err != nil {
		t.Fatalf("ResetAGC: %v", err)
	}

	var calImg []byte
	for _, c := range chip.calls {
		if c[0] == opCalibrateImage {
			calImg = c
		}
	}
	if calImg == nil {
		t.Fatal("no image calibration after Calibrate; an 868 MHz node would be left on the 902-928 MHz default")
	}
	f1, f2 := imageCalBand(869_525_000)
	if calImg[1] != f1 || calImg[2] != f2 {
		t.Errorf("image calibrated for band %#02x/%#02x, want %#02x/%#02x for 869.525 MHz", calImg[1], calImg[2], f1, f2)
	}
	if !chip.sawOrder(opCalibrate, opCalibrateImage) {
		t.Error("image calibration ran before Calibrate, which then reset it to the default band")
	}
}

func TestResetAGCPreservesBoostedGain(t *testing.T) {
	chip := &fakeChip{rxGain: RxGainBoosted}
	d := newTestSX126x(chip, 4)
	d.frequency = 917_375_000
	chip.clearGainOnCalibrate = true

	if err := d.ResetAGC(); err != nil {
		t.Fatalf("ResetAGC: %v", err)
	}
	if chip.rxGain != RxGainBoosted {
		t.Errorf("RX gain after reset = %#02x, want boosted (%#02x) re-applied", chip.rxGain, RxGainBoosted)
	}
}

func TestRxBoostedGainRoundTrips(t *testing.T) {
	chip := &fakeChip{rxGain: RxGainPowerSaving}
	d := newTestSX126x(chip, 4)

	if on, err := d.RxBoostedGain(); err != nil || on {
		t.Fatalf("RxBoostedGain() = %v, %v; want false", on, err)
	}
	if err := d.SetRxBoostedGain(true); err != nil {
		t.Fatalf("SetRxBoostedGain: %v", err)
	}
	if chip.rxGain != RxGainBoosted {
		t.Errorf("register = %#02x, want %#02x", chip.rxGain, RxGainBoosted)
	}
	if on, err := d.RxBoostedGain(); err != nil || !on {
		t.Errorf("RxBoostedGain() = %v, %v; want true", on, err)
	}
}

func TestScanChannelReportsActivityAndClearsUp(t *testing.T) {
	for _, tc := range []struct {
		name     string
		detected bool
	}{{"busy", true}, {"free", false}} {
		t.Run(tc.name, func(t *testing.T) {
			chip := &fakeChip{cadDetect: tc.detected}
			d := newTestSX126x(chip, 4)
			d.sf = 7

			busy, err := d.ScanChannel()
			if err != nil {
				t.Fatalf("ScanChannel: %v", err)
			}
			if busy != tc.detected {
				t.Errorf("busy = %v, want %v", busy, tc.detected)
			}
			if chip.irq&(IRQCadDone|IRQCadDetected) != 0 {
				t.Error("CAD interrupt left set; the next read would take it for a packet")
			}
			if !d.InRecvMode() {
				t.Error("receiver not re-armed after the scan")
			}
			var cad []byte
			for _, c := range chip.calls {
				if c[0] == opSetCadParams {
					cad = c
				}
			}
			if cad == nil {
				t.Fatal("CAD parameters were never set")
			}
			if cad[2] != 7+cadPeakForSF {
				t.Errorf("detPeak = %d, want %d (SF7 + 13)", cad[2], 7+cadPeakForSF)
			}
		})
	}
}

func TestScanChannelClearsUpWhenNotReceiving(t *testing.T) {
	chip := &fakeChip{cadDetect: true}
	d := newTestSX126x(chip, 4)
	d.sf = 7
	d.stop, d.recvArmed = nil, false

	if _, err := d.ScanChannel(); err != nil {
		t.Fatalf("ScanChannel: %v", err)
	}
	if chip.irq&(IRQCadDone|IRQCadDetected) != 0 {
		t.Errorf("CAD interrupt %#04x left set with no re-arm to clear it", chip.irq)
	}
}

func TestWakeFromSleepDoesNotDeadlock(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.stop, d.recvArmed = nil, false

	if err := d.Sleep(); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if !chip.asleep {
		t.Fatal("chip did not go to sleep")
	}

	done := make(chan error, 1)
	go func() { done <- d.Wake() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wake: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wake blocked: it waited on BUSY before the transmission that clears it")
	}
	if chip.asleep {
		t.Error("chip still asleep after Wake")
	}
}

func TestResetAGCSurvivesTheWarmSleep(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.frequency = 917_375_000

	done := make(chan error, 1)
	go func() { done <- d.ResetAGC() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResetAGC: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResetAGC blocked waking the chip after its warm sleep")
	}
	if !chip.sawOrder(opSetSleep, opCalibrate, opCalibrateImage) {
		t.Errorf("never reached calibration: %#v", chip.ops)
	}
	if !d.InRecvMode() {
		t.Error("receiver not re-armed after a successful reset")
	}
}

func TestFailedResetAGCLeavesTheReceiverVisiblyDown(t *testing.T) {
	chip := &fakeChip{failOp: opCalibrate}
	d := newTestSX126x(chip, 4)
	d.frequency = 917_375_000

	if err := d.ResetAGC(); err == nil {
		t.Fatal("ResetAGC reported success though calibration failed")
	}
	if d.InRecvMode() {
		t.Error("a half-completed reset still reports the receiver as armed; the watchdog would never fire")
	}
}

func TestWakeClearsTheSpuriousOscillatorError(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.stop, d.recvArmed = nil, false

	if err := d.Sleep(); err != nil {
		t.Fatalf("Sleep: %v", err)
	}
	if err := d.Wake(); err != nil {
		t.Fatalf("Wake: %v", err)
	}
	errs, err := d.DeviceErrors()
	if err != nil {
		t.Fatalf("DeviceErrors: %v", err)
	}
	if errs&DeviceErrXoscStart != 0 {
		t.Errorf("device errors = %#04x after Wake; XOSC_START_ERR left latched", errs)
	}
}

func TestResetAGCLeavesDeviceErrorsClean(t *testing.T) {
	chip := &fakeChip{}
	d := newTestSX126x(chip, 4)
	d.frequency = 917_375_000

	if err := d.ResetAGC(); err != nil {
		t.Fatalf("ResetAGC: %v", err)
	}
	errs, err := d.DeviceErrors()
	if err != nil {
		t.Fatalf("DeviceErrors: %v", err)
	}
	if errs != 0 {
		t.Errorf("device errors = %#04x after ResetAGC, want clean", errs)
	}
}
