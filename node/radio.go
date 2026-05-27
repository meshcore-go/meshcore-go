package node

import (
	"time"

	meshcore "github.com/meshcore-go/meshcore-go"
)

type Radio interface {
	SendData(data []byte) error
	SetDataHandler(func(pkt *meshcore.Packet))
	SetRawDataHandler(func(data []byte, snr int8, rssi int8, hasSignalInfo bool))
	AddOutboundHandler(h func([]byte))
	Close() error
}

// TxRadio is a Radio that supports prioritized enqueuing and serialized
// transmission. Implementations serialize sends through an internal queue,
// preventing concurrent writes to the underlying transport.
type TxRadio interface {
	Radio
	// Enqueue adds data to the transmit queue at the given priority.
	// Lower priority number = higher priority. delay postpones the
	// earliest send time. Returns false if the queue is full.
	Enqueue(data []byte, priority uint8, delay time.Duration) bool
	// TxQueueLen returns the number of entries currently in the queue.
	TxQueueLen() int
}
