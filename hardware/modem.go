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

type DataFrameHandler = func(data []byte, snr float32, rssi int8, hasSignalInfo bool)

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

// WithHandlerWorkers enables a bounded worker pool for user handler
// invocation. When n > 0, dispatched frames are forwarded to a job channel
// of capacity n and consumed by n worker goroutines, isolating slow user
// handlers from the inbound drain loop. When 0 (default), handlers run
// inline on the drain goroutine.
func WithHandlerWorkers(n int) ModemOption {
	return func(m *KissModem) {
		if n < 0 {
			n = 0
		}
		m.handlerWorkers = n
	}
}

// WithHandlerWatchdog enables per-dispatch latency tracking. When a single
// dispatchFrame call exceeds the threshold, the modem emits a warning and
// increments ModemStats.HandlerSlow. Zero (default) disables the watchdog.
func WithHandlerWatchdog(threshold time.Duration) ModemOption {
	return func(m *KissModem) {
		if threshold < 0 {
			threshold = 0
		}
		m.handlerWatchdog = threshold
	}
}

// ModemStats holds runtime counters reported by Stats.
type ModemStats struct {
	InboundDroppedOldest uint64
	InboundDroppedNew    uint64
	RxMetaTimeouts       uint64
	RxMetaMisattributed  uint64
	HandlerSlow          uint64
	HwDecodeErrors       uint64
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
	closed  atomic.Bool
	drainWg sync.WaitGroup

	// handlerWorkers > 0 enables a bounded worker pool for user handler
	// invocation. When zero, handlers run inline on the drain goroutine.
	handlerWorkers  int
	handlerJobs     chan *KissFrame
	handlerWg       sync.WaitGroup
	handlerWatchdog time.Duration

	// stat counters (atomic)
	statDropOldest    atomic.Uint64
	statDropNew       atomic.Uint64
	statMetaTimeout   atomic.Uint64
	statMetaMisattrib atomic.Uint64
	statSlowHandler   atomic.Uint64
	statHwDecodeErr   atomic.Uint64

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
	pendingSeq   uint64

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
	if m.handlerWorkers > 0 {
		m.handlerJobs = make(chan *KissFrame, m.handlerWorkers)
		for i := 0; i < m.handlerWorkers; i++ {
			m.handlerWg.Add(1)
			go m.handlerWorker()
		}
	}
	m.drainWg.Add(1)
	go m.drainInbound()
	t.SetFrameHandler(m.onFrame)
	t.SetErrorHandler(m.onError)
	return m
}

// Stats returns a snapshot of modem runtime counters.
func (m *KissModem) Stats() ModemStats {
	return ModemStats{
		InboundDroppedOldest: m.statDropOldest.Load(),
		InboundDroppedNew:    m.statDropNew.Load(),
		RxMetaTimeouts:       m.statMetaTimeout.Load(),
		RxMetaMisattributed:  m.statMetaMisattrib.Load(),
		HandlerSlow:          m.statSlowHandler.Load(),
		HwDecodeErrors:       m.statHwDecodeErr.Load(),
	}
}

func (m *KissModem) drainInbound() {
	defer m.drainWg.Done()
	for {
		select {
		case <-m.done:
			return
		case frame := <-m.inbound:
			m.dispatchFrame(frame)
		case done := <-m.flush:
			// Drain any remaining buffered frames before signalling.
		drainLoop:
			for {
				select {
				case frame := <-m.inbound:
					m.dispatchFrame(frame)
				default:
					break drainLoop
				}
			}
			close(done)
		}
	}
}

func (m *KissModem) handlerWorker() {
	defer m.handlerWg.Done()
	for {
		select {
		case <-m.done:
			return
		case frame, ok := <-m.handlerJobs:
			if !ok {
				return
			}
			m.invokeHandlers(frame)
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
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	m.pendingMu.Lock()
	if m.pendingTimer != nil {
		m.pendingTimer.Stop()
		m.pendingTimer = nil
	}
	m.pendingFrame = nil
	m.pendingMu.Unlock()
	close(m.done)
	m.drainWg.Wait()
	if m.handlerWorkers > 0 {
		m.handlerWg.Wait()
	}
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
			pending.SNR = snrDBFromWire(int8(frame.Data[1]))
			pending.RSSI = int8(frame.Data[2])
			pending.HasSignalInfo = true
			m.enqueueFrame(pending)
		} else if pending != nil {
			// Meta frame with truncated payload; dispatch without meta.
			m.enqueueFrame(pending)
		} else {
			// Meta arrived with no pending data frame — almost always means
			// a prior data frame was already flushed (timeout or replaced).
			// Drop the orphaned meta to prevent it being misattributed to
			// the next data frame.
			m.statMetaMisattrib.Add(1)
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
			m.statMetaMisattrib.Add(1)
		}
		m.pendingSeq++
		seq := m.pendingSeq
		m.pendingFrame = frame
		m.pendingTimer = time.AfterFunc(rxMetaTimeout, func() {
			m.flushPending(seq)
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
// seq guards against the timer firing after the pending slot has already
// been replaced or consumed by a later data frame / meta arrival.
func (m *KissModem) flushPending(seq uint64) {
	m.pendingMu.Lock()
	if m.pendingSeq != seq || m.pendingFrame == nil {
		m.pendingMu.Unlock()
		return
	}
	pending := m.pendingFrame
	m.pendingFrame = nil
	m.pendingTimer = nil
	m.pendingMu.Unlock()

	m.statMetaTimeout.Add(1)
	m.enqueueFrame(pending)
}

func (m *KissModem) enqueueFrame(frame *KissFrame) {
	if m.closed.Load() {
		return
	}
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
			m.statDropOldest.Add(1)
			m.log.Warn("inbound buffer full, dropped oldest frame", "dropped_total", m.statDropOldest.Load())
		case <-m.done:
		default:
			m.statDropNew.Add(1)
			m.log.Warn("inbound buffer full, dropped new frame", "dropped_total", m.statDropNew.Load())
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
	if m.handlerWorkers > 0 {
		select {
		case m.handlerJobs <- frame:
		case <-m.done:
		}
		return
	}
	m.invokeHandlers(frame)
}

func (m *KissModem) invokeHandlers(frame *KissFrame) {
	if m.handlerWatchdog > 0 {
		start := time.Now()
		defer func() {
			if elapsed := time.Since(start); elapsed > m.handlerWatchdog {
				m.statSlowHandler.Add(1)
				m.log.Warn("kiss handler exceeded watchdog",
					"elapsed", elapsed,
					"threshold", m.handlerWatchdog,
					"command", frame.Command)
			}
		}()
	}

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
		m.statHwDecodeErr.Add(1)
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
