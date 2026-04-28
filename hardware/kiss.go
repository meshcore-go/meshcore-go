package hardware

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// KISS Protocol Constants
	KISS_FEND  = 0xC0 // Frame End
	KISS_FESC  = 0xDB // Frame Escape
	KISS_TFEND = 0xDC // Transposed Frame End
	KISS_TFESC = 0xDD // Transposed Frame Escape

	// KISS Frame Limits
	KISS_MAX_FRAME_SIZE  = 512
	KISS_MAX_PACKET_SIZE = 255

	// KISS Command Masks
	KISS_MASK_PORT = 0xF0
	KISS_MASK_CMD  = 0x0F

	// KISS Commands
	KISS_CMD_DATA        = 0x00
	KISS_CMD_TXDELAY     = 0x01
	KISS_CMD_PERSISTENCE = 0x02
	KISS_CMD_SLOTTIME    = 0x03
	KISS_CMD_TXTAIL      = 0x04
	KISS_CMD_FULLDUPLEX  = 0x05
	KISS_CMD_SETHARDWARE = 0x06
	KISS_CMD_RETURN      = 0xFF

	// KISS Default Parameters
	KISS_DEFAULT_TXDELAY     = 50
	KISS_DEFAULT_PERSISTENCE = 63
	KISS_DEFAULT_SLOTTIME    = 10

	// Hardware Commands (sub-commands of KISS_CMD_SETHARDWARE)
	HW_CMD_GET_IDENTITY      = 0x01
	HW_CMD_GET_RANDOM        = 0x02
	HW_CMD_VERIFY_SIGNATURE  = 0x03
	HW_CMD_SIGN_DATA         = 0x04
	HW_CMD_ENCRYPT_DATA      = 0x05
	HW_CMD_DECRYPT_DATA      = 0x06
	HW_CMD_KEY_EXCHANGE      = 0x07
	HW_CMD_HASH              = 0x08
	HW_CMD_SET_RADIO         = 0x09
	HW_CMD_SET_TX_POWER      = 0x0A
	HW_CMD_GET_RADIO         = 0x0B
	HW_CMD_GET_TX_POWER      = 0x0C
	HW_CMD_GET_CURRENT_RSSI  = 0x0D
	HW_CMD_IS_CHANNEL_BUSY   = 0x0E
	HW_CMD_GET_AIRTIME       = 0x0F
	HW_CMD_GET_NOISE_FLOOR   = 0x10
	HW_CMD_GET_VERSION       = 0x11
	HW_CMD_GET_STATS         = 0x12
	HW_CMD_GET_BATTERY       = 0x13
	HW_CMD_GET_MCU_TEMP      = 0x14
	HW_CMD_GET_SENSORS       = 0x15
	HW_CMD_GET_DEVICE_NAME   = 0x16
	HW_CMD_PING              = 0x17
	HW_CMD_REBOOT            = 0x18
	HW_CMD_SET_SIGNAL_REPORT = 0x19
	HW_CMD_GET_SIGNAL_REPORT = 0x1A

	// Hardware Response Codes (command | 0x80)
	HW_RESP_OK      = 0xF0
	HW_RESP_ERROR   = 0xF1
	HW_RESP_TX_DONE = 0xF8 // Unsolicited: transmission complete
	HW_RESP_RX_META = 0xF9 // Unsolicited: received packet metadata

	// Hardware Error Codes
	HW_ERR_INVALID_LENGTH = 0x01
	HW_ERR_INVALID_PARAM  = 0x02
	HW_ERR_NO_CALLBACK    = 0x03
	HW_ERR_MAC_FAILED     = 0x04
	HW_ERR_UNKNOWN_CMD    = 0x05
	HW_ERR_ENCRYPT_FAILED = 0x06

	// Firmware Version
	KISS_FIRMWARE_VERSION = 1
)

var (
	ErrFrameTooShort    = errors.New("kiss: frame too short")
	ErrFrameNoFEND      = errors.New("kiss: frame missing FEND markers")
	ErrInvalidEscape    = errors.New("kiss: invalid escape sequence")
	ErrIncompleteEscape = errors.New("kiss: incomplete escape sequence at end of data")
)

// KissFrame represents a decoded KISS frame with its command byte and payload data.
type KissFrame struct {
	Port    int
	Command byte
	Data    []byte

	SNR  int8
	RSSI int8
}

// RadioConfig holds the radio configuration parameters.
type RadioConfig struct {
	FreqHz uint32
	BwHz   uint32
	SF     uint8
	CR     uint8
}

// ToBytes encodes a RadioConfig to its wire format (little-endian).
func (r *RadioConfig) ToBytes() []byte {
	buf := make([]byte, 10)
	binary.LittleEndian.PutUint32(buf[0:4], r.FreqHz)
	binary.LittleEndian.PutUint32(buf[4:8], r.BwHz)
	buf[8] = r.SF
	buf[9] = r.CR
	return buf
}

// RadioConfigFromBytes decodes a RadioConfig from its wire format (little-endian).
func RadioConfigFromBytes(data []byte) (*RadioConfig, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("kiss: radio config too short: %d bytes", len(data))
	}
	return &RadioConfig{
		FreqHz: binary.LittleEndian.Uint32(data[0:4]),
		BwHz:   binary.LittleEndian.Uint32(data[4:8]),
		SF:     data[8],
		CR:     data[9],
	}, nil
}

// HwResp returns the hardware response code for a given command.
// Response code = command | 0x80.
func HwResp(cmd byte) byte {
	return cmd | 0x80
}

