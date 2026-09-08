package sx12xx

// Command opcodes.
const (
	opSetSleep              = 0x84
	opSetStandby            = 0x80
	opSetFs                 = 0xC1
	opSetTx                 = 0x83
	opSetRx                 = 0x82
	opStopTimerOnPreamble   = 0x9F
	opSetRxDutyCycle        = 0x94
	opSetCad                = 0xC5
	opSetTxContinuousWave   = 0xD1
	opSetTxInfinitePream    = 0xD2
	opSetRegulatorMode      = 0x96
	opCalibrate             = 0x89
	opCalibrateImage        = 0x98
	opSetPaConfig           = 0x95
	opSetRxTxFallbackMode   = 0x93
	opWriteRegister         = 0x0D
	opReadRegister          = 0x1D
	opWriteBuffer           = 0x0E
	opReadBuffer            = 0x1E
	opSetDioIrqParams       = 0x08
	opGetIrqStatus          = 0x12
	opClearIrqStatus        = 0x02
	opSetDIO2AsRfSwitch     = 0x9D
	opSetDIO3AsTCXOCtrl     = 0x97
	opSetRfFrequency        = 0x86
	opSetPacketType         = 0x8A
	opGetPacketType         = 0x11
	opSetTxParams           = 0x8E
	opSetModulationParams   = 0x8B
	opSetPacketParams       = 0x8C
	opSetCadParams          = 0x88
	opSetBufferBaseAddress  = 0x8F
	opSetLoRaSymbNumTimeout = 0xA0
	opGetStatus             = 0xC0
	opGetRxBufferStatus     = 0x13
	opGetPacketStatus       = 0x14
	opGetRssiInst           = 0x15
	opGetStats              = 0x10
	opResetStats            = 0x00
	opGetDeviceErrors       = 0x17
	opClearDeviceErrors     = 0x07
)

// Register addresses (16-bit).
const (
	regWhiteningInitialMSBFsk = 0x06B8 // FSK whitening seed (2 bytes)
	regCRCInitialMSBFsk       = 0x06BC // FSK CRC initial value (2 bytes) + polynomial
	regSyncWord0Fsk           = 0x06C0 // FSK sync word, up to 8 bytes
	regNodeAddressFsk         = 0x06CD // FSK node + broadcast address
	regIQPolaritySetup        = 0x0736 // bit2 inverted-IQ workaround
	regLoRaSyncWordMSB        = 0x0740 // LoRa sync word (2 bytes)
	regTxModulation           = 0x0889 // bit2 BW500 workaround
	regRxGain                 = 0x08AC // RX gain (boosted/power-saving)
	regTxClampConfig          = 0x08D8 // OR 0x1E antenna-resistance workaround
	regFEMRxPatch             = 0x08B5 // OR 0x01 improves RX behind an external FEM
	regOCPConfiguration       = 0x08E7 // over-current protection level
	regRTCControl             = 0x0902 // RX-timeout workaround
	regXTATrim                = 0x0911 // crystal trim A
	regXTBTrim                = 0x0912 // crystal trim B
	regEventMask              = 0x0944 // OR 0x02 RX-timeout workaround
)

// SetStandby modes (opcode 0x80).
const (
	StandbyRC   = 0x00 // STDBY_RC: 13 MHz RC oscillator
	StandbyXOSC = 0x01 // STDBY_XOSC: 32 MHz crystal oscillator
)

// SetSleep configuration bits (opcode 0x84).
const (
	SleepColdStart    = 0x00 // configuration lost on wake
	SleepColdStartRTC = 0x01 // cold start, RTC wakeup enabled
	SleepWarmStart    = 0x04 // configuration retained on wake
	SleepWarmStartRTC = 0x05 // warm start, RTC wakeup enabled
)

// SetRxTxFallbackMode targets (opcode 0x93): the mode entered after Tx or Rx.
const (
	FallbackFS          = 0x40 // frequency synthesis
	FallbackStandbyXOSC = 0x30 // STDBY_XOSC
	FallbackStandbyRC   = 0x20 // STDBY_RC (default)
)

// SetRegulatorMode parameter (opcode 0x96).
const (
	RegulatorLDO     = 0x00 // LDO only
	RegulatorDCDCLDO = 0x01 // DC-DC + LDO
)

// SetPacketType parameter (opcode 0x8A) and GetPacketType return value.
const (
	PacketTypeFSK  = 0x00 // GFSK/FSK modem
	PacketTypeLoRa = 0x01 // LoRa modem
)

// Calibrate block bitmask (opcode 0x89).
const (
	CalibRC64k    = 0x01
	CalibRC13M    = 0x02
	CalibPLL      = 0x04
	CalibADCPulse = 0x08
	CalibADCBulkN = 0x10
	CalibADCBulkP = 0x20
	CalibImage    = 0x40
	CalibAll      = 0x7F // bits 0-6; bit 7 is reserved
)

