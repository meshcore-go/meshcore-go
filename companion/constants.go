package companion

// SupportedProtocolVersion is the protocol version this library advertises to
// the firmware via CMD_DEVICE_QUERY (app_target_ver). Firmware 1.16 gates only
// one response-format behavior on it: app_target_ver >= 3 selects the V3
// contact/channel message-receive frames (RESP_*_MSG_RECV_V3, which prepend SNR
// + 2 reserved bytes). There are no higher app-version gates, so 3 unlocks every
// 1.16 response format this library parses.
const SupportedProtocolVersion = 3

// Serial frame types.
const (
	FrameTypeIncoming byte = 0x3e
	FrameTypeOutgoing byte = 0x3c
)

// BLE UUIDs.
const (
	BLEServiceUUID          = "6E400001-B5A3-F393-E0A9-E50E24DCCA9E"
	BLECharacteristicRxUUID = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"
	BLECharacteristicTxUUID = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E"
)

// Command codes.
const (
	CmdAppStart             = 1
	CmdSendTxtMsg           = 2
	CmdSendChannelTxtMsg    = 3
	CmdGetContacts          = 4
	CmdGetDeviceTime        = 5
	CmdSetDeviceTime        = 6
	CmdSendSelfAdvert       = 7
	CmdSetAdvertName        = 8
	CmdAddUpdateContact     = 9
	CmdSyncNextMessage      = 10
	CmdSetRadioParams       = 11
	CmdSetRadioTxPower      = 12
	CmdResetPath            = 13
	CmdSetAdvertLatLon      = 14
	CmdRemoveContact        = 15
	CmdShareContact         = 16
	CmdExportContact        = 17
	CmdImportContact        = 18
	CmdReboot               = 19
	CmdGetBattAndStorage    = 20
	CmdSetTuningParams      = 21
	CmdDeviceQuery          = 22
	CmdExportPrivateKey     = 23
	CmdImportPrivateKey     = 24
	CmdSendRawData          = 25
	CmdSendLogin            = 26
	CmdSendStatusReq        = 27
	CmdHasConnection        = 28
	CmdLogout               = 29
	CmdGetContactByKey      = 30
	CmdGetChannel           = 31
	CmdSetChannel           = 32
	CmdSignStart            = 33
	CmdSignData             = 34
	CmdSignFinish           = 35
	CmdSendTracePath        = 36
	CmdSetDevicePin         = 37
	CmdSetOtherParams       = 38
	CmdSendTelemetryReq     = 39
	CmdGetCustomVars        = 40
	CmdSetCustomVar         = 41
	CmdGetAdvertPath        = 42
	CmdGetTuningParams      = 43
	CmdSendBinaryReq        = 50
	CmdFactoryReset         = 51
	CmdSendPathDiscoveryReq = 52
	CmdSetFloodScopeKey     = 54
	CmdSendControlData      = 55
	CmdGetStats             = 56
	CmdSendAnonReq          = 57
	CmdSetAutoAddConfig     = 58
	CmdGetAutoAddConfig     = 59
	CmdGetAllowedRepeatFreq = 60
	CmdSetPathHashMode      = 61
	CmdSendChannelData      = 62
	CmdSetDefaultFloodScope = 63
	CmdGetDefaultFloodScope = 64
	CmdSendRawPacket        = 65
)

// Response codes.
const (
	RespOk                = 0
	RespErr               = 1
	RespContactsStart     = 2
	RespContact           = 3
	RespEndOfContacts     = 4
	RespSelfInfo          = 5
	RespSent              = 6
	RespContactMsgRecv    = 7
	RespChannelMsgRecv    = 8
	RespCurrTime          = 9
	RespNoMoreMessages    = 10
	RespExportContact     = 11
	RespBattAndStorage    = 12
	RespDeviceInfo        = 13
	RespPrivateKey        = 14
	RespDisabled          = 15
	RespContactMsgRecvV3  = 16
	RespChannelMsgRecvV3  = 17
	RespChannelInfo       = 18
	RespSignStart         = 19
	RespSignature         = 20
	RespCustomVars        = 21
	RespAdvertPath        = 22
	RespTuningParams      = 23
	RespStats             = 24
	RespAutoAddConfig     = 25
	RespAllowedRepeatFreq = 26
	RespChannelDataRecv   = 27
	RespDefaultFloodScope = 28
)

// Push codes (asynchronous firmware notifications, codes >= 0x80).
const (
	PushAdvert                byte = 0x80
	PushPathUpdated           byte = 0x81
	PushSendConfirmed         byte = 0x82
	PushMsgWaiting            byte = 0x83
	PushRawData               byte = 0x84
	PushLoginSuccess          byte = 0x85
	PushLoginFail             byte = 0x86
	PushStatusResponse        byte = 0x87
	PushLogRxData             byte = 0x88
	PushTraceData             byte = 0x89
	PushNewAdvert             byte = 0x8A
	PushTelemetryResponse     byte = 0x8B
	PushBinaryResponse        byte = 0x8C
	PushPathDiscoveryResponse byte = 0x8D
	PushControlData           byte = 0x8E
	PushContactDeleted        byte = 0x8F
	PushContactsFull          byte = 0x90
)

const (
	StatsTypeCore    = 0
	StatsTypeRadio   = 1
	StatsTypePackets = 2
)

const (
	AutoAddOverwriteOldest = 0x01
	AutoAddChat            = 0x02
	AutoAddRepeater        = 0x04
	AutoAddRoomServer      = 0x08
	AutoAddSensor          = 0x10
)

// Error codes (used in RespErr responses).
const (
	ErrUnsupportedCmd = 1
	ErrNotFound       = 2
	ErrTableFull      = 3
	ErrBadState       = 4
	ErrFileIoError    = 5
	ErrIllegalArg     = 6
)

// Frame size limits.
const (
	MaxFrameSize    = 176 // firmware BaseSerialInterface.h MAX_FRAME_SIZE = 176 ("+4 for transport codes / region scoping")
	FrameHeaderSize = 3   // type(1) + length(2)
)
