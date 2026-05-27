package meshcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

type Packet struct {
	Header         byte
	PathLength     uint8
	Path           []byte
	Payload        []byte
	TransportCode1 uint16 // Little Endian
	TransportCode2 uint16 // Little Endian

	SNR           int8
	RSSI          int8
	HasSignalInfo bool

	pathHashSize  uint8
	pathHashCount uint8
}

func MakeHeader(routeType, payloadType, payloadVer byte) byte {
	return (payloadVer << 6) | (payloadType << 2) | routeType
}

func IsValidPathLen(pathLen uint8) bool {
	hashCount := pathLen & 63
	hashSize := (pathLen >> 6) + 1
	if hashSize == 4 {
		return false
	}
	return int(hashCount)*int(hashSize) <= MaxPathSize
}

func PacketFromBytes(data []byte) (*Packet, error) {
	packet := &Packet{}
	buffer := bytes.NewBuffer(data)

	headerByte, err := buffer.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	packet.Header = headerByte

	routeType := headerByte & PacketRouteMask
	hasTransportCodes := routeType == RouteTypeTransportFlood || routeType == RouteTypeTransportDirect

	if hasTransportCodes {
		if err := binary.Read(buffer, binary.LittleEndian, &packet.TransportCode1); err != nil {
			return nil, fmt.Errorf("reading transport code 1: %w", err)
		}
		if err := binary.Read(buffer, binary.LittleEndian, &packet.TransportCode2); err != nil {
			return nil, fmt.Errorf("reading transport code 2: %w", err)
		}
	}

	pathLenByte, err := buffer.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("reading path length: %w", err)
	}
	packet.PathLength = pathLenByte

	if !IsValidPathLen(pathLenByte) {
		return nil, fmt.Errorf("invalid path length byte: 0x%02x", pathLenByte)
	}

	packet.pathHashSize = pathLenByte>>6 + 1
	packet.pathHashCount = pathLenByte & 63
	pathByteLength := int(packet.pathHashCount) * int(packet.pathHashSize)

	if buffer.Len() < pathByteLength {
		return nil, fmt.Errorf("not enough data for path: need %d bytes, have %d", pathByteLength, buffer.Len())
	}
	packet.Path = buffer.Next(pathByteLength)

	payload := buffer.Bytes()
	if len(payload) > MaxPacketPayload {
		return nil, fmt.Errorf("payload too large: %d bytes, max %d", len(payload), MaxPacketPayload)
	}
	packet.Payload = payload

	return packet, nil
}

func (p *Packet) ToBytes() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	if err := binary.Write(buffer, binary.LittleEndian, p.Header); err != nil {
		return nil, fmt.Errorf("writing header: %w", err)
	}

	routeType := p.RouteType()
	hasTransportCodes := routeType == RouteTypeTransportFlood || routeType == RouteTypeTransportDirect

	if hasTransportCodes {
		if err := binary.Write(buffer, binary.LittleEndian, p.TransportCode1); err != nil {
			return nil, fmt.Errorf("writing transport code 1: %w", err)
		}
		if err := binary.Write(buffer, binary.LittleEndian, p.TransportCode2); err != nil {
			return nil, fmt.Errorf("writing transport code 2: %w", err)
		}
	}

	if err := binary.Write(buffer, binary.LittleEndian, p.PathLength); err != nil {
		return nil, fmt.Errorf("writing path length: %w", err)
	}

	if err := binary.Write(buffer, binary.LittleEndian, p.Path); err != nil {
		return nil, fmt.Errorf("writing path: %w", err)
	}

	if err := binary.Write(buffer, binary.LittleEndian, p.Payload); err != nil {
		return nil, fmt.Errorf("writing payload: %w", err)
	}

	return buffer.Bytes(), nil
}

func (p *Packet) PathHashSize() uint8 {
	return p.pathHashSize
}

func (p *Packet) PathHashCount() uint8 {
	return p.pathHashCount
}

func (p *Packet) PathHashes() [][]byte {
	var pathItems [][]byte
	buffer := bytes.NewBuffer(p.Path)

	for i := 0; i < int(p.pathHashCount); i++ {
		pathBytes := buffer.Next(int(p.pathHashSize))
		pathItems = append(pathItems, pathBytes)
	}

	return pathItems
}

func (p *Packet) RouteType() byte {
	return p.Header & PacketRouteMask
}

