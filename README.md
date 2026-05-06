# meshcore-go

Go implementation of the [MeshCore](https://github.com/meshcore-dev/MeshCore) protocol. Provides encode/decode for all protocol payload types, cryptographic operations, identity/key management, the companion protocol (frame layer + command/response serialization), transport layers (Serial/TCP), a high-level client, hardware modem abstraction (KISS framing), and a node runtime (routing, peer management, scheduling).

## Package Structure

```
meshcore-go/
  *.go                        # Core protocol: packets, payloads, crypto, identity, Cayenne LPP
  companion/
    *.go                      # Companion protocol: frames, commands, responses, push codes
    client/
      client.go               # High-level Client with typed methods for all commands
      modem.go                # CompanionModem: bridges Client to node.Modem interface
    transport/                 # Separate module (companion/transport/go.mod)
      transport.go            # Transport interface + shared read/write loop
      serial.go               # Serial transport (go.bug.st/serial)
      tcp.go                  # TCP transport
  hardware/
    modem.go                  # Hardware modem abstraction
    kiss.go                   # KISS framing protocol
    transport/                # Separate module (hardware/transport/go.mod)
      transport.go            # Transport interface
      serial.go               # Serial transport
      tcp.go                  # TCP transport
  node/
    node.go                   # Node runtime: startup, identity, radio integration
    router.go                 # Packet routing and forwarding
    peer.go                   # Peer tracking and management
    mux.go                    # Radio multiplexer (multi-modem support)
    scheduler.go              # TX scheduling and duty-cycle management
    channel.go                # Channel/group configuration
    dedup.go                  # Packet deduplication
    dispatch.go               # Incoming packet dispatch
    txqueue.go                # Transmit queue with priority
    airtime.go                # Airtime estimation
    selfadvert.go             # Periodic self-advertisement
```

## Installation

```bash
# Core protocol + companion + client
go get github.com/meshcore-go/meshcore-go

# Transport layer (separate module, brings in go.bug.st/serial)
go get github.com/meshcore-go/meshcore-go/companion/transport
```

## Quick Start

### TCP Connection

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
    t := transport.NewTCPTransport(transport.TCPConfig{
        Address: "localhost:5000",
    })

    c := client.New(t)
    c.SetErrorHandler(func(err error) {
        log.Printf("error: %v", err)
    })

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
    fmt.Printf("Firmware: %s (v%d)\n", info.FirmwareBuildDate, info.FirmwareVersion)

    contacts, err := c.GetContacts(ctx)
    if err != nil {
        log.Fatal(err)
    }
    for _, c := range contacts {
        fmt.Printf("  %s (%x)\n", c.AdvertName, c.PublicKey[:6])
    }
}
```

### Serial Connection

```go
t := transport.NewSerialTransport(transport.SerialConfig{
    Port:     "/dev/ttyUSB0",
    BaudRate: 115200,
})
c := client.New(t)
// ... same API as TCP
```

### Push Handlers

```go
c.OnPush(companion.PushMsgWaiting, func(resp companion.Response) {
    msg := resp.Data.(companion.PushMsgWaitingResponse)
    log.Printf("message waiting from %x", msg.SenderPrefix)
})

c.OnPush(companion.PushSendConfirmed, func(resp companion.Response) {
    ack := resp.Data.(companion.PushSendConfirmedResponse)
    log.Printf("ACK confirmed: %08x (round trip: %dms)", ack.AckCode, ack.RoundTrip)
})
```

### Core Protocol (without companion layer)

```go
import meshcore "github.com/meshcore-go/meshcore-go"

// Decode a packet
pkt, err := meshcore.PacketFromBytes(rawData)
fmt.Println(pkt.PayloadTypeString()) // "TXT_MSG"

// Decode a text message payload
msg, err := meshcore.TextMessageFromBytes(pkt.Payload)
if msg.VerifyMAC(sharedSecret) {
    plaintext := msg.Decrypt(sharedSecret)
}