// SetCadParams defaults (opcode 0x88), per Semtech DS.SX1261-2.W.APP rev 1.1 p.92.
const (
	CadOn4Symb   = 0x02
	CadDetMin    = 10
	cadPeakForSF = 13 // detPeak = spreading factor + 13
)

// CalibrateImage band-edge byte pairs (opcode 0x98).
const (
	CalImg430 = 0x6B
	CalImg440 = 0x6F
	CalImg470 = 0x75
	CalImg510 = 0x81
	CalImg779 = 0xC1
	CalImg787 = 0xC5
	CalImg863 = 0xD7
	CalImg870 = 0xDB
	CalImg902 = 0xE1
	CalImg928 = 0xE9
)

// SetDIO2AsRfSwitchCtrl parameter (opcode 0x9D).
const (
	DIO2AsIRQ      = 0x00 // DIO2 behaves as a normal IRQ line
	DIO2AsRfSwitch = 0x01 // DIO2 drives the RF switch
)

// TCXO control voltages for SetDIO3AsTCXOCtrl (opcode 0x97).
const (
	TCXO1_6V = 0x00
	TCXO1_7V = 0x01
	TCXO1_8V = 0x02
	TCXO2_2V = 0x03
	TCXO2_4V = 0x04
	TCXO2_7V = 0x05
	TCXO3_0V = 0x06
	TCXO3_3V = 0x07
)

// PA ramp times for SetTxParams (opcode 0x8E).
const (
	PARamp10U   = 0x00
	PARamp20U   = 0x01
	PARamp40U   = 0x02
	PARamp80U   = 0x03
	PARamp200U  = 0x04
	PARamp800U  = 0x05
	PARamp1700U = 0x06
	PARamp3400U = 0x07
)

// SetPaConfig deviceSel values (opcode 0x95).
const (
	PADeviceSX1262 = 0x00 // also SX1268
	PADeviceSX1261 = 0x01
	PALutDefault   = 0x01 // paLut byte is always 0x01
)

// LoRa spreading factors for SetModulationParams (opcode 0x8B), valid 5..12.
const (
	LoRaSF5  = 0x05
	LoRaSF6  = 0x06
	LoRaSF7  = 0x07
	LoRaSF8  = 0x08
	LoRaSF9  = 0x09
	LoRaSF10 = 0x0A
	LoRaSF11 = 0x0B
	LoRaSF12 = 0x0C
)

// LoRa bandwidth register values for SetModulationParams (opcode 0x8B).
const (
	LoRaBW7800   = 0x00
	LoRaBW10400  = 0x08
	LoRaBW15600  = 0x01
	LoRaBW20800  = 0x09
	LoRaBW31250  = 0x02
	LoRaBW41700  = 0x0A
	LoRaBW62500  = 0x03
	LoRaBW125000 = 0x04
	LoRaBW250000 = 0x05
	LoRaBW500000 = 0x06
)

// LoRa coding rate register values for SetModulationParams (opcode 0x8B).
const (
	LoRaCR4_5 = 0x01
	LoRaCR4_6 = 0x02
	LoRaCR4_7 = 0x03
	LoRaCR4_8 = 0x04
)

// LoRa low-data-rate optimization for SetModulationParams (opcode 0x8B).
const (
	LoRaLDROOff = 0x00
	LoRaLDROOn  = 0x01
)

// LoRa header types for SetPacketParams (opcode 0x8C).
const (
	LoRaHeaderExplicit = 0x00 // variable length, header transmitted
	LoRaHeaderImplicit = 0x01 // fixed length, no header
)

// LoRa CRC selection for SetPacketParams (opcode 0x8C).
const (
	LoRaCRCOff = 0x00
	LoRaCRCOn  = 0x01
)

// LoRa IQ polarity for SetPacketParams (opcode 0x8C).
const (
	LoRaIQStandard = 0x00
	LoRaIQInverted = 0x01
)

// LoRa sync words written to regLoRaSyncWordMSB (0x0740).
const (
	LoRaSyncWordPublic  = 0x3444 // public LoRaWAN network
	LoRaSyncWordPrivate = 0x1424 // private network
)

// LoRa RX gain values written to regRxGain (0x08AC).
const (
	RxGainPowerSaving = 0x94
	RxGainBoosted     = 0x96
)

// FSK/GFSK pulse shapes for SetModulationParams (opcode 0x8B).
const (
	FskPulseNoFilter     = 0x00
	FskPulseGaussianBT03 = 0x08
	FskPulseGaussianBT05 = 0x09
	FskPulseGaussianBT07 = 0x0A
	FskPulseGaussianBT1  = 0x0B
)

