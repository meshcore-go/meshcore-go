package sx12xx

// SX1276/77/78/79 register transport, bring-up and lifecycle.
// Datasheet: https://www.semtech.com/products/wireless-rf/lora-connect/sx1276

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"periph.io/x/conn/v3"
	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/physic"
	"periph.io/x/conn/v3/spi"
)

// SX127xOpts holds the configuration for an SX127x device.
type SX127xOpts struct {
	// Speed is the maximum SPI clock (default 8 MHz).
	Speed physic.Frequency

	// ResetPin is the required gpioreg name of the NRST line.
	ResetPin string
	// Dio0Pin is the optional gpioreg name of the DIO0 completion-interrupt line
	// (when empty, completion is detected by polling RegIrqFlags).
	Dio0Pin string
	// Dio1Pin is the optional gpioreg name of the DIO1 line.
	Dio1Pin string

	// CSPin is the optional gpioreg name of a manually driven active-low
	// chip-select, for boards whose NSS is an ordinary GPIO.
	CSPin string

	// TxEnPin and RxEnPin are the optional gpioreg names of the external
	// RF-switch control lines, driven high for transmit and receive respectively.
	TxEnPin string
	RxEnPin string

	// TxLedPin and RxLedPin are gpioreg names of optional activity LEDs, pulsed
	// for LedPulse when a packet leaves and when the receiver detects one.
	TxLedPin string
	RxLedPin string
	// LedPulse is how long the activity LEDs stay lit. Default DefaultLedPulse.
	LedPulse time.Duration

	// PaBoost selects the PA_BOOST output pin rather than RFO (default true).
	PaBoost bool

	// OcpMilliamps is the over-current protection limit programmed into RegOcp
	// (0 selects 100 mA).
	OcpMilliamps int
}

// DefaultSX127xOpts holds the default options; ResetPin must still be set.
var DefaultSX127xOpts = SX127xOpts{
	Speed:        8 * physic.MegaHertz,
	PaBoost:      true,
	OcpMilliamps: 100,
}

// SX127x is a handle to an SX1276/77/78/79 transceiver on an SPI bus, safe for
// concurrent use.
type SX127x struct {
	c    spi.Conn
	opts SX127xOpts

	reset gpio.PinIO
	dio0  gpio.PinIO // optional
	dio1  gpio.PinIO // optional
	cs    gpio.PinIO // optional manual chip-select
	txen  gpio.PinIO // optional RF-switch transmit enable
	rxen  gpio.PinIO // optional RF-switch receive enable

	mu sync.Mutex

	packets   chan Packet
	recvArmed bool
	recvErr   error
	dropped   atomic.Uint64
	nRecv     atomic.Uint64
	nSent     atomic.Uint64

	txLed    *blinker
	rxLed    *blinker
	nRecvErr atomic.Uint64
	nCRCErr  atomic.Uint64
	// stop is nil when no continuous-RX goroutine is running.
	stop chan struct{}
	wg   sync.WaitGroup

	// Cached configuration consulted by the LoRa/FSK setters.
	longRange      bool
	lowFrequency   bool
	frequency      uint32 // Hz
	implicitHeader bool
	version        byte // 0x12 or 0x22
}

