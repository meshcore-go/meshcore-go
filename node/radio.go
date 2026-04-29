package node

import meshcore "github.com/meshcore-go/meshcore-go"

type Radio interface {
	SendData(data []byte) error
	SetDataHandler(func(pkt *meshcore.Packet))
	SetRawDataHandler(func(data []byte, snr int8, rssi int8))
	AddOutboundHandler(h func([]byte))
	Close() error
}
