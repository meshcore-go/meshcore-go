package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const rxMetaTimeout = 1 * time.Second

// Transport is the interface that hardware transports must implement.
type Transport interface {
	Connect(ctx context.Context) error
	Close() error
	Send(data []byte) error
	SetFrameHandler(func(*KissFrame))
	SetErrorHandler(func(error))
}

// FrameHandler is called when a KISS frame is received.
type FrameHandler = func(*KissFrame)

// HwFrameHandler is called when a hardware sub-command frame is received.
type HwFrameHandler = func(subCmd byte, data []byte)

type DataFrameHandler = func(data []byte, snr int8, rssi int8)

// ModemOption configures a KissModem.
type ModemOption func(*KissModem)

// WithSignalReport enables RX metadata collection. When enabled, the modem
// sends HW_CMD_SET_SIGNAL_REPORT(0x01) on Connect and queues incoming data
// frames until the corresponding HW_RESP_RX_META arrives (or a timeout
// expires), populating the KissFrame's SNR and RSSI fields before dispatch.
func WithSignalReport(enabled bool) ModemOption {
	return func(m *KissModem) {
		m.signalReport = enabled
	}
}

// KissModem represents a KISS TNC modem connection.
type KissModem struct {
	transport    Transport
	kissPort     int
	signalReport bool

	errMu sync.RWMutex
	errH  func(error)

	frameMu sync.RWMutex
	frameH  FrameHandler

	hwMu  sync.RWMutex
	hwMap map[byte][]HwFrameHandler

	dataMu sync.RWMutex
	dataH  DataFrameHandler

	pendingMu    sync.Mutex
	pendingFrame *KissFrame
	pendingTimer *time.Timer
}

// NewKissModem creates a new KissModem using the given transport.
func NewKissModem(t Transport, opts ...ModemOption) *KissModem {
	m := &KissModem{
		transport: t,
		hwMap:     make(map[byte][]HwFrameHandler),
	}
	for _, opt := range opts {
		opt(m)
	}
	t.SetFrameHandler(m.onFrame)
	t.SetErrorHandler(m.onError)
	return m
}

// Connect opens the transport connection. If signal reporting is enabled,
// the modem is instructed to send RX metadata after each received packet.
func (m *KissModem) Connect(ctx context.Context) error {
	if err := m.transport.Connect(ctx); err != nil {
		return err
	}
	if m.signalReport {
		if err := m.SendHardwareCommand(HW_CMD_SET_SIGNAL_REPORT, []byte{0x01}); err != nil {
			return fmt.Errorf("enable signal report: %w", err)
		}
	}
	return nil
}

// Close shuts down the transport connection.
func (m *KissModem) Close() error {
	m.pendingMu.Lock()
	if m.pendingTimer != nil {
		m.pendingTimer.Stop()
		m.pendingTimer = nil
	}
	m.pendingFrame = nil
	m.pendingMu.Unlock()
	return m.transport.Close()
}

// SetErrorHandler sets the callback for transport errors.
func (m *KissModem) SetErrorHandler(h func(error)) {
	m.errMu.Lock()
	m.errH = h
	m.errMu.Unlock()
}

// SetFrameHandler sets a callback invoked for every received KISS frame.
func (m *KissModem) SetFrameHandler(h FrameHandler) {
	m.frameMu.Lock()
	m.frameH = h
	m.frameMu.Unlock()
}

// SetDataHandler sets the callback for received data frames (KISS_CMD_DATA).
func (m *KissModem) SetDataHandler(h DataFrameHandler) {
	m.dataMu.Lock()
	m.dataH = h
	m.dataMu.Unlock()
}

// OnHwResponse registers a handler for a specific hardware sub-command response.
func (m *KissModem) OnHwResponse(subCmd byte, h HwFrameHandler) {
	m.hwMu.Lock()
	m.hwMap[subCmd] = append(m.hwMap[subCmd], h)
	m.hwMu.Unlock()
}

// SendData sends a data frame.
func (m *KissModem) SendData(data []byte) error {
	frame := EncodeFrame(m.kissPort, KISS_CMD_DATA, data)
	return m.transport.Send(frame)
}

// SendHardwareCommand sends a hardware sub-command with the given payload.
func (m *KissModem) SendHardwareCommand(subCmd byte, data []byte) error {
	frame := EncodeHardwareFrame(m.kissPort, subCmd, data)
	return m.transport.Send(frame)
}

// SetRadio configures the radio parameters.
func (m *KissModem) SetRadio(config *RadioConfig) error {
	return m.SendHardwareCommand(HW_CMD_SET_RADIO, config.ToBytes())
}

// SetTxPower configures the transmit power.
func (m *KissModem) SetTxPower(power uint8) error {
	return m.SendHardwareCommand(HW_CMD_SET_TX_POWER, []byte{power})
}

// GetRadio requests the current radio configuration.
func (m *KissModem) GetRadio() error {
	return m.SendHardwareCommand(HW_CMD_GET_RADIO, nil)
}

// GetTxPower requests the current transmit power.
func (m *KissModem) GetTxPower() error {
	return m.SendHardwareCommand(HW_CMD_GET_TX_POWER, nil)
}

// GetVersion requests the firmware version.
func (m *KissModem) GetVersion() error {
	return m.SendHardwareCommand(HW_CMD_GET_VERSION, nil)
}

// GetStats requests modem statistics.
func (m *KissModem) GetStats() error {
	return m.SendHardwareCommand(HW_CMD_GET_STATS, nil)
}