// NewSX127x opens an SX127x on port and leaves it in LoRa standby; a nil opts
// uses DefaultSX127xOpts, but ResetPin is always required.
func NewSX127x(port spi.Port, opts *SX127xOpts) (*SX127x, error) {
	if opts == nil {
		opts = &DefaultSX127xOpts
	}
	o := *opts
	if o.Speed <= 0 {
		o.Speed = 8 * physic.MegaHertz
	}
	if o.OcpMilliamps <= 0 {
		o.OcpMilliamps = 100
	}
	if o.ResetPin == "" {
		return nil, errors.New("sx127x: ResetPin is required")
	}

	c, err := port.Connect(o.Speed, spi.Mode0, 8)
	if err != nil {
		return nil, fmt.Errorf("sx127x: spi connect: %w", err)
	}

	d := &SX127x{c: c, opts: o}

	if d.reset = gpioreg.ByName(o.ResetPin); d.reset == nil {
		return nil, fmt.Errorf("sx127x: reset pin %q not found", o.ResetPin)
	}
	if o.Dio0Pin != "" {
		if d.dio0 = gpioreg.ByName(o.Dio0Pin); d.dio0 == nil {
			return nil, fmt.Errorf("sx127x: dio0 pin %q not found", o.Dio0Pin)
		}
		if err := d.dio0.In(gpio.PullDown, gpio.RisingEdge); err != nil {
			return nil, fmt.Errorf("sx127x: configuring dio0 pin: %w", err)
		}
	}
	if o.Dio1Pin != "" {
		if d.dio1 = gpioreg.ByName(o.Dio1Pin); d.dio1 == nil {
			return nil, fmt.Errorf("sx127x: dio1 pin %q not found", o.Dio1Pin)
		}
		if err := d.dio1.In(gpio.PullDown, gpio.RisingEdge); err != nil {
			return nil, fmt.Errorf("sx127x: configuring dio1 pin: %w", err)
		}
	}

	if o.CSPin != "" {
		if d.cs = gpioreg.ByName(o.CSPin); d.cs == nil {
			return nil, fmt.Errorf("sx127x: cs pin %q not found", o.CSPin)
		}
		if err := d.cs.Out(gpio.High); err != nil {
			return nil, fmt.Errorf("sx127x: configuring cs pin: %w", err)
		}
	}

	if d.txLed, err = newBlinker(o.TxLedPin, "tx", o.LedPulse); err != nil {
		return nil, fmt.Errorf("sx127x: %w", err)
	}
	if d.rxLed, err = newBlinker(o.RxLedPin, "rx", o.LedPulse); err != nil {
		return nil, fmt.Errorf("sx127x: %w", err)
	}
	if o.TxEnPin != "" {
		if d.txen = gpioreg.ByName(o.TxEnPin); d.txen == nil {
			return nil, fmt.Errorf("sx127x: txen pin %q not found", o.TxEnPin)
		}
		if err := d.txen.Out(gpio.Low); err != nil {
			return nil, fmt.Errorf("sx127x: configuring txen pin: %w", err)
		}
	}
	if o.RxEnPin != "" {
		if d.rxen = gpioreg.ByName(o.RxEnPin); d.rxen == nil {
			return nil, fmt.Errorf("sx127x: rxen pin %q not found", o.RxEnPin)
		}
		if err := d.rxen.Out(gpio.Low); err != nil {
			return nil, fmt.Errorf("sx127x: configuring rxen pin: %w", err)
		}
	}

	if err := d.begin(); err != nil {
		return nil, err
	}
	return d, nil
}

// ConfigurationLost reports whether the chip has returned to its reset state,
// as a brown-out or ESD event does. begin sets the RegLna boost bits, which
// reset to 0, so their absence means every other setting is gone too and
// re-arming alone would leave a deaf receiver looking healthy.
func (d *SX127x) ConfigurationLost() (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, err := d.readReg(regLna)
	if err != nil {
		return false, err
	}
	return v&0x03 != lnaBoostHfOn, nil
}

