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
	// ErrRecvStuck is reported (wrapped) when the watchdog finds the radio out
	// of receive and cannot re-arm it. Unlike the errors raised by the
	// listen-before-talk path, it means the chip has stopped responding.
	ErrRecvStuck = errors.New("receiver stuck out of receive")
)
