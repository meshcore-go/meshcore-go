package meshcore

import (
	"encoding/hex"
	"math"
	"strings"
	"testing"
)

func TestLPPDecode(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		want    []LPPReading
		wantErr bool
	}{
		{name: "empty input", hex: "", want: []LPPReading{}},
		{name: "digital input", hex: "010007", want: []LPPReading{{Channel: 1, Type: LPPDigitalInput, Value: float64(7)}}},
		{name: "digital output", hex: "010109", want: []LPPReading{{Channel: 1, Type: LPPDigitalOutput, Value: float64(9)}}},
		{name: "analog input signed", hex: "0102FB2E", want: []LPPReading{{Channel: 1, Type: LPPAnalogInput, Value: -12.34}}},
		{name: "analog output signed", hex: "010311D7", want: []LPPReading{{Channel: 1, Type: LPPAnalogOutput, Value: 45.67}}},
		{name: "generic sensor", hex: "0164075BCD15", want: []LPPReading{{Channel: 1, Type: LPPGenericSensor, Value: float64(123456789)}}},
		{name: "luminosity", hex: "0165012C", want: []LPPReading{{Channel: 1, Type: LPPLuminosity, Value: float64(300)}}},
		{name: "presence", hex: "016601", want: []LPPReading{{Channel: 1, Type: LPPPresence, Value: float64(1)}}},
		{name: "temperature positive", hex: "016700D7", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 21.5}}},
		{name: "temperature negative", hex: "0267FFC8", want: []LPPReading{{Channel: 2, Type: LPPTemperature, Value: -5.6}}},
		{name: "relative humidity", hex: "01686F", want: []LPPReading{{Channel: 1, Type: LPPRelativeHumidity, Value: 55.5}}},
		{name: "accelerometer", hex: "017104D2F63C0001", want: []LPPReading{{Channel: 1, Type: LPPAccelerometer, Value: LPPAccelValue{X: 1.234, Y: -2.5, Z: 0.001}}}},
		{name: "barometric pressure", hex: "01732794", want: []LPPReading{{Channel: 1, Type: LPPBarometricPressure, Value: 1013.2}}},
		{name: "voltage signed", hex: "0174FB2E", want: []LPPReading{{Channel: 1, Type: LPPVoltage, Value: -12.34}}},
		{name: "current", hex: "0175FEBF", want: []LPPReading{{Channel: 1, Type: LPPCurrent, Value: -0.321}}},
		{name: "frequency", hex: "01760000C350", want: []LPPReading{{Channel: 1, Type: LPPFrequency, Value: float64(50000)}}},
		{name: "percentage", hex: "017855", want: []LPPReading{{Channel: 1, Type: LPPPercentage, Value: float64(85)}}},
		{name: "altitude", hex: "0179FF88", want: []LPPReading{{Channel: 1, Type: LPPAltitude, Value: float64(-120)}}},
		{name: "concentration", hex: "017D01C2", want: []LPPReading{{Channel: 1, Type: LPPConcentration, Value: float64(450)}}},
		{name: "power", hex: "0180028F", want: []LPPReading{{Channel: 1, Type: LPPPower, Value: float64(655)}}},
		{name: "distance", hex: "018200003039", want: []LPPReading{{Channel: 1, Type: LPPDistance, Value: 12.345}}},
		{name: "energy", hex: "018300010932", want: []LPPReading{{Channel: 1, Type: LPPEnergy, Value: 67.89}}},
		{name: "direction", hex: "0184010E", want: []LPPReading{{Channel: 1, Type: LPPDirection, Value: float64(270)}}},
		{name: "unix time", hex: "01856553F100", want: []LPPReading{{Channel: 1, Type: LPPUnixTime, Value: float64(1700000000)}}},
		{name: "gyrometer", hex: "018604D2E9D20000", want: []LPPReading{{Channel: 1, Type: LPPGyrometer, Value: LPPGyroValue{X: 12.34, Y: -56.78, Z: 0}}}},
		{name: "colour", hex: "0187010203", want: []LPPReading{{Channel: 1, Type: LPPColour, Value: LPPColourValue{R: 1, G: 2, B: 3}}}},
		{name: "gps", hex: "0188003039FFA460003039", want: []LPPReading{{Channel: 1, Type: LPPGPS, Value: LPPGPSValue{Latitude: 1.2345, Longitude: -2.3456, Altitude: 123.45}}}},
		{name: "switch", hex: "018E01", want: []LPPReading{{Channel: 1, Type: LPPSwitch, Value: float64(1)}}},
		{name: "end marker stops parsing", hex: "016700D70000026864", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 21.5}}},
		{name: "channel zero any type stops parsing", hex: "016700C8006764", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 20.0}}},
		{name: "multi sensor payload", hex: "016700C80268640374014A", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 20.0}, {Channel: 2, Type: LPPRelativeHumidity, Value: 50.0}, {Channel: 3, Type: LPPVoltage, Value: 3.3}}},
		{name: "polyline two points", hex: "05F009E3002710004E20E3", want: []LPPReading{{Channel: 5, Type: LPPPolyline, Value: LPPPolylineValue{Factor: 227, Coordinates: []LPPCoordinate{{Latitude: 1.0, Longitude: 2.0}, {Latitude: 1.0003, Longitude: 1.9998}}}}}},
		{name: "polyline mid payload", hex: "016700C805F009E3002710004E20E30374014A", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 20.0}, {Channel: 5, Type: LPPPolyline, Value: LPPPolylineValue{Factor: 227, Coordinates: []LPPCoordinate{{Latitude: 1.0, Longitude: 2.0}, {Latitude: 1.0003, Longitude: 1.9998}}}}, {Channel: 3, Type: LPPVoltage, Value: 3.3}}},
		{name: "polyline bad size", hex: "05F005E300271000", wantErr: true},
		{name: "truncated input", hex: "016700", wantErr: true},
		{name: "partial header is end of data", hex: "01", want: []LPPReading{}},
		{name: "dangling header is end of data", hex: "016700C8" + "0268", want: []LPPReading{{Channel: 1, Type: LPPTemperature, Value: 20.0}}},
		{name: "unknown type", hex: "01FF00", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			got, err := LPPDecode(data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(readings) = %d, want %d", len(got), len(tt.want))
			}

			for i := range got {
				assertReadingEqual(t, got[i], tt.want[i])
			}
		})
	}
}

