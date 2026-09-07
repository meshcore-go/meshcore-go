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

// DefaultTxTimeout is the TX_DONE wait without WithTxAirtimeEstimator; the firmware may hold a packet for TXDELAY + 1.5x airtime(255) + 1.5x airtime(len).
const DefaultTxTimeout = 15 * time.Second

var (
	ErrTxTimeout = errors.New("kiss: tx done timeout")
	// Deprecated: HW_ERR_TX_BUSY is ambiguous and does not resolve SendData.
	ErrTxBusy       = errors.New("kiss: radio busy")
	ErrTxFailed     = errors.New("kiss: tx failed")
	ErrTxPending    = errors.New("kiss: previous transmission outcome pending")
	ErrModemClosed  = errors.New("kiss: modem closed")
	ErrDisconnected = errors.New("kiss: transport disconnected")
	// ErrPacketSize is returned by SendData for a payload outside 1..KISS_MAX_PACKET_SIZE bytes.
	ErrPacketSize = errors.New("kiss: packet must be 1..255 bytes")
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
		m.signalRequested = enabled
	}
}

// WithInboundBuffer sets the capacity of the inbound frame channel.
// Defaults to DefaultInboundBuffer (1024).
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
// not arrive within the given timeout, SendData returns ErrTxTimeout. If the
// hardware reports the transmit failed (a TX_DONE result byte other than 0x01),
// SendData returns ErrTxFailed. Pass 0 to disable flow control entirely.
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

// WithTxAirtimeEstimator sizes the TX_DONE wait from the firmware's budget
// (TXDELAY + 1.5x airtime(255) + 1.5x airtime(len) + 1 s) instead of the
// fixed WithTxFlowControl timeout. Pass hardware.LoRaAirtimeEstimator.
func WithTxAirtimeEstimator(estimator func(packetLen int) uint32) ModemOption {
	return func(m *KissModem) { m.txEstimator = estimator }
}

// WithHandlerWorkers runs non-DATA callbacks on n worker goroutines so slow
// hardware-response handlers do not stall the drain. DATA callbacks always run
// serially in receive order on the drain, as node handlers require.
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
	HwErrors             uint64 // HW_RESP_ERROR frames received
	TxOutcomeLost        uint64 // TX_DONE waits abandoned by a reconnect
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
	txEstimator   func(int) uint32
	sendMu        sync.Mutex
	txMu          sync.Mutex
	txPending     chan error

	inbound        chan *KissFrame
	flush          chan chan struct{}
	done           chan struct{}
	closed         atomic.Bool
	drainWg        sync.WaitGroup
	handlersActive atomic.Int32

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
	statHwError       atomic.Uint64
	statTxLost        atomic.Uint64

	errMu sync.RWMutex
	errH  func(error)

	frameMu sync.RWMutex
	frameH  FrameHandler

	hwMu  sync.RWMutex
	hwMap map[byte][]HwFrameHandler

	dataMu sync.RWMutex
	dataH  DataFrameHandler

	signalMu        sync.Mutex
	signalRequested bool
	pendingMu       sync.Mutex
	pendingFrame    *KissFrame
	pendingTimer    *time.Timer
	pendingSeq      uint64

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
		HwErrors:             m.statHwError.Load(),
		TxOutcomeLost:        m.statTxLost.Load(),
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

// Connect opens the transport, abandons any TX_DONE still awaited from the
// previous connection, and pushes the signal-report setting to the firmware
// (whose own default is enabled).
func (m *KissModem) Connect(ctx context.Context) error {
	if m.closed.Load() {
		return ErrModemClosed
	}
	if err := m.transport.Connect(ctx); err != nil {
		return err
	}
	if m.closed.Load() {
		_ = m.transport.Close()
		return ErrModemClosed
	}
	m.releaseTx()
	m.signalMu.Lock()
	defer m.signalMu.Unlock()
	val := byte(0)
	if m.signalRequested {
		val = 1
	}
	if err := m.SendHardwareCommand(HW_CMD_SET_SIGNAL_REPORT, []byte{val}); err != nil {
		_ = m.transport.Close()
		return fmt.Errorf("configure signal report: %w", err)
	}
	return nil
}

// releaseTx drops an outstanding TX_DONE wait; after a reconnect the firmware
// may have rebooted, so the old outcome is unknowable.
func (m *KissModem) releaseTx() {
	m.txMu.Lock()
	defer m.txMu.Unlock()
	if m.txPending != nil {
		m.txPending = nil
		m.statTxLost.Add(1)
	}
}

