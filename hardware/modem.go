package hardware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const rxMetaTimeout = 1 * time.Second

// DefaultInboundBuffer is the default capacity of the inbound frame channel.
const DefaultInboundBuffer = 1024

const DefaultTxTimeout = 5 * time.Second

var (
	ErrTxTimeout = errors.New("kiss: tx done timeout")
	ErrTxBusy    = errors.New("kiss: radio busy")
	ErrTxFailed  = errors.New("kiss: tx failed")
)

// Transport is the interface that hardware transports must implement.
type Transport interface {
	Connect(ctx context.Context) error
	Close() error
	Send(data []byte) error
	SetFrameHandler(func(*KissFrame))
	SetErrorHandler(func(error))
	Dead() <-chan struct{}
}

// FrameHandler is called when a KISS frame is received.
type FrameHandler = func(*KissFrame)

// HwFrameHandler is called when a hardware sub-command frame is received.
type HwFrameHandler = func(subCmd byte, data []byte)

type DataFrameHandler = func(data []byte, snr int8, rssi int8, hasSignalInfo bool)

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

// WithInboundBuffer sets the capacity of the inbound frame channel.
// Defaults to DefaultInboundBuffer (64).
func WithInboundBuffer(size int) ModemOption {
	return func(m *KissModem) {
		m.inboundSize = size
	}
}

// WithLogger sets the structured logger for the modem.
// Defaults to slog.Default() if not provided.
func WithLogger(l *slog.Logger) ModemOption {
	return func(m *KissModem) {
		m.log = l
	}
}

// WithTxFlowControl configures TX flow control. Enabled by default with a
// 5-second timeout. After each SendData call, the modem waits for
// HW_RESP_TX_DONE from the hardware before returning. If the response does
// not arrive within the given timeout, SendData returns ErrTxTimeout.
// Pass 0 to disable flow control entirely.
func WithTxFlowControl(timeout time.Duration) ModemOption {
	return func(m *KissModem) {
		if timeout == 0 {
			m.txFlowControl = false
			m.txTimeout = 0
		} else {
			m.txFlowControl = true
			m.txTimeout = timeout
		}
	}
}

// WithTxAirtimeEstimator is deprecated and has no effect. TX timeout is now a
// fixed value (DefaultTxTimeout) that acts as a dead-radio detector. The
// firmware handles CSMA timing internally and always sends TX_DONE.
// Airtime estimation should only be used for queue scheduling, not TX timeout.
func WithTxAirtimeEstimator(estimator func(packetLen int) uint32) ModemOption {
	return func(m *KissModem) {
		// no-op: TX timeout is fixed, airtime estimation is for queue scheduling only
	}
}

// KissModem represents a KISS TNC modem connection.
type KissModem struct {
	transport    Transport
	kissPort     int
	signalReport bool
	inboundSize  int
	log          *slog.Logger

	txFlowControl bool
	txTimeout     time.Duration
	txResult      chan error
	txPending     atomic.Bool

	inbound chan *KissFrame
	flush   chan chan struct{}
	done    chan struct{}
	drainWg sync.WaitGroup

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

	outboundMu sync.RWMutex
	outboundH  []func([]byte)
}