func TestLPPEncode(t *testing.T) {
	tests := []struct {
		name    string
		build   func(e *LPPEncoder)
		wantHex string
	}{
		{name: "digital input", build: func(e *LPPEncoder) { e.AddDigitalInput(1, 7) }, wantHex: "010007"},
		{name: "digital output", build: func(e *LPPEncoder) { e.AddDigitalOutput(1, 9) }, wantHex: "010109"},
		{name: "analog input", build: func(e *LPPEncoder) { e.AddAnalogInput(1, -12.34) }, wantHex: "0102FB2E"},
		{name: "analog output", build: func(e *LPPEncoder) { e.AddAnalogOutput(1, 45.67) }, wantHex: "010311D7"},
		{name: "generic sensor", build: func(e *LPPEncoder) { e.AddGenericSensor(1, 123456789) }, wantHex: "0164075BCD15"},
		{name: "luminosity", build: func(e *LPPEncoder) { e.AddLuminosity(1, 300) }, wantHex: "0165012C"},
		{name: "presence", build: func(e *LPPEncoder) { e.AddPresence(1, 1) }, wantHex: "016601"},
		{name: "temperature negative", build: func(e *LPPEncoder) { e.AddTemperature(1, -5.6) }, wantHex: "0167FFC8"},
		{name: "humidity", build: func(e *LPPEncoder) { e.AddRelativeHumidity(1, 55.5) }, wantHex: "01686F"},
		{name: "accelerometer", build: func(e *LPPEncoder) { e.AddAccelerometer(1, 1.234, -2.5, 0.001) }, wantHex: "017104D2F63C0001"},
		{name: "barometric pressure", build: func(e *LPPEncoder) { e.AddBarometricPressure(1, 1013.2) }, wantHex: "01732794"},
		{name: "voltage", build: func(e *LPPEncoder) { e.AddVoltage(1, -12.34) }, wantHex: "0174FB2E"},
		{name: "current", build: func(e *LPPEncoder) { e.AddCurrent(1, -0.321) }, wantHex: "0175FEBF"},
		{name: "frequency", build: func(e *LPPEncoder) { e.AddFrequency(1, 50000) }, wantHex: "01760000C350"},
		{name: "percentage", build: func(e *LPPEncoder) { e.AddPercentage(1, 85) }, wantHex: "017855"},
		{name: "altitude", build: func(e *LPPEncoder) { e.AddAltitude(1, -120) }, wantHex: "0179FF88"},
		{name: "concentration", build: func(e *LPPEncoder) { e.AddConcentration(1, 450) }, wantHex: "017D01C2"},
		{name: "power", build: func(e *LPPEncoder) { e.AddPower(1, 655) }, wantHex: "0180028F"},
		{name: "distance", build: func(e *LPPEncoder) { e.AddDistance(1, 12.345) }, wantHex: "018200003039"},
		{name: "energy", build: func(e *LPPEncoder) { e.AddEnergy(1, 67.89) }, wantHex: "018300010932"},
		{name: "direction", build: func(e *LPPEncoder) { e.AddDirection(1, 270) }, wantHex: "0184010E"},
		{name: "unix time", build: func(e *LPPEncoder) { e.AddUnixTime(1, 1700000000) }, wantHex: "01856553F100"},
		{name: "gyrometer", build: func(e *LPPEncoder) { e.AddGyrometer(1, 12.34, -56.78, 0) }, wantHex: "018604D2E9D20000"},
		{name: "colour", build: func(e *LPPEncoder) { e.AddColour(1, 1, 2, 3) }, wantHex: "0187010203"},
		{name: "gps", build: func(e *LPPEncoder) { e.AddGPS(1, 1.2345, -2.3456, 123.45) }, wantHex: "0188003039FFA460003039"},
		{name: "switch", build: func(e *LPPEncoder) { e.AddSwitch(1, 1) }, wantHex: "018E01"},
		{name: "multi-value", build: func(e *LPPEncoder) {
			e.AddTemperature(1, 20)
			e.AddRelativeHumidity(2, 50)
			e.AddVoltage(3, 3.3)
		}, wantHex: "016700C80268640374014A"},
		{name: "polyline two points", build: func(e *LPPEncoder) {
			_ = e.AddPolyline(5, []LPPCoordinate{{Latitude: 1.0, Longitude: 2.0}, {Latitude: 1.0003, Longitude: 1.9998}}, LPPPrec0_0001, LPPSimplifyNone)
		}, wantHex: "05F009E3002710004E20E3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewLPPEncoder()
			tt.build(e)

			gotHex := hex.EncodeToString(e.Bytes())
			if !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("Bytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestLPPRoundTrip(t *testing.T) {
	e := NewLPPEncoder()
	e.AddTemperature(1, -2.3)
	e.AddRelativeHumidity(2, 57.5)
	e.AddVoltage(3, -3.14)
	e.AddGPS(4, 1.2345, -2.3456, 123.45)
	e.AddAccelerometer(5, 1.234, -2.5, 0.001)

	readings, err := LPPDecode(e.Bytes())
	if err != nil {
		t.Fatalf("LPPDecode(): %v", err)
	}

	want := []LPPReading{
		{Channel: 1, Type: LPPTemperature, Value: -2.3},
		{Channel: 2, Type: LPPRelativeHumidity, Value: 57.5},
		{Channel: 3, Type: LPPVoltage, Value: -3.14},
		{Channel: 4, Type: LPPGPS, Value: LPPGPSValue{Latitude: 1.2345, Longitude: -2.3456, Altitude: 123.45}},
		{Channel: 5, Type: LPPAccelerometer, Value: LPPAccelValue{X: 1.234, Y: -2.5, Z: 0.001}},
	}

	if len(readings) != len(want) {
		t.Fatalf("len(readings) = %d, want %d", len(readings), len(want))
	}

	for i := range readings {
		assertReadingEqual(t, readings[i], want[i])
	}
}

func TestLPPPolylineRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		coords     []LPPCoordinate
		precision  LPPPrecision
		simplify   LPPSimplification
		wantFactor byte
		minCoords  int // decoded coords should be at least this many
		tolerance  float64
	}{
		{
			name:       "two points full precision",
			coords:     []LPPCoordinate{{Latitude: 1.0, Longitude: 2.0}, {Latitude: 1.0003, Longitude: 1.9998}},
			precision:  LPPPrec0_0001,
			simplify:   LPPSimplifyNone,
			wantFactor: 227,
			minCoords:  2,
			tolerance:  0.0001,
		},
		{
			name: "short track douglas-peucker",
			coords: []LPPCoordinate{
				{Latitude: 52.5200, Longitude: 13.4050},
				{Latitude: 52.5203, Longitude: 13.4052},
				{Latitude: 52.5206, Longitude: 13.4055},
				{Latitude: 52.5209, Longitude: 13.4058},
			},
			precision:  LPPPrec0_0001,
			simplify:   LPPSimplifyDouglasPeucker,
			wantFactor: 227,
			minCoords:  2,
			tolerance:  0.0002,
		},
		{
			name: "track lower precision factor",
			coords: []LPPCoordinate{
				{Latitude: 40.0000, Longitude: -74.0000},
				{Latitude: 40.0020, Longitude: -74.0020},
				{Latitude: 40.0040, Longitude: -74.0040},
			},
			precision:  LPPPrec0_001,
			simplify:   LPPSimplifyNone,
			wantFactor: 230,
			minCoords:  2,
			tolerance:  0.002,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewLPPEncoder()
			if err := e.AddPolyline(7, tt.coords, tt.precision, tt.simplify); err != nil {
				t.Fatalf("AddPolyline(): %v", err)
			}

			readings, err := LPPDecode(e.Bytes())
			if err != nil {
				t.Fatalf("LPPDecode(): %v", err)
			}
			if len(readings) != 1 {
				t.Fatalf("len(readings) = %d, want 1", len(readings))
			}
			if readings[0].Channel != 7 || readings[0].Type != LPPPolyline {
				t.Fatalf("got channel %d type %d, want 7/%d", readings[0].Channel, readings[0].Type, LPPPolyline)
			}

			pl, ok := readings[0].Value.(LPPPolylineValue)
			if !ok {
				t.Fatalf("value type = %T, want LPPPolylineValue", readings[0].Value)
			}
			if pl.Factor != tt.wantFactor {
				t.Errorf("factor = %d, want %d", pl.Factor, tt.wantFactor)
			}
			if len(pl.Coordinates) < tt.minCoords {
				t.Fatalf("decoded %d coords, want >= %d", len(pl.Coordinates), tt.minCoords)
			}

			// First and last decoded points must track the input within the
			// chosen precision's tolerance.
			first, last := pl.Coordinates[0], pl.Coordinates[len(pl.Coordinates)-1]
			wantFirst, wantLast := tt.coords[0], tt.coords[len(tt.coords)-1]
			if math.Abs(first.Latitude-wantFirst.Latitude) > tt.tolerance || math.Abs(first.Longitude-wantFirst.Longitude) > tt.tolerance {
				t.Errorf("first coord = %#v, want ~%#v", first, wantFirst)
			}
			if math.Abs(last.Latitude-wantLast.Latitude) > tt.tolerance || math.Abs(last.Longitude-wantLast.Longitude) > tt.tolerance {
				t.Errorf("last coord = %#v, want ~%#v", last, wantLast)
			}
		})
	}
}

