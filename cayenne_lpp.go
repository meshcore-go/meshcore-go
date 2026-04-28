package meshcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	LPPDigitalInput       byte = 0
	LPPDigitalOutput      byte = 1
	LPPAnalogInput        byte = 2
	LPPAnalogOutput       byte = 3
	LPPGenericSensor      byte = 100
	LPPLuminosity         byte = 101
	LPPPresence           byte = 102
	LPPTemperature        byte = 103
	LPPRelativeHumidity   byte = 104
	LPPAccelerometer      byte = 113
	LPPBarometricPressure byte = 115
	LPPVoltage            byte = 116
	LPPCurrent            byte = 117
	LPPFrequency          byte = 118
	LPPPercentage         byte = 120
	LPPAltitude           byte = 121
	LPPConcentration      byte = 125
	LPPPower              byte = 128
	LPPDistance           byte = 130
	LPPEnergy             byte = 131
	LPPDirection          byte = 132
	LPPUnixTime           byte = 133
	LPPGyrometer          byte = 134
	LPPColour             byte = 135
	LPPGPS                byte = 136
	LPPSwitch             byte = 142
)

type LPPReading struct {
	Channel byte
	Type    byte
	Value   interface{}
}

type LPPGPSValue struct {
	Latitude  float64
	Longitude float64
	Altitude  float64
}

type LPPAccelValue struct {
	X float64
	Y float64
	Z float64
}

type LPPGyroValue struct {
	X float64
	Y float64
	Z float64
}

type LPPColourValue struct {
	R byte
	G byte
	B byte
}

func LPPDecode(data []byte) ([]LPPReading, error) {
	readings := make([]LPPReading, 0)

	for i := 0; i < len(data); {
		if len(data)-i < 2 {
			return nil, fmt.Errorf("truncated lpp header: need 2 bytes, have %d", len(data)-i)
		}

		channel := data[i]
		typ := data[i+1]
		i += 2

		if channel == 0 && typ == 0 {
			break
		}

		need := 0
		switch typ {
		case LPPDigitalInput, LPPDigitalOutput, LPPPresence, LPPRelativeHumidity, LPPPercentage, LPPSwitch:
			need = 1
		case LPPAnalogInput, LPPAnalogOutput, LPPLuminosity, LPPTemperature, LPPBarometricPressure, LPPVoltage, LPPCurrent, LPPAltitude, LPPConcentration, LPPPower, LPPDirection:
			need = 2
		case LPPGenericSensor, LPPFrequency, LPPDistance, LPPEnergy, LPPUnixTime:
			need = 4
		case LPPAccelerometer, LPPGyrometer:
			need = 6
		case LPPColour:
			need = 3
		case LPPGPS:
			need = 9
		default:
			return nil, fmt.Errorf("unknown lpp type: %d", typ)
		}

		if len(data)-i < need {
			return nil, fmt.Errorf("truncated lpp payload for channel %d type %d: need %d bytes, have %d", channel, typ, need, len(data)-i)
		}

		payload := data[i : i+need]
		i += need

		reading := LPPReading{Channel: channel, Type: typ}

		switch typ {
		case LPPDigitalInput, LPPDigitalOutput, LPPPresence, LPPPercentage, LPPSwitch:
			reading.Value = float64(payload[0])
		case LPPRelativeHumidity:
			reading.Value = float64(payload[0]) / 2
		case LPPAnalogInput, LPPAnalogOutput:
			reading.Value = float64(int16(binary.BigEndian.Uint16(payload))) / 100
		case LPPLuminosity, LPPConcentration, LPPPower, LPPDirection:
			reading.Value = float64(binary.BigEndian.Uint16(payload))
		case LPPTemperature:
			reading.Value = float64(int16(binary.BigEndian.Uint16(payload))) / 10
		case LPPBarometricPressure:
			reading.Value = float64(binary.BigEndian.Uint16(payload)) / 10
		case LPPVoltage:
			reading.Value = float64(int16(binary.BigEndian.Uint16(payload))) / 100
		case LPPCurrent:
			reading.Value = float64(int16(binary.BigEndian.Uint16(payload))) / 1000
		case LPPAltitude:
			reading.Value = float64(int16(binary.BigEndian.Uint16(payload)))
		case LPPGenericSensor, LPPFrequency, LPPUnixTime:
			reading.Value = float64(binary.BigEndian.Uint32(payload))
		case LPPDistance, LPPEnergy:
			reading.Value = float64(binary.BigEndian.Uint32(payload)) / 1000
		case LPPAccelerometer:
			reading.Value = LPPAccelValue{
				X: float64(int16(binary.BigEndian.Uint16(payload[0:2]))) / 1000,
				Y: float64(int16(binary.BigEndian.Uint16(payload[2:4]))) / 1000,
				Z: float64(int16(binary.BigEndian.Uint16(payload[4:6]))) / 1000,
			}
		case LPPGyrometer:
			reading.Value = LPPGyroValue{
				X: float64(int16(binary.BigEndian.Uint16(payload[0:2]))) / 100,
				Y: float64(int16(binary.BigEndian.Uint16(payload[2:4]))) / 100,
				Z: float64(int16(binary.BigEndian.Uint16(payload[4:6]))) / 100,
			}
		case LPPColour:
			reading.Value = LPPColourValue{R: payload[0], G: payload[1], B: payload[2]}
		case LPPGPS:
			reading.Value = LPPGPSValue{
				Latitude:  float64(decodeInt24(payload[0:3])) / 10000,
				Longitude: float64(decodeInt24(payload[3:6])) / 10000,
				Altitude:  float64(decodeInt24(payload[6:9])) / 100,
			}
		}

		readings = append(readings, reading)
	}

	return readings, nil
}