// EncodeHardwareFrame builds a KISS hardware command frame (KISS_CMD_SETHARDWARE)
// with the given sub-command and data payload.
func EncodeHardwareFrame(port int, subCmd byte, data []byte) []byte {
	payload := make([]byte, 0, len(data)+1)
	payload = append(payload, subCmd)
	payload = append(payload, data...)
	return EncodeFrame(port, KISS_CMD_SETHARDWARE, payload)
}

// DecodeHardwareFrame extracts the sub-command and payload from a KissFrame
// that has Command == KISS_CMD_SETHARDWARE.
func DecodeHardwareFrame(frame *KissFrame) (subCmd byte, data []byte, err error) {
	if frame.Command != KISS_CMD_SETHARDWARE {
		return 0, nil, fmt.Errorf("kiss: not a hardware frame, command=0x%02X", frame.Command)
	}
	if len(frame.Data) < 1 {
		return 0, nil, ErrFrameTooShort
	}
	return frame.Data[0], frame.Data[1:], nil
}

// EscapeData applies KISS byte-stuffing to the given data. Any FEND (0xC0) byte
// is replaced with FESC TFEND (0xDB 0xDC), and any FESC (0xDB) byte is replaced
// with FESC TFESC (0xDB 0xDD).
func EscapeData(data []byte) []byte {
	escaped := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case KISS_FEND:
			escaped = append(escaped, KISS_FESC, KISS_TFEND)
		case KISS_FESC:
			escaped = append(escaped, KISS_FESC, KISS_TFESC)
		default:
			escaped = append(escaped, b)
		}
	}
	return escaped
}

// UnescapeData reverses KISS byte-stuffing. FESC TFEND (0xDB 0xDC) is restored
// to FEND (0xC0), and FESC TFESC (0xDB 0xDD) is restored to FESC (0xDB).
func UnescapeData(data []byte) ([]byte, error) {
	unescaped := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == KISS_FESC {
			i++
			if i >= len(data) {
				return nil, ErrIncompleteEscape
			}
			switch data[i] {
			case KISS_TFEND:
				unescaped = append(unescaped, KISS_FEND)
			case KISS_TFESC:
				unescaped = append(unescaped, KISS_FESC)
			default:
				return nil, fmt.Errorf("%w: 0xDB 0x%02X", ErrInvalidEscape, data[i])
			}
		} else {
			unescaped = append(unescaped, data[i])
		}
	}
	return unescaped, nil
}

// EncodeFrame builds a complete KISS frame: FEND, command byte, escaped data, FEND.
// The command byte encodes the port (high nibble) and command (low nibble).
func EncodeFrame(port int, command byte, data []byte) []byte {
	cmdByte := byte(port<<4) | (command & KISS_MASK_CMD)
	escaped := EscapeData(data)

	frame := make([]byte, 0, len(escaped)+3)
	frame = append(frame, KISS_FEND)
	frame = append(frame, cmdByte)
	frame = append(frame, escaped...)
	frame = append(frame, KISS_FEND)
	return frame
}

// EncodeDataFrame builds a KISS data frame (command=0x00) on port 0.
func EncodeDataFrame(data []byte) []byte {
	return EncodeFrame(0, KISS_CMD_DATA, data)
}

// DecodeFrame parses a raw KISS frame (with FEND delimiters) into a KissFrame.
func DecodeFrame(raw []byte) (*KissFrame, error) {
	if len(raw) < 3 {
		return nil, ErrFrameTooShort
	}

	// Strip leading/trailing FEND markers
	start := 0
	end := len(raw)
	if raw[start] == KISS_FEND {
		start++
	}
	if raw[end-1] == KISS_FEND {
		end--
	}

	if start >= end {
		return nil, ErrFrameTooShort
	}

	cmdByte := raw[start]
	port := int((cmdByte & KISS_MASK_PORT) >> 4)
	command := cmdByte & KISS_MASK_CMD

	payload := raw[start+1 : end]
	data, err := UnescapeData(payload)
	if err != nil {
		return nil, fmt.Errorf("kiss: decode frame: %w", err)
	}

	return &KissFrame{
		Port:    port,
		Command: command,
		Data:    data,
	}, nil
}

// ExtractFrames extracts all complete KISS frames from a byte stream.
// It returns the decoded frames and any remaining bytes that don't form a
// complete frame (useful for streaming/buffered reads).
func ExtractFrames(stream []byte) ([]*KissFrame, []byte) {
	var frames []*KissFrame

	// Skip any bytes before the first FEND
	start := -1
	for i, b := range stream {
		if b == KISS_FEND {
			start = i
			break
		}
	}
	if start == -1 {
		return nil, nil
	}

	i := start
	for i < len(stream) {
		// Skip consecutive FEND bytes (inter-frame fill)
		for i < len(stream) && stream[i] == KISS_FEND {
			i++
		}
		if i >= len(stream) {
			break
		}

		// Find the closing FEND
		frameStart := i - 1 // include the preceding FEND
		found := false
		for j := i; j < len(stream); j++ {
			if stream[j] == KISS_FEND {
				frameData := stream[frameStart : j+1]
				frame, err := DecodeFrame(frameData)
				if err == nil {
					frames = append(frames, frame)
				}
				i = j + 1
				found = true
				break
			}
		}
		if !found {
			// Incomplete frame — return remainder
			return frames, stream[frameStart:]
		}
	}

	return frames, nil
}
