package meshcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
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
	LPPPolyline           byte = 240
)

// LPPMinPolylineSize is the smallest valid polyline payload: 1 size byte +
// 1 factor byte + 3 lat bytes + 3 lon bytes (no deltas).
const LPPMinPolylineSize = 8

// lppDefaultMaxSize matches the common ElectronicCats `CayenneLPP lpp(255)`
// buffer size. It only bounds polyline encoding (the polyline payload is
// capped at maxSize-2, mirroring the firmware).
const lppDefaultMaxSize = 255

// polylineScale is the fixed 0.0001° base resolution (ElectronicCats
// CayenneLPPPolyline scaleFactor).
const polylineScale = 10000.0

// LPPPrecision selects the polyline delta-compression precision. The values
// are the on-wire factor bytes (227-239) used by ElectronicCats; the inline
// resolution is shown alongside each.
type LPPPrecision byte

const (
	LPPPrec0_0001 LPPPrecision = 227 // 0.0001°
	LPPPrec0_0002 LPPPrecision = 228 // 0.0002°
	LPPPrec0_0005 LPPPrecision = 229 // 0.0005°
	LPPPrec0_001  LPPPrecision = 230 // 0.001°
	LPPPrec0_002  LPPPrecision = 231 // 0.002°
	LPPPrec0_005  LPPPrecision = 232 // 0.005°
	LPPPrec0_01   LPPPrecision = 233 // 0.01°
	LPPPrec0_02   LPPPrecision = 234 // 0.02°
	LPPPrec0_05   LPPPrecision = 235 // 0.05°
	LPPPrec0_1    LPPPrecision = 236 // 0.1°
	LPPPrec0_2    LPPPrecision = 237 // 0.2°
	LPPPrec0_5    LPPPrecision = 238 // 0.5°
	LPPPrec1_0    LPPPrecision = 239 // 1.0°
)

// LPPSimplification selects the polyline simplification algorithm applied
// before delta encoding.
type LPPSimplification byte

const (
	LPPSimplifyNone                  LPPSimplification = 0 // no simplification
	LPPSimplifyPerpendicularDistance LPPSimplification = 1 // fast inline merge
	LPPSimplifyDouglasPeucker        LPPSimplification = 2 // Douglas-Peucker (default)
)

// polylineValueMap maps the special precision factor bytes (>=227) to their
// quantization factor, mirroring ElectronicCats CayenneLPPPolyline s_valueMap.
var polylineValueMap = map[byte]float64{
	227: 1.0, 228: 2.0, 229: 5.0, 230: 10.0, 231: 20.0, 232: 50.0,
	233: 100.0, 234: 200.0, 235: 500.0, 236: 1000.0, 237: 2000.0,
	238: 5000.0, 239: 10000.0,
}

