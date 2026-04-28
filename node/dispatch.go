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

	if pkt.PayloadType() == meshcore.PayloadTypeTrace {
		if !n.router.dedup.hasSeen(pkt) {
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
		return
	}

	if n.getIdentity().Matches(adv.PublicKey) {
		return
	}

	if !adv.Verify() {
		return
	}

	n.peers.Update(adv, pkt.SNR, pkt.RSSI, pkt.Path)
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
