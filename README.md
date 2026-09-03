# meshcore-go

Go implementation of the [MeshCore](https://github.com/meshcore-dev/MeshCore) protocol, tracking firmware **v1.17.1**. Provides encode/decode for every protocol payload type, the cryptography, identity and key management, the companion serial protocol (frames, commands, responses, pushes) with a typed client, a KISS modem driver for bare radios, Serial/TCP transports for both, and a node runtime (routing, peers, channels, regions, retries, scheduling).

## Package Structure

```
meshcore-go/
  *.go                        # Core protocol: packet, payload types, crypto, identity,
                              # channels, regions, dedup, Cayenne LPP
  companion/
    constants.go              # Command / response / push / error codes, txt types
    frame.go                  # 0x3c/0x3e framing + streaming FrameParser
    commands.go               # Command encoders (ToBytes)
    responses.go              # Response and push parsers (ParseResponse)
    client/
      client.go               # Client: typed methods for every command, push handlers
      modem.go                # CompanionModem: adapts Client to node.Modem
    transport/                # Separate module (own go.mod, brings in go.bug.st/serial)
      transport.go            # Shared read/write loop, reconnect with backoff, TX queue
      serial.go, tcp.go       # Serial and TCP transports
  hardware/
    kiss.go                   # KISS framing, hardware sub-commands, RadioConfig
    modem.go                  # KissModem: RX metadata pairing, TX flow control
    airtime.go                # LoRa airtime estimator
    transport/                # Separate module (own go.mod)
      transport.go            # Shared read loop with frame resync
      serial.go, tcp.go       # Serial and TCP transports
  node/
    node.go                   # Node: identity, options, send helpers, lifecycle
    router.go                 # Flood/direct routing and forwarding policy
    dispatch.go               # Inbound packet dispatch
    peer.go                   # PeerTable: LRU peers, learned paths
    channel.go, region.go     # Channel table, RegionMap (flood scopes)
    crypto.go                 # Bounded shared-secret cache
    ack.go, retry.go          # ACK tracking and retransmission
    mux.go                    # RadioMux: several modems behind one TX engine
    queued_radio.go           # QueuedRadio: single radio behind a TX engine
    tx_engine.go, txqueue.go  # Priority queue, airtime budget, drain loop
    airtime.go, flood_delay.go# Duty-cycle budget, flood retransmit delay
    selfadvert.go             # Periodic self-advertisement
    radio.go                  # Radio / Modem interfaces
```

## Installation

```bash
# Core protocol + companion + client + hardware + node
go get github.com/meshcore-go/meshcore-go

# Transports are separate modules (they bring in go.bug.st/serial)
go get github.com/meshcore-go/meshcore-go/companion/transport
go get github.com/meshcore-go/meshcore-go/hardware/transport
```

## Quick Start

### Companion device over TCP

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/meshcore-go/meshcore-go/companion/client"
    "github.com/meshcore-go/meshcore-go/companion/transport"
)

func main() {
    t := transport.NewTCPTransport(transport.TCPConfig{Address: "localhost:5000"})

    c := client.New(t)
    c.SetErrorHandler(func(err error) { log.Printf("error: %v", err) })

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := c.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    info, err := c.DeviceQuery(ctx)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Firmware %s (%s), %d contacts max\n",
        info.FirmwareVersionStr, info.FirmwareBuildDate, info.MaxContacts)

    contacts, err := c.GetContacts(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, ct := range contacts {
        fmt.Printf("  %s (%x)\n", ct.AdvertName, ct.PublicKey[:6])
    }
}
```

### Serial

```go
t := transport.NewSerialTransport(transport.SerialConfig{
    Port:     "/dev/ttyUSB0",
    BaudRate: 115200,
})
c := client.New(t)
// same API as TCP
```

### Push handlers

`OnPush` returns an unsubscribe function.

```go
stop := c.OnPush(companion.PushSendConfirmed, func(resp companion.Response) {
    ack := resp.Data.(companion.PushSendConfirmedResponse)
    log.Printf("ACK %08x confirmed, round trip %d ms", ack.AckCode, ack.RoundTrip)
})
defer stop()