// Encode an advert
appData := &meshcore.AdvertAppData{
    Type: "CHAT",
    Name: "my-node",
    Lat:  -368700000,
    Lon:  1749200000,
}
```

## API Overview

### Core Protocol (`meshcore`)

| Type | Description |
|------|-------------|
| `Packet` | Mesh packet header, path, payload |
| `TextMessage` | Encrypted text message with MAC |
| `Advert` / `AdvertAppData` | Node advertisement with Ed25519 signing |
| `Identity` / `LocalIdentity` | Ed25519 key management and key exchange |
| `Ack` | Acknowledgement with CRC |
| `Request` / `Response` | Encrypted request/response payloads |
| `GroupText` / `GroupData` | Channel-based group messaging |
| `AnonReq` | Anonymous request |
| `Path` | Routing path data |
| `Control` | Control messages |
| `Trace` | Path trace with hash chain |
| `MultiPart` | Multi-part message fragments |
| `RawCustom` | Raw custom payload |

Crypto: `DeriveSharedSecret`, `EncryptThenMAC`, `MACThenDecrypt`, AES-128-ECB.

Cayenne LPP: `LPPEncoder` (26 sensor types) and `LPPDecode`.

### Companion Protocol (`companion`)

55 command types, 28 response types, 14 push notification types. Frame layer with streaming parser.

### Client (`companion/client`)

Typed methods wrapping all companion commands:

- Device: `DeviceQuery`, `AppStart`, `SetDeviceTime`, `SyncDeviceTime`, `GetBatteryVoltage`, `Reboot`, `FactoryReset`
- Contacts: `GetContacts`, `GetContactsSince`, `AddUpdateContact`, `RemoveContact`, `ShareContact`, `ExportContact`, `ImportContact`, `GetContactByKey`
- Messaging: `SendTextMessage`, `SendChannelTextMessage`, `SendChannelData`, `GetWaitingMessages`
- Radio: `SetRadioParams`, `SetTxPower`, `SetTuningParams`, `GetTuningParams`
- Configuration: `SetAdvertName`, `SetAdvertLatLon`, `SetChannel`, `GetChannel`, `SetAutoAddConfig`, `SetDevicePin`, `SetOtherParams`, `SetFloodScope`
- Security: `ExportPrivateKey`, `ImportPrivateKey`, `SendLogin`, `Logout`, `SignStart`, `SignData`, `SignFinish`
- Network: `SendSelfAdvert`, `SendTracePath`, `SendPathDiscoveryReq`, `HasConnection`, `GetStats`, `GetAdvertPath`, `ResetPath`
- Raw/Advanced: `SendRawData`, `SendControlData`, `SendBinaryReq`, `SendAnonReq`, `SendTelemetryReq`, `GetCustomVars`, `SetCustomVar`

`CompanionModem` adapts the Client to the `node.Modem` interface for bridging companion devices into the node runtime.

### Transport (`companion/transport`)

| Type | Description |
|------|-------------|
| `Transport` | Interface: `Connect`, `Close`, `Send`, `SetResponseHandler`, `SetErrorHandler` |
| `SerialTransport` | Serial port via `go.bug.st/serial` |
| `TCPTransport` | TCP socket via `net.Dial` |

### Hardware (`hardware`)

| Type | Description |
|------|-------------|
| `Modem` | Hardware modem abstraction for raw packet send/receive |
| `KISSEncoder` / `KISSDecode` | KISS framing for serial packet transport |

`hardware/transport` is a separate module providing Serial and TCP transports for direct hardware connections.

#### Backpressure & Reliability

The `KissModem` buffers up to 64 inbound frames (configurable via `WithInboundBuffer`). If the buffer fills (e.g., handlers are slow), the **oldest frame is dropped** to keep the transport read loop flowing — the modem will never block on receive. A warning is logged when this occurs.

Each transport exposes a `Dead() <-chan struct{}` channel that closes when the read loop exits (I/O error, disconnection). Select on this to detect a dead transport and trigger reconnection at a higher layer.

If the byte stream becomes corrupted (remainder exceeds max frame size without a valid FEND), the parser discards the buffered bytes and resyncs at the next frame boundary.

#### TX Flow Control

TX flow control is **enabled by default** (5-second fixed timeout). After each `SendData` call, the modem blocks until `HW_RESP_TX_DONE` is received from the firmware. If the response doesn't arrive within the timeout, `SendData` returns `ErrTxTimeout` and the packet is re-enqueued by the transmit queue for retry on the next drain cycle.

For production use, provide an airtime estimator via `WithTxAirtimeEstimator(fn)` to compute per-packet timeouts dynamically as **1.5× estimated airtime** (matching the C++ MeshCore dispatcher). Use `WithTxFlowControl(0)` to disable flow control entirely.

### Node Runtime (`node`)

| Type | Description |
|------|-------------|
| `Node` | Full mesh node: identity, radio, routing, scheduling |
| `Router` | Packet routing and forwarding decisions |
| `RadioMux` | Multi-modem multiplexer |
| `Peer` / `PeerTable` | Peer tracking and path management |
| `Scheduler` | TX scheduling with duty-cycle enforcement |
| `TxQueue` | Priority-based transmit queue |

#### Handler Contract

Packet handlers registered via `Node.OnPacket` and `Radio.SetDataHandler` are invoked **synchronously** on the receive goroutine. Handlers must return promptly — a blocking handler stalls the entire receive pipeline for that radio. If you need to do slow work (I/O, network calls, heavy computation), dispatch to your own goroutine or channel from within the handler.

## Development

This project uses Go workspaces for multi-module development:

```bash
# Run all tests
go test ./...

# Run transport tests (separate module)
cd companion/transport && go test ./...

# Coverage
go test -coverprofile=cover.out ./...
go tool cover -func=cover.out
```

## License

See [LICENSE](LICENSE) for details.
