package node

import (
	meshcore "github.com/meshcore-go/meshcore-go"
)

// RouteAction describes what the router decided to do with a received packet.
type RouteAction int

const (
	RouteActionDrop    RouteAction = iota // duplicate or unroutable — discard
	RouteActionDeliver                    // deliver to local handlers
	RouteActionForward                    // forward (relay) and deliver locally
)

type router struct {
	dedup meshcore.DedupCache
	node  *Node

	send       func([]byte) error
	sendDirect func([]byte) error
}

// route inspects an incoming packet and returns the action the node should take.
// For ForwardAction, the packet has already been modified (path appended/consumed)
// and transmitted. The node should still deliver it to local handlers.
func (r *router) route(pkt *meshcore.Packet) RouteAction {
	if pkt.IsRouteDirect() && pkt.PathHashCount() > 0 {
		return r.routeDirect(pkt)
	}

	if pkt.IsRouteFlood() {
		return r.routeFlood(pkt)
	}

	if r.dedup.HasSeen(pkt) {
		return RouteActionDrop
	}
	return RouteActionDeliver
}

func (r *router) routeDirect(pkt *meshcore.Packet) RouteAction {
	hashes := pkt.PathHashes()
	if len(hashes) == 0 {
		return RouteActionDeliver
	}

	if !r.node.getIdentity().IsHashMatch(hashes[0]) {
		if r.node != nil && r.node.canAcceptPacket(pkt) {
			if r.dedup.HasSeen(pkt) {
				return RouteActionDrop
			}
			return RouteActionDeliver
		}
		return RouteActionDrop
	}

	if r.dedup.HasSeen(pkt) {
		return RouteActionDrop
	}

	if !r.canForward(pkt) {
		return RouteActionDeliver
	}

	pkt.RemoveFirstPathHash()
	r.forwardDirect(pkt)
	return RouteActionForward
}

func (r *router) routeFlood(pkt *meshcore.Packet) RouteAction {
	if r.dedup.HasSeen(pkt) {
		return RouteActionDrop
	}

	if !r.canForward(pkt) {
		return RouteActionDeliver
	}

	hashSize := int(pkt.PathHashSize())
	newCount := int(pkt.PathHashCount()) + 1
	if newCount*hashSize > meshcore.MaxPathSize {
		return RouteActionDeliver
	}

	clone := r.clonePacket(pkt)
	if clone == nil {
		return RouteActionDeliver
	}
	pk := r.node.getIdentity().PublicKey()
	clone.AppendPathHash(pk[:])
	r.forward(clone)

	return RouteActionDeliver
}

func (r *router) canForward(pkt *meshcore.Packet) bool {
	if r.node == nil {
		return false
	}
	r.node.cbMu.RLock()
	f := r.node.allowForward
	r.node.cbMu.RUnlock()
	if f == nil {
		return false
	}
	return f(pkt)
}

func (r *router) forward(pkt *meshcore.Packet) {
	if r.send == nil {
		return
	}
	data, err := pkt.ToBytes()
	if err != nil {
		return
	}
	_ = r.send(data)
}

func (r *router) forwardDirect(pkt *meshcore.Packet) {
	send := r.sendDirect
	if send == nil {
		send = r.send
	}
	if send == nil {
		return
	}
	data, err := pkt.ToBytes()
	if err != nil {
		return
	}
	_ = send(data)
}

func (r *router) clonePacket(pkt *meshcore.Packet) *meshcore.Packet {
	data, err := pkt.ToBytes()
	if err != nil {
		return nil
	}
	clone, err := meshcore.PacketFromBytes(data)
	if err != nil {
		return nil
	}
	clone.SNR = pkt.SNR
	clone.RSSI = pkt.RSSI
	clone.HasSignalInfo = pkt.HasSignalInfo
	return clone
}
