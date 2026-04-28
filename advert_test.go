package meshcore

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

func TestAdvertFromBytes(t *testing.T) {
	tests := []struct {
		name          string
		hex           string // full advert payload (pubkey + timestamp + signature + appdata)
		wantErr       bool
		wantPubKey    string // hex, 32 bytes
		wantTimestamp uint32
		wantSignature string // hex, 64 bytes
		wantAppData   string // hex, raw app data
		wantType      string
		wantName      string
		wantLat       int32
		wantLon       int32
		wantFeat1     uint16
		wantFeat2     uint16
		wantVerified  bool
	}{
		{
			name:          "repeater advert with latlon and name",
			hex:           "D7B4B39CCF9F10D3B0D0597C0E5F0D5886B8DF23C7F8AD0CC702F932B9A3BA86C0A4C369C3CE366496C4C607A8DFBAA80B0ED295B67222F55D6AC91EEEE6F38ED5F07A741B80A58B565DCA0263EBF2431D0528F57822EA5D2E0F6737CEE22F23D19D95069204F8C4FD4D4F7C0AF09F92A54645524755532054524947",
			wantPubKey:    "D7B4B39CCF9F10D3B0D0597C0E5F0D5886B8DF23C7F8AD0CC702F932B9A3BA86",
			wantTimestamp: 1774429376,
			wantSignature: "C3CE366496C4C607A8DFBAA80B0ED295B67222F55D6AC91EEEE6F38ED5F07A741B80A58B565DCA0263EBF2431D0528F57822EA5D2E0F6737CEE22F23D19D9506",
			wantAppData:   "9204F8C4FD4D4F7C0AF09F92A54645524755532054524947",
			wantLat:       -37423100,
			wantLon:       175918925,
			wantName:      "💥FERGUS TRIG",
			wantType:      "REPEATER",
			wantVerified:  true,
		},
		{
			name:          "chat advert with name",
			hex:           "49A10AA4BF939B21052B3216C571A9D6D49BEC3BCD54F716778AF1A3D06BD7BF9A1AC269B1396C0F2D9C9A3B038A170F01E3E5A40DEDC0A7C7FF2AA1C34CD41917AABBE28E52A82D5A0A8EBB98E693BAA2D380EDD16CCDFB02B72F12FFE599FE5F77ED0281F09F8F8DEFB88F484C5A204F62732031202D204C6574734D657368203162",
			wantPubKey:    "49A10AA4BF939B21052B3216C571A9D6D49BEC3BCD54F716778AF1A3D06BD7BF",
			wantTimestamp: 1774328474,
			wantSignature: "B1396C0F2D9C9A3B038A170F01E3E5A40DEDC0A7C7FF2AA1C34CD41917AABBE28E52A82D5A0A8EBB98E693BAA2D380EDD16CCDFB02B72F12FFE599FE5F77ED02",
			wantName:      "🏍️HLZ Obs 1 - LetsMesh 1b",
			wantType:      "CHAT",
			wantVerified:  true,
		},
		{
			name:    "empty input returns error",
			hex:     "",
			wantErr: true,
		},
		{
			name:    "too short for timestamp",
			hex:     "E5549C5F2D5BED2FACF65F266BB6DFE0A6B37D6A00998D4A168AC0926F58878B",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := hex.DecodeString(tt.hex)
			if err != nil {
				t.Fatalf("bad test hex: %v", err)
			}

			advert, err := AdvertFromBytes(data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantPubKey != "" {
				if got := advert.PublicKey.String(); !strings.EqualFold(got, tt.wantPubKey) {
					t.Errorf("PublicKey = %s, want %s", got, tt.wantPubKey)
				}
			}

			if advert.Timestamp != tt.wantTimestamp {
				t.Errorf("Timestamp = 0x%08x, want 0x%08x", advert.Timestamp, tt.wantTimestamp)
			}

			if tt.wantSignature != "" {
				if got := hex.EncodeToString(advert.Signature); !strings.EqualFold(got, tt.wantSignature) {
					t.Errorf("Signature = %s, want %s", got, tt.wantSignature)
				}
			}

			if tt.wantAppData != "" {
				if got := hex.EncodeToString(advert.RawAppData); !strings.EqualFold(got, tt.wantAppData) {
					t.Errorf("RawAppData = %s, want %s", got, tt.wantAppData)
				}
			}

			if got := advert.Verify(); got != tt.wantVerified {
				t.Errorf("Verify() = %v, want %v", got, tt.wantVerified)
			}

			appData := advert.AppData()

			if tt.wantType != "" {
				if appData.Type != tt.wantType {
					t.Errorf("AppData.Type = %q, want %q", appData.Type, tt.wantType)
				}
			}
			if tt.wantName != "" {
				if appData.Name != tt.wantName {
					t.Errorf("AppData.Name = %q, want %q", appData.Name, tt.wantName)
				}
			}
			if appData.Lat != tt.wantLat {
				t.Errorf("AppData.Lat = %d, want %d", appData.Lat, tt.wantLat)
			}
			if appData.Lon != tt.wantLon {
				t.Errorf("AppData.Lon = %d, want %d", appData.Lon, tt.wantLon)
			}
			if appData.Feat1 != tt.wantFeat1 {
				t.Errorf("AppData.Feat1 = %d, want %d", appData.Feat1, tt.wantFeat1)
			}
			if appData.Feat2 != tt.wantFeat2 {
				t.Errorf("AppData.Feat2 = %d, want %d", appData.Feat2, tt.wantFeat2)
			}
		})
	}
}