func TestLPPPolylineErrors(t *testing.T) {
	e := NewLPPEncoder()

	if err := e.AddPolyline(1, []LPPCoordinate{{Latitude: 1, Longitude: 2}}, LPPPrec0_0001, LPPSimplifyDouglasPeucker); err == nil {
		t.Error("expected error for single-coordinate polyline")
	}
	if err := e.AddPolyline(1, nil, LPPPrec0_0001, LPPSimplifyNone); err == nil {
		t.Error("expected error for empty polyline")
	}
	if got := len(e.Bytes()); got != 0 {
		t.Fatalf("encoder wrote %d bytes on error, want 0", got)
	}

	// Overflow: a tiny buffer cannot hold even a minimal polyline record.
	small := NewLPPEncoderSize(8)
	err := small.AddPolyline(1, []LPPCoordinate{{Latitude: 1, Longitude: 2}, {Latitude: 1.001, Longitude: 2.001}}, LPPPrec0_0001, LPPSimplifyNone)
	if err == nil {
		t.Error("expected overflow error for undersized encoder")
	}
}

func TestLPPEncoderReset(t *testing.T) {
	e := NewLPPEncoder()
	e.AddTemperature(1, 12.3)

	if got := len(e.Bytes()); got == 0 {
		t.Fatal("expected encoded bytes before reset")
	}

	e.Reset()

	if got := len(e.Bytes()); got != 0 {
		t.Fatalf("len(Bytes()) after Reset = %d, want 0", got)
	}
}