// begin runs bring-up; it takes no lock, being called before d is published.
func (d *SX127x) begin() error {
	if err := d.reset_(); err != nil {
		return err
	}

	v, err := d.readReg(regVersion)
	if err != nil {
		return fmt.Errorf("sx127x: reading version: %w", err)
	}
	if v == versionSX1272 {
		return fmt.Errorf("sx127x: SX1272 (RegVersion %#02x) is not supported; its ModemConfig layout differs from the SX1276 family", v)
	}
	if v != versionSX1276 {
		return fmt.Errorf("sx127x: unexpected RegVersion %#02x (want %#02x); wrong wiring or device?", v, versionSX1276)
	}
	d.version = v

	if err := d.switchModem(true); err != nil {
		return err
	}
	if err := d.setMode(modeStandby); err != nil {
		return err
	}

	// RadioLib's begin defaults gain to 0, which is setGain(0): AGC plus boost.
	if err := d.writeBits(regLna, lnaBoostHfOn, 0, 2); err != nil {
		return err
	}
	if err := d.writeReg(regFifoTxBaseAddr, 0x00); err != nil {
		return err
	}
	if err := d.writeReg(regFifoRxBaseAddr, 0x00); err != nil {
		return err
	}
	return nil
}

// --- Low-level transport ---------------------------------------------------

// spiTx runs one SPI transaction, holding the manual chip-select low for its
// duration when one is configured.
func (d *SX127x) spiTx(w, r []byte) error {
	if d.cs != nil {
		if err := d.cs.Out(gpio.Low); err != nil {
			return fmt.Errorf("sx127x: cs assert: %w", err)
		}
		defer d.cs.Out(gpio.High)
	}
	return d.c.Tx(w, r)
}

func (d *SX127x) readReg(addr byte) (byte, error) {
	w := []byte{addr & sx127xReadMask, 0x00}
	r := make([]byte, 2)
	if err := d.spiTx(w, r); err != nil {
		return 0, fmt.Errorf("sx127x: readReg %#02x: %w", addr, err)
	}
	return r[1], nil
}

func (d *SX127x) writeReg(addr, val byte) error {
	w := []byte{addr | sx127xWriteMask, val}
	if err := d.spiTx(w, nil); err != nil {
		return fmt.Errorf("sx127x: writeReg %#02x: %w", addr, err)
	}
	return nil
}

// writeBits read-modify-writes length bits of addr at position, taking the
// field value from the low bits of data. Constants in this package hold their
// register-level value, so callers shift them down: writeBits(r, cSomething>>n, n, w).
func (d *SX127x) writeBits(addr, data, position, length byte) error {
	cur, err := d.readReg(addr)
	if err != nil {
		return err
	}
	field := byte(0xFF >> (8 - length))
	mask := field << position
	val := ((data & field) << position) | (cur &^ mask)
	return d.writeReg(addr, val)
}

// readBurst reads n bytes from addr; regFifo does not auto-increment, so it is
// read one byte at a time.
func (d *SX127x) readBurst(addr byte, n int) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	if addr == regFifo {
		out := make([]byte, n)
		for i := 0; i < n; i++ {
			b, err := d.readReg(regFifo)
			if err != nil {
				return nil, err
			}
			out[i] = b
		}
		return out, nil
	}
	w := make([]byte, 1+n)
	w[0] = addr & sx127xReadMask
	r := make([]byte, len(w))
	if err := d.spiTx(w, r); err != nil {
		return nil, fmt.Errorf("sx127x: readBurst %#02x: %w", addr, err)
	}
	return r[1:], nil
}

// writeBurst writes data from addr; regFifo does not auto-increment, so it is
// written one byte at a time.
func (d *SX127x) writeBurst(addr byte, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if addr == regFifo {
		for _, b := range data {
			if err := d.writeReg(regFifo, b); err != nil {
				return err
			}
		}
		return nil
	}
	w := make([]byte, 1+len(data))
	w[0] = addr | sx127xWriteMask
	copy(w[1:], data)
	if err := d.spiTx(w, nil); err != nil {
		return fmt.Errorf("sx127x: writeBurst %#02x: %w", addr, err)
	}
	return nil
}

// --- Mode / frequency primitives -------------------------------------------

// setMode writes the transceiver mode bits into RegOpMode, preserving the
// cached LongRangeMode and band bits.
func (d *SX127x) setMode(mode byte) error {
	v := mode & modeMaskBits
	if d.longRange {
		v |= modeLongRangeMode
	}
	if d.lowFrequency {
		v |= modeLowFrequencyOn
	}
	return d.writeReg(regOpMode, v)
}