// Close stops dispatch and shuts down the transport.
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
	err := m.transport.Close()
	if m.handlersActive.Load() == 0 {
		m.drainWg.Wait()
		if m.handlerWorkers > 0 {
			m.handlerWg.Wait()
		}
	}
	return err
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

// SendData serializes transmissions. A timed-out or failed send keeps the
// TX_DONE slot until the firmware reports, so later sends return ErrTxPending;
// Connect clears it.
func (m *KissModem) SendData(data []byte) error {
	if len(data) == 0 || len(data) > KISS_MAX_PACKET_SIZE {
		return ErrPacketSize
	}
	if m.closed.Load() {
		return ErrModemClosed
	}
	m.outboundMu.RLock()
	handlers := m.outboundH
	m.outboundMu.RUnlock()
	for _, h := range handlers {
		h(data)
	}

	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	if m.closed.Load() {
		return ErrModemClosed
	}
	dead := m.transport.Dead()
	select {
	case <-dead:
		return ErrDisconnected
	default:
	}

	var result chan error
	if m.txFlowControl {
		m.txMu.Lock()
		if m.txPending != nil {
			m.txMu.Unlock()
			return ErrTxPending
		}
		result = make(chan error, 1)
		m.txPending = result
		m.txMu.Unlock()
	}
	if err := m.transport.Send(EncodeFrame(m.kissPort, KISS_CMD_DATA, data)); err != nil {
		// A failed write may still have delivered the frame to the firmware.
		return err
	}
	if m.closed.Load() {
		return ErrModemClosed
	}
	if !m.txFlowControl {
		return nil
	}

	timer := time.NewTimer(m.txWait(len(data)))
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		return ErrTxTimeout
	case <-dead:
		return ErrDisconnected
	case <-m.done:
		return ErrModemClosed
	}
}

// txWait is the firmware's worst-case TX budget when an estimator is set.
func (m *KissModem) txWait(packetLen int) time.Duration {
	if m.txEstimator == nil {
		return m.txTimeout
	}
	airtime := time.Duration(m.txEstimator(KISS_MAX_PACKET_SIZE)+m.txEstimator(packetLen)) * time.Millisecond
	return 500*time.Millisecond + airtime*3/2 + time.Second
}

func (m *KissModem) completeTx(data []byte) {
	m.txMu.Lock()
	defer m.txMu.Unlock()
	if m.txPending == nil {
		return
	}
	var err error
	if len(data) < 1 || data[0] != 0x01 {
		err = ErrTxFailed
	}
	m.txPending <- err
	m.txPending = nil
}

// SendKissCommand sends a standard KISS command (TXDELAY, PERSISTENCE,
// SLOTTIME, TXTAIL, FULLDUPLEX; delays in 10 ms units). The firmware silently
// drops frames over KISS_MAX_FRAME_SIZE unescaped bytes, so those fail here.
func (m *KissModem) SendKissCommand(cmd byte, data []byte) error {
	if m.closed.Load() {
		return ErrModemClosed
	}
	if len(data) > KISS_MAX_FRAME_SIZE-1 {
		return ErrFrameTooLarge
	}
	return m.transport.Send(EncodeFrame(m.kissPort, cmd, data))
}

// SendHardwareCommand sends a hardware sub-command with the given payload.
func (m *KissModem) SendHardwareCommand(subCmd byte, data []byte) error {
	return m.SendKissCommand(KISS_CMD_SETHARDWARE, append([]byte{subCmd}, data...))
}

// SetRadio configures the radio (CR 5..8). The firmware replies HW_RESP_OK, not
// HwResp(HW_CMD_SET_RADIO), and does not check that the radio accepted the values.
func (m *KissModem) SetRadio(config *RadioConfig) error {
	return m.SendHardwareCommand(HW_CMD_SET_RADIO, config.ToBytes())
}

// SetTxPower sets TX power in dBm. The firmware replies HW_RESP_OK, not HwResp(HW_CMD_SET_TX_POWER).
func (m *KissModem) SetTxPower(power uint8) error {
	return m.SendHardwareCommand(HW_CMD_SET_TX_POWER, []byte{power})
}

// GetRadio requests the radio config. The reply is the firmware's cached SetRadio values, all zero until the host has set them.
func (m *KissModem) GetRadio() error {
	return m.SendHardwareCommand(HW_CMD_GET_RADIO, nil)
}

