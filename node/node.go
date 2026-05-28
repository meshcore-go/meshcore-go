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
	txRadio    TxRadio
	router     router
	peers      *PeerTable
	secrets    *secretCache
	acks       *ackTracker
	retries    *retryTracker
	channels   channelTable
	regions    *RegionMap
	log        *slog.Logger
	txCfg      nodeTxConfig

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

type nodeTxConfig struct {
	airtimeFactor    float64
	dutyCycleWindow  time.Duration
	airtimeEstimator AirtimeEstimator
	maxTxQueue       int
}

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
		n.txCfg.airtimeEstimator = est
	}
}

func WithAirtimeFactor(factor float64) Option {
	return func(n *Node) {
		n.txCfg.airtimeFactor = factor
	}
}

func WithDutyCycleWindow(d time.Duration) Option {
	return func(n *Node) {
		n.txCfg.dutyCycleWindow = d
	}
}

func WithMaxTxQueue(max int) Option {
	return func(n *Node) {
		n.txCfg.maxTxQueue = max
	}
}

func WithMaxChannels(max int) Option {
	return func(n *Node) {
		n.channels = newChannelTable(max)
	}
}

func New(identity meshcore.LocalIdentity, radio Radio, opts ...Option) *Node {
	n := &Node{
		identity: identity,
		radio:    radio,
		peers:    NewPeerTable(DefaultMaxPeers),
		secrets:  newSecretCache(identity),
		channels: newChannelTable(DefaultMaxChannels),
		regions:  NewRegionMap(),
		txCfg: nodeTxConfig{
			airtimeFactor:   DefaultAirtimeFactor,
			dutyCycleWindow: DefaultDutyCycleWindow,
			maxTxQueue:      DefaultMaxTxQueue,
		},
		advertInterval: DefaultAdvertInterval,
		handlers:       make(map[byte][]PacketHandler),
		done:           make(chan struct{}),
	}
	n.router.node = n
	for _, opt := range opts {
		opt(n)
	}
	if n.log == nil {
		n.log = slog.Default()
	}

	txRadio, ok := radio.(TxRadio)
	if !ok {
		queuedOpts := []QueuedRadioOption{
			WithQueuedRadioLogger(n.log),
			WithQueuedRadioErrorHandler(n.dispatchError),
			WithQueuedRadioMaxQueue(n.txCfg.maxTxQueue),
		}
		if n.txCfg.airtimeEstimator != nil {
			queuedOpts = append(queuedOpts, WithQueuedRadioAirtimeBudget(newAirtimeBudget(n.txCfg.airtimeFactor, n.txCfg.dutyCycleWindow, n.txCfg.airtimeEstimator)))
		}
		txRadio = NewQueuedRadio(radio, n.done, queuedOpts...)
		n.radio = txRadio
	} else if n.txCfg.airtimeEstimator != nil {
		n.log.Warn("WithAirtimeEstimator ignored: radio already implements TxRadio; configure airtime on the RadioMux or TxRadio directly")
	}
	n.txRadio = txRadio

	airtimeEstimator := n.txCfg.airtimeEstimator
	n.router.send = func(data []byte, priority uint8) error {
		delay := time.Duration(0)
		if airtimeEstimator != nil {
			delay = FloodRetransmitDelay(airtimeEstimator(len(data)))
		}
		if !n.txRadio.Enqueue(data, priority, delay) {
			return ErrTxQueueFull
		}
		return nil
	}
	n.router.sendDirect = func(data []byte) error {
		if !n.txRadio.Enqueue(data, PriorityDirectRelay, 0) {
			return ErrTxQueueFull
		}
		return nil
	}

	n.acks = newACKTracker(n.done)
	n.retries = newRetryTracker(n.sendPacketRaw, n.done)
	n.radio.SetDataHandler(n.onData)

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

// NotifyACK feeds an ACK CRC into the tracker as if it were received over
// the air. Use this when an ACK is extracted from a PathReturn packet's
// extra data rather than arriving as a standalone ACK packet.
func (n *Node) NotifyACK(crc uint32) {
	n.acks.notifyCRC(crc)
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
	return n.sendPacketRaw(pkt)
}

// SendPacketDelayed enqueues a packet with explicit priority and delay.
// Use for ACK responses and other timing-sensitive replies.
func (n *Node) SendPacketDelayed(pkt *meshcore.Packet, priority uint8, delay time.Duration) error {
	n.router.dedup.MarkSeen(pkt)
	data, err := pkt.ToBytes()
	if err != nil {
		return err
	}
	if !n.txRadio.Enqueue(data, priority, delay) {
		return ErrTxQueueFull
	}
	return nil
}

func (n *Node) sendPacketRaw(pkt *meshcore.Packet) error {
	data, err := pkt.ToBytes()
	if err != nil {
		return err
	}
	if !n.txRadio.Enqueue(data, PrioritySend, 0) {
		return ErrTxQueueFull
	}
	return nil
}

type GroupSendResult struct {
	Confirmed bool
}

type DMSendResult struct {
	Confirmed bool
	RoundTrip time.Duration
}

func (n *Node) SendGroupText(
	ch *meshcore.ChannelEntry,
	payload *meshcore.GroupTextPayload,
	pathHashSize uint8,
	retryTimeout time.Duration,
	maxRetries int,
	onResult func(GroupSendResult),
) error {
	grp, err := payload.Encrypt(ch.Hash, ch.PSK[:])
	if err != nil {
		return err
	}
	grpBytes, err := grp.ToBytes()
	if err != nil {
		return err
	}

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeGrpTxt, 0),
		PathLength: (pathHashSize - 1) << 6,
		Path:       []byte{},
		Payload:    grpBytes,
	}

	if err := n.sendPacketRaw(pkt); err != nil {
		return err
	}

	n.retries.track(pkt, maxRetries, retryTimeout,
		func() {
			if onResult != nil {
				onResult(GroupSendResult{Confirmed: true})
			}
		},
		func() {
			if onResult != nil {
				onResult(GroupSendResult{Confirmed: false})
			}
		},
	)

	return nil
}

