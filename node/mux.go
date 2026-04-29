package node

import (
	"sync"

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
	Radio
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
	v.mu.RLock()
	handlers := v.outboundH
	v.mu.RUnlock()
	for _, h := range handlers {
		h(data)
	}
	return v.mux.modem.SendData(data)
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
// via its PacketFilter.
type RadioMux struct {
	modem  Modem
	mu     sync.RWMutex
	radios []*virtualRadio
}

func NewRadioMux(modem Modem) *RadioMux {
	m := &RadioMux{modem: modem}
	modem.SetDataHandler(m.onData)
	return m
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

// compile-time check: *virtualRadio implements Radio
var _ MuxRadio = (*virtualRadio)(nil)