func TestAdvertToBytes(t *testing.T) {
	tests := []struct {
		name    string
		advert  Advert
		wantHex string
	}{
		{
			name: "repeater advert with latlon and name",
			advert: Advert{
				PublicKey:  hexIdentity(t, "D7B4B39CCF9F10D3B0D0597C0E5F0D5886B8DF23C7F8AD0CC702F932B9A3BA86"),
				Timestamp:  1774429376,
				Signature:  hexDecode(t, "C3CE366496C4C607A8DFBAA80B0ED295B67222F55D6AC91EEEE6F38ED5F07A741B80A58B565DCA0263EBF2431D0528F57822EA5D2E0F6737CEE22F23D19D9506"),
				RawAppData: hexDecode(t, "9204F8C4FD4D4F7C0AF09F92A54645524755532054524947"),
			},
			wantHex: "D7B4B39CCF9F10D3B0D0597C0E5F0D5886B8DF23C7F8AD0CC702F932B9A3BA86C0A4C369C3CE366496C4C607A8DFBAA80B0ED295B67222F55D6AC91EEEE6F38ED5F07A741B80A58B565DCA0263EBF2431D0528F57822EA5D2E0F6737CEE22F23D19D95069204F8C4FD4D4F7C0AF09F92A54645524755532054524947",
		},
		{
			name: "chat advert with name only",
			advert: Advert{
				PublicKey:  hexIdentity(t, "49A10AA4BF939B21052B3216C571A9D6D49BEC3BCD54F716778AF1A3D06BD7BF"),
				Timestamp:  1774328474,
				Signature:  hexDecode(t, "B1396C0F2D9C9A3B038A170F01E3E5A40DEDC0A7C7FF2AA1C34CD41917AABBE28E52A82D5A0A8EBB98E693BAA2D380EDD16CCDFB02B72F12FFE599FE5F77ED02"),
				RawAppData: hexDecode(t, "81F09F8F8DEFB88F484C5A204F62732031202D204C6574734D657368203162"),
			},
			wantHex: "49A10AA4BF939B21052B3216C571A9D6D49BEC3BCD54F716778AF1A3D06BD7BF9A1AC269B1396C0F2D9C9A3B038A170F01E3E5A40DEDC0A7C7FF2AA1C34CD41917AABBE28E52A82D5A0A8EBB98E693BAA2D380EDD16CCDFB02B72F12FFE599FE5F77ED0281F09F8F8DEFB88F484C5A204F62732031202D204C6574734D657368203162",
		},
		{
			name: "minimal advert with no appdata",
			advert: Advert{
				PublicKey:  Identity{},
				Timestamp:  0,
				Signature:  make([]byte, 64),
				RawAppData: []byte{},
			},
			wantHex: "00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.advert.ToBytes()
			if err != nil {
				t.Fatalf("unexpected encode error: %v", err)
			}
			if gotHex := hex.EncodeToString(got); !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("ToBytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestAdvertAppDataToBytes(t *testing.T) {
	tests := []struct {
		name    string
		appData AdvertAppData
		wantHex string
	}{
		{
			name: "repeater with latlon and name",
			appData: AdvertAppData{
				Type: "REPEATER",
				Lat:  -37423100,
				Lon:  175918925,
				Name: "💥FERGUS TRIG",
			},
			wantHex: "9204F8C4FD4D4F7C0AF09F92A54645524755532054524947",
		},
		{
			name: "chat with name only",
			appData: AdvertAppData{
				Type: "CHAT",
				Name: "🏍️HLZ Obs 1 - LetsMesh 1b",
			},
			wantHex: "81F09F8F8DEFB88F484C5A204F62732031202D204C6574734D657368203162",
		},
		{
			name: "sensor with feat1 and feat2",
			appData: AdvertAppData{
				Type:  "SENSOR",
				Feat1: 0x1234,
				Feat2: 0xABCD,
			},
			wantHex: "643412CDAB",
		},
		{
			name: "none type bare",
			appData: AdvertAppData{
				Type: "NONE",
			},
			wantHex: "00",
		},
		{
			name: "room with all fields",
			appData: AdvertAppData{
				Type:  "ROOM",
				Lat:   100000,
				Lon:   -200000,
				Feat1: 0x0001,
				Feat2: 0xFFFF,
				Name:  "TestRoom",
			},
			wantHex: "F3A0860100C0F2FCFF0100FFFF54657374526F6F6D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.appData.ToBytes()
			if err != nil {
				t.Fatalf("unexpected encode error: %v", err)
			}
			if gotHex := hex.EncodeToString(got); !strings.EqualFold(gotHex, tt.wantHex) {
				t.Errorf("ToBytes() = %s, want %s", gotHex, tt.wantHex)
			}
		})
	}
}

func TestAdvertSign(t *testing.T) {
	pub, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	pubKeyID, err := NewIdentityFromBytes(pub)
	if err != nil {
		t.Fatalf("NewIdentityFromBytes() error = %v", err)
	}

	appData := &AdvertAppData{
		Type: "REPEATER",
		Name: "signed advert",
		Lat:  123456789,
		Lon:  -987654321,
	}

	rawAppData, err := appData.ToBytes()
	if err != nil {
		t.Fatalf("appData.ToBytes() error = %v", err)
	}

	advert := &Advert{
		PublicKey:  pubKeyID,
		Timestamp:  1774429376,
		RawAppData: rawAppData,
	}

	advert.Sign(privateKey)

	if got := advert.Verify(); !got {
		t.Fatal("Verify() = false, want true")
	}

	encoded, err := advert.ToBytes()
	if err != nil {
		t.Fatalf("advert.ToBytes() error = %v", err)
	}

	decoded, err := AdvertFromBytes(encoded)
	if err != nil {
		t.Fatalf("AdvertFromBytes() error = %v", err)
	}

	if !decoded.PublicKey.Matches(advert.PublicKey) {
		t.Errorf("PublicKey = %s, want %s", decoded.PublicKey, advert.PublicKey)
	}
	if decoded.Timestamp != advert.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, advert.Timestamp)
	}
	if !bytes.Equal(decoded.Signature, advert.Signature) {
		t.Errorf("Signature = %x, want %x", decoded.Signature, advert.Signature)
	}
	if !bytes.Equal(decoded.RawAppData, advert.RawAppData) {
		t.Errorf("RawAppData = %x, want %x", decoded.RawAppData, advert.RawAppData)
	}
	if got := decoded.Verify(); !got {
		t.Fatal("decoded Verify() = false, want true")
	}

	decodedAppData := decoded.AppData()
	if decodedAppData.Type != appData.Type {
		t.Errorf("AppData.Type = %q, want %q", decodedAppData.Type, appData.Type)
	}
	if decodedAppData.Name != appData.Name {
		t.Errorf("AppData.Name = %q, want %q", decodedAppData.Name, appData.Name)
	}
	if decodedAppData.Lat != appData.Lat {
		t.Errorf("AppData.Lat = %d, want %d", decodedAppData.Lat, appData.Lat)
	}
	if decodedAppData.Lon != appData.Lon {
		t.Errorf("AppData.Lon = %d, want %d", decodedAppData.Lon, appData.Lon)
	}
}

