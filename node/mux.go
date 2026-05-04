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

	txMu  sync.Mutex
	queue *txQueue
	done  chan struct{}
}

type MuxOption func(*RadioMux)

func WithMuxLogger(l *slog.Logger) MuxOption {
	return func(m *RadioMux) {
		m.log = l
	}
}

func WithMuxErrorHandler(h func(error)) MuxOption {
	return func(m *RadioMux) {
		m.errH = h
	}
}

func WithMuxMaxTxQueue(max int) MuxOption {
	return func(m *RadioMux) {
		m.queue = newTxQueue(max)
	}
}

func NewRadioMux(modem Modem, opts ...MuxOption) *RadioMux {
	m := &RadioMux{
		modem: modem,
		queue: newTxQueue(DefaultMaxTxQueue),
		done:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.log == nil {
		m.log = slog.Default()
	}
	modem.SetDataHandler(m.onData)
	go m.txLoop()
	return m
}

func (m *RadioMux) enqueue(data []byte, priority uint8, delay time.Duration) bool {
	m.txMu.Lock()
	ok := m.queue.add(data, priority, time.Now().Add(delay))
	m.txMu.Unlock()
	return ok
}

func (m *RadioMux) txQueueLen() int {
	m.txMu.Lock()
	defer m.txMu.Unlock()
	return m.queue.len()
}

func (m *RadioMux) txLoop() {
	ticker := time.NewTicker(queuedRadioTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.drainQueue()
		}
	}
}

func (m *RadioMux) drainQueue() {
	for {
		m.txMu.Lock()
		entry := m.queue.pop(time.Now())
		m.txMu.Unlock()
		if entry == nil {
			return
		}
		_ = m.modem.SendData(entry.data)
	}
}

func (m *RadioMux) Stop() {
	close(m.done)
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