func TestEncodeDecodeInt24(t *testing.T) {
	tests := []struct {
		name    string
		value   int32
		wantHex string
	}{
		{name: "zero", value: 0, wantHex: "000000"},
		{name: "positive", value: 123456, wantHex: "01E240"},
		{name: "negative", value: -123456, wantHex: "FE1DC0"},
		{name: "minus one", value: -1, wantHex: "FFFFFF"},
		{name: "max positive", value: 8388607, wantHex: "7FFFFF"},
		{name: "min negative", value: -8388608, wantHex: "800000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeInt24(tt.value)
			gotHex := hex.EncodeToString(encoded[:])
			if !strings.EqualFold(gotHex, tt.wantHex) {
				t.Fatalf("encodeInt24(%d) = %s, want %s", tt.value, gotHex, tt.wantHex)
			}

			decoded := decodeInt24(encoded[:])
			if decoded != tt.value {
				t.Errorf("decodeInt24(encodeInt24(%d)) = %d, want %d", tt.value, decoded, tt.value)
			}
		})
	}
}

func assertReadingEqual(t *testing.T, got, want LPPReading) {
	t.Helper()

	if got.Channel != want.Channel {
		t.Fatalf("channel = %d, want %d", got.Channel, want.Channel)
	}
	if got.Type != want.Type {
		t.Fatalf("type = %d, want %d", got.Type, want.Type)
	}

	switch wantValue := want.Value.(type) {
	case float64:
		gotValue, ok := got.Value.(float64)
		if !ok {
			t.Fatalf("value type = %T, want float64", got.Value)
		}
		if math.Abs(gotValue-wantValue) > 1e-6 {
			t.Fatalf("value = %v, want %v", gotValue, wantValue)
		}
	case LPPGPSValue:
		gotValue, ok := got.Value.(LPPGPSValue)
		if !ok {
			t.Fatalf("value type = %T, want LPPGPSValue", got.Value)
		}
		if math.Abs(gotValue.Latitude-wantValue.Latitude) > 1e-6 || math.Abs(gotValue.Longitude-wantValue.Longitude) > 1e-6 || math.Abs(gotValue.Altitude-wantValue.Altitude) > 1e-6 {
			t.Fatalf("value = %#v, want %#v", gotValue, wantValue)
		}
	case LPPAccelValue:
		gotValue, ok := got.Value.(LPPAccelValue)
		if !ok {
			t.Fatalf("value type = %T, want LPPAccelValue", got.Value)
		}
		if math.Abs(gotValue.X-wantValue.X) > 1e-6 || math.Abs(gotValue.Y-wantValue.Y) > 1e-6 || math.Abs(gotValue.Z-wantValue.Z) > 1e-6 {
			t.Fatalf("value = %#v, want %#v", gotValue, wantValue)
		}
	case LPPGyroValue:
		gotValue, ok := got.Value.(LPPGyroValue)
		if !ok {
			t.Fatalf("value type = %T, want LPPGyroValue", got.Value)
		}
		if math.Abs(gotValue.X-wantValue.X) > 1e-6 || math.Abs(gotValue.Y-wantValue.Y) > 1e-6 || math.Abs(gotValue.Z-wantValue.Z) > 1e-6 {
			t.Fatalf("value = %#v, want %#v", gotValue, wantValue)
		}
	case LPPColourValue:
		gotValue, ok := got.Value.(LPPColourValue)
		if !ok {
			t.Fatalf("value type = %T, want LPPColourValue", got.Value)
		}
		if gotValue != wantValue {
			t.Fatalf("value = %#v, want %#v", gotValue, wantValue)
		}
	case LPPPolylineValue:
		gotValue, ok := got.Value.(LPPPolylineValue)
		if !ok {
			t.Fatalf("value type = %T, want LPPPolylineValue", got.Value)
		}
		if gotValue.Factor != wantValue.Factor {
			t.Fatalf("polyline factor = %d, want %d", gotValue.Factor, wantValue.Factor)
		}
		if len(gotValue.Coordinates) != len(wantValue.Coordinates) {
			t.Fatalf("polyline coords len = %d, want %d", len(gotValue.Coordinates), len(wantValue.Coordinates))
		}
		for i := range wantValue.Coordinates {
			if math.Abs(gotValue.Coordinates[i].Latitude-wantValue.Coordinates[i].Latitude) > 1e-6 ||
				math.Abs(gotValue.Coordinates[i].Longitude-wantValue.Coordinates[i].Longitude) > 1e-6 {
				t.Fatalf("polyline coord[%d] = %#v, want %#v", i, gotValue.Coordinates[i], wantValue.Coordinates[i])
			}
		}
	default:
		t.Fatalf("unsupported want value type %T", want.Value)
	}
}

