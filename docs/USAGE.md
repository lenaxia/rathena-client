# rathena-client — Integration Guide

**Audience**: LLMs and developers integrating this library into a consumer (primarily goKore).

**Module**: `github.com/lenaxia/rathena-client`

This guide covers how to use every public package. Read it in order — later sections build on earlier ones.

---

## Table of Contents

1. [What the library does and does not do](#1-what-the-library-does-and-does-not-do)
2. [Package map](#2-package-map)
3. [pkg/packing — bit-packing codecs](#3-pkgpacking--bit-packing-codecs)
4. [pkg/events — typed S→C event structs](#4-pkgevents--typed-sc-event-structs)
5. [pkg/send — typed C→S request structs](#5-pkgsend--typed-cs-request-structs)
6. [pkg/decode — decode functions (S→C)](#6-pkgdecode--decode-functions-sc)
7. [pkg/encode — encode functions (C→S)](#7-pkgencode--encode-functions-cs)
8. [pkg/session — framer and dispatcher](#8-pkgsession--framer-and-dispatcher)
9. [pkg/fsm — full auth sequencer](#9-pkgfsm--full-auth-sequencer)
10. [End-to-end integration pattern](#10-end-to-end-integration-pattern)
11. [Performance contract](#11-performance-contract)
12. [Concurrency model](#12-concurrency-model)
13. [Error handling](#13-error-handling)
14. [Known limitations](#14-known-limitations)

---

## 1. What the library does and does not do

**Does:**
- Drives the three-phase rAthena auth sequence (login → char → map) via `pkg/fsm`
- Frames incoming TCP streams into typed packets via `pkg/session`
- Decodes raw packet bytes into typed Go structs via `pkg/decode`
- Encodes send requests into raw bytes via `pkg/encode`
- Handles C→S packet ID obfuscation (per-packetver LCG) via `MapSession.Encode`
- Handles C→S packet ID shuffle (per-packetver lookup table) via `session.ShuffledCtoSID`

**Does not:**
- Open network connections (`net.Dial`) — the caller provides a `Dialer`
- Own `net.Conn` after auth completes — after `OnReady` fires, `conn` belongs to the caller
- Run goroutines — all operations are synchronous in the caller's goroutine
- Allocate in the decode hot path — `Feed()` is 0 allocs/op in steady state
- Have external dependencies — `go.mod` has zero `require` entries

---

## 2. Package map

```
pkg/
    packing/    DecodePosDir, EncodePosDir, DecodeMoveData, EncodeMoveData
    events/     281 typed event structs (one per semantic action, S→C)
    send/       152 typed request structs (one per semantic action, C→S)
    decode/     282 generated decode functions: FooAction_0xNNNN(data, packetver)
    encode/     126 generated encode functions: EncodeFooAction(req, packetver)
    session/    LoginSession, CharSession, MapSession + framing engine
    fsm/        ConnectionFSM
```

Generated packages (`events`, `send`, `decode`, `encode`, and the generated parts of `session`) are committed to the repository. They are regenerated only when rAthena source changes or `semantics/mappings.yaml` is updated.

---

## 3. pkg/packing — bit-packing codecs

Two packed binary formats appear throughout the rAthena wire protocol. They are implemented once here; all generated decode functions call these helpers.

### 3-byte position+direction (WBUFPOS / PosDir[3])

Appears in `packet_idle_unit`, `packet_unit_walking`, and others as the `PosDir[3]` field.

```go
import "github.com/lenaxia/rathena-client/pkg/packing"

// Decode
x, y, dir := packing.DecodePosDir(someEvent.PosDir[:])
// x, y: uint16 map coordinates (0–1023)
// dir:  uint8 direction (0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE)

// Encode
posDir := packing.EncodePosDir(x, y, dir) // returns [3]byte
```

### 6-byte movement record (WBUFPOS2 / MoveData[6])

Appears in walking-unit packets as the `MoveData[6]` field.

```go
fromX, fromY, toX, toY, sx0, sy0 := packing.DecodeMoveData(someEvent.MoveData[:])
// fromX/fromY: origin cell (uint16)
// toX/toY:     destination cell (uint16)
// sx0/sy0:     sub-cell interpolation offsets (uint8, 4 bits each — ignore for bot use)

// IMPORTANT: byte 5 of MoveData is NOT a direction value.
// There is no direction in the 6-byte format.
// Extracting direction as (data[5] & 0xF0) >> 4 is a known goKore v1 bug — do not repeat it.

moveData := packing.EncodeMoveData(fromX, fromY, toX, toY, sx0, sy0) // returns [6]byte
```

---

## 4. pkg/events — typed S→C event structs

One struct per semantic action. All fields are Go primitive types or fixed-size byte arrays — no pointers, no slices, no `interface{}`.

```go
import "github.com/lenaxia/rathena-client/pkg/events"

// Example: events.ActorExists — fired when a stationary entity enters viewport
type ActorExists struct {
    ID          uint32   // Actor unique identifier (rAthena: GID)
    CharID      uint32   // Character ID for players (rAthena: GUID)
    Type        int16    // Job class ID (rAthena: job)
    Object_type uint8    // Entity type: 0=PC, 5=MOB (rAthena: objecttype)
    PosDir      [3]byte  // Packed position+direction — use packing.DecodePosDir
    Walk_speed  int16    // Movement speed in ms/cell (rAthena: speed)
    HP          int32    // Current HP
    MaxHP       int32    // Maximum HP
    Name        string   // Entity name
    // ... many more fields
}
```

**Key design points:**
- `PosDir [3]byte` and `MoveData [6]byte` are raw packed bytes, not decoded coordinates. Call `packing.DecodePosDir` / `packing.DecodeMoveData` to unpack them.
- String fields (`Name`) are decoded from null-terminated C strings by the decode functions.
- Fields absent in older PACKETVER variants are zero-valued — check `packetver` if you need to distinguish "field is zero" from "field is absent".
- Event structs are passed **by value** to callbacks. They are stack-allocated inside generated decode functions and the Go compiler must not escape them to the heap (verified by benchmarks).

---

## 5. pkg/send — typed C→S request structs

One struct per outbound action. Built by the caller and passed to an encode function.

```go
import "github.com/lenaxia/rathena-client/pkg/send"

// Example: NPC contact request
type NpcContact struct {
    NPCID uint32 // NPC actor ID
    Type  uint8  // Contact type (1=talk)
}

// Example: skill use request
type SkillUse struct {
    SkillID  uint16
    Level    uint16
    TargetID uint32
}

// Example: send a chat message
type SendChat struct {
    Message string
}
```

The structs are simple value types. Populate them and pass to the corresponding `pkg/encode` function.

---

## 6. pkg/decode — decode functions (S→C)

One decode function per (semantic action, packet ID) pair. Named `ActionName_0xNNNN(data []byte, packetver uint32) events.ActionName`.

```go
import (
    "github.com/lenaxia/rathena-client/pkg/decode"
    "github.com/lenaxia/rathena-client/pkg/events"
)

// Called inside a registered handler:
ms.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
    e := decode.ActorExists_0x09FF(data, pv)
    // e is events.ActorExists, on the stack, 0 allocs
    handleActor(e)
})

ms.RegisterHandler(0x0078, func(data []byte, pv uint32) {
    e := decode.ActorExists_0x0078(data, pv)
    handleActor(e)
})
```

**How to find the right decode function for a packet ID:**

The naming convention is `CamelCaseActionName_0xNNNN` where:
- `CamelCaseActionName` matches the `events.ActionName` struct (e.g., `ActorExists` → `decode.ActorExists_*`)
- `0xNNNN` is the specific packet ID (one action may have multiple packet IDs across PACKETVER ranges)

To find which decode function handles a given packet ID, search `pkg/decode/` for the hex ID:
```bash
grep -rl "0x09FF" pkg/decode/
```

**Multiple packet IDs for the same action:**

Multiple packet IDs often map to the same semantic action (the server changed IDs in different PACKETVER). Register a handler for each:

```go
// ActorExists is sent as 0x0078, 0x01D8, or 0x09FF depending on rAthena version
for _, id := range []uint16{0x0078, 0x01D8, 0x09FF} {
    id := id
    ms.RegisterHandler(id, func(data []byte, pv uint32) {
        var e events.ActorExists
        switch id {
        case 0x0078: e = decode.ActorExists_0x0078(data, pv)
        case 0x01D8: e = decode.ActorExists_0x01D8(data, pv)
        case 0x09FF: e = decode.ActorExists_0x09FF(data, pv)
        }
        handleActor(e)
    })
}
```

**PACKETVER handling inside decode functions:**

Each decode function handles PACKETVER variation internally via `if packetver >= YYYYMMDD` chains. The `packetver` argument is passed through from `session.Feed()` — you do not need to branch on it at the call site.

---

## 7. pkg/encode — encode functions (C→S)

One encode function per semantic action. Named `EncodeFooAction(req send.FooAction, packetver uint32) T` where `T` is either a fixed-size `[N]byte` array or `[]byte` (variable-length).

```go
import (
    "github.com/lenaxia/rathena-client/pkg/encode"
    "github.com/lenaxia/rathena-client/pkg/send"
)

// Encode an NPC contact request — returns [7]byte (fixed size, no alloc)
pkt := encode.EncodeNpcContact(send.NpcContact{NPCID: 12345, Type: 1}, packetver)

// Apply C→S obfuscation (if enabled on the session)
// ⚠ Use the correct pattern — read the ID, XOR it, write it back
id := binary.LittleEndian.Uint16(pkt[0:2])
mapSess.Encode(&id)
binary.LittleEndian.PutUint16(pkt[0:2], id)

// Write to the connection
conn.Write(pkt[:])
```

**Correct obfuscation pattern:**

`MapSession.Encode` takes a `*uint16` pointing to the packet ID. The FSM demonstrates the correct pattern:

```go
// fsm/fsm.go pattern:
func encodePacketID(s *session.MapSession, pkt []byte) {
    id := binary.LittleEndian.Uint16(pkt[0:2])
    s.Encode(&id)
    binary.LittleEndian.PutUint16(pkt[0:2], id)
}
```

Apply this to every C→S packet before writing to the socket when obfuscation is enabled.

**Packet ID shuffle (clif_shuffle.hpp):**

For some PACKETVER ranges, the C→S packet IDs are shuffled. The shuffle is already baked into the generated encode functions — they emit the shuffled ID directly. You do not need to call `session.ShuffledCtoSID` explicitly when using the generated encode functions.

---

## 8. pkg/session — framer and dispatcher

Three session types: `LoginSession`, `CharSession`, `MapSession`. All share the same interface pattern.

### Creating a session

```go
import "github.com/lenaxia/rathena-client/pkg/session"

packetver := uint32(20180307)

loginSess := session.NewLoginSession(packetver)
charSess  := session.NewCharSession(packetver)
mapSess   := session.NewMapSession(packetver)
```

The `packetver` value selects the correct packet length table and is passed through to every handler callback.

### Registering handlers

```go
// Register a handler for packet ID 0x09FF
mapSess.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
    e := decode.ActorExists_0x09FF(data, pv)
    // data includes the 2-byte packet ID header
    // pv is the packetver the session was created with
})
```

A second call with the same packet ID overwrites the previous handler.

### Feeding data

```go
// In your read loop:
buf := make([]byte, 4096)
for {
    n, err := conn.Read(buf)
    if n > 0 {
        if feedErr := mapSess.Feed(buf[:n]); feedErr != nil {
            var unk session.ErrUnknownPacket
            if errors.As(feedErr, &unk) {
                // Stream desynced — unknown packet ID 0xNNNN
                // Close the connection immediately.
                conn.Close()
                return
            }
            // Other feed error
        }
    }
    if err != nil { break }
}
```

**Feed guarantees:**
- Synchronous — callbacks fire in the `Feed` call, in the caller's goroutine
- Reentrant-safe within a single goroutine, but NOT goroutine-safe across goroutines
- Frame accumulation: partial frames are buffered internally; `Feed` may be called with any chunk size
- Unknown packet ID returns `ErrUnknownPacket` once and then silences subsequent calls (stream is irrecoverable; close the connection)

### C→S obfuscation on MapSession

```go
// After creating MapSession, enable obfuscation if keys exist for this packetver.
k0, k1, k2 := session.ObfuscationKeysFor(packetver)
if k0|k1|k2 != 0 {
    mapSess.EnableObfuscation(k0, k1, k2)
}

// Before each C→S write, obfuscate the packet ID in-place:
pkt := encode.EncodeSomeRequest(req, packetver)
id := binary.LittleEndian.Uint16(pkt[0:2])
mapSess.Encode(&id)
binary.LittleEndian.PutUint16(pkt[0:2], id)
conn.Write(pkt)
```

**Only C→S packets need obfuscation.** S→C packets received via `Feed` are never obfuscated.

### SetLength (advanced)

`SetLength(id uint16, length int16)` overrides the packet length table entry for a specific ID. It is intended for:
- Auth-phase setup in the FSM (e.g., overriding 0x0081 from 3 to 28 on the char server to handle `HC_NOTIFY_ZONESVR`)
- Testing with synthetic frames

Callers should rarely need this directly.

---

## 9. pkg/fsm — full auth sequencer

`ConnectionFSM` drives the complete login → char → map authentication sequence synchronously.

### Creating and configuring the FSM

```go
import (
    "context"
    "net"

    "github.com/lenaxia/rathena-client/pkg/fsm"
    "github.com/lenaxia/rathena-client/pkg/session"
)

f := fsm.New(
    fsm.ServerConfig{
        LoginAddr:   "127.0.0.1:6900",
        Packetver:   20180307,        // YYYYMMDD
        StepTimeout: 30 * time.Second, // per-step deadline (default 30s if zero)
    },
    fsm.Credentials{
        Username: "admin",
        Password: "admin",
        CharSlot: 0, // used when OnCharList is not registered
    },
    func(ctx context.Context, addr string) (net.Conn, error) {
        return net.Dial("tcp", addr)
    },
)
```

### Registering callbacks

All callbacks are optional. The FSM uses defaults when they are not registered.

```go
// Called after login accept — choose which char server to connect to.
// Receives the advertised server list; returns the index to use. Default: index 0.
f.OnCharServerList(func(servers []fsm.CharServerInfo) int {
    for i, s := range servers {
        if s.Name == "Ragnarok" {
            return i
        }
    }
    return 0
})

// Called when the full character list has been assembled.
// Receives raw CHARACTER_INFO bytes (variable layout per PACKETVER).
// Returns the slot number to select. Default: Credentials.CharSlot.
f.OnCharList(func(rawChars []byte) uint8 {
    // parse rawChars with your own codec
    return 0
})

// Called when the map server accepts entry and the map-loaded sequence completes.
// The FSM passes the ready MapSession and live net.Conn. After this call,
// the FSM releases conn — you own it.
f.OnReady(func(ms *session.MapSession, conn net.Conn) {
    // Register your game handlers on ms
    ms.RegisterHandler(0x09FF, func(data []byte, pv uint32) { ... })

    // Start your read loop in a goroutine (outside the library)
    go func() {
        defer conn.Close()
        buf := make([]byte, 65536)
        for {
            n, err := conn.Read(buf)
            if n > 0 { ms.Feed(buf[:n]) }
            if err != nil { return }
        }
    }()
})

// Called on any unrecoverable error during auth.
f.OnFailed(func(err error) {
    log.Printf("auth failed: %v", err)
})

// Called when SC_NOTIFY_BAN (0x0081) is received during auth.
f.OnServerNotify(func(code uint8) {
    log.Printf("server ban code: %d", code)
})
```

### Running the FSM

```go
// Connect blocks until OnReady or OnFailed fires, then returns.
// May be called again on the same FSM for reconnection (auth state is reset).
if err := f.Connect(context.Background()); err != nil {
    log.Fatal(err)
}
```

**Connect is fully synchronous** in the caller's goroutine. It dials three separate TCP connections in sequence (login → char → map), feeds each session until the auth step completes, then hands off the map connection to `OnReady`.

### Auth sequence detail

```
Connect(ctx)
  → dial LoginAddr
  → send 0x0064 CA_LOGIN (or 0x01DD for older clients)
  → read until 0x0069/0x0AC4 (accepted) or 0x006A/0x083E/0x0081 (refused)
  → call OnCharServerList → choose server
  → dial chosen CharServer
  → send 0x0065 CH_ENTER
  → read 4-byte account ID echo (raw, not framed)
  → read until char list assembled (0x006B or 0x09A0+0x099D pages)
  → call OnCharList → choose slot
  → send 0x0066 CH_SELECT_CHAR
  → read until 0x0081/0x0AC5 (zone server address)
  → dial MapServer
  → send 0x0436 CZ_ENTER (with C→S obfuscation if keys exist)
  → read until 0x0073/0x02EB/0x0A18 ZC_ACCEPT_ENTER
  → send 0x007D CZ_NOTIFY_ACTORINIT + 0x007E/0x0360 CZ_REQUEST_TIME
  → call OnReady(mapSess, conn)
  → return nil
```

---

## 10. End-to-end integration pattern

The canonical goKore integration pattern:

```go
package network

import (
    "context"
    "encoding/binary"
    "errors"
    "net"

    "github.com/lenaxia/rathena-client/pkg/decode"
    "github.com/lenaxia/rathena-client/pkg/encode"
    "github.com/lenaxia/rathena-client/pkg/events"
    "github.com/lenaxia/rathena-client/pkg/fsm"
    "github.com/lenaxia/rathena-client/pkg/packing"
    "github.com/lenaxia/rathena-client/pkg/send"
    "github.com/lenaxia/rathena-client/pkg/session"
)

type Connector struct {
    fsm        *fsm.ConnectionFSM
    ms         *session.MapSession
    conn       net.Conn
    packetver  uint32
}

func NewConnector(loginAddr string, pv uint32, user, pass string) *Connector {
    c := &Connector{packetver: pv}

    c.fsm = fsm.New(
        fsm.ServerConfig{LoginAddr: loginAddr, Packetver: pv},
        fsm.Credentials{Username: user, Password: pass},
        func(ctx context.Context, addr string) (net.Conn, error) {
            return net.Dial("tcp", addr)
        },
    )

    c.fsm.OnReady(func(ms *session.MapSession, conn net.Conn) {
        c.ms = ms
        c.conn = conn

        // Register game-state handlers
        ms.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
            e := decode.ActorExists_0x09FF(data, pv)
            c.onActorExists(e)
        })
        ms.RegisterHandler(0x09DB, func(data []byte, pv uint32) {
            e := decode.ActorMoved_0x09DB(data, pv)
            c.onActorMoved(e)
        })

        // Start read loop outside the library
        go c.readLoop()
    })

    return c
}

func (c *Connector) Connect(ctx context.Context) error {
    return c.fsm.Connect(ctx)
}

func (c *Connector) readLoop() {
    buf := make([]byte, 65536)
    for {
        n, err := c.conn.Read(buf)
        if n > 0 {
            if ferr := c.ms.Feed(buf[:n]); ferr != nil {
                var unk session.ErrUnknownPacket
                if errors.As(ferr, &unk) {
                    // Unknown packet ID — stream desynced
                    c.conn.Close()
                    return
                }
            }
        }
        if err != nil {
            return
        }
    }
}

// SendNpcContact talks to an NPC by actor ID.
func (c *Connector) SendNpcContact(npcID uint32) error {
    pkt := encode.EncodeNpcContact(send.NpcContact{NPCID: npcID, Type: 1}, c.packetver)
    id := binary.LittleEndian.Uint16(pkt[0:2])
    c.ms.Encode(&id)
    binary.LittleEndian.PutUint16(pkt[0:2], id)
    _, err := c.conn.Write(pkt[:])
    return err
}

func (c *Connector) onActorExists(e events.ActorExists) {
    x, y, dir := packing.DecodePosDir(e.PosDir[:])
    _ = x; _ = y; _ = dir
    // update world model
}

func (c *Connector) onActorMoved(e events.ActorMoved) {
    fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])
    _ = fromX; _ = fromY; _ = toX; _ = toY
    // update world model
}
```

**Note on `MapSession.Packetver()`**: this accessor is not currently exposed. Use the `packetver` value you passed to `fsm.ServerConfig` instead.

---

## 11. Performance contract

These are hard constraints verified by benchmarks, not aspirational targets.

| Path | Target |
|---|---|
| `session.Feed()` steady state | 0 allocs/op, < 200 ns/op (fixed packet) |
| `decode.ActorExists_0x09FF` | 0 allocs/op, < 500 ns/op |
| `encode.EncodeNpcContact` | 0 allocs/op, < 100 ns/op |

**Why 0 allocs matters at scale**: at 1000 concurrent bots, any allocation in the decode path multiplies by 1000 per packet received. The library's zero-alloc contract is what makes bot-scale deployments feasible.

Verify before shipping:

```bash
go test -bench=. -benchmem ./pkg/...
# Output should show 0 allocs/op on all hot-path benchmarks
```

---

## 12. Concurrency model

The library is **not goroutine-safe**. Each session (`LoginSession`, `CharSession`, `MapSession`) must be accessed from a single goroutine.

The intended model:
- The FSM runs in the caller's goroutine during auth (`Connect` blocks).
- After `OnReady` fires, the caller starts a **single** goroutine that owns the read loop and calls `Feed`.
- All handlers fire synchronously inside `Feed`, in that same goroutine.
- Sends (writing to `conn`) may be done from any goroutine that serializes access to `conn`. Encoding a packet (calling `encode.EncodeXxx` and `mapSess.Encode`) must be done from the goroutine that owns the session.

**Pattern for multi-goroutine access:**

If game-state handlers need to enqueue work for other goroutines, use a channel:

```go
type Event struct {
    ActorExists *events.ActorExists
    ActorMoved  *events.ActorMoved
}

eventCh := make(chan Event, 1024)

ms.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
    e := decode.ActorExists_0x09FF(data, pv)
    // e is on the stack; copy it before sending to channel
    copy := e
    select {
    case eventCh <- Event{ActorExists: &copy}:
    default: // drop if consumer is slow
    }
})
```

Note that taking the address of a stack-allocated event struct causes it to escape to the heap. This adds an allocation per event. If allocation-free dispatch is required, process the event inline inside the handler.

---

## 13. Error handling

### FSM errors

`Connect` returns an error and calls `OnFailed(err)` on any unrecoverable auth failure. Use one or the other, not both.

Error types to expect:
- `"fsm: dial login ...: ..."` — network dial failure
- `"fsm: login refused (code=N)"` — server rejected credentials
- `"fsm: server notify ban (code=N)"` — ban notification (also calls `OnServerNotify`)
- `"fsm: unknown packet 0xNNNN"` — unexpected packet during auth (server version mismatch?)
- `"fsm: char server refused entry (code=N)"` — char server rejected entry
- `"fsm: map server refused entry (code=N)"` — map server rejected entry
- `context.DeadlineExceeded` — step timeout

### Session Feed errors

`Feed` returns `ErrUnknownPacket{ID: 0xNNNN}` when an unrecognised packet ID is encountered. The stream is irrecoverably desynced; close the connection immediately.

```go
var unk session.ErrUnknownPacket
if errors.As(err, &unk) {
    log.Printf("unknown packet 0x%04x — reconnecting", unk.ID)
    conn.Close()
    // schedule reconnect
}
```

After returning `ErrUnknownPacket` once, the session is faulted. Subsequent `Feed` calls return `nil` silently.

---

## 14. Known limitations

### Character list parsing

`OnCharList` receives raw `CHARACTER_INFO` bytes rather than parsed `[]events.CharacterInfo`. The `CHARACTER_INFO` struct layout varies significantly by PACKETVER and the generated decoder is a stub in the current implementation. The consumer must parse the raw bytes with its own codec.

### RE-client skill packet variants

For servers running the kRO RE client build in the date windows `20151104–20180704` or `20200902–20211118`, three skill packets arrive on different IDs with a different `SKILLDATA` layout (`0x0B31` / `0x0B32` / `0x0B33` instead of `0x0111` / `0x010F` / `0x07E1`, and the `name[24]` field is replaced by `level2`). The current library targets the main kRO client only; these RE-specific IDs are not decoded. See `docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`.

### Ragnarok Zero client

Three Zero-server-only packets (`ZC_QUEST_DIALOG` 0x0BA6, `ZC_QUEST_DIALOG_MENU_LIST` 0x0BA7, `ZC_MONOLOG_DIALOG` 0x0BA9) have empty SKIP stubs. The Zero server is a separate game mode unrelated to main or RE kRO.

### Homunculus and mercenary packets

Generated decode stubs exist for homunculus packets but have known field-type truncation bugs (`hp`/`maxHp` `uint32→uint16`, `exp`/`expNext` `int64→uint32`). Mercenary packets are absent entirely. Neither is planned for Phase 7.

### No `MapSession.Packetver()` accessor

The `packetver` passed to `NewMapSession` is not exposed as a public getter. Consumers should track the packetver separately (e.g., from `ServerConfig.Packetver`).
