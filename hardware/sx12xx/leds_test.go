package sx12xx

import (
	"sync"
	"testing"
	"time"

	"periph.io/x/conn/v3/gpio"
)

// recordingPin counts the times an LED was lit and reports its current level.
type recordingPin struct {
	gpio.PinOut
	mu    sync.Mutex
	highs int
	level gpio.Level
}

func (p *recordingPin) Out(l gpio.Level) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l == gpio.High {
		p.highs++
	}
	p.level = l
	return nil
}

func (p *recordingPin) Name() string { return "LED" }

func (p *recordingPin) state() (highs int, level gpio.Level) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.highs, p.level
}

func newTestBlinker(pulse time.Duration) (*blinker, *recordingPin) {
	pin := &recordingPin{}
	b := &blinker{pin: pin, role: "tx", pulse: pulse}
	b.timer = time.AfterFunc(pulse, b.darken)
	b.timer.Stop()
	return b, pin
}

func TestBlinkerLightsThenDarkensOnItsOwn(t *testing.T) {
	b, pin := newTestBlinker(30 * time.Millisecond)

	b.blink()
	if _, level := pin.state(); level != gpio.High {
		t.Fatal("LED was not lit")
	}
	time.Sleep(90 * time.Millisecond)
	if _, level := pin.state(); level != gpio.Low {
		t.Error("LED stayed lit past its pulse")
	}
}

func TestBlinkerExtendsOnePulseForBackToBackTraffic(t *testing.T) {
	b, pin := newTestBlinker(60 * time.Millisecond)

	b.blink()
	time.Sleep(40 * time.Millisecond)
	b.blink()
	// Past the first pulse's deadline: a second timer racing the first would
	// have darkened the LED between the two packets.
	time.Sleep(40 * time.Millisecond)
	if _, level := pin.state(); level != gpio.High {
		t.Error("LED went dark between back-to-back packets")
	}
	time.Sleep(90 * time.Millisecond)
	if _, level := pin.state(); level != gpio.Low {
		t.Error("LED never darkened after the traffic stopped")
	}
}

func TestNilBlinkerIsAWorkingNoOp(t *testing.T) {
	var b *blinker
	b.blink()
	b.darken()
}

func TestTxLedBlinksOnlyWhenAPacketActuallyLeaves(t *testing.T) {
	chip := &fakeChip{failOp: opSetTx}
	d := newTestSX126x(chip, 4)
	b, pin := newTestBlinker(time.Second)
	d.txLed = b

	if err := d.Transmit([]byte{1, 2, 3}, time.Second); err == nil {
		t.Fatal("Transmit reported success although the chip refused SetTx")
	}
	if highs, _ := pin.state(); highs != 0 {
		t.Errorf("TX LED blinked %d times for a transmission that never left, want 0", highs)
	}

	chip.failOp = 0
	if err := d.Transmit([]byte{1, 2, 3}, time.Second); err != nil {
		t.Fatalf("Transmit: %v", err)
	}
	if highs, _ := pin.state(); highs != 1 {
		t.Errorf("TX LED blinked %d times for one sent packet, want 1", highs)
	}
	if n := d.Stats().PacketsSent; n != 1 {
		t.Errorf("PacketsSent = %d after one transmission, want 1", n)
	}
}

func TestRxLedBlinksForAPacketThatFailedCRC(t *testing.T) {
	chip := &fakeChip{irq: IRQRxDone | IRQCRCErr, payload: []byte{1, 2, 3}}
	d := newTestSX126x(chip, 4)
	b, pin := newTestBlinker(time.Second)
	d.rxLed = b

	d.drainPending()

	if highs, _ := pin.state(); highs != 1 {
		t.Errorf("RX LED blinked %d times for energy that failed CRC, want 1", highs)
	}
	if len(d.packets) != 0 {
		t.Error("a packet that failed CRC was delivered")
	}
}

func TestSX127xLedsFollowTheSamePaths(t *testing.T) {
	regs := &fakeRegs{regs: map[byte]byte{}}
	d := newTestSX127x(regs, 4)
	tx, txPin := newTestBlinker(time.Second)
	rx, rxPin := newTestBlinker(time.Second)
	d.txLed, d.rxLed = tx, rx

	regs.regs[regIrqFlags] = lrIrqRxDone | lrIrqPayloadCrcErr
	d.drainPending()
	if highs, _ := rxPin.state(); highs != 1 {
		t.Errorf("RX LED blinked %d times for energy that failed CRC, want 1", highs)
	}

	if err := d.Transmit([]byte{1, 2, 3}, time.Second); err != nil {
		t.Fatalf("Transmit: %v", err)
	}
	if highs, _ := txPin.state(); highs != 1 {
		t.Errorf("TX LED blinked %d times for one sent packet, want 1", highs)
	}
}

func TestBlinkNeverWaitsOnAConcurrentDarken(t *testing.T) {
	b, _ := newTestBlinker(time.Second)

	b.mu.Lock()
	defer b.mu.Unlock()

	returned := make(chan struct{})
	go func() { b.blink(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("blink blocked on a held lock; a slow GPIO backend would stall the receive path")
	}
}
