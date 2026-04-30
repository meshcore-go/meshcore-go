package node

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

const DefaultAdvertInterval = 60 * time.Minute

var ErrTxQueueFull = errors.New("transmit queue full")

type PacketHandler func(pkt *meshcore.Packet)

// Transmit priority levels matching MeshCore C++. Lower = higher priority.
const (
	PriorityDirectRelay uint8 = 0
	PriorityFloodRelay  uint8 = 1
	PrioritySend        uint8 = 4
)

type Node struct {
	identityMu sync.RWMutex
	identity   meshcore.LocalIdentity
	radio      Radio
	router     router
	peers      *PeerTable
	secrets    *secretCache
	acks       *ackTracker
	channels   channelTable
	regions    *RegionMap
	scheduler  *txScheduler
	log        *slog.Logger

	airtimeFactor    float64
	dutyCycleWindow  time.Duration
	airtimeEstimator AirtimeEstimator
	maxTxQueue       int

	advertData     *meshcore.AdvertAppData
	advertInterval time.Duration

	cbMu         sync.RWMutex
	errH         func(error)
	allowForward func(*meshcore.Packet) bool
	allowPacket  func(*meshcore.Packet) bool

	handlerMu sync.RWMutex
	handlers  map[byte][]PacketHandler

	stopOnce sync.Once
	done     chan struct{}
}

type Option func(*Node)

func WithErrorHandler(h func(error)) Option {
	return func(n *Node) {
		n.errH = h
	}
}

func WithLogger(l *slog.Logger) Option {
	return func(n *Node) {
		n.log = l
	}
}

func WithAllowForwardHandler(f func(*meshcore.Packet) bool) Option {
	return func(n *Node) {
		n.allowForward = f
	}
}

func WithAllowPacketHandler(f func(*meshcore.Packet) bool) Option {
	return func(n *Node) {
		n.allowPacket = f
	}
}

func WithMaxPeers(max int) Option {
	return func(n *Node) {
		n.peers = NewPeerTable(max)
	}
}

func WithAdvertData(data meshcore.AdvertAppData) Option {
	return func(n *Node) {
		n.advertData = &data
	}
}

func WithAdvertInterval(d time.Duration) Option {
	return func(n *Node) {
		n.advertInterval = d
	}
}

// WithChannels pre-populates channels starting at index 0.
// Entries beyond the configured max channels are silently ignored.
func WithChannels(chs ...*meshcore.ChannelEntry) Option {
	return func(n *Node) {
		for i, ch := range chs {
			if i >= n.channels.maxChannels() {
				break
			}
			n.channels.channels[i] = ch
		}
	}
}

func WithRegions(regions ...*meshcore.Region) Option {
	return func(n *Node) {
		for _, r := range regions {
			n.regions.Add(r)
		}
	}
}

func WithAirtimeEstimator(est AirtimeEstimator) Option {
	return func(n *Node) {
		n.airtimeEstimator = est
	}
}

func WithAirtimeFactor(factor float64) Option {
	return func(n *Node) {
		n.airtimeFactor = factor
	}
}

func WithDutyCycleWindow(d time.Duration) Option {
	return func(n *Node) {
		n.dutyCycleWindow = d
	}
}

func WithMaxTxQueue(max int) Option {
	return func(n *Node) {
		n.maxTxQueue = max
	}
}

func WithMaxChannels(max int) Option {
	return func(n *Node) {
		n.channels = newChannelTable(max)
	}
}

func New(identity meshcore.LocalIdentity, radio Radio, opts ...Option) *Node {
	n := &Node{
		identity:        identity,
		radio:           radio,
		peers:           NewPeerTable(DefaultMaxPeers),
		secrets:         newSecretCache(identity),
		channels:        newChannelTable(DefaultMaxChannels),
		regions:         NewRegionMap(),
		airtimeFactor:   DefaultAirtimeFactor,
		dutyCycleWindow: DefaultDutyCycleWindow,
		maxTxQueue:      DefaultMaxTxQueue,
		advertInterval:  DefaultAdvertInterval,
		handlers:        make(map[byte][]PacketHandler),
		done:            make(chan struct{}),
	}
	n.router.node = n
	for _, opt := range opts {
		opt(n)
	}
	if n.log == nil {
		n.log = slog.Default()
	}

	sendFn := func(data []byte) error {
		return radio.SendData(data)
	}

	if n.airtimeEstimator != nil {
		budget := newAirtimeBudget(n.airtimeFactor, n.dutyCycleWindow, n.airtimeEstimator)
		n.scheduler = newTxScheduler(budget, n.maxTxQueue, sendFn, n.done)
		n.scheduler.setErrorHandler(func(err error) {
			n.dispatchError(err)
		})
		n.router.send = func(data []byte) error {
			estAirtime := n.airtimeEstimator(len(data))
			delay := FloodRetransmitDelay(estAirtime)
			if !n.scheduler.enqueue(data, PriorityFloodRelay, delay) {
				return ErrTxQueueFull
			}
			return nil
		}
		n.router.sendDirect = func(data []byte) error {
			if !n.scheduler.enqueue(data, PriorityDirectRelay, 0) {
				return ErrTxQueueFull
			}
			return nil
		}
	} else {
		n.router.send = sendFn
	}

	n.acks = newACKTracker(n.done)
	radio.SetDataHandler(n.onData)

	if n.advertData != nil {
		go n.selfAdvert()
	}

	return n
}

