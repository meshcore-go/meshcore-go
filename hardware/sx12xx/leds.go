package sx12xx

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
)

// DefaultLedPulse is roughly one packet's airtime, so the LED is lit while the
// radio is busy rather than flashing for an arbitrary interval. Packets closer
// together than the pulse read as one blink.
const DefaultLedPulse = 200 * time.Millisecond

// blinker drives one activity LED. Lighting it writes the pin inline, so the
// backend behind gpioreg must be cheap; darkening is left to a timer. The nil
// blinker is a working no-op, so an unwired LED needs no checks at the call
// sites.
type blinker struct {
	pin   gpio.PinOut
	role  string
	pulse time.Duration

	mu    sync.Mutex
	timer *time.Timer

	// A failing write fails on every packet, so it is reported once.
	warnOnce sync.Once
}

func newBlinker(name, role string, pulse time.Duration) (*blinker, error) {
	if name == "" {
		return nil, nil
	}
	pin := gpioreg.ByName(name)
	if pin == nil {
		return nil, fmt.Errorf("led pin %q not found", name)
	}
	if err := pin.Out(gpio.Low); err != nil {
		return nil, fmt.Errorf("led pin %q: %w", name, err)
	}
	if pulse <= 0 {
		pulse = DefaultLedPulse
	}
	b := &blinker{pin: pin, role: role, pulse: pulse}
	// AfterFunc is the only timer whose Reset schedules a callback.
	b.timer = time.AfterFunc(pulse, b.darken)
	b.timer.Stop()
	return b, nil
}

// blink lights the LED for one pulse. Back-to-back traffic extends the pulse
// rather than racing two timers.
func (b *blinker) blink() {
	if b == nil {
		return
	}
	// Dropping a blink loses one cosmetic pulse; waiting would put a concurrent
	// darken's GPIO write on the receive path.
	if !b.mu.TryLock() {
		return
	}
	defer b.mu.Unlock()
	b.set(gpio.High)
	b.timer.Reset(b.pulse)
}

func (b *blinker) darken() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.timer.Stop()
	b.set(gpio.Low)
}

// set writes the pin, reporting a failure once. Callers hold b.mu.
func (b *blinker) set(level gpio.Level) {
	if err := b.pin.Out(level); err != nil {
		b.warnOnce.Do(func() {
			slog.Warn("sx12xx: activity LED write failed, LED will stay dark",
				"role", b.role, "pin", b.pin.Name(), "err", err)
		})
	}
}
