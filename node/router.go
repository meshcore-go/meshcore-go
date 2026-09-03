package node

import (
	meshcore "github.com/meshcore-go/meshcore-go"
)

// RouteAction describes what the router decided to do with a received packet.
type RouteAction int

const (
	RouteActionDrop    RouteAction = iota // duplicate or unroutable — discard, no local delivery
	RouteActionDeliver                    // deliver to local handlers (a flood packet may also have been re-flooded)
	RouteActionForward                    // relayed to the next hop only — NOT delivered locally
)

// router requires a non-nil node.
type router struct {
	dedup meshcore.DedupCache
	node  *Node

	send       func(data []byte, priority uint8) error
	sendDirect func([]byte) error
}

// route returns the action for an incoming packet; a direct relay consumes pkt's path in place.
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
	// payload[0]&0x80 marks a zero-hop-only control packet.
	if pkt.PayloadType() == meshcore.PayloadTypeControl && len(pkt.Payload) > 0 && pkt.Payload[0]&0x80 != 0 {
		return RouteActionDrop
	}

	hashes := pkt.PathHashes()
	if len(hashes) == 0 {
		return RouteActionDeliver
	}

	if r.node.Identity().IsHashMatch(hashes[0]) && r.canForward(pkt) {
		if r.dedup.HasSeen(pkt) {
			return RouteActionDrop
		}
		pkt.RemoveFirstPathHash()
		r.forwardDirect(pkt)
		return RouteActionForward
	}

	if r.node.canAcceptPacket(pkt) {
		if r.dedup.HasSeen(pkt) {
			return RouteActionDrop
		}
		return RouteActionDeliver
	}
	return RouteActionDrop
}

func (r *router) routeFlood(pkt *meshcore.Packet) RouteAction {
	if r.dedup.HasSeen(pkt) {
		return RouteActionDrop
	}

	if !r.canForward(pkt) || !r.floodForwardable(pkt) {
		return RouteActionDeliver
	}

	hashSize := int(pkt.PathHashSize())
	newCount := int(pkt.PathHashCount()) + 1
	if newCount*hashSize > meshcore.MaxPathSize {
		return RouteActionDeliver
	}

	clone := pkt.Clone()
	pk := r.node.Identity().PublicKey()
	clone.AppendPathHash(pk[:])
	r.forward(clone, uint8(newCount))

	return RouteActionDeliver
}

// floodForwardable reports whether this payload type is flood-relayed.
func (r *router) floodForwardable(pkt *meshcore.Packet) bool {
	switch pkt.PayloadType() {
	case meshcore.PayloadTypeAck, meshcore.PayloadTypePath, meshcore.PayloadTypeReq,
		meshcore.PayloadTypeResponse, meshcore.PayloadTypeTxtMsg, meshcore.PayloadTypeAnonReq,
		meshcore.PayloadTypeGrpTxt, meshcore.PayloadTypeGrpData:
		return true
	case meshcore.PayloadTypeAdvert:
		adv, err := meshcore.AdvertFromBytes(pkt.Payload)
		return err == nil && !r.node.Identity().Matches(adv.PublicKey) && adv.Verify()
	}
	return false
}

func (r *router) canForward(pkt *meshcore.Packet) bool {
	r.node.cbMu.RLock()
	f := r.node.allowForward
	r.node.cbMu.RUnlock()
	if f == nil {
		return false
	}
	return f(pkt)
}

func (r *router) forward(pkt *meshcore.Packet, priority uint8) {
	if r.send == nil {
		return
	}
	data, err := pkt.ToBytes()
	if err == nil {
		err = r.send(data, priority)
	}
	if err != nil {
		r.node.dispatchError(err)
	}
}

func (r *router) forwardDirect(pkt *meshcore.Packet) {
	if r.sendDirect == nil {
		return
	}
	data, err := pkt.ToBytes()
	if err == nil {
		err = r.sendDirect(data)
	}
	if err != nil {
		r.node.dispatchError(err)
	}
}
