package client

import (
	"context"
	"sync"

	"github.com/meshcore-go/meshcore-go/companion"
)

// CompanionModem adapts a companion Client to the node.Modem interface,
// allowing a RadioMux (and therefore Nodes) to send and receive raw mesh
// packets through a device running MeshCore firmware.
//
// Incoming packets arrive via two push notifications:
//   - PushLogRxData (0x88): fired for every received packet (group text,
//     adverts, etc.) — this is the primary data source.
//   - PushRawData (0x84): fired only for PAYLOAD_TYPE_RAW_CUSTOM packets
//     that are direct-routed.
//
// Outgoing packets are transmitted with SendRawData (empty routing path —
// the full packet bytes are supplied by the node layer).
type CompanionModem struct {
	client *Client
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	dataH func(data []byte, snr int8, rssi int8, hasSignalInfo bool)

	outboundMu sync.RWMutex
	outboundH  []func([]byte)
}

// NewCompanionModem creates a CompanionModem backed by the given Client.
// The supplied context governs the lifetime of outbound send operations;
// cancel it (or call Close) to unblock any pending sends.
func NewCompanionModem(ctx context.Context, c *Client) *CompanionModem {
	ctx, cancel := context.WithCancel(ctx)
	m := &CompanionModem{
		client: c,
		ctx:    ctx,
		cancel: cancel,
	}
	c.OnPush(companion.PushRawData, m.onRawData)
	c.OnPush(companion.PushLogRxData, m.onLogRxData)
	return m
}

func (m *CompanionModem) AddOutboundHandler(h func([]byte)) {
	m.outboundMu.Lock()
	m.outboundH = append(m.outboundH, h)
	m.outboundMu.Unlock()
}

// SendData transmits raw packet bytes through the companion device.
func (m *CompanionModem) SendData(data []byte) error {
	m.outboundMu.RLock()
	handlers := m.outboundH
	m.outboundMu.RUnlock()
	for _, h := range handlers {
		h(data)
	}
	return m.client.SendRawData(m.ctx, nil, data)
}

// SetDataHandler registers the callback invoked for each incoming raw
// mesh packet. The handler receives the packet bytes, SNR, and RSSI
// exactly as reported by the firmware.
func (m *CompanionModem) SetDataHandler(h func(data []byte, snr int8, rssi int8, hasSignalInfo bool)) {
	m.mu.Lock()
	m.dataH = h
	m.mu.Unlock()
}

// Close cancels the internal context, unblocking any pending sends.
// It does not close the underlying Client.
func (m *CompanionModem) Close() {
	m.cancel()
}

func (m *CompanionModem) onRawData(resp companion.Response) {
	raw, ok := resp.Data.(companion.PushRawDataResponse)
	if !ok {
		return
	}
	m.mu.Lock()
	h := m.dataH
	m.mu.Unlock()
	if h != nil {
		h(raw.Payload, raw.LastSNR, raw.LastRSSI, true)
	}
}

func (m *CompanionModem) onLogRxData(resp companion.Response) {
	rx, ok := resp.Data.(companion.PushLogRxDataResponse)
	if !ok {
		return
	}
	m.mu.Lock()
	h := m.dataH
	m.mu.Unlock()
	if h != nil {
		h(rx.Raw, rx.LastSNR, rx.LastRSSI, true)
	}
}