func TestLPPPolyline_ExtendedDelta(t *testing.T) {
	e := NewLPPEncoder()
	coords := []LPPCoordinate{{Latitude: 10, Longitude: 20}, {Latitude: 15, Longitude: 25}}
	for _, simp := range []LPPSimplification{LPPSimplifyNone, LPPSimplifyPerpendicularDistance} {
		e.Reset()
		if err := e.AddPolyline(1, coords, LPPPrec0_0001, simp); err != nil {
			t.Fatalf("simplification %d: %v", simp, err)
		}
		b := e.Bytes()
		if len(b) > lppDefaultMaxSize || int(b[2]) != len(b)-2 {
			t.Fatalf("simplification %d: len %d, size byte %d", simp, len(b), b[2])
		}
		rs, err := LPPDecode(b)
		if err != nil || len(rs) != 1 {
			t.Fatalf("decode: %v (%d readings)", err, len(rs))
		}
		pl := rs[0].Value.(LPPPolylineValue)
		if pl.Factor != byte(LPPPrec0_0001) || len(pl.Coordinates) != len(b)-2-7 {
			t.Fatalf("factor %d, %d coords for %d delta bytes", pl.Factor, len(pl.Coordinates), len(b)-2-8)
		}
		if pl.Coordinates[0] != (LPPCoordinate{Latitude: 10, Longitude: 20}) {
			t.Fatalf("first coord %+v", pl.Coordinates[0])
		}
		for i := 1; i < len(pl.Coordinates); i++ {
			dLat := pl.Coordinates[i].Latitude - pl.Coordinates[i-1].Latitude
			dLon := pl.Coordinates[i].Longitude - pl.Coordinates[i-1].Longitude
			if dLat <= 0 || dLat > 0.00071 || dLon <= 0 || dLon > 0.00071 {
				t.Fatalf("step %d: dLat %g dLon %g", i, dLat, dLon)
			}
		}
	}
}