// FSK/GFSK double-side-band RX bandwidth register values (opcode 0x8B).
const (
	FskBW4800   = 0x1F
	FskBW5800   = 0x17
	FskBW7300   = 0x0F
	FskBW9700   = 0x1E
	FskBW11700  = 0x16
	FskBW14600  = 0x0E
	FskBW19500  = 0x1D
	FskBW23400  = 0x15
	FskBW29300  = 0x0D
	FskBW39000  = 0x1C
	FskBW46900  = 0x14
	FskBW58600  = 0x0C
	FskBW78200  = 0x1B
	FskBW93800  = 0x13
	FskBW117300 = 0x0B
	FskBW156200 = 0x1A
	FskBW187200 = 0x12
	FskBW234300 = 0x0A
	FskBW312000 = 0x19
	FskBW373600 = 0x11
	FskBW467000 = 0x09
)

// FSK preamble detector length for SetPacketParams (opcode 0x8C).
const (
	FskPreambleDetOff   = 0x00
	FskPreambleDet8Bit  = 0x04
	FskPreambleDet16Bit = 0x05
	FskPreambleDet24Bit = 0x06
	FskPreambleDet32Bit = 0x07
)

// FSK address comparison for SetPacketParams (opcode 0x8C).
const (
	FskAddrCompOff           = 0x00
	FskAddrCompNode          = 0x01
	FskAddrCompNodeBroadcast = 0x02
)

// FSK packet length mode for SetPacketParams (opcode 0x8C).
const (
	FskPacketFixed    = 0x00 // known/fixed length
	FskPacketVariable = 0x01 // length transmitted in packet
)

// FSK CRC types for SetPacketParams (opcode 0x8C).
const (
	FskCRCOff      = 0x01
	FskCRC1Byte    = 0x00
	FskCRC2Byte    = 0x02
	FskCRC1ByteInv = 0x04
	FskCRC2ByteInv = 0x06
)

// FSK whitening for SetPacketParams (opcode 0x8C).
const (
	FskWhiteningOff = 0x00
	FskWhiteningOn  = 0x01
)

// IRQ flags (16-bit masks) for SetDioIrqParams (0x08), GetIrqStatus (0x12) and
// ClearIrqStatus (0x02).
const (
	IRQTxDone           = 0x0001
	IRQRxDone           = 0x0002
	IRQPreambleDetected = 0x0004
	IRQSyncWordValid    = 0x0008
	IRQHeaderValid      = 0x0010
	IRQHeaderErr        = 0x0020
	IRQCRCErr           = 0x0040
	IRQCadDone          = 0x0080
	IRQCadDetected      = 0x0100
	IRQTimeout          = 0x0200
	IRQAll              = 0x03FF
)

// SetCadParams symbol counts (opcode 0x88).
const (
	CadSymbol1  = 0x00
	CadSymbol2  = 0x01
	CadSymbol4  = 0x02
	CadSymbol8  = 0x03
	CadSymbol16 = 0x04
)

// SetCadParams exit modes (opcode 0x88).
const (
	CadExitStandbyRC = 0x00
	CadExitRx        = 0x01
)

// Status byte fields, returned by GetStatus (0xC0) and as the leading byte of
// most read commands.
const (
	StatusModeMask = 0x70 // bits 6:4 chip mode
	StatusCmdMask  = 0x0E // bits 3:1 command status

	// Chip mode values (already masked into bits 6:4).
	StatusModeStandbyRC   = 0x20
	StatusModeStandbyXOSC = 0x30
	StatusModeFS          = 0x40
	StatusModeRx          = 0x50
	StatusModeTx          = 0x60

	// Command status values (already shifted into bits 3:1).
	StatusCmdDataAvailable = 0x04
	StatusCmdTimeout       = 0x06
	StatusCmdError         = 0x08
	StatusCmdFailedExec    = 0x0A
	StatusCmdTxDone        = 0x0C
)

// Device error bits returned by GetDeviceErrors (opcode 0x17).
const (
	DeviceErrRC64kCalib = 0x0001
	DeviceErrRC13MCalib = 0x0002
	DeviceErrPLLCalib   = 0x0004
	DeviceErrADCCalib   = 0x0008
	DeviceErrImgCalib   = 0x0010
	DeviceErrXoscStart  = 0x0020
	DeviceErrPLLLock    = 0x0040
	DeviceErrPaRamp     = 0x0100
)

// Tx/Rx timeout sentinels, 24-bit values in 15.625 us steps.
const (
	TxSingle     = 0x000000 // no timeout (single shot)
	RxSingle     = 0x000000 // single receive, no timeout
	RxContinuous = 0xFFFFFF // never time out
	timeoutMax   = 0x00FFFFFF
)

const xtalFreqHz = 32_000_000