func TestAdvertAppDataFromBytes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := AdvertAppDataFromBytes([]byte{})
		if err == nil {
			t.Fatal("expected error for empty input, got nil")
		}
	})

	t.Run("latlon flag set but no lat bytes", func(t *testing.T) {
		_, err := AdvertAppDataFromBytes([]byte{AdvertTypeChat | AdvertLatLonMask})
		if err == nil {
			t.Fatal("expected error for truncated lat, got nil")
		}
	})

	t.Run("latlon flag set but no lon bytes", func(t *testing.T) {
		data := []byte{AdvertTypeChat | AdvertLatLonMask, 0x00, 0x00, 0x00, 0x00}
		_, err := AdvertAppDataFromBytes(data)
		if err == nil {
			t.Fatal("expected error for truncated lon, got nil")
		}
	})

	t.Run("feat1 flag set but no feat1 bytes", func(t *testing.T) {
		_, err := AdvertAppDataFromBytes([]byte{AdvertTypeChat | AdvertFeat1Mask})
		if err == nil {
			t.Fatal("expected error for truncated feat1, got nil")
		}
	})

	t.Run("feat2 flag set but no feat2 bytes", func(t *testing.T) {
		_, err := AdvertAppDataFromBytes([]byte{AdvertTypeChat | AdvertFeat2Mask})
		if err == nil {
			t.Fatal("expected error for truncated feat2, got nil")
		}
	})

	t.Run("all flags parse correctly", func(t *testing.T) {
		data := []byte{
			AdvertTypeRepeater | AdvertLatLonMask | AdvertFeat1Mask | AdvertFeat2Mask | AdvertNameMask,
			0x01, 0x00, 0x00, 0x00,
			0x02, 0x00, 0x00, 0x00,
			0x03, 0x00,
			0x04, 0x00,
			'H', 'i',
		}
		ad, err := AdvertAppDataFromBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ad.Type != "REPEATER" {
			t.Errorf("Type = %q, want REPEATER", ad.Type)
		}
		if ad.Lat != 1 {
			t.Errorf("Lat = %d, want 1", ad.Lat)
		}
		if ad.Lon != 2 {
			t.Errorf("Lon = %d, want 2", ad.Lon)
		}
		if ad.Feat1 != 3 {
			t.Errorf("Feat1 = %d, want 3", ad.Feat1)
		}
		if ad.Feat2 != 4 {
			t.Errorf("Feat2 = %d, want 4", ad.Feat2)
		}
		if ad.Name != "Hi" {
			t.Errorf("Name = %q, want Hi", ad.Name)
		}
	})

	t.Run("none type", func(t *testing.T) {
		ad, err := AdvertAppDataFromBytes([]byte{AdvertTypeNone})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ad.Type != "NONE" {
			t.Errorf("Type = %q, want NONE", ad.Type)
		}
	})

	t.Run("room type", func(t *testing.T) {
		ad, err := AdvertAppDataFromBytes([]byte{AdvertTypeRoom})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ad.Type != "ROOM" {
			t.Errorf("Type = %q, want ROOM", ad.Type)
		}
	})

	t.Run("sensor type", func(t *testing.T) {
		ad, err := AdvertAppDataFromBytes([]byte{AdvertTypeSensor})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ad.Type != "SENSOR" {
			t.Errorf("Type = %q, want SENSOR", ad.Type)
		}
	})
}

