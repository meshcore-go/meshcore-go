package sx12xx

import "errors"

// Sentinel errors returned by the Transmit/Receive paths.
var (
	// ErrTimeout is returned (wrapped) when a transmit or receive does not
	// complete before its deadline.
	ErrTimeout = errors.New("operation timed out")
	// ErrCRC is returned (wrapped) when a received packet fails its CRC check.
	ErrCRC = errors.New("crc error")
	// ErrTCXOStart is returned (wrapped) when the SX126x reports XOSC_START_ERR
	// after being configured to power a TCXO from DIO3.
	ErrTCXOStart = errors.New("oscillator failed to start")
	// ErrHeader is returned (wrapped) when a received LoRa packet has a header
	// error.
	ErrHeader = errors.New("header error")
	// ErrCommandFailed is returned (wrapped) when the SX126x reports that it
	// rejected or could not execute a command.
	ErrCommandFailed = errors.New("command rejected by the chip")
	// ErrNotLoRaModem is returned (wrapped) when the LoRa packet path is used
	// while the chip is configured for FSK. The FSK helpers configure the
	// modem; they do not give Transmit and ReceiveContinuous an FSK layout.
	ErrNotLoRaModem = errors.New("modem is not in LoRa mode")
	// ErrRecvStuck is reported (wrapped) when the watchdog finds the radio out
	// of receive and cannot re-arm it. Unlike the errors raised by the
	// listen-before-talk path, it means the chip has stopped responding.
	ErrRecvStuck = errors.New("receiver stuck out of receive")
)
