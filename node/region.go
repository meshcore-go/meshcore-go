package node

import (
	"sync"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type RegionMap struct {
	mu       sync.RWMutex
	regions  []*meshcore.Region
	wildcard meshcore.Region
	nextID   uint16
}

func NewRegionMap() *RegionMap {
	return &RegionMap{
		wildcard: meshcore.Region{
			Name: "*",
		},
		nextID: 1,
	}
}

func (rm *RegionMap) Wildcard() *meshcore.Region {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return &rm.wildcard
}

func (rm *RegionMap) SetWildcardFlags(flags uint8) {
	rm.mu.Lock()
	rm.wildcard.Flags = flags
	rm.mu.Unlock()
}

func (rm *RegionMap) Add(r *meshcore.Region) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if len(rm.regions) >= meshcore.MaxRegions {
		return false
	}
	if r.ID == 0 {
		r.ID = rm.nextID
		rm.nextID++
	}
	rm.regions = append(rm.regions, r)
	return true
}

func (rm *RegionMap) Remove(name string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for i, r := range rm.regions {
		if r.Name == name {
			rm.regions = append(rm.regions[:i], rm.regions[i+1:]...)
			return true
		}
	}
	return false
}

func (rm *RegionMap) Get(name string) *meshcore.Region {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for _, r := range rm.regions {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func (rm *RegionMap) All() []*meshcore.Region {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	out := make([]*meshcore.Region, len(rm.regions))
	copy(out, rm.regions)
	return out
}

func (rm *RegionMap) Len() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return len(rm.regions)
}

// FindFloodMatch returns the region that matches a flood packet, or nil.
//
// For transport-routed packets: checks TransportCode1 against each region
// that doesn't have RegionDenyFlood set.
//
// For non-transport packets: returns the wildcard if it allows flood,
// nil if wildcard denies flood.
func (rm *RegionMap) FindFloodMatch(pkt *meshcore.Packet) *meshcore.Region {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if pkt.IsTransport() {
		for _, r := range rm.regions {
			if r.Flags&meshcore.RegionDenyFlood != 0 {
				continue
			}
			if r.MatchesPacket(pkt) {
				return r
			}
		}
		return nil
	}

	if rm.wildcard.Flags&meshcore.RegionDenyFlood != 0 {
		return nil
	}
	return &rm.wildcard
}