type LPPEncoder struct {
	buf bytes.Buffer
}

func NewLPPEncoder() *LPPEncoder {
	return &LPPEncoder{}
}

func (e *LPPEncoder) Bytes() []byte {
	return append([]byte(nil), e.buf.Bytes()...)
}

func (e *LPPEncoder) Reset() {
	e.buf.Reset()
}

func (e *LPPEncoder) writeHeader(channel, typ byte) {
	_ = e.buf.WriteByte(channel)
	_ = e.buf.WriteByte(typ)
}

func (e *LPPEncoder) writeUint16(v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	_, _ = e.buf.Write(b[:])
}

func (e *LPPEncoder) writeInt16(v int16) {
	e.writeUint16(uint16(v))
}

func (e *LPPEncoder) writeUint32(v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	_, _ = e.buf.Write(b[:])
}

func (e *LPPEncoder) writeInt24(v int32) {
	b := encodeInt24(v)
	_, _ = e.buf.Write(b[:])
}

func (e *LPPEncoder) AddDigitalInput(channel byte, value byte) {
	e.writeHeader(channel, LPPDigitalInput)
	_ = e.buf.WriteByte(value)
}

func (e *LPPEncoder) AddDigitalOutput(channel byte, value byte) {
	e.writeHeader(channel, LPPDigitalOutput)
	_ = e.buf.WriteByte(value)
}

func (e *LPPEncoder) AddAnalogInput(channel byte, value float64) {
	e.writeHeader(channel, LPPAnalogInput)
	e.writeInt16(int16(value * 100))
}

func (e *LPPEncoder) AddAnalogOutput(channel byte, value float64) {
	e.writeHeader(channel, LPPAnalogOutput)
	e.writeInt16(int16(value * 100))
}

func (e *LPPEncoder) AddGenericSensor(channel byte, value uint32) {
	e.writeHeader(channel, LPPGenericSensor)
	e.writeUint32(value)
}

func (e *LPPEncoder) AddLuminosity(channel byte, lux uint16) {
	e.writeHeader(channel, LPPLuminosity)
	e.writeUint16(lux)
}

func (e *LPPEncoder) AddPresence(channel byte, value byte) {
	e.writeHeader(channel, LPPPresence)
	_ = e.buf.WriteByte(value)
}

func (e *LPPEncoder) AddTemperature(channel byte, celsius float64) {
	e.writeHeader(channel, LPPTemperature)
	e.writeInt16(int16(celsius * 10))
}