func (n *Node) SendTextMessage(
	peer meshcore.Identity,
	text []byte,
	flags byte,
	timestamp time.Time,
	path []byte,
	pathHashSize uint8,
	timeout time.Duration,
	onResult func(DMSendResult),
) error {
	self := n.getIdentity()
	secret, err := n.secrets.get(peer)
	if err != nil {
		return err
	}

	plaintext := meshcore.BuildTextPlaintext(timestamp, flags, text)
	ackCRC := meshcore.CalcAckHash(plaintext, self.PublicKeyBytes())

	msg, err := meshcore.NewTextMessage(self, peer, plaintext, secret)
	if err != nil {
		return err
	}
	msgBytes, err := msg.ToBytes()
	if err != nil {
		return err
	}

	isDirect := len(path) > 0
	var routeType byte
	if isDirect {
		routeType = meshcore.RouteTypeDirect
	} else {
		routeType = meshcore.RouteTypeFlood
	}

	pkt := &meshcore.Packet{
		Header:     meshcore.MakeHeader(routeType, meshcore.PayloadTypeTxtMsg, 0),
		PathLength: (pathHashSize - 1) << 6,
		Path:       path,
		Payload:    msgBytes,
	}
	if isDirect {
		pkt.PathLength |= uint8(len(path) / int(pathHashSize))
	}

	if err := n.sendPacketRaw(pkt); err != nil {
		return err
	}
	n.router.dedup.MarkSeen(pkt)

	var maxRetries int
	var directRetries int
	if isDirect {
		maxRetries = 5
		directRetries = 3
	} else {
		maxRetries = 3
		directRetries = 0
	}

	attempt := 1
	var registerACK func()
	registerACK = func() {
		n.acks.expect(ackCRC, timeout,
			func(rt time.Duration) {
				if onResult != nil {
					onResult(DMSendResult{Confirmed: true, RoundTrip: rt})
				}
			},
			func() {
				attempt++
				if attempt > maxRetries {
					if onResult != nil {
						onResult(DMSendResult{Confirmed: false})
					}
					return
				}

				retryPkt := n.buildRetryPacket(pkt, attempt, directRetries, pathHashSize)
				if err := n.sendPacketRaw(retryPkt); err != nil {
					if onResult != nil {
						onResult(DMSendResult{Confirmed: false})
					}
					return
				}
				registerACK()
			},
		)
	}
	registerACK()

	return nil
}

func (n *Node) buildRetryPacket(original *meshcore.Packet, attempt, directRetries int, pathHashSize uint8) *meshcore.Packet {
	if attempt <= directRetries {
		return &meshcore.Packet{
			Header:     original.Header,
			PathLength: original.PathLength,
			Path:       original.Path,
			Payload:    original.Payload,
		}
	}
	return &meshcore.Packet{
		Header:     meshcore.MakeHeader(meshcore.RouteTypeFlood, meshcore.PayloadTypeTxtMsg, 0),
		PathLength: (pathHashSize - 1) << 6,
		Path:       []byte{},
		Payload:    original.Payload,
	}
}

func (n *Node) TxQueueLen() int {
	return n.txRadio.TxQueueLen()
}

// TxStats returns runtime counters from the underlying transmit engine.
// Returns the zero value if the underlying radio does not expose stats.
func (n *Node) TxStats() TxStats {
	type txStatser interface{ TxStats() TxStats }
	if s, ok := n.txRadio.(txStatser); ok {
		return s.TxStats()
	}
	return TxStats{}
}

// Stop signals shutdown to background goroutines and closes the radio.
// It is safe to call Stop multiple times; only the first call closes the
// radio.
func (n *Node) Stop() {
	n.stopOnce.Do(func() {
		close(n.done)
		if n.radio != nil {
			_ = n.radio.Close()
		}
	})
}

func (n *Node) Stopped() <-chan struct{} {
	return n.done
}
