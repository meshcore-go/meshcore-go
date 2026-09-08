package sx12xx

import (
	"testing"
	"time"

	"periph.io/x/conn/v3"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpiotest"
	"periph.io/x/conn/v3/spi"
)

type fakeRegs struct {
	regs   map[byte]byte
	fifo   []byte
	fifoAt int
	reads  int
}

func (f *fakeRegs) Tx(w, r []byte) error {
	addr := w[0]
	if addr&0x80 != 0 { // write
		reg := addr & 0x7F
		if reg == regIrqFlags { // writing a 1 clears the flag
			f.regs[reg] &^= w[1]
			return nil
		}
		f.regs[reg] = w[1]
		if reg == regOpMode && w[1]&0x07 == modeTx {
			f.regs[regIrqFlags] |= lrIrqTxDone
		}
		return nil
	}
	if addr == regFifo {
		f.reads++
		for i := 1; i < len(r); i++ {
			if f.fifoAt < len(f.fifo) {
				r[i] = f.fifo[f.fifoAt]
				f.fifoAt++
			}
		}
		return nil
	}
	for i := 1; i < len(r); i++ {
		r[i] = f.regs[addr]
	}
	return nil
}

func (f *fakeRegs) TxPackets([]spi.Packet) error { return nil }
func (f *fakeRegs) Duplex() conn.Duplex          { return conn.Full }
func (f *fakeRegs) String() string               { return "fakeRegs" }

func newTestSX127x(regs *fakeRegs, buffer int) *SX127x {
	d := &SX127x{c: regs, dio0: &gpiotest.Pin{N: "DIO0", L: gpio.Low}}
	d.packets = make(chan Packet, buffer)
	d.stop = make(chan struct{})
	d.recvArmed = true
	return d
}

func TestSX127xDrainPendingRescuesACompletedPacket(t *testing.T) {
	regs := &fakeRegs{regs: map[byte]byte{
		regIrqFlags:          lrIrqRxDone,
		regRxNbBytes:         3,
		regFifoRxCurrentAddr: 0,
	}, fifo: []byte{0x11, 0x22, 0x33}}
	d := newTestSX127x(regs, 4)

	d.drainPending()

	select {
	case pkt := <-d.packets:
		if len(pkt.Payload) != 3 || pkt.Payload[0] != 0x11 {
			t.Errorf("payload = %x, want 112233", pkt.Payload)
		}
	default:
		t.Fatal("a completed packet was not drained before the receiver was torn down")
	}
	if regs.regs[regIrqFlags]&lrIrqRxDone != 0 {
		t.Error("RxDone was left set after the drain")
	}
}

func TestSX127xDrainPendingDropsACorruptPacket(t *testing.T) {
	regs := &fakeRegs{regs: map[byte]byte{
		regIrqFlags:  lrIrqRxDone | lrIrqPayloadCrcErr,
		regRxNbBytes: 3,
	}, fifo: []byte{1, 2, 3}}
	d := newTestSX127x(regs, 4)

	d.drainPending()

	if len(d.packets) != 0 {
		t.Errorf("delivered %d packets that failed CRC, want 0", len(d.packets))
	}
	if regs.reads != 0 {
		t.Errorf("read the FIFO %d times for a CRC-failed packet, want 0", regs.reads)
	}
}

func TestSX127xDrainPendingIgnoresAnEmptyReceiver(t *testing.T) {
	regs := &fakeRegs{regs: map[byte]byte{regIrqFlags: 0}}
	d := newTestSX127x(regs, 4)

	d.drainPending()

	if regs.reads != 0 || len(d.packets) != 0 {
		t.Errorf("drained %d packets from an empty receiver (%d fifo reads)", len(d.packets), regs.reads)
	}
}

func TestSX127xEnqueueDropsOldestRatherThanBlocking(t *testing.T) {
	d := newTestSX127x(&fakeRegs{regs: map[byte]byte{}}, 2)

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
		t.Errorf("dropped %d packets, want 3", got)
	}
}

func TestSX127xTransmitDrainsAPendingPacketFirst(t *testing.T) {
	regs := &fakeRegs{regs: map[byte]byte{
		regIrqFlags:          lrIrqRxDone,
		regRxNbBytes:         2,
		regFifoRxCurrentAddr: 0,
	}, fifo: []byte{0xAA, 0xBB}}
	d := newTestSX127x(regs, 4)

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