// GetTxPower requests TX power. The reply is the cached SetTxPower value, zero until set.
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

// GetMCUTemp requests the MCU temperature. The reply carries an int16 of
// tenths of a degree Celsius; a board that cannot measure it answers
// HW_RESP_ERROR / HW_ERR_NO_CALLBACK instead.
func (m *KissModem) GetMCUTemp() error {
	return m.SendHardwareCommand(HW_CMD_GET_MCU_TEMP, nil)
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

// Reboot asks the firmware to reboot. It replies HW_RESP_OK, then the connection drops.
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

// SetSignalReport requests a mode change; firmware's 0x9A reply updates local metadata pairing.
func (m *KissModem) SetSignalReport(enabled bool) error {
	val := byte(0x00)
	if enabled {
		val = 0x01
	}
	m.signalMu.Lock()
	defer m.signalMu.Unlock()
	if err := m.SendHardwareCommand(HW_CMD_SET_SIGNAL_REPORT, []byte{val}); err != nil {
		return err
	}
	m.signalRequested = enabled
	return nil
}

func (m *KissModem) setSignalMode(enabled bool) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	m.signalReport = enabled
	if !enabled && m.pendingFrame != nil {
		if m.pendingTimer != nil {
			m.pendingTimer.Stop()
			m.pendingTimer = nil
		}
		m.enqueueFrame(m.pendingFrame)
		m.pendingFrame = nil
	}
}

// GetSignalReport requests the signal report setting.
func (m *KissModem) GetSignalReport() error {
	return m.SendHardwareCommand(HW_CMD_GET_SIGNAL_REPORT, nil)
}

func (m *KissModem) onFrame(frame *KissFrame) {
	if m.closed.Load() {
		return
	}
	if frame.Command == KISS_CMD_SETHARDWARE && len(frame.Data) > 0 {
		switch frame.Data[0] {
		case HW_RESP_TX_DONE:
			m.completeTx(frame.Data[1:])
		case HW_RESP_ERROR:
			m.statHwError.Add(1)
		case HwResp(HW_CMD_GET_SIGNAL_REPORT):
			if len(frame.Data) == 2 {
				m.setSignalMode(frame.Data[1] != 0)
			}
		}
	}
	m.onFrameWithSignalReport(frame)
}

func (m *KissModem) onFrameWithSignalReport(frame *KissFrame) {
	m.pendingMu.Lock()
	defer m.pendingMu.Unlock()
	if m.closed.Load() {
		return
	}
	if !m.signalReport {
		m.enqueueFrame(frame)
		return
	}
	// HW_RESP_RX_META: enrich the pending data frame and dispatch it.
	if frame.Command == KISS_CMD_SETHARDWARE && len(frame.Data) >= 1 && frame.Data[0] == HW_RESP_RX_META {
		pending := m.pendingFrame
		if pending != nil {
			if m.pendingTimer != nil {
				m.pendingTimer.Stop()
				m.pendingTimer = nil
			}
			m.pendingFrame = nil
		}

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

		m.enqueueFrame(frame)
		return
	}

	// Data frame: queue it and wait for RX_META.
	if frame.Command == KISS_CMD_DATA {
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
	defer m.pendingMu.Unlock()
	if m.pendingSeq != seq || m.pendingFrame == nil {
		return
	}
	pending := m.pendingFrame
	m.pendingFrame = nil
	m.pendingTimer = nil
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

func (m *KissModem) dispatchFrame(frame *KissFrame) {
	if m.handlerWorkers > 0 && frame.Command != KISS_CMD_DATA {
		select {
		case m.handlerJobs <- frame:
		case <-m.done:
		}
		return
	}
	m.invokeHandlers(frame)
}

func (m *KissModem) invokeHandlers(frame *KissFrame) {
	m.handlersActive.Add(1)
	defer m.handlersActive.Add(-1)
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

	if subCmd == HW_RESP_ERROR {
		m.handleHwError(data)
	}

	m.hwMu.RLock()
	handlers := m.hwMap[subCmd]
	m.hwMu.RUnlock()
	for _, h := range handlers {
		h(subCmd, data)
	}
}

// HW_ERR_TX_BUSY can mean output backpressure, not rejection of the current TX.
func (m *KissModem) handleHwError(data []byte) {
	code := byte(0)
	if len(data) > 0 {
		code = data[0]
	}
	m.dispatchError(fmt.Errorf("kiss: hardware error: %w", HwErrorFor(code)))
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