// NewKissModem creates a new KissModem using the given transport.
func NewKissModem(t Transport, opts ...ModemOption) *KissModem {
	m := &KissModem{
		transport:     t,
		inboundSize:   DefaultInboundBuffer,
		txFlowControl: true,
		txTimeout:     DefaultTxTimeout,
		hwMap:         make(map[byte][]HwFrameHandler),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	m.inbound = make(chan *KissFrame, m.inboundSize)
	m.flush = make(chan chan struct{})
	m.done = make(chan struct{})
	if m.txFlowControl {
		m.txResult = make(chan error, 1)
	}
	m.drainWg.Add(1)
	go m.drainInbound()
	t.SetFrameHandler(m.onFrame)
	t.SetErrorHandler(m.onError)
	return m
}

func (m *KissModem) drainInbound() {
	defer m.drainWg.Done()
	for {
		select {
		case frame, ok := <-m.inbound:
			if !ok {
				return
			}
			m.dispatchFrame(frame)
		case done := <-m.flush:
			// Drain any remaining buffered frames before signalling.
			for {
				select {
				case frame, ok := <-m.inbound:
					if !ok {
						close(done)
						return
					}
					m.dispatchFrame(frame)
				default:
					close(done)
					goto cont
				}
			}
		cont:
		}
	}
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
	close(m.done)
	close(m.inbound)
	m.drainWg.Wait()
	return m.transport.Close()
}

// Dead returns a channel that is closed when the underlying transport's read
// loop has exited. This indicates the modem is no longer receiving packets.
func (m *KissModem) Dead() <-chan struct{} {
	return m.transport.Dead()
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

func (m *KissModem) AddOutboundHandler(h func([]byte)) {
	m.outboundMu.Lock()
	m.outboundH = append(m.outboundH, h)
	m.outboundMu.Unlock()
}

// SendData sends a data frame. If TX flow control is enabled, blocks until
// HW_RESP_TX_DONE or HW_RESP_ERROR is received, or the TX timeout expires.
func (m *KissModem) SendData(data []byte) error {
	m.outboundMu.RLock()
	handlers := m.outboundH
	m.outboundMu.RUnlock()
	for _, h := range handlers {
		h(data)
	}

	if m.txFlowControl {
		select {
		case <-m.txResult:
		default:
		}
	}

	frame := EncodeFrame(m.kissPort, KISS_CMD_DATA, data)
	m.txPending.Store(true)
	if err := m.transport.Send(frame); err != nil {
		m.txPending.Store(false)
		return err
	}

	if !m.txFlowControl {
		return nil
	}

	select {
	case err := <-m.txResult:
		m.txPending.Store(false)
		return err
	case <-time.After(m.txTimeout):
		m.txPending.Store(false)
		return ErrTxTimeout
	case <-m.done:
		m.txPending.Store(false)
		return nil
	}
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
	m.enqueueFrame(frame)
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
			pending.HasSignalInfo = true
			m.enqueueFrame(pending)
		} else if pending != nil {
			m.enqueueFrame(pending)
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
			m.enqueueFrame(stale)
		}
		return
	}

	// All other frames (non-data, non-RX_META) dispatch immediately.
	m.enqueueFrame(frame)
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
		m.enqueueFrame(pending)
	}
}

func (m *KissModem) enqueueFrame(frame *KissFrame) {
	select {
	case m.inbound <- frame:
	case <-m.done:
		return
	default:
		select {
		case <-m.inbound:
		default:
		}
		select {
		case m.inbound <- frame:
			m.log.Warn("inbound buffer full, dropped oldest frame")
		case <-m.done:
		default:
			m.log.Warn("inbound buffer full, dropped new frame")
		}
	}
}

// Flush blocks until all currently enqueued frames have been dispatched.
// Intended for testing.
func (m *KissModem) Flush() {
	done := make(chan struct{})
	select {
	case m.flush <- done:
		<-done
	case <-m.done:
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
			dh(frame.Data, frame.SNR, frame.RSSI, frame.HasSignalInfo)
		}
	case KISS_CMD_SETHARDWARE:
		m.dispatchHwFrame(frame)
	}
}

func (m *KissModem) dispatchHwFrame(frame *KissFrame) {
	subCmd, data, err := DecodeHardwareFrame(frame)
	if err != nil {
		m.log.Debug("failed to decode hardware frame", "error", err)
		m.dispatchError(fmt.Errorf("kiss: decode hw frame: %w", err))
		return
	}

	if subCmd == HW_RESP_TX_DONE && m.txResult != nil && m.txPending.Load() {
		select {
		case m.txResult <- nil:
		default:
		}
	}

	if subCmd == HW_RESP_ERROR && m.txResult != nil && m.txPending.Load() && len(data) >= 1 {
		if data[0] == HW_ERR_TX_BUSY {
			select {
			case m.txResult <- ErrTxBusy:
			default:
			}
		} else {
			// Non-TX error (e.g. HW_ERR_ENCRYPT_FAILED, HW_ERR_UNKNOWN_CMD) —
			// ignore for TX flow control; the 5s timeout handles real TX failures.
			m.log.Debug("ignoring non-TX hardware error during send", "code", data[0])
		}
	}

	m.hwMu.RLock()
	handlers := m.hwMap[subCmd]
	m.hwMu.RUnlock()
	for _, h := range handlers {
		h(subCmd, data)
	}
}

func (m *KissModem) onError(err error) {
	m.log.Debug("transport error", "error", err)
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