func (e *LPPEncoder) AddRelativeHumidity(channel byte, rh float64) {
	e.writeHeader(channel, LPPRelativeHumidity)
	_ = e.buf.WriteByte(byte(rh * 2))
}

func (e *LPPEncoder) AddAccelerometer(channel byte, x, y, z float64) {
	e.writeHeader(channel, LPPAccelerometer)
	e.writeInt16(int16(x * 1000))
	e.writeInt16(int16(y * 1000))
	e.writeInt16(int16(z * 1000))
}

func (e *LPPEncoder) AddBarometricPressure(channel byte, hpa float64) {
	e.writeHeader(channel, LPPBarometricPressure)
	e.writeUint16(uint16(hpa * 10))
}

func (e *LPPEncoder) AddVoltage(channel byte, volts float64) {
	e.writeHeader(channel, LPPVoltage)
	e.writeInt16(int16(volts * 100))
}

func (e *LPPEncoder) AddCurrent(channel byte, amps float64) {
	e.writeHeader(channel, LPPCurrent)
	e.writeInt16(int16(amps * 1000))
}

func (e *LPPEncoder) AddFrequency(channel byte, hz uint32) {
	e.writeHeader(channel, LPPFrequency)
	e.writeUint32(hz)
}

func (e *LPPEncoder) AddPercentage(channel byte, pct byte) {
	e.writeHeader(channel, LPPPercentage)
	_ = e.buf.WriteByte(pct)
}

func (e *LPPEncoder) AddAltitude(channel byte, meters float64) {
	e.writeHeader(channel, LPPAltitude)
	e.writeInt16(int16(meters))
}

func (e *LPPEncoder) AddConcentration(channel byte, ppm uint16) {
	e.writeHeader(channel, LPPConcentration)
	e.writeUint16(ppm)
}

func (e *LPPEncoder) AddPower(channel byte, watts uint16) {
	e.writeHeader(channel, LPPPower)
	e.writeUint16(watts)
}

func (e *LPPEncoder) AddDistance(channel byte, meters float64) {
	e.writeHeader(channel, LPPDistance)
	e.writeUint32(uint32(meters * 1000))
}

func (e *LPPEncoder) AddEnergy(channel byte, kwh float64) {
	e.writeHeader(channel, LPPEnergy)
	e.writeUint32(uint32(kwh * 1000))
}

func (e *LPPEncoder) AddDirection(channel byte, degrees uint16) {
	e.writeHeader(channel, LPPDirection)
	e.writeUint16(degrees)
}

func (e *LPPEncoder) AddUnixTime(channel byte, timestamp uint32) {
	e.writeHeader(channel, LPPUnixTime)
	e.writeUint32(timestamp)
}

func (e *LPPEncoder) AddGyrometer(channel byte, x, y, z float64) {
	e.writeHeader(channel, LPPGyrometer)
	e.writeInt16(int16(x * 100))
	e.writeInt16(int16(y * 100))
	e.writeInt16(int16(z * 100))
}

func (e *LPPEncoder) AddColour(channel byte, r, g, b byte) {
	e.writeHeader(channel, LPPColour)
	_ = e.buf.WriteByte(r)
	_ = e.buf.WriteByte(g)
	_ = e.buf.WriteByte(b)
}

func (e *LPPEncoder) AddGPS(channel byte, lat, lon, alt float64) {
	e.writeHeader(channel, LPPGPS)
	e.writeInt24(int32(lat * 10000))
	e.writeInt24(int32(lon * 10000))
	e.writeInt24(int32(alt * 100))
}

func (e *LPPEncoder) AddSwitch(channel byte, value byte) {
	e.writeHeader(channel, LPPSwitch)
	_ = e.buf.WriteByte(value)
}

func encodeInt24(v int32) [3]byte {
	u := uint32(v) & 0x00FFFFFF
	return [3]byte{byte(u >> 16), byte(u >> 8), byte(u)}
}

func decodeInt24(b []byte) int32 {
	v := int32(b[0])<<16 | int32(b[1])<<8 | int32(b[2])
	if v&0x00800000 != 0 {
		v |= ^int32(0x00FFFFFF)
	}
	return v
}