func (n *Node) Identity() meshcore.LocalIdentity {
	n.identityMu.RLock()
	id := n.identity
	n.identityMu.RUnlock()
	return id
}

func (n *Node) SetIdentity(id meshcore.LocalIdentity) {
	n.identityMu.Lock()
	n.identity = id
	n.secrets.reset(id)
	n.identityMu.Unlock()
}

func (n *Node) getIdentity() meshcore.LocalIdentity {
	n.identityMu.RLock()
	id := n.identity
	n.identityMu.RUnlock()
	return id
}

func (n *Node) Radio() Radio {
	return n.radio
}

func (n *Node) Peers() *PeerTable {
	return n.peers
}

func (n *Node) SharedSecret(peer meshcore.Identity) ([]byte, error) {
	return n.secrets.get(peer)
}

func (n *Node) ExpectACK(crc uint32, timeout time.Duration, onACK func(time.Duration), onTimeout func()) {
	n.acks.expect(crc, timeout, onACK, onTimeout)
}

func (n *Node) CancelACK(crc uint32) {
	n.acks.cancel(crc)
}

func (n *Node) SetChannel(idx int, ch *meshcore.ChannelEntry) bool {
	return n.channels.set(idx, ch)
}

func (n *Node) RemoveChannel(idx int) bool {
	return n.channels.remove(idx)
}

func (n *Node) Channel(idx int) *meshcore.ChannelEntry {
	return n.channels.get(idx)
}

func (n *Node) ChannelsByHash(hash byte) []*meshcore.ChannelEntry {
	return n.channels.findByHash(hash)
}

func (n *Node) Channels() []*meshcore.ChannelEntry {
	return n.channels.all()
}

func (n *Node) DecryptGroupText(pkt *meshcore.Packet) (*meshcore.GroupTextPayload, *meshcore.ChannelEntry, error) {
	return n.channels.decryptGroupText(pkt)
}

func (n *Node) DecryptGroupData(pkt *meshcore.Packet) ([]byte, *meshcore.ChannelEntry, error) {
	return n.channels.decryptGroupData(pkt)
}

func (n *Node) Regions() *RegionMap {
	return n.regions
}

func (n *Node) AddRegion(r *meshcore.Region) bool {
	return n.regions.Add(r)
}

func (n *Node) RemoveRegion(name string) bool {
	return n.regions.Remove(name)
}

func (n *Node) Region(name string) *meshcore.Region {
	return n.regions.Get(name)
}

func (n *Node) SetErrorHandler(h func(error)) {
	n.cbMu.Lock()
	n.errH = h
	n.cbMu.Unlock()
}

func (n *Node) SetAllowForwardHandler(f func(*meshcore.Packet) bool) {
	n.cbMu.Lock()
	n.allowForward = f
	n.cbMu.Unlock()
}

func (n *Node) SetAllowPacketHandler(f func(*meshcore.Packet) bool) {
	n.cbMu.Lock()
	n.allowPacket = f
	n.cbMu.Unlock()
}

func (n *Node) canAcceptPacket(pkt *meshcore.Packet) bool {
	n.cbMu.RLock()
	f := n.allowPacket
	n.cbMu.RUnlock()
	return f != nil && f(pkt)
}

func (n *Node) OnPacket(payloadType byte, h PacketHandler) {
	n.handlerMu.Lock()
	n.handlers[payloadType] = append(n.handlers[payloadType], h)
	n.handlerMu.Unlock()
}

func (n *Node) SendPacket(pkt *meshcore.Packet) error {
	n.router.dedup.MarkSeen(pkt)
	data, err := pkt.ToBytes()
	if err != nil {
		return err
	}
	if n.scheduler != nil {
		if !n.scheduler.enqueue(data, PrioritySend, 0) {
			return ErrTxQueueFull
		}
		return nil
	}
	return n.radio.SendData(data)
}

func (n *Node) TxQueueLen() int {
	if n.scheduler == nil {
		return 0
	}
	return n.scheduler.queueLen()
}

func (n *Node) Stop() {
	n.stopOnce.Do(func() {
		close(n.done)
	})
}

func (n *Node) Stopped() <-chan struct{} {
	return n.done
}
