package node

import (
	meshcore "github.com/meshcore-go/meshcore-go"
)

func (n *Node) onData(pkt *meshcore.Packet) {
	select {
	case <-n.done:
		return
	default:
	}

	n.retries.handlePacket(pkt)

	if pkt.PayloadType() == meshcore.PayloadTypeTrace {
		if !n.router.dedup.HasSeen(pkt) {
			n.dispatchPacket(pkt)
		}
		return
	}

	action := n.router.route(pkt)
	if action == RouteActionDrop {
		return
	}

	if pkt.PayloadType() == meshcore.PayloadTypeAdvert {
		n.handleAdvert(pkt)
	}

	if pkt.PayloadType() == meshcore.PayloadTypeAck {
		n.acks.handleACK(pkt)
	}

	n.dispatchPacket(pkt)
}

func (n *Node) handleAdvert(pkt *meshcore.Packet) {
	adv, err := meshcore.AdvertFromBytes(pkt.Payload)
	if err != nil {
		n.log.Debug("failed to parse advert", "error", err)
		n.dispatchError(err)
		return
	}

	if n.getIdentity().Matches(adv.PublicKey) {
		return
	}

	if !adv.Verify() {
		return
	}

	n.peers.Update(adv, pkt.SNR, pkt.RSSI, pkt.HasSignalInfo, pkt.Path)
}

func (n *Node) dispatchPacket(pkt *meshcore.Packet) {
	n.handlerMu.RLock()
	handlers := n.handlers[pkt.PayloadType()]
	n.handlerMu.RUnlock()

	for _, h := range handlers {
		h(pkt)
	}
}

func (n *Node) dispatchError(err error) {
	n.cbMu.RLock()
	h := n.errH
	n.cbMu.RUnlock()
	if h != nil {
		h(err)
	}
}
