package node

import (
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type Modem interface {
	SendData(data []byte) error
	SetDataHandler(func(data []byte, snr int8, rssi int8))
	AddOutboundHandler(h func([]byte))
}

// PacketFilter decides whether a virtual radio wants to handle a packet.
type PacketFilter func(pkt *meshcore.Packet) bool

// MuxRadio extends Radio with packet filtering for use with RadioMux.
type MuxRadio interface {
	TxRadio
	SetPacketFilter(f PacketFilter)
}

type virtualRadio struct {
	mux       *RadioMux
	filter    PacketFilter
	dataH     func(*meshcore.Packet)
	rawDataH  func([]byte, int8, int8)
	outboundH []func([]byte)
	mu        sync.RWMutex
}

func (v *virtualRadio) SendData(data []byte) error {
	if !v.Enqueue(data, PrioritySend, 0) {
		return ErrTxQueueFull
	}
	return nil
}

func (v *virtualRadio) Enqueue(data []byte, priority uint8, delay time.Duration) bool {
	v.mu.RLock()
	handlers := v.outboundH
	v.mu.RUnlock()
	for _, h := range handlers {
		h(data)
	}
	return v.mux.enqueue(data, priority, delay)
}

func (v *virtualRadio) TxQueueLen() int {
	return v.mux.txQueueLen()
}

func (v *virtualRadio) AddOutboundHandler(h func([]byte)) {
	v.mu.Lock()
	v.outboundH = append(v.outboundH, h)
	v.mu.Unlock()
}

func (v *virtualRadio) SetDataHandler(h func(*meshcore.Packet)) {
	v.mu.Lock()
	v.dataH = h
	v.mu.Unlock()
}

func (v *virtualRadio) SetRawDataHandler(h func([]byte, int8, int8)) {
	v.mu.Lock()
	v.rawDataH = h
	v.mu.Unlock()
}

func (v *virtualRadio) SetPacketFilter(f PacketFilter) {
	v.mu.Lock()
	v.filter = f
	v.mu.Unlock()
}

func (v *virtualRadio) Close() error {
	v.mux.remove(v)
	return nil
}

func (v *virtualRadio) wants(pkt *meshcore.Packet) bool {
	v.mu.RLock()
	f := v.filter
	v.mu.RUnlock()
	if f == nil {
		return true
	}
	return f(pkt)
}

func (v *virtualRadio) deliver(pkt *meshcore.Packet, raw []byte, snr int8, rssi int8) {
	v.mu.RLock()
	h := v.dataH
	rh := v.rawDataH
	v.mu.RUnlock()
	if h != nil {
		h(pkt)
	}
	if rh != nil {
		rh(raw, snr, rssi)
	}
}

// RadioMux shares a single Modem across multiple virtual radios.
// Incoming packets are delivered to each virtual radio that accepts them
// via its PacketFilter. Outgoing packets are serialized through a shared
// transmit queue to prevent concurrent writes to the modem.
type RadioMux struct {
	modem  Modem
	log    *slog.Logger
	errH   func(error)
	mu     sync.RWMutex
	radios []*virtualRadio

	tx       *txEngine
	done     chan struct{}
	stopOnce sync.Once
}

type muxConfig struct {
	log              *slog.Logger
	errH             func(error)
	txOpts           []txEngineOption
	airtimeEstimator AirtimeEstimator
	airtimeFactor    float64
	dutyCycleWindow  time.Duration
}

type MuxOption func(*muxConfig)

func WithMuxLogger(l *slog.Logger) MuxOption {
	return func(c *muxConfig) {
		c.log = l
		c.txOpts = append(c.txOpts, withTxLogger(l))
	}
}

func WithMuxErrorHandler(h func(error)) MuxOption {
	return func(c *muxConfig) {
		c.errH = h
		c.txOpts = append(c.txOpts, withTxErrorHandler(h))
	}
}

func WithMuxMaxTxQueue(max int) MuxOption {
	return func(c *muxConfig) {
		c.txOpts = append(c.txOpts, withTxMaxQueue(max))
	}
}

func WithMuxAirtimeEstimator(est AirtimeEstimator) MuxOption {
	return func(c *muxConfig) {
		c.airtimeEstimator = est
	}
}

func WithMuxAirtimeFactor(f float64) MuxOption {
	return func(c *muxConfig) {
		c.airtimeFactor = f
	}
}

func WithMuxDutyCycleWindow(d time.Duration) MuxOption {
	return func(c *muxConfig) {
		c.dutyCycleWindow = d
	}
}

func WithMuxRetryable(fn func(error) bool) MuxOption {
	return func(c *muxConfig) {
		c.txOpts = append(c.txOpts, withTxRetryable(fn))
	}
}

func NewRadioMux(modem Modem, opts ...MuxOption) *RadioMux {
	cfg := muxConfig{
		airtimeFactor:   DefaultAirtimeFactor,
		dutyCycleWindow: DefaultDutyCycleWindow,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.airtimeEstimator != nil {
		cfg.txOpts = append(cfg.txOpts, withTxAirtimeBudget(newAirtimeBudget(cfg.airtimeFactor, cfg.dutyCycleWindow, cfg.airtimeEstimator)))
	}
	if cfg.log == nil {
		cfg.log = slog.Default()
	}
	m := &RadioMux{
		modem: modem,
		log:   cfg.log,
		errH:  cfg.errH,
		done:  make(chan struct{}),
	}
	m.tx = newTxEngine(modem.SendData, m.done, cfg.txOpts...)
	modem.SetDataHandler(m.onData)
	return m
}

func (m *RadioMux) enqueue(data []byte, priority uint8, delay time.Duration) bool {
	return m.tx.enqueue(data, priority, delay)
}

func (m *RadioMux) txQueueLen() int {
	return m.tx.queueLen()
}

func (m *RadioMux) Stop() {
	m.stopOnce.Do(func() {
		close(m.done)
	})
}

func (m *RadioMux) NewRadio() MuxRadio {
	v := &virtualRadio{
		mux: m,
	}
	m.mu.Lock()
	m.radios = append(m.radios, v)
	m.mu.Unlock()
	return v
}

func (m *RadioMux) remove(v *virtualRadio) {
	m.mu.Lock()
	for i, r := range m.radios {
		if r == v {
			m.radios = append(m.radios[:i], m.radios[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
}

func (m *RadioMux) onData(data []byte, snr int8, rssi int8) {
	pkt, err := meshcore.PacketFromBytes(data)
	if err != nil {
		m.log.Debug("failed to parse packet", "error", err, "data_len", len(data), "hex", hex.EncodeToString(data))
		if m.errH != nil {
			m.errH(err)
		}
		return
	}
	pkt.SNR = snr
	pkt.RSSI = rssi

	m.mu.RLock()
	radios := make([]*virtualRadio, len(m.radios))
	copy(radios, m.radios)
	m.mu.RUnlock()

	for _, v := range radios {
		if v.wants(pkt) {
			clone, err := clonePacket(pkt)
			if err != nil {
				continue
			}
			rawCopy := make([]byte, len(data))
			copy(rawCopy, data)
			v.deliver(clone, rawCopy, snr, rssi)
		}
	}
}

func clonePacket(pkt *meshcore.Packet) (*meshcore.Packet, error) {
	data, err := pkt.ToBytes()
	if err != nil {
		return nil, err
	}
	clone, err := meshcore.PacketFromBytes(data)
	if err != nil {
		return nil, err
	}
	clone.SNR = pkt.SNR
	clone.RSSI = pkt.RSSI
	return clone, nil
}

var _ MuxRadio = (*virtualRadio)(nil)