func TestAdvertFromBytesAppDataParseError(t *testing.T) {
	pubkey := make([]byte, 32)
	timestamp := []byte{0x00, 0x00, 0x00, 0x00}
	signature := make([]byte, 64)
	badAppData := []byte{AdvertTypeChat | AdvertLatLonMask}

	data := make([]byte, 0, 32+4+64+1)
	data = append(data, pubkey...)
	data = append(data, timestamp...)
	data = append(data, signature...)
	data = append(data, badAppData...)

	_, err := AdvertFromBytes(data)
	if err == nil {
		t.Fatal("expected error from bad appdata, got nil")
	}
}

func TestAdvertFlags(t *testing.T) {
	tests := []struct {
		name      string
		rawByte   byte
		wantFlags byte
	}{
		{"none bare", AdvertTypeNone, AdvertTypeNone},
		{"chat with name", AdvertTypeChat | AdvertNameMask, AdvertTypeChat | AdvertNameMask},
		{"repeater with latlon", AdvertTypeRepeater | AdvertLatLonMask, AdvertTypeRepeater | AdvertLatLonMask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advert := &Advert{RawAppData: []byte{tt.rawByte}}
			if got := advert.Flags(); got != tt.wantFlags {
				t.Errorf("Flags() = 0x%02X, want 0x%02X", got, tt.wantFlags)
			}
		})
	}
}