type LPPReading struct {
	Channel byte
	Type    byte
	Value   any
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

// LPPCoordinate is a single decoded polyline vertex.
type LPPCoordinate struct {
	Latitude  float64
	Longitude float64
}

// LPPPolylineValue holds a decoded polyline: the raw factor/precision byte and
// the reconstructed coordinate list.
type LPPPolylineValue struct {
	Factor      byte
	Coordinates []LPPCoordinate
}

func LPPDecode(data []byte) ([]LPPReading, error) {
	readings := make([]LPPReading, 0)

	for i := 0; i+2 < len(data); {
		channel := data[i]
		typ := data[i+1]
		i += 2

		// MeshCore's LPPReader treats channel 0 as the end-of-data marker
		// (any type), and telemetry channels start at 1 (TELEM_CHANNEL_SELF).
		if channel == 0 {
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
		case LPPPolyline:
			// Polyline is variable length: the first payload byte is the
			// total polyline size (including itself and the factor byte).
			if len(data)-i < 1 {
				return nil, fmt.Errorf("truncated lpp polyline for channel %d: missing size byte", channel)
			}
			need = int(data[i])
			if need < LPPMinPolylineSize {
				return nil, fmt.Errorf("invalid lpp polyline size %d for channel %d: minimum is %d", need, channel, LPPMinPolylineSize)
			}
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
		case LPPPolyline:
			reading.Value = decodePolyline(payload)
		}

		readings = append(readings, reading)
	}

	return readings, nil
}

type LPPEncoder struct {
	buf bytes.Buffer
	// maxSize mirrors the ElectronicCats `CayenneLPP(size)` buffer size. It
	// only bounds polyline encoding; the simple Add* methods are unbounded.
	maxSize int
}

func NewLPPEncoder() *LPPEncoder {
	return &LPPEncoder{maxSize: lppDefaultMaxSize}
}

// NewLPPEncoderSize creates an encoder whose polyline output matches an
// ElectronicCats `CayenneLPP(size)` instance (polyline payload capped at
// size-2 bytes). Use this when you need byte-identical polyline output for a
// specific firmware buffer size.
func NewLPPEncoderSize(size int) *LPPEncoder {
	return &LPPEncoder{maxSize: size}
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

// AddPolyline encodes a GPS track (>=2 coordinates) as an LPP polyline,
// byte-compatible with ElectronicCats CayenneLPP::addPolyline. The first point
// is stored at full 0.0001° resolution; subsequent points are stored as packed
// signed-nibble deltas at the chosen precision. Returns an error if fewer than
// 2 coordinates are given or the encoded record would overflow the encoder's
// max size.
func (e *LPPEncoder) AddPolyline(channel byte, coords []LPPCoordinate, precision LPPPrecision, simplification LPPSimplification) error {
	return e.addPolyline(channel, coords, byte(precision), simplification)
}

// AddPolylineFactor is like AddPolyline but takes a raw quantization factor
// (1-199, where 1 == 0.0001° and the resolution scales linearly) instead of a
// named precision. This mirrors the factor-based ElectronicCats encode overload.
func (e *LPPEncoder) AddPolylineFactor(channel byte, coords []LPPCoordinate, factor byte, simplification LPPSimplification) error {
	return e.addPolyline(channel, coords, factor, simplification)
}

func (e *LPPEncoder) addPolyline(channel byte, coords []LPPCoordinate, factor byte, simplification LPPSimplification) error {
	maxSize := e.maxSize
	if maxSize <= 0 {
		maxSize = lppDefaultMaxSize
	}

	enc := polylineEncoder{maxSize: maxSize - 2}
	payload := enc.encode(coords, factor, simplification)
	if payload == nil {
		return fmt.Errorf("cayenne: polyline needs at least 2 valid coordinates and a valid factor")
	}

	if e.buf.Len()+len(payload)+2 > maxSize {
		return fmt.Errorf("cayenne: polyline overflow: need %d bytes, max %d", e.buf.Len()+len(payload)+2, maxSize)
	}

	e.writeHeader(channel, LPPPolyline)
	_, _ = e.buf.Write(payload)
	return nil
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

// polylineFactor maps a factor/precision byte to its quantization factor,
// mirroring ElectronicCats CayenneLPPPolyline::getFactor. Returns 0 for
// invalid factors (0, or reserved values >=200 not in the precision map).
func polylineFactor(factor byte) float64 {
	switch {
	case factor == 0:
		return 0.0
	case factor < 200:
		return float64(factor)
	default:
		return polylineValueMap[factor] // 0.0 if absent
	}
}

// packDelta packs two signed 4-bit deltas into one byte, matching the
// little-endian DeltaCoord{int8_t dLat:4; int8_t dLon:4;} layout used by
// ElectronicCats on mainstream targets (dLat in the low nibble).
func packDelta(dLat, dLon int) byte {
	return byte((dLon&0x0F)<<4 | (dLat & 0x0F))
}

// unpackDelta is the inverse of packDelta, sign-extending each nibble.
func unpackDelta(b byte) (dLat, dLon int) {
	dLat = int(b & 0x0F)
	if dLat >= 8 {
		dLat -= 16
	}
	dLon = int(b>>4) & 0x0F
	if dLon >= 8 {
		dLon -= 16
	}
	return dLat, dLon
}

func lppAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// decodePolyline reconstructs a polyline from its on-wire payload (including
// the leading size byte), mirroring ElectronicCats CayenneLPPPolyline::decode.
func decodePolyline(buf []byte) LPPPolylineValue {
	var out LPPPolylineValue
	if len(buf) < 7 {
		return out
	}

	out.Factor = buf[1]
	dFactor := polylineFactor(buf[1])
	if dFactor == 0.0 {
		return out
	}

	// One coordinate per delta byte (buf[8:]) plus the initial point.
	out.Coordinates = make([]LPPCoordinate, 0, max(len(buf)-7, 1))

	// The initial lat/lon are signed 24-bit big-endian values (same layout as
	// decodeInt24), scaled by the factor.
	prevLat := int32(float64(decodeInt24(buf[2:5])) * dFactor)
	prevLon := int32(float64(decodeInt24(buf[5:8])) * dFactor)

	out.Coordinates = append(out.Coordinates, LPPCoordinate{
		Latitude:  float64(prevLat) / polylineScale,
		Longitude: float64(prevLon) / polylineScale,
	})

	if len(buf) == 7 {
		return out
	}

	for _, b := range buf[8:] {
		dLatN, dLonN := unpackDelta(b)
		prevLat += int32(float64(dLatN) * dFactor)
		prevLon += int32(float64(dLonN) * dFactor)
		out.Coordinates = append(out.Coordinates, LPPCoordinate{
			Latitude:  float64(prevLat) / polylineScale,
			Longitude: float64(prevLon) / polylineScale,
		})
	}

	return out
}

// polylineEncoder is a faithful port of ElectronicCats CayenneLPPPolyline's
// stateful encoder (error-feedback delta quantization with intermediate-point
// insertion and optional perpendicular-distance merging).
type polylineEncoder struct {
	buf     []byte
	maxSize int

	prevLat float64
	prevLon float64
	errLat  float64
	errLon  float64
}

// encode mirrors CayenneLPPPolyline::encode(coords, factor, simplification).
// Returns nil for fewer than 2 coordinates or an invalid factor.
func (p *polylineEncoder) encode(coords []LPPCoordinate, factor byte, simplification LPPSimplification) []byte {
	p.buf = p.buf[:0]
	p.prevLat, p.prevLon, p.errLat, p.errLon = 0, 0, 0, 0

	if len(coords) < 2 {
		return nil
	}
	dFactor := polylineFactor(factor)
	if dFactor == 0.0 {
		return nil
	}

	coords2 := coords
	if simplification == LPPSimplifyDouglasPeucker {
		coords2 = douglasPeucker(coords, dFactor/polylineScale*0.5)
	}
	if len(coords2) < 2 {
		return nil
	}

	optimize := simplification == LPPSimplifyPerpendicularDistance

	// The initial point is encoded twice: once to seed the encoder and again
	// at the end so the rewritten header byte[0] carries the final length.
	lat0 := coords2[0].Latitude * polylineScale / dFactor
	lon0 := coords2[0].Longitude * polylineScale / dFactor

	p.pushFirst(lat0, lon0, factor)
	for i := 1; i < len(coords2) && len(p.buf) < p.maxSize; i++ {
		c := coords2[i]
		if math.Abs(c.Latitude) > 90.0 || math.Abs(c.Longitude) > 180.0 {
			break
		}
		p.push(c.Latitude*polylineScale/dFactor, c.Longitude*polylineScale/dFactor, optimize)
	}
	p.pushFirst(lat0, lon0, factor)

	return p.buf
}

func (p *polylineEncoder) pushFirst(lat, lon float64, factor byte) {
	roundLat := int32(math.Round(lat))
	roundLon := int32(math.Round(lon))
	p.writeHeader(roundLat, roundLon, factor)
	p.errLat = lat - float64(roundLat)
	p.errLon = lon - float64(roundLon)
	p.prevLat = lat
	p.prevLon = lon
}

func (p *polylineEncoder) writeHeader(lat, lon int32, factor byte) {
	for len(p.buf) < LPPMinPolylineSize {
		p.buf = append(p.buf, 0)
	}
	p.buf[0] = byte(len(p.buf))
	p.buf[1] = factor
	la := encodeInt24(lat)
	lo := encodeInt24(lon)
	copy(p.buf[2:5], la[:])
	copy(p.buf[5:8], lo[:])
}

func (p *polylineEncoder) push(lat, lon float64, optimize bool) {
	// Defensive bound: the intermediate-point recursion below can otherwise
	// append unbounded deltas for pathologically distant points (overflowing
	// the 1-byte size field). Realistic tracks never reach maxSize here, so
	// this never alters output that the reference encoder can also produce.
	if len(p.buf) >= p.maxSize {
		return
	}

	dLat := (lat - p.prevLat) + p.errLat
	dLon := (lon - p.prevLon) + p.errLon

	roundLat := int(math.Round(dLat))
	roundLon := int(math.Round(dLon))

	switch {
	case lppAbs(roundLat) < 1 && lppAbs(roundLon) < 1:
		// Zero delta after rounding: drop the point but keep the error.
	case lppAbs(roundLat) < 8 && lppAbs(roundLon) < 8:
		p.writeDelta(roundLat, roundLon, optimize)
	default:
		// Delta too large for one nibble: insert an intermediate point.
		divisor := math.Ceil(math.Max(math.Abs(dLat/7.0), math.Abs(dLon/7.0)))
		p.push(p.prevLat+dLat/divisor, p.prevLon+dLon/divisor, optimize)
		p.push(lat, lon, optimize)
		return
	}

	p.errLat = dLat - float64(roundLat)
	p.errLon = dLon - float64(roundLon)
	p.prevLat = lat
	p.prevLon = lon
}

func (p *polylineEncoder) writeDelta(dLat, dLon int, optimize bool) {
	if optimize && len(p.buf) > LPPMinPolylineSize {
		prevDLat, prevDLon := unpackDelta(p.buf[len(p.buf)-1])
		sumLat := prevDLat + dLat
		sumLon := prevDLon + dLon
		if lppAbs(sumLat) < 8 && lppAbs(sumLon) < 8 {
			denom := math.Sqrt(float64(sumLat*sumLat + sumLon*sumLon))
			dist := math.Abs(float64(sumLat)*-1.0*float64(prevDLon)+float64(prevDLat)*float64(sumLon)) / denom
			if dist < 0.5 {
				p.buf[len(p.buf)-1] = packDelta(sumLat, sumLon)
				return
			}
		}
	}
	p.buf = append(p.buf, packDelta(dLat, dLon))
}

// douglasPeucker simplifies a coordinate list, mirroring
// CayenneLPPPolyline::douglasPeucker.
func douglasPeucker(points []LPPCoordinate, epsilon float64) []LPPCoordinate {
	if len(points) < 2 {
		return nil
	}

	dmax := 0.0
	index := 0
	end := len(points) - 1
	for i := 1; i < end; i++ {
		d := perpendicularDistance(points[i], points[0], points[end])
		if d > dmax {
			index = i
			dmax = d
		}
	}

	if dmax > epsilon {
		rec1 := douglasPeucker(points[:index+1], epsilon)
		rec2 := douglasPeucker(points[index:], epsilon)

		out := make([]LPPCoordinate, 0, len(rec1)+len(rec2))
		if len(rec1) > 0 {
			out = append(out, rec1[:len(rec1)-1]...)
		}
		out = append(out, rec2...)
		if len(out) < 2 {
			return nil
		}
		return out
	}

	return []LPPCoordinate{points[0], points[end]}
}

// perpendicularDistance is the point-to-line distance helper used by
// douglasPeucker, mirroring CayenneLPPPolyline::distance.
func perpendicularDistance(point, lineStart, lineEnd LPPCoordinate) float64 {
	dLat := lineEnd.Latitude - lineStart.Latitude
	dLon := lineEnd.Longitude - lineStart.Longitude

	mag := math.Sqrt(dLat*dLat + dLon*dLon)
	if mag > 0.0 {
		dLat /= mag
		dLon /= mag
	}

	pvx := point.Latitude - lineStart.Latitude
	pvy := point.Longitude - lineStart.Longitude

	pvdot := dLat*pvx + dLon*pvy
	dsx := pvdot * dLat
	dsy := pvdot * dLon

	ax := pvx - dsx
	ay := pvy - dsy

	return math.Sqrt(ax*ax + ay*ay)
}