func TestLPPPolyline_PerpendicularDistanceMerge(t *testing.T) {
	coords := []LPPCoordinate{
		{Latitude: 1.0000, Longitude: 2.0000},
		{Latitude: 1.0002, Longitude: 2.0002},
		{Latitude: 1.0004, Longitude: 2.0004},
		{Latitude: 1.0006, Longitude: 2.0006},
	}
	plain, merged := NewLPPEncoder(), NewLPPEncoder()
	if err := plain.AddPolyline(1, coords, LPPPrec0_0001, LPPSimplifyNone); err != nil {
		t.Fatal(err)
	}
	if err := merged.AddPolyline(1, coords, LPPPrec0_0001, LPPSimplifyPerpendicularDistance); err != nil {
		t.Fatal(err)
	}
	if len(plain.Bytes()) != 2+8+3 || len(merged.Bytes()) != 2+8+1 {
		t.Fatalf("plain %d bytes, merged %d bytes", len(plain.Bytes()), len(merged.Bytes()))
	}
	rs, err := LPPDecode(merged.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	pl := rs[0].Value.(LPPPolylineValue)
	last := pl.Coordinates[len(pl.Coordinates)-1]
	if math.Abs(last.Latitude-1.0006) > 1e-9 || math.Abs(last.Longitude-2.0006) > 1e-9 {
		t.Fatalf("merged end point %+v", last)
	}
}

func FuzzLPPDecode(f *testing.F) {
	for _, h := range []string{
		"016700C80268640374014A", "05F009E3002710004E20E3", "0188003039FFA460003039",
		"016700C805F009E3002710004E20E30374014A", "05F005E300271000", "01FF00", "",
	} {
		b, _ := hex.DecodeString(h)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		rs, err := LPPDecode(data)
		if err != nil {
			return
		}
		for _, r := range rs {
			if r.Channel == 0 {
				t.Fatal("channel 0 is the end marker and must not be returned")
			}
			if pl, ok := r.Value.(LPPPolylineValue); ok && r.Type != LPPPolyline {
				t.Fatalf("polyline value on type %d: %+v", r.Type, pl)
			}
		}
	})
}