func TestAdvertType(t *testing.T) {
	tests := []struct {
		name     string
		rawByte  byte
		wantType byte
	}{
		{"none", AdvertTypeNone, AdvertTypeNone},
		{"chat", AdvertTypeChat, AdvertTypeChat},
		{"repeater", AdvertTypeRepeater | AdvertLatLonMask | AdvertNameMask, AdvertTypeRepeater},
		{"room", AdvertTypeRoom | AdvertFeat1Mask, AdvertTypeRoom},
		{"sensor", AdvertTypeSensor | AdvertFeat2Mask, AdvertTypeSensor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advert := &Advert{RawAppData: []byte{tt.rawByte}}
			if got := advert.Type(); got != tt.wantType {
				t.Errorf("Type() = %d, want %d", got, tt.wantType)
			}
		})
	}
}

func TestAdvertTypeString(t *testing.T) {
	tests := []struct {
		name       string
		rawByte    byte
		wantString string
	}{
		{"NONE", AdvertTypeNone, "NONE"},
		{"CHAT", AdvertTypeChat, "CHAT"},
		{"REPEATER", AdvertTypeRepeater, "REPEATER"},
		{"ROOM", AdvertTypeRoom, "ROOM"},
		{"SENSOR", AdvertTypeSensor, "SENSOR"},
		{"unknown", 0x0F, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advert := &Advert{RawAppData: []byte{tt.rawByte}}
			if got := advert.TypeString(); got != tt.wantString {
				t.Errorf("TypeString() = %q, want %q", got, tt.wantString)
			}
		})
	}
}

func TestAdvertAppDataToBytesUnknownType(t *testing.T) {
	ad := &AdvertAppData{Type: "INVALID"}
	_, err := ad.ToBytes()
	if err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}

func TestAdvertFromBytesValidation(t *testing.T) {
	t.Run("99 bytes too short for min advert", func(t *testing.T) {
		data := make([]byte, MinAdvertSize-1)
		_, err := AdvertFromBytes(data)
		if err == nil {
			t.Fatal("expected error for 99-byte input, got nil")
		}
	})

	t.Run("exactly 100 bytes with no appdata", func(t *testing.T) {
		data := make([]byte, MinAdvertSize)
		data[0] = 0x01
		advert, err := AdvertFromBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(advert.RawAppData) != 0 {
			t.Errorf("RawAppData len = %d, want 0", len(advert.RawAppData))
		}
	})

	t.Run("oversized app_data", func(t *testing.T) {
		data := make([]byte, MinAdvertSize+MaxAdvertDataSize+1)
		_, err := AdvertFromBytes(data)
		if err == nil {
			t.Fatal("expected error for oversized app_data, got nil")
		}
	})

	t.Run("max valid app_data", func(t *testing.T) {
		data := make([]byte, MinAdvertSize+MaxAdvertDataSize)
		data[MinAdvertSize] = AdvertTypeChat | AdvertNameMask
		for i := MinAdvertSize + 1; i < len(data); i++ {
			data[i] = 'A'
		}
		advert, err := AdvertFromBytes(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(advert.RawAppData) != MaxAdvertDataSize {
			t.Errorf("RawAppData len = %d, want %d", len(advert.RawAppData), MaxAdvertDataSize)
		}
	})
}

func TestAdvertFlagsEmptyAppData(t *testing.T) {
	advert := &Advert{RawAppData: []byte{}}
	if got := advert.Flags(); got != 0 {
		t.Errorf("Flags() = 0x%02X, want 0x00 for empty RawAppData", got)
	}
}

func hexDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

func hexIdentity(t *testing.T, s string) Identity {
	t.Helper()
	id, err := NewIdentityFromHex(s)
	if err != nil {
		t.Fatalf("bad identity hex %q: %v", s, err)
	}
	return id
}
