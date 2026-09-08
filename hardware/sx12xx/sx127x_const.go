package sx12xx

// SX127x register addresses and bitfields, unexported to avoid colliding with
// the exported SX126x constants.

const (
	sx127xWriteMask = 0x80 // OR into address byte to write
	sx127xReadMask  = 0x7F // AND into address byte to read
)

// Register addresses. Several are dual-mapped: the LoRa or FSK meaning is
// selected by RegOpMode LongRangeMode, which changes only in SLEEP.
const (
	regFifo               = 0x00 // does not auto-increment
	regOpMode             = 0x01
	regBitrateMsbFsk      = 0x02
	regBitrateLsbFsk      = 0x03
	regFdevMsbFsk         = 0x04 // bits 5:0
	regFdevLsbFsk         = 0x05
	regFrfMsb             = 0x06
	regFrfMid             = 0x07
	regFrfLsb             = 0x08
	regPaConfig           = 0x09
	regPaRamp             = 0x0A
	regOcp                = 0x0B
	regLna                = 0x0C
	regFifoAddrPtr        = 0x0D
	regRxConfigFsk        = 0x0D
	regFifoTxBaseAddr     = 0x0E
	regFifoRxBaseAddr     = 0x0F
	regFifoRxCurrentAddr  = 0x10 // start of last packet received
	regIrqFlagsMask       = 0x11
	regIrqFlags           = 0x12
	regRxBwFsk            = 0x12
	regRxNbBytes          = 0x13
	regRxHeaderCntMsb     = 0x14
	regRxHeaderCntLsb     = 0x15
	regRxPacketCntMsb     = 0x16
	regRxPacketCntLsb     = 0x17
	regModemStat          = 0x18
	regPktSnrValue        = 0x19 // signed, /4 = dB
	regPktRssiValue       = 0x1A
	regRssiValue          = 0x1B
	regHopChannel         = 0x1C
	regModemConfig1       = 0x1D
	regModemConfig2       = 0x1E
	regSymbTimeoutLsb     = 0x1F
	regPreambleDetectFsk  = 0x1F
	regPreambleMsb        = 0x20
	regPreambleLsb        = 0x21
	regPayloadLength      = 0x22
	regMaxPayloadLength   = 0x23
	regHopPeriod          = 0x24
	regFifoRxByteAddr     = 0x25
	regModemConfig3       = 0x26
	regSyncConfigFsk      = 0x27
	regFreqErrorMsb       = 0x28
	regFreqErrorMid       = 0x29
	regFreqErrorLsb       = 0x2A
	regRssiWideband       = 0x2C // random-number source
	regSyncValue1Fsk      = 0x28
	regPacketConfig1Fsk   = 0x30
	regPacketConfig2Fsk   = 0x31
	regPayloadLengthFsk   = 0x32
	regDetectOptimize     = 0x31
	regInvertIQ           = 0x33
	regFifoThreshFsk      = 0x35
	regHighBwOptimize1    = 0x36
	regDetectionThreshold = 0x37
	regSyncWordLoRa       = 0x39
	regHighBwOptimize2    = 0x3A
	regInvertIQ2          = 0x3B
	regDioMapping1        = 0x40
	regDioMapping2        = 0x41
	regVersion            = 0x42
	regTcxo               = 0x4B
	regPaDac              = 0x4D // +20 dBm high-power PA enable
	regFormerTemp         = 0x5B
	regAgcRef             = 0x61
	regAgcThresh1         = 0x62
	regAgcThresh2         = 0x63
	regAgcThresh3         = 0x64
	regPll                = 0x70
)

// RegOpMode (0x01) bit definitions.
const (
	modeLongRangeMode   = 0x80 // 1 = LoRa, 0 = FSK/OOK; change only in SLEEP
	modeAccessSharedReg = 0x40 // access FSK registers while in LoRa mode
	modeLowFrequencyOn  = 0x08 // low-frequency band (< 525 MHz)

	// Modulation type, bits 6:5 (FSK/OOK modem only).
	modeModulationFsk = 0x00
	modeModulationOok = 0x20

	// Transceiver mode, bits 2:0.
	modeMaskBits     = 0x07
	modeSleep        = 0x00
	modeStandby      = 0x01
	modeFsTx         = 0x02
	modeTx           = 0x03
	modeFsRx         = 0x04
	modeRxContinuous = 0x05
	modeRxSingle     = 0x06
	modeCad          = 0x07
)

// RegModemConfig1 (0x1D): BW in bits 7:4, CR in bits 3:1, implicit header in
// bit0.
const (
	mc1ImplicitHeaderOn = 0x01

	// Bandwidth, pre-shifted into position.
	mc1Bw7800   = 0x00
	mc1Bw10400  = 0x10
	mc1Bw15600  = 0x20
	mc1Bw20800  = 0x30
	mc1Bw31250  = 0x40
	mc1Bw41700  = 0x50
	mc1Bw62500  = 0x60
	mc1Bw125000 = 0x70
	mc1Bw250000 = 0x80
	mc1Bw500000 = 0x90

	// Coding rate, pre-shifted into position.
	mc1Cr4_5 = 0x02
	mc1Cr4_6 = 0x04
	mc1Cr4_7 = 0x06
	mc1Cr4_8 = 0x08
)

// RegModemConfig2 (0x1E): SF in bits 7:4.
const (
	mc2TxContinuous       = 0x08
	mc2RxPayloadCrcOn     = 0x04
	mc2SymbTimeoutMsbMask = 0x03

	// Spreading factor, pre-shifted into position.
	mc2Sf6  = 0x60
	mc2Sf7  = 0x70
	mc2Sf8  = 0x80
	mc2Sf9  = 0x90
	mc2Sf10 = 0xA0
	mc2Sf11 = 0xB0
	mc2Sf12 = 0xC0
)