func (p *Packet) RouteTypeString() string {
	switch p.RouteType() {
	case RouteTypeFlood:
		return "FLOOD"
	case RouteTypeDirect:
		return "DIRECT"
	case RouteTypeTransportFlood:
		return "TRANSPORT_FLOOD"
	case RouteTypeTransportDirect:
		return "TRANSPORT_DIRECT"
	default:
		return ""
	}
}

func (p *Packet) PayloadType() byte {
	return p.Header >> PacketTypeShift & PacketTypeMask
}

func (p *Packet) PayloadTypeString() string {
	switch p.PayloadType() {
	case PayloadTypeReq:
		return "REQ"
	case PayloadTypeResponse:
		return "RESPONSE"
	case PayloadTypeTxtMsg:
		return "TXT_MSG"
	case PayloadTypeAck:
		return "ACK"
	case PayloadTypeAdvert:
		return "ADVERT"
	case PayloadTypeGrpTxt:
		return "GRP_TXT"
	case PayloadTypeGrpData:
		return "GRP_DATA"
	case PayloadTypeAnonReq:
		return "ANON_REQ"
	case PayloadTypePath:
		return "PATH"
	case PayloadTypeTrace:
		return "TRACE"
	case PayloadTypeMultiPart:
		return "MULTI_PART"
	case PayloadTypeControl:
		return "CONTROL"
	case PayloadTypeRawCustom:
		return "RAW_CUSTOM"
	default:
		return ""
	}
}

func (p *Packet) PayloadVer() byte {
	return p.Header >> PacketVerShift & PacketVerMask
}

func (p *Packet) Validate() error {
	if !IsValidPathLen(p.PathLength) {
		return fmt.Errorf("invalid path length byte: 0x%02x", p.PathLength)
	}
	if len(p.Payload) > MaxPacketPayload {
		return fmt.Errorf("payload too large: %d bytes, max %d", len(p.Payload), MaxPacketPayload)
	}
	return nil
}

func (p *Packet) IsRouteFlood() bool {
	rt := p.RouteType()
	return rt == RouteTypeFlood || rt == RouteTypeTransportFlood
}

func (p *Packet) IsRouteDirect() bool {
	rt := p.RouteType()
	return rt == RouteTypeDirect || rt == RouteTypeTransportDirect
}

func (p *Packet) IsTransport() bool {
	rt := p.RouteType()
	return rt == RouteTypeTransportFlood || rt == RouteTypeTransportDirect
}

// AppendPathHash appends a path hash to the packet's path. It returns false
// if the path is already full (would exceed MaxPathSize).
func (p *Packet) AppendPathHash(hash []byte) bool {
	hashSize := int(p.PathHashSize())
	if len(hash) < hashSize {
		return false
	}
	newCount := int(p.pathHashCount) + 1
	if newCount*hashSize > MaxPathSize {
		return false
	}
	p.Path = append(p.Path, hash[:hashSize]...)
	p.pathHashCount = uint8(newCount)
	p.PathLength = (p.pathHashSize-1)<<6 | p.pathHashCount
	return true
}

// RemoveFirstPathHash removes the first hash from the packet's path,
// shifting remaining hashes left. Returns false if the path is empty.
func (p *Packet) RemoveFirstPathHash() bool {
	if p.pathHashCount == 0 {
		return false
	}
	hashSize := int(p.pathHashSize)
	p.Path = p.Path[hashSize:]
	p.pathHashCount--
	p.PathLength = (p.pathHashSize-1)<<6 | p.pathHashCount
	return true
}

const PacketHashSize = 8

// PacketHash computes the dedup fingerprint for this packet.
// Matches MeshCore's Packet::calculatePacketHash: SHA256(payloadType + payload)
// truncated to 8 bytes. TRACE packets also include PathLength to handle
// revisited nodes. Returns the hash as a [PacketHashSize]byte.
func (p *Packet) PacketHash() [PacketHashSize]byte {
	h := sha256.New()
	h.Write([]byte{p.PayloadType()})
	if p.PayloadType() == PayloadTypeTrace {
		h.Write([]byte{p.PathLength, 0}) // uint16 LE to match C++ sizeof(uint16_t)
	}
	h.Write(p.Payload)
	sum := h.Sum(nil)
	var out [PacketHashSize]byte
	copy(out[:], sum[:PacketHashSize])
	return out
}