// GetBattery requests battery status.
func (m *KissModem) GetBattery() error {
	return m.SendHardwareCommand(HW_CMD_GET_BATTERY, nil)
}

// GetDeviceName requests the device name.
func (m *KissModem) GetDeviceName() error {
	return m.SendHardwareCommand(HW_CMD_GET_DEVICE_NAME, nil)
}

// Ping sends a ping command.
func (m *KissModem) Ping() error {
	return m.SendHardwareCommand(HW_CMD_PING, nil)
}

// Reboot sends a reboot command.
func (m *KissModem) Reboot() error {
	return m.SendHardwareCommand(HW_CMD_REBOOT, nil)
}

// GetCurrentRssi requests the current RSSI reading.
func (m *KissModem) GetCurrentRssi() error {
	return m.SendHardwareCommand(HW_CMD_GET_CURRENT_RSSI, nil)
}

// GetNoiseFloor requests the noise floor reading.
func (m *KissModem) GetNoiseFloor() error {
	return m.SendHardwareCommand(HW_CMD_GET_NOISE_FLOOR, nil)
}

// IsChannelBusy requests the channel busy status.
func (m *KissModem) IsChannelBusy() error {
	return m.SendHardwareCommand(HW_CMD_IS_CHANNEL_BUSY, nil)
}

// SetSignalReport enables or disables signal reporting on the modem.
func (m *KissModem) SetSignalReport(enabled bool) error {
	val := byte(0x00)
	if enabled {
		val = 0x01
	}
	return m.SendHardwareCommand(HW_CMD_SET_SIGNAL_REPORT, []byte{val})
}

// GetSignalReport requests the signal report setting.
func (m *KissModem) GetSignalReport() error {
	return m.SendHardwareCommand(HW_CMD_GET_SIGNAL_REPORT, nil)
}

func (m *KissModem) onFrame(frame *KissFrame) {
	if m.signalReport {
		m.onFrameWithSignalReport(frame)
		return
	}
	m.dispatchFrame(frame)
}

func (m *KissModem) onFrameWithSignalReport(frame *KissFrame) {
	// HW_RESP_RX_META: enrich the pending data frame and dispatch it.
	if frame.Command == KISS_CMD_SETHARDWARE && len(frame.Data) >= 1 && frame.Data[0] == HW_RESP_RX_META {
		m.pendingMu.Lock()
		pending := m.pendingFrame
		if pending != nil {
			if m.pendingTimer != nil {
				m.pendingTimer.Stop()
				m.pendingTimer = nil
			}
			m.pendingFrame = nil
		}
		m.pendingMu.Unlock()

		if pending != nil && len(frame.Data) >= 3 {
			pending.SNR = int8(frame.Data[1])
			pending.RSSI = int8(frame.Data[2])
			m.dispatchFrame(pending)
		} else if pending != nil {
			m.dispatchFrame(pending)
		}

		// Still dispatch the HW frame to registered hw handlers.
		m.dispatchHwFrame(frame)
		return
	}

	// Data frame: queue it and wait for RX_META.
	if frame.Command == KISS_CMD_DATA {
		m.pendingMu.Lock()
		stale := m.pendingFrame
		if stale != nil {
			if m.pendingTimer != nil {
				m.pendingTimer.Stop()
				m.pendingTimer = nil
			}
			m.pendingFrame = nil
		}
		m.pendingFrame = frame
		m.pendingTimer = time.AfterFunc(rxMetaTimeout, func() {
			m.flushPending()
		})
		m.pendingMu.Unlock()

		// Flush the stale frame that never got its metadata.
		if stale != nil {
			m.dispatchFrame(stale)
		}
		return
	}

	// All other frames (non-data, non-RX_META) dispatch immediately.
	m.dispatchFrame(frame)
}

// flushPending dispatches the pending data frame without metadata (timeout).
func (m *KissModem) flushPending() {
	m.pendingMu.Lock()
	pending := m.pendingFrame
	if pending != nil {
		m.pendingFrame = nil
		m.pendingTimer = nil
	}
	m.pendingMu.Unlock()

	if pending != nil {
		m.dispatchFrame(pending)
	}
}

func (m *KissModem) dispatchFrame(frame *KissFrame) {
	m.frameMu.RLock()
	fh := m.frameH
	m.frameMu.RUnlock()
	if fh != nil {
		fh(frame)
	}

	switch frame.Command {
	case KISS_CMD_DATA:
		m.dataMu.RLock()
		dh := m.dataH
		m.dataMu.RUnlock()
		if dh != nil {
			dh(frame.Data, frame.SNR, frame.RSSI)
		}
	case KISS_CMD_SETHARDWARE:
		m.dispatchHwFrame(frame)
	}
}

func (m *KissModem) dispatchHwFrame(frame *KissFrame) {
	subCmd, data, err := DecodeHardwareFrame(frame)
	if err != nil {
		m.dispatchError(fmt.Errorf("kiss: decode hw frame: %w", err))
		return
	}
	m.hwMu.RLock()
	handlers := m.hwMap[subCmd]
	m.hwMu.RUnlock()
	for _, h := range handlers {
		h(subCmd, data)
	}
}

func (m *KissModem) onError(err error) {
	m.dispatchError(err)
}

func (m *KissModem) dispatchError(err error) {
	m.errMu.RLock()
	h := m.errH
	m.errMu.RUnlock()
	if h != nil {
		h(err)
	}
}
