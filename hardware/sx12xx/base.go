package sx12xx

import (
	"fmt"
	"time"

	"periph.io/x/conn/v3/gpio"
)

// rxPollStep bounds how long the receive goroutine holds the device lock.
const rxPollStep = 25 * time.Millisecond

// inboundBuffer is how many received packets are buffered for the consumer.
const inboundBuffer = 64

// rfSetLevel drives p to level, treating a nil pin as a no-op.
func rfSetLevel(p gpio.PinIO, level gpio.Level) {
	if p != nil {
		_ = p.Out(level)
	}
}

func rfSetTx(txen, rxen gpio.PinIO)   { rfSetLevel(rxen, gpio.Low); rfSetLevel(txen, gpio.High) }
func rfSetRx(txen, rxen gpio.PinIO)   { rfSetLevel(txen, gpio.Low); rfSetLevel(rxen, gpio.High) }
func rfSetIdle(txen, rxen gpio.PinIO) { rfSetLevel(txen, gpio.Low); rfSetLevel(rxen, gpio.Low) }

// BaseSx12xx is the operational surface shared by the SX126x and SX127x LoRa
// transceivers.
type BaseSx12xx interface {
	// SetFrequency sets the RF carrier frequency in Hz.
	SetFrequency(hz uint32) error
	// SetTxPower sets the transmit power in dBm (clamped to the chip/PA range).
	SetTxPower(dBm int) error
	// SetModulationParams sets the LoRa spreading factor (5/6..12), bandwidth and
	// coding rate; ldroAuto enables low-data-rate optimization automatically when
	// the symbol duration warrants it.
	SetModulationParams(sf int, bw Bandwidth, cr CodingRate, ldroAuto bool) error
	// SetPacketParams sets the LoRa packet format: preamble length (symbols),
	// explicit vs implicit header, CRC and IQ inversion.
	SetPacketParams(preambleLen uint16, explicitHeader, crc, invertIq bool) error
	// SetPayloadLength sets the fixed payload length used for implicit-header
	// reception (ignored for explicit-header packets).
	SetPayloadLength(length byte) error
	// SetPublicNetwork selects the public/LoRaWAN sync word when true, otherwise
	// the private one.
	SetPublicNetwork(public bool) error
	// Transmit sends one payload, blocking until transmission completes or the
	// timeout elapses (zero waits indefinitely).
	Transmit(payload []byte, timeout time.Duration) error
	// Receive waits for and returns one packet, or an error on timeout/CRC error.
	Receive(timeout time.Duration) (*Packet, error)
	// ReceiveContinuous streams received packets on the returned channel until
	// Halt is called, which closes the channel.
	ReceiveContinuous() (<-chan Packet, error)
	// PacketStatus returns the last received packet's RSSI (dBm) and SNR (dB).
	PacketStatus() (rssi, snr float64, err error)
	// Standby places the radio in standby.
	Standby() error
	// Sleep places the radio in sleep.
	Sleep() error
	// Halt stops any continuous reception and returns the radio to standby.
	Halt() error

	// String identifies the device.
	fmt.Stringer
}

// RadioStats are the lifetime counters a radio keeps.
type RadioStats struct {
	// PacketsRecv is packets successfully read out of the chip.
	PacketsRecv uint64
	// PacketsSent is completed transmissions.
	PacketsSent uint64
	// PacketsRecvErrors is packets that raised an interrupt but could not be
	// read out of the FIFO.
	PacketsRecvErrors uint64
	// PacketsCRCErrors is packets discarded for a CRC or header error.
	PacketsCRCErrors uint64
	// PacketsDropped is packets read out but discarded by a slow consumer.
	PacketsDropped uint64
}