// RegModemConfig3 (0x26) fields.
const (
	mc3LowDataRateOptimize = 0x08 // mandatory when symbol time > 16 ms
	mc3AgcAutoOn           = 0x04
)

// RegDetectOptimize (0x31) bits 2:0 and RegDetectionThreshold (0x37).
const (
	detectOptimizeMask = 0xF8
	detectOptimizeSf6  = 0x05
	detectOptimizeSf7  = 0x03 // SF7..SF12
	detectThresholdSf6 = 0x0C
	detectThresholdSf7 = 0x0A // SF7..SF12
)

// RegPaConfig (0x09), RegPaDac (0x4D) and RegOcp (0x0B) fields.
const (
	paSelectBoost = 0x80 // PA_BOOST pin, up to +20 dBm
	paSelectRfo   = 0x00 // RFO pin, max +14 dBm
	paMaxPower7   = 0x70 // Pmax = 15 dBm
	paMaxPower6   = 0x60 // Pmax = 14.4 dBm

	paDacMask    = 0x07
	paDacDefault = 0x84 // +17 dBm max
	paDacBoost20 = 0x87 // +20 dBm on PA_BOOST

	ocpOn       = 0x20
	ocpTrimMask = 0x1F
)

// RegLna (0x0C): bits 1:0. Reset is 0x20, gain G1 with boost off.
const lnaBoostHfOn = 0x03 // 150% LNA current

// RegIrqFlags (0x12): write 1 to clear a flag; the same bit set in
// RegIrqFlagsMask (0x11) disables that interrupt.
const (
	lrIrqCadDetected   = 0x01
	lrIrqFhssChange    = 0x02
	lrIrqCadDone       = 0x04
	lrIrqTxDone        = 0x08
	lrIrqValidHeader   = 0x10
	lrIrqPayloadCrcErr = 0x20
	lrIrqRxDone        = 0x40
	lrIrqRxTimeout     = 0x80
	lrIrqAll           = 0xFF
)

// RegDioMapping1 (0x40) DIO0 mapping in LoRa mode, bits 7:6.
const (
	dio0MappingMask = 0x3F
	dio0RxDone      = 0x00
	dio0TxDone      = 0x40
	dio0CadDone     = 0x80
)

// RegSyncWord (0x39): 1-byte LoRa sync word.
const (
	loraSyncWordPrivate = 0x12
	loraSyncWordPublic  = 0x34
)

// RegVersion (0x42) values used as a bring-up probe.
const (
	versionSX1276 = 0x12
	versionSX1272 = 0x22 // different RSSI offset
)

// RegModemStat (0x18) live status bits.
const (
	modemStatSignalDetected = 0x01
	modemStatSignalSync     = 0x02
	modemStatRxOngoing      = 0x04
	modemStatHeaderValid    = 0x08
	modemStatModemClear     = 0x10
)

// RSSI offsets are additive: dBm = offset + register value.
const (
	rssiOffsetHF = -157
	rssiOffsetLF = -164
	rssiOffset72 = -139
	rssiBandEdge = 525_000_000
)

// RegTcxo (0x4B) input selection.
const (
	tcxoCrystal = 0x00
	tcxoTcxo    = 0x10
)

// RegInvertIQ (0x33) / RegInvertIQ2 (0x3B) values.
const (
	invertIqRxMask = 0x40
	invertIqTxMask = 0x01
	invertIq2On    = 0x19
	invertIq2Off   = 0x1D
)

// FSK/OOK register bitfields (RegOpMode LongRangeMode == 0).
const (
	// RegPreambleDetect (0x1F).
	fskPreambleDetectorOn = 0x80
	fskPreambleDet1Byte   = 0x00
	fskPreambleDet2Byte   = 0x20
	fskPreambleDet3Byte   = 0x40
	fskPreambleDet4Byte   = 0x60

	// RegSyncConfig (0x27).
	fskAutoRestartRxWaitPll = 0x80
	fskPreamblePolarityAA   = 0x00
	fskPreamblePolarity55   = 0x20
	fskSyncOn               = 0x10
	fskSyncSizeMask         = 0x07 // (n-1) sync bytes

	// RegPacketConfig1 (0x30).
	fskPacketFormatVariable = 0x80
	fskPacketFormatFixed    = 0x00
	fskDcFreeOff            = 0x00
	fskDcFreeManchester     = 0x20
	fskDcFreeWhitening      = 0x40
	fskCrcOn                = 0x10
	fskCrcAutoClearOff      = 0x08
	fskAddrFilterOff        = 0x00

	// RegPacketConfig2 (0x31).
	fskDataModePacket     = 0x40
	fskDataModeContinuous = 0x00
	fskIoHomeOn           = 0x20

	// RegFifoThresh (0x35).
	fskTxStartFifoNotEmpty = 0x80
	fskTxStartFifoLevel    = 0x00
	fskFifoThresholdMask   = 0x3F
)

const (
	regSyncValueBaseFsk = 0x28 // RegSyncValue1..8 at 0x28..0x2F
	regPreambleMsbFsk   = 0x25
	regPreambleLsbFsk   = 0x26
)

// sx127xXtalFreqHz is the reference crystal frequency; Fstep =
// sx127xXtalFreqHz / 2^19 = 61.03515625 Hz.
const sx127xXtalFreqHz = 32_000_000

// sx127xFstepDiv is the 2^19 divisor in Frf = freqHz * 2^19 / Fxosc.
const sx127xFstepDiv = 1 << 19