c.OnPush(companion.PushMsgWaiting, func(companion.Response) {
    msgs, err := c.GetWaitingMessages(ctx)
    // msgs holds contact texts, channel texts and channel datagrams
})
```

Requests to remote nodes (`SendLogin`, `SendStatusReq`, `SendTelemetryReq`, `SendBinaryReq`, `SendTracePath`) return once the firmware has queued the packet. The result arrives later as the matching push (`PushLoginSuccess`, `PushStatusResponse`, and so on).

### Core protocol only

```go
import meshcore "github.com/meshcore-go/meshcore-go"

pkt, err := meshcore.PacketFromBytes(raw)
fmt.Println(pkt.PayloadTypeString()) // "TXT_MSG"

msg, err := meshcore.TextMessageFromBytes(pkt.Payload)
plain := msg.Decrypt(sharedSecret) // nil when the MAC does not verify

grp, err := meshcore.GroupTextFromBytes(pkt.Payload)
post, err := grp.DecryptStruct(channelKey) // errors.Is(err, meshcore.ErrBadMAC) on a wrong key

appData := meshcore.AdvertAppData{Type: "CHAT", Name: "my-node", Lat: -368700000, Lon: 1749200000}
```

### A node behind a companion radio

```go
modem := client.NewCompanionModem(ctx, c)          // sends with CMD_SEND_RAW_PACKET
radio := node.NewQueuedRadio(modem, done)
n := node.New(identity, radio,
    node.WithAdvertData(appData),
    node.WithAllowForwardHandler(func(*meshcore.Packet) bool { return true }),
)
```

## API Overview

### Core protocol (`meshcore`)

| Type | Description |
|------|-------------|
| `Packet` | Header, encoded path, payload; `Clone`, `PathHashes`, `Validate` |
| `TextMessage`, `Request`, `Response`, `Path`, `AnonReq`, `GroupText`, `GroupData` | Encrypted payloads: `FromBytes`, `ToBytes`, `VerifyMAC`, `Decrypt`; `GroupText` and `Path` add `DecryptStruct` |
| `Advert` / `AdvertAppData` | Signed node advertisement; names are truncated at a UTF-8 boundary to fit 32 bytes |
| `Ack`, `Control`, `Trace`, `MultiPart`, `RawCustom` | Remaining payload types |
| `Identity` / `LocalIdentity` | Ed25519 keys, seed and firmware expanded-key import, key exchange |
| `ChannelEntry`, `Region` | Channel PSK/hash derivation, flood-scope transport keys |
| `DedupCache` | 160-entry packet-hash ring, as in the firmware |

Crypto: `DeriveSharedSecret`, `EncryptThenMAC`, `MACThenDecrypt` (AES-128-ECB plus truncated HMAC-SHA256). MAC failures return `ErrBadMAC`; truncated input returns `ErrTooShort`.

Cayenne LPP: `LPPEncoder` (27 data types including polyline) and `LPPDecode`, matching the firmware's own reader.

SNR on the wire is quarter-dB; `SNRFromWire` and `PathSNRdB` convert to real dB.

### Companion protocol (`companion`)

58 commands, 29 responses, 17 pushes. `ParseResponse` dispatches through a code-to-parser table; unknown codes come back with the raw payload. Path-length bytes in advert-path, path-discovery and trace frames are decoded with the firmware's hash-size encoding, and signed plain text exposes the sender prefix separately from the text.

### Client (`companion/client`)

One typed method per command, all taking a `context.Context` except `Reboot` and `FactoryReset`, which get no reply. Commands are serialised internally because the firmware answers in order with no correlation id.

- Device: `DeviceQuery`, `AppStart`, `SetDeviceTime`, `SyncDeviceTime`, `GetDeviceTime`, `GetBattAndStorage`, `GetStats`, `Reboot`, `FactoryReset`
- Contacts: `GetContacts`, `GetContactsSince`, `GetContactByKey`, `AddUpdateContact`, `AddUpdateContactFull`, `RemoveContact`, `ShareContact`, `ExportContact`, `ImportContact`, `ResetPath`, `GetAdvertPath`
- Messaging: `SendTextMessage`, `SendChannelTextMessage`, `SendChannelData`, `SendChannelDataFlood`, `GetWaitingMessages`
- Radio and tuning: `SetRadioParams`, `SetTxPower`, `SetTuningParams`, `GetTuningParams`, `GetAllowedRepeatFreq`, `SetPathHashMode`
- Configuration: `SetAdvertName`, `SetAdvertLatLon`, `SetChannel`, `GetChannel`, `SetAutoAddConfig`, `GetAutoAddConfig`, `SetDevicePin`, `SetOtherParams`, `GetCustomVars`, `SetCustomVar`
- Flood scopes: `SetFloodScope`, `SetFloodScopeUnscoped`, `SetDefaultFloodScope`, `ClearDefaultFloodScope`, `GetDefaultFloodScope`
- Security: `ExportPrivateKey`, `ImportPrivateKey`, `SignStart`, `SignData`, `SignFinish`
- Remote nodes: `SendLogin`, `Logout`, `HasConnection`, `SendStatusReq`, `SendTelemetryReq`, `SendBinaryReq`, `SendAnonReq`, `SendTracePath`, `SendPathDiscoveryReq`
- Raw: `SendSelfAdvert`, `SendRawPacket`, `SendPacket`, `SendRawData`, `SendControlData`

`CompanionModem` adapts the Client to `node.Modem`. It transmits with `SendRawPacket`, so the node's packets go on air unchanged, and receives every packet the radio hears via the log-RX push. `Close` detaches it from the Client.

### Transports (`companion/transport`, `hardware/transport`)

Both modules provide `SerialTransport` (via `go.bug.st/serial`) and `TCPTransport`. The companion transport reconnects with exponential backoff and queues outbound commands while offline (oldest dropped when full); `Send` after `Close` returns `ErrClosed`. The hardware transport exposes `Dead()`, which closes when the current connection's read loop exits, and can be closed and connected again.

### Hardware (`hardware`)

| Name | Description |
|------|-------------|
| `KissModem` | Raw packet send/receive over a KISS TNC; pairs each data frame with its RX metadata |
| `EncodeFrame`, `DecodeFrame`, `ExtractFrames`, `EscapeData`, `UnescapeData` | KISS framing |
| `EncodeHardwareFrame`, `DecodeHardwareFrame`, `RadioConfig` | Hardware sub-commands |
| `LoRaAirtimeEstimator` | Airtime from radio parameters; accepts coding rate as 1-4 or 5-8 |

Inbound frames are buffered (1024 by default, `WithInboundBuffer`); when full, the oldest is dropped with a warning so the read loop never blocks. Corrupted streams resync at the next frame boundary.

TX flow control is on by default with a fixed 5-second timeout. `SendData` rejects empty or over-255-byte packets up front, then waits for `HW_RESP_TX_DONE`; a result byte other than success returns `ErrTxFailed`, and no reply returns `ErrTxTimeout`. Hardware error frames seen mid-send are ignored, because firmware 1.17 also raises `HW_ERR_TX_BUSY` for host-output backpressure. `ErrTxBusy` remains exported but is no longer returned.

### Node runtime (`node`)

| Type | Description |
|------|-------------|
| `Node` | Mesh node: identity, radio, routing, channels, regions, retries; options are order-independent |
| `RadioMux` | Several modems behind one TX engine; `Stop` detaches from the modems |
| `QueuedRadio` | One radio behind a TX engine with `ErrTxQueueFull` backpressure |
| `Peer` / `PeerTable` | LRU peer table; out-paths learned from adverts are stored in send order |
| `RegionMap` | Flood scopes and transport-key lookup |

Routing follows the firmware: only ACK, PATH, REQ, RESPONSE, TXT_MSG, ANON_REQ, GRP_TXT, GRP_DATA and verified ADVERT packets are re-flooded, and a direct packet with hops remaining is relayed but not delivered locally. Forwarding is opt-in through `WithAllowForwardHandler`.

#### Handler contract

Handlers registered with `Node.OnPacket` and `Radio.SetDataHandler` run synchronously on the receive goroutine. Return promptly; hand slow work to your own goroutine or channel.

## Development

```bash
go test ./...                              # root, companion, hardware, node
(cd companion/transport && go test ./...)  # separate modules
(cd hardware/transport && go test ./...)
go test -race ./node/... ./hardware/...
go test -run=^$ -fuzz=FuzzPacketFromBytes -fuzztime=30s .
```

## License

See [LICENSE](LICENSE) for details.