// switchModem selects the LoRa or FSK/OOK modem and leaves the chip in sleep,
// the only state in which LongRangeMode may change.
func (d *SX127x) switchModem(longRange bool) error {
	// Sleep under the current modem first.
	if err := d.setMode(modeSleep); err != nil {
		return err
	}
	d.longRange = longRange
	v := byte(modeSleep)
	if longRange {
		v |= modeLongRangeMode
	}
	if d.lowFrequency {
		v |= modeLowFrequencyOn
	}
	return d.writeReg(regOpMode, v)
}

// setFrequency programs the RF carrier in Hz (Frf = hz * 2^19 / Fxosc) and
// caches the band flag for the next setMode.
func (d *SX127x) setFrequency(hz uint32) error {
	frf := uint32(uint64(hz) * sx127xFstepDiv / sx127xXtalFreqHz)
	if err := d.writeReg(regFrfMsb, byte(frf>>16)); err != nil {
		return err
	}
	if err := d.writeReg(regFrfMid, byte(frf>>8)); err != nil {
		return err
	}
	if err := d.writeReg(regFrfLsb, byte(frf)); err != nil {
		return err
	}
	d.frequency = hz
	d.lowFrequency = hz < rssiBandEdge
	return nil
}

// --- Lifecycle / public surface --------------------------------------------

// reset_ pulses NRST low then high and waits for the chip to boot.
func (d *SX127x) reset_() error {
	if err := d.reset.Out(gpio.Low); err != nil {
		return fmt.Errorf("sx127x: reset: %w", err)
	}
	time.Sleep(time.Millisecond)
	if err := d.reset.Out(gpio.High); err != nil {
		return fmt.Errorf("sx127x: reset: %w", err)
	}
	// Datasheet: 5 ms to be ready after the rising edge.
	time.Sleep(5 * time.Millisecond)
	return nil
}

// Sleep places the chip in sleep mode, its lowest-power state.
func (d *SX127x) Sleep() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop != nil {
		return errors.New("sx127x: busy receiving continuously; call Halt first")
	}
	return d.setMode(modeSleep)
}

// Standby places the chip in standby mode, ready to be configured or to start a
// TX/RX.
func (d *SX127x) Standby() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stop != nil {
		return errors.New("sx127x: busy receiving continuously; call Halt first")
	}
	return d.setMode(modeStandby)
}

// Halt stops continuous reception, idles the RF switch and returns the chip to
// standby.
func (d *SX127x) Halt() error {
	d.mu.Lock()
	if d.stop != nil {
		close(d.stop)
		d.stop = nil
	}
	d.recvArmed = false
	d.mu.Unlock()
	d.wg.Wait()

	d.txLed.darken()
	d.rxLed.darken()

	d.mu.Lock()
	defer d.mu.Unlock()
	d.rfIdle()
	return d.setMode(modeStandby)
}

// Version returns the RegVersion silicon revision (0x12 for SX1276/77/78/79,
// 0x22 for SX1272).
func (d *SX127x) Version() (byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.readReg(regVersion)
}

// String implements conn.Resource.
func (d *SX127x) String() string {
	return fmt.Sprintf("SX127x{%s}", d.c)
}

// --- RF switch helpers ------------------------------------------------------

// rfTx, rfRx and rfIdle steer the external RF switch, and are no-ops when the
// TxEn/RxEn pins are unset.
func (d *SX127x) rfTx()   { rfSetTx(d.txen, d.rxen) }
func (d *SX127x) rfRx()   { rfSetRx(d.txen, d.rxen) }
func (d *SX127x) rfIdle() { rfSetIdle(d.txen, d.rxen) }

var _ conn.Resource = &SX127x{}
var _ BaseSx12xx = &SX127x{}
