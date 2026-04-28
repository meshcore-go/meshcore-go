package meshcore

// Packet header field masks and shifts.
const (
	PacketRouteMask byte = 0x03 // 2-bits
	PacketTypeShift byte = 2
	PacketTypeMask  byte = 0x0F // 4-bits
	PacketVerShift  byte = 6
	PacketVerMask   byte = 0x03 // 2-bits
)

// Route types.
const (
	RouteTypeTransportFlood  byte = 0x00 // flood mode + transport codes
	RouteTypeFlood           byte = 0x01 // flood mode, needs 'path' to be built up (max 64 bytes)
	RouteTypeDirect          byte = 0x02 // direct route, 'path' is supplied
	RouteTypeTransportDirect byte = 0x03 // direct route + transport codes
)

// Payload types.
const (
	PayloadTypeReq       byte = 0x00 // request (prefixed with dest/src hashes, MAC) (enc data: timestamp, blob)
	PayloadTypeResponse  byte = 0x01 // response to REQ or ANON_REQ (prefixed with dest/src hashes, MAC) (enc data: timestamp, blob)
	PayloadTypeTxtMsg    byte = 0x02 // a plain text message (prefixed with dest/src hashes, MAC) (enc data: timestamp, text)
	PayloadTypeAck       byte = 0x03 // a simple ack
	PayloadTypeAdvert    byte = 0x04 // a node advertising its Identity
	PayloadTypeGrpTxt    byte = 0x05 // an (unverified) group text message (prefixed with channel hash, MAC) (enc data: timestamp, "name: msg")
	PayloadTypeGrpData   byte = 0x06 // an (unverified) group datagram (prefixed with channel hash, MAC) (enc data: timestamp, blob)
	PayloadTypeAnonReq   byte = 0x07 // generic request (prefixed with dest_hash, ephemeral pub_key, MAC) (enc data: ...)
	PayloadTypePath      byte = 0x08 // returned path (prefixed with dest/src hashes, MAC) (enc data: path, extra)
	PayloadTypeTrace     byte = 0x09 // trace a path, collecting SNR for each hop
	PayloadTypeMultiPart byte = 0x0A // packet is one of a set of packets
	PayloadTypeControl   byte = 0x0B // a control/discovery packet
	PayloadTypeRawCustom byte = 0x0F // custom packet as raw bytes, for applications with custom encryption, payloads, etc
)

// Advert types.
const (
	AdvertTypeNone     byte = 0
	AdvertTypeChat     byte = 1
	AdvertTypeRepeater byte = 2
	AdvertTypeRoom     byte = 3
	AdvertTypeSensor   byte = 4
)

// Advert field masks.
const (
	AdvertLatLonMask byte = 0x10
	AdvertFeat1Mask  byte = 0x20
	AdvertFeat2Mask  byte = 0x40
	AdvertNameMask   byte = 0x80
)

// Protocol size limits.
const (
	PubKeySize        = 32
	SignatureSize     = 64
	MaxAdvertDataSize = 32
	MaxPacketPayload  = 184
	MaxPathSize       = 64
	MaxTransUnit      = 255
	MaxTextLen        = 10 * 16 // 10 * CIPHER_BLOCK_SIZE = 160 bytes

	// MinAdvertSize is the minimum advert payload: pubkey + timestamp + signature.
	MinAdvertSize = PubKeySize + 4 + SignatureSize // 100 bytes
)
