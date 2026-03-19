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
8. [pkg/session — framer, dispatcher, FSM, and semantic API](#8-pkgsession--framer-dispatcher-fsm-and-semantic-api)
9. [End-to-end integration pattern](#9-end-to-end-integration-pattern)
10. [Performance contract](#10-performance-contract)
11. [Concurrency model](#11-concurrency-model)
12. [Error handling](#12-error-handling)
13. [Known limitations](#13-known-limitations)

---

## 1. What the library does and does not do

**Does:**
- Drives the three-phase rAthena auth sequence (login → char → map) via `ConnectionFSM`
- Frames incoming TCP streams into typed packets via `LoginSession`, `CharSession`, `MapSession`
- Dispatches received packets to typed, version-agnostic callbacks via `RegisterSemanticHandler`
- Encodes and sends typed request structs via `session.Send`
- Handles C→S packet ID shuffle and XOR obfuscation internally — callers never see raw packet IDs

**Does not:**
- Open network connections (`net.Dial`) — the caller provides a `Dialer`
- Own `net.Conn` after auth completes — after `OnReady` fires, `conn` belongs to the caller
- Run goroutines — all operations are synchronous in the caller's goroutine
- Allocate in the decode hot path — `Feed()` is 0 allocs/op in steady state
- Have external dependencies — `go.mod` has zero `require` entries
- Expose raw packet IDs or packetver-conditional logic in its public API

---

## 2. Package map

```
pkg/
    packing/    DecodePosDir, EncodePosDir, DecodeMoveData, EncodeMoveData
    events/     281 typed event structs (one per semantic action, S→C)
    send/       152 typed request structs (one per semantic action, C→S)
    decode/     282 generated decode functions: ActionName_0xNNNN(data, packetver)
    encode/     178 generated encode functions + shuffle table (internal)
    session/    LoginSession, CharSession, MapSession
                ConnectionFSM (auth sequencer)
                SemanticAction enum + RegisterSemanticHandler + Send
```

Generated packages (`events`, `send`, `decode`, `encode`, and the generated parts of `session`) are committed to the repository. They are regenerated only when rAthena source changes or `semantics/mappings.yaml` is updated.

There is no `pkg/fsm` — `ConnectionFSM` lives in `pkg/session`.

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
- `PosDir [3]byte` and `MoveData [6]byte` are raw packed bytes. Call `packing.DecodePosDir` / `packing.DecodeMoveData` to unpack.
- `string` fields (`Name`) are zero-copy aliases into the session receive buffer. They are valid only for the duration of the handler callback. To retain them, copy first: `name = string([]byte(e.Name))`.
- Fields absent in older PACKETVER variants are zero-valued.
- Event structs are passed **by value** to callbacks. They are stack-allocated inside decode functions and do not escape to the heap.

---

## 5. pkg/send — typed C→S request structs

One struct per outbound action. Populate and pass to `session.Send`.

```go
import "github.com/lenaxia/rathena-client/pkg/send"

// Move to map coordinates
type MoveTo struct {
    X uint16
    Y uint16
}

// Talk to an NPC
type NpcContact struct {
    NPCID uint32
    Type  uint8
}

// Use a skill
type SkillUse struct {
    SkillID  uint16
    Level    uint16
    TargetID uint32
}

// Send a chat message
type PublicChat struct {
    Name    string
    Message string
}
```

---

## 6. pkg/decode — decode functions (S→C)

Decode functions exist for advanced use or for the `pkg/session` internal dispatch layer. **Consumers using `RegisterSemanticHandler` never call decode functions directly** — the dispatch table calls them internally.

If you need to call a decode function directly (e.g. in a test or for a packet not yet in the semantic dispatch table):

```go
import "github.com/lenaxia/rathena-client/pkg/decode"

// Named ActionName_0xNNNN(data []byte, packetver uint32) events.ActionName
e := decode.ActorExists_0x09FF(data, pv)
// e is events.ActorExists, stack-allocated, 0 allocs
```

---

## 7. pkg/encode — encode functions (C→S)

Encode functions are called internally by `session.Send` via the registered encoder table. **Consumers using `session.Send` never call encode functions directly.**

The generated encode functions handle packetver-dependent shuffle internally. The `pkg/encode` package registers all encoders at `init()` time — you must import it with a blank import to trigger registration:

```go
import _ "github.com/lenaxia/rathena-client/pkg/encode"
```

This import is typically placed in the same file that imports `pkg/session`.

---

## 8. pkg/session — framer, dispatcher, FSM, and semantic API

This is the primary package goKore interacts with. It contains:
- `LoginSession`, `CharSession`, `MapSession` — PACKETVER-aware TCP framers
- `ConnectionFSM` — drives the full login → char → map auth sequence
- `SemanticAction` enum — 460 named protocol actions
- `RegisterSemanticHandler` — registers a typed callback for all packetver variants of an action
- `Send` — encodes and sends a typed request by semantic action

### Creating a session

```go
import "github.com/lenaxia/rathena-client/pkg/session"

// Sessions are created by NewMapSession/NewLoginSession/NewCharSession.
// The ConnectionFSM creates them internally during auth.
// Consumers typically receive a *MapSession from the OnReady callback.
mapSess := session.NewMapSession(20180307)
```

### Feeding data

```go
buf := make([]byte, 65536)
for {
    n, err := conn.Read(buf)
    if n > 0 {
        if feedErr := mapSess.Feed(buf[:n]); feedErr != nil {
            var unk session.ErrUnknownPacket
            if errors.As(feedErr, &unk) {
                // Variable-length packet with corrupt embedded length — stream desynced.
                conn.Close()
                return
            }
        }
    }
    if err != nil { break }
}
```

**Feed guarantees:**
- Synchronous — callbacks fire in the `Feed` call, in the caller's goroutine
- Frame accumulation: partial frames are buffered; `Feed` may be called with any chunk size
- Unknown packet ID: the receive buffer is cleared, the `UnknownPacketFunc` callback fires, and `Feed` returns `nil`. Not an error — the next `Feed` call starts clean
- `ErrUnknownPacket` is only returned for genuine stream corruption: a variable-length packet whose embedded length field is less than 4. After this the session is permanently faulted

### RegisterSemanticHandler — the primary receive API

`RegisterSemanticHandler` registers a typed callback for a semantic action. It automatically covers **all packetver variants** of that action. No packet IDs required.

```go
import (
    "github.com/lenaxia/rathena-client/pkg/events"
    "github.com/lenaxia/rathena-client/pkg/session"
)

// Register once — automatically handles 0x0078, 0x01D8, 0x02EC, 0x09FF
session.RegisterSemanticHandler(ms, session.ActionActorExists, func(e events.ActorExists) {
    x, y, dir := packing.DecodePosDir(e.PosDir[:])
    // update world model
})

// Register once — automatically handles 0x007B, 0x01DA, 0x022C, 0x09DB, 0x09FD
session.RegisterSemanticHandler(ms, session.ActionActorMoved, func(e events.ActorMoved) {
    fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])
    // update world model
})

session.RegisterSemanticHandler(ms, session.ActionStatUpdate, func(e events.StatUpdate) {
    // handles 0x00B0, 0x00B1, 0x00BE, 0x0B25
})
```

**Type safety**: `E` in `func(e E)` must be the concrete event struct value type (e.g. `events.ActorExists`), not a pointer. Passing a pointer-receiver func will compile but panic at the first dispatch.

**Buffer aliasing**: string fields in `e` (e.g. `e.Name`) are zero-copy aliases into the session receive buffer. They are valid only for the duration of the callback. Copy before returning if you need to retain them.

**A second `RegisterSemanticHandler` call for the same action** silently overwrites the first registration for all packet IDs it covers.

### Send — the primary send API

`Send` encodes a request by semantic action, applies packetver-dependent shuffle and XOR obfuscation internally, and writes the result to `w`.

```go
import (
    "github.com/lenaxia/rathena-client/pkg/send"
    "github.com/lenaxia/rathena-client/pkg/session"
    _ "github.com/lenaxia/rathena-client/pkg/encode"
)

// Move to coordinates — shuffle and obfuscation applied internally
err := session.Send(ms, conn, session.ActionMoveTo, send.MoveTo{X: 128, Y: 214})

// Use a skill
err = session.Send(ms, conn, session.ActionSkillUse, send.SkillUse{
    SkillID:  26,  // SM_BASH
    Level:    5,
    TargetID: npcActorID,
})

// Send a chat message
err = session.Send(ms, conn, session.ActionPublicChat, send.PublicChat{
    Name:    "PlayerName",
    Message: "hello",
})
```

`Send` returns `ErrWrongSendType{Action: action}` if `req` is not the correct `send.*` struct for the action — the error message includes the action name. It returns an error (not a panic) for an unknown or receive-only action.

### SemanticAction enum

```go
// SemanticAction is uint16. All 460 actions are named constants:
session.ActionActorExists
session.ActionActorMoved
session.ActionActorConnected
session.ActionStatUpdate
session.ActionMoveTo
session.ActionPublicChat
session.ActionSkillUse
// ... 454 more

// String method for logging
fmt.Println(session.ActionActorMoved.String()) // "ActionActorMoved"
```

### Handling unknown packets

```go
ms.SetUnknownPacketHandler(func(ev session.UnknownPacketEvent) {
    // ev.ID          — the unrecognised packet ID
    // ev.Packetver   — PACKETVER this session was built with
    // ev.Time        — wall time at moment of detection
    // ev.RecentPackets — last ≤3 dispatched packets before the unknown ID
    // ev.RawBuffer   — snapshot of the receive buffer from the unknown ID onward

    botManager.HandleUnknownPacket(botID, ev)
})
```

`ev` is fully self-contained and heap-allocated — safe to retain or pass to channels.

`UnknownPacketEvent` is also delivered to `SetTraceFunc` (see below) — both fire independently for the same event.

### Debuggability — `SetTraceFunc`, `IsFaulted`, `UnhandledPackets`

#### SetTraceFunc — unified wire and semantic trace hook

A single callback that receives every observable event on the session. Zero overhead when nil.

```go
ms.SetTraceFunc(func(ev session.TraceEvent) {
    switch e := ev.(type) {

    case session.WireInbound:
        // Every complete inbound frame, after framing, before dispatch.
        // e.ID        — packet ID
        // e.Frame     — heap-allocated full frame bytes (safe to retain)
        // e.Packetver — PACKETVER this session was built with
        // e.Time      — wall time at receipt
        log.Printf("← 0x%04X (%d bytes)", e.ID, len(e.Frame))

    case session.WireOutbound:
        // Every outbound frame written by session.Send, after obfuscation.
        // e.Action    — semantic action that produced this frame
        // e.Frame     — heap-allocated full frame bytes as written to wire
        // e.Packetver — PACKETVER
        // e.Time      — wall time at send
        log.Printf("→ %v (%d bytes)", e.Action, len(e.Frame))

    case session.SemanticIn:
        // Decoded event struct, correlated with the preceding WireInbound.
        // e.Action — semantic action
        // e.ID     — wire packet ID that was decoded
        // e.Event  — concrete events.* struct value (type-switch on it)
        // e.Frame  — independent heap-allocated copy of the same frame
        if actor, ok := e.Event.(events.ActorMoved); ok {
            log.Printf("← %v AID=%d", e.Action, actor.AID)
        }

    case session.SemanticOut:
        // send.* request struct, correlated with the preceding WireOutbound.
        // e.Action  — semantic action
        // e.Request — original send.* struct passed to Send
        // e.Frame   — same heap-allocated bytes as paired WireOutbound
        if req, ok := e.Request.(send.MoveTo); ok {
            log.Printf("→ %v X=%d Y=%d", e.Action, req.X, req.Y)
        }

    case session.UnknownPacketEvent:
        // Unknown packet ID — raw buffer included.
        // Also fires via SetUnknownPacketHandler (both are independent).
        log.Printf("? unknown 0x%04X raw=%d bytes", e.ID, len(e.RawBuffer))
    }
})
```

`SetTraceFunc` is available on `MapSession`, `LoginSession`, and `CharSession`. On `LoginSession` and `CharSession` only `WireInbound` and `UnknownPacketEvent` fire (there is no `Send` on those session types).

**Performance**: when `TraceFunc` is nil, there are zero allocations and a single nil-check branch on the hot path. `BenchmarkFeed_SmallFixedPacket` remains 0 allocs/op with nil trace.

#### IsFaulted — stream corruption detection

```go
if ms.IsFaulted() {
    // Feed() returned ErrUnknownPacket (corrupt embedded length field).
    // All subsequent Feed() calls are silent no-ops.
    // Close the connection and create a new session.
    conn.Close()
}
```

#### UnhandledPackets — handler registration gap detection

```go
// Count of frames that arrived with a known packet ID (in the length table)
// but no registered handler. Non-zero means RegisterSemanticHandler was
// not called for some action the server is sending.
gaps := ms.UnhandledPackets()
if gaps > 0 {
    log.Printf("WARNING: %d packets silently dropped (no handler registered)", gaps)
}
```

### ConnectionFSM — auth sequencer

`ConnectionFSM` drives the complete login → char → map authentication sequence synchronously.

```go
import (
    "context"
    "net"
    "github.com/lenaxia/rathena-client/pkg/session"
    _ "github.com/lenaxia/rathena-client/pkg/encode"
)

f := session.New(
    session.ServerConfig{
        LoginAddr:   "127.0.0.1:6900",
        Packetver:   20180307,
        StepTimeout: 30 * time.Second,
    },
    session.Credentials{
        Username: "admin",
        Password: "admin",
        CharSlot: 0,
    },
    func(ctx context.Context, addr string) (net.Conn, error) {
        return net.Dial("tcp", addr)
    },
)
```

**Callbacks** (all optional):

```go
// Choose which char server to connect to. Default: index 0.
f.OnCharServerList(func(servers []session.CharServerInfo) int { return 0 })

// Choose which character slot to use. Default: Credentials.CharSlot.
f.OnCharList(func(rawChars []byte) uint8 { return 0 })

// Called when map entry completes. The FSM releases conn after this call.
f.OnReady(func(ms *session.MapSession, conn net.Conn, info session.ReadyInfo) {
    // info.X, info.Y  — initial tile coordinates
    // info.Dir        — facing direction (0–7)
    // info.StartTime  — server tick from entry packet

    // Register semantic handlers
    session.RegisterSemanticHandler(ms, session.ActionActorExists, func(e events.ActorExists) {
        // ...
    })

    // Start read loop outside the library
    go readLoop(ms, conn)
})

// Called on any unrecoverable auth error.
f.OnFailed(func(err error) { log.Printf("auth failed: %v", err) })

// Called when SC_NOTIFY_BAN (0x0081) arrives during auth.
f.OnServerNotify(func(code uint8) { log.Printf("ban code: %d", code) })

// Called after char selection, before map phase. Use to initialize self-actor state.
f.OnIdentity(func(id session.IdentityInfo) {
    log.Printf("aid=%d cid=%d slot=%d", id.AccountID, id.CharID, id.SelectedSlot)
})
```

**Running the FSM:**

```go
// Blocks until OnReady or OnFailed fires.
// May be called again on the same FSM for reconnection.
if err := f.Connect(context.Background()); err != nil {
    log.Fatal(err)
}
```

**Auth sequence:**

```
Connect(ctx)
  → dial LoginAddr
  → send CA_LOGIN
  → read until AC_ACCEPT_LOGIN / AC_REFUSE_LOGIN / SC_NOTIFY_BAN
  → call OnCharServerList → choose server
  → dial chosen CharServer
  → send CH_ENTER
  → read until char list assembled (HC_ACCEPT_ENTER + HC_NOTIFY_ZONESVR or paged variants)
  → call OnCharList → choose slot
  → send CH_SELECT_CHAR
  → call OnIdentity(accountID, charID, slot, sex)
  → dial MapServer
  → send CZ_ENTER (with obfuscation if keys exist for this packetver)
  → read until ZC_ACCEPT_ENTER
  → send CZ_NOTIFY_ACTORINIT + CZ_REQUEST_TIME
  → call OnReady(mapSess, conn, ReadyInfo{X, Y, Dir, StartTime, Font, Sex})
  → return nil
```

---

## 9. End-to-end integration pattern

```go
package network

import (
    "context"
    "errors"
    "net"

    "github.com/lenaxia/rathena-client/pkg/events"
    "github.com/lenaxia/rathena-client/pkg/packing"
    "github.com/lenaxia/rathena-client/pkg/send"
    "github.com/lenaxia/rathena-client/pkg/session"
    _ "github.com/lenaxia/rathena-client/pkg/encode"
)

type Connector struct {
    fsm       *session.ConnectionFSM
    ms        *session.MapSession
    conn      net.Conn
    packetver uint32
}

func NewConnector(loginAddr string, pv uint32, user, pass string) *Connector {
    c := &Connector{packetver: pv}

    c.fsm = session.New(
        session.ServerConfig{LoginAddr: loginAddr, Packetver: pv},
        session.Credentials{Username: user, Password: pass},
        func(ctx context.Context, addr string) (net.Conn, error) {
            return net.Dial("tcp", addr)
        },
    )

    c.fsm.OnReady(func(ms *session.MapSession, conn net.Conn, info session.ReadyInfo) {
        c.ms = ms
        c.conn = conn

        // Register handlers by semantic action — no packet IDs needed
        session.RegisterSemanticHandler(ms, session.ActionActorExists, func(e events.ActorExists) {
            c.onActorExists(e)
        })
        session.RegisterSemanticHandler(ms, session.ActionActorMoved, func(e events.ActorMoved) {
            c.onActorMoved(e)
        })
        session.RegisterSemanticHandler(ms, session.ActionStatUpdate, func(e events.StatUpdate) {
            c.onStatUpdate(e)
        })

        // Set unknown-packet diagnostic handler
        ms.SetUnknownPacketHandler(func(ev session.UnknownPacketEvent) {
            // log or report ev.ID, ev.RawBuffer, ev.RecentPackets
        })

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

// MoveTo sends a walk request. No packet ID, no shuffle, no obfuscation needed.
func (c *Connector) MoveTo(x, y uint16) error {
    return session.Send(c.ms, c.conn, session.ActionMoveTo, send.MoveTo{X: x, Y: y})
}

// UseSkill sends a skill use request.
func (c *Connector) UseSkill(skillID, level uint16, targetID uint32) error {
    return session.Send(c.ms, c.conn, session.ActionSkillUse, send.SkillUse{
        SkillID:  skillID,
        Level:    level,
        TargetID: targetID,
    })
}

func (c *Connector) onActorExists(e events.ActorExists) {
    x, y, dir := packing.DecodePosDir(e.PosDir[:])
    _ = x; _ = y; _ = dir
}

func (c *Connector) onActorMoved(e events.ActorMoved) {
    fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])
    _ = fromX; _ = fromY; _ = toX; _ = toY
}

func (c *Connector) onStatUpdate(e events.StatUpdate) {
    // e.Type, e.Value — which stat changed and to what value
}
```

### Same-server warp (ZC_NPCACK_MAPMOVE)

When a warp portal keeps the player on the **same** map server:

```go
session.RegisterSemanticHandler(ms, session.ActionMapChanged, func(e events.MapChanged) {
    // e.MapName, e.XPos, e.YPos — destination

    // Ack the warp — same as initial map entry
    session.Send(ms, conn, session.ActionMapLoaded, send.MapLoaded{})
    session.Send(ms, conn, session.ActionTimeSyncResponse, send.TimeSyncResponse{ClientTime: 0})

    // Update position state
    currentMap = e.MapName
    posX, posY = e.XPos, e.YPos
})
```

---

## 10. Performance contract

These are hard constraints verified by benchmarks, not aspirational targets.

| Path | Target |
|---|---|
| `session.Feed()` steady state | 0 allocs/op, < 200 ns/op (fixed packet) |
| `session.Feed()` with `SetTraceFunc` enabled | 2 allocs/op (frame copy per packet) — expected overhead |
| `RegisterSemanticHandler` dispatch | 1 alloc/op (interface{} boxing — unavoidable by design), < 200 ns/op |
| `session.Send` (nil trace) | 0 allocs/op for fixed-size packets |
| `session.Send` with `SetTraceFunc` enabled | 2-4 allocs/op (frame copy + trace events) — expected overhead |

**Why 0 allocs matters at scale**: at 1000 concurrent bots, any allocation in the decode path multiplies by 1000 per packet received.

```bash
go test -bench=. -benchmem ./pkg/...
```

---

## 11. Concurrency model

The library is **not goroutine-safe**. Each session must be accessed from a single goroutine.

The intended model:
- The FSM runs in the caller's goroutine during auth (`Connect` blocks).
- After `OnReady` fires, the caller starts a **single** goroutine that owns the read loop and calls `Feed`.
- All `RegisterSemanticHandler` callbacks fire synchronously inside `Feed`, in that same goroutine.
- `session.Send` and calls to `conn.Write` may be done from any goroutine that serializes access to both the session and the connection.

**Pattern for multi-goroutine access:**

```go
eventCh := make(chan events.ActorExists, 1024)

session.RegisterSemanticHandler(ms, session.ActionActorExists, func(e events.ActorExists) {
    // e is on the stack. Copy it before sending to channel (taking its address
    // causes heap escape and adds 1 alloc per event).
    copy := e
    select {
    case eventCh <- copy:
    default: // drop if consumer is slow
    }
})
```

---

## 12. Error handling

### FSM errors

`Connect` returns an error and calls `OnFailed(err)` on any unrecoverable auth failure.

Error strings to expect:
- `"fsm: dial login ...: ..."` — network dial failure
- `"fsm: login refused (code=N)"` — server rejected credentials
- `"fsm: server notify ban (code=N)"` — ban notification
- `"fsm: char server refused entry (code=N)"` — char server rejection
- `"fsm: map server refused entry (code=N)"` — map server rejection
- `context.DeadlineExceeded` — step timeout

### session.Send errors

- `session.ErrWrongSendType{Action: action}` — `req` is not the correct `send.*` type; error message includes the action name
- Generic error — action is unknown or has no registered encoder (receive-only action)

```go
if err := session.Send(ms, conn, session.ActionMoveTo, send.MoveTo{X: 100, Y: 200}); err != nil {
    if errors.Is(err, session.ErrWrongSendType{}) {
        // programming error — wrong struct passed for the action
        // err.Error() = "session: Send called with wrong request type for action ActionMoveTo"
    }
}
```

### Feed errors

`Feed` returns `nil` for unknown packet IDs. The buffer is cleared and `UnknownPacketFunc` fires.

`Feed` returns `ErrUnknownPacket{ID: 0xNNNN}` only when a variable-length packet carries an embedded length value less than 4. After this the session is permanently faulted.

```go
if err := ms.Feed(buf[:n]); err != nil {
    var unk session.ErrUnknownPacket
    if errors.As(err, &unk) {
        log.Printf("corrupt embedded length in packet 0x%04x — reconnecting", unk.ID)
        conn.Close()
    }
}
```

---

## 13. Known limitations

### Character list parsing

`OnCharList` receives raw `CHARACTER_INFO` bytes rather than a parsed slice. The `CHARACTER_INFO` struct layout varies significantly by PACKETVER and the generated decoder is a stub. The consumer must parse the raw bytes with its own codec.

### RE-client skill packet variants

For servers running the kRO RE client build in `20151104–20180704` or `20200902–20211118`, three skill packets arrive on different IDs with a different `SKILLDATA` layout. The current library targets the main kRO client only. See `docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`.

### Ragnarok Zero client

Three Zero-server-only packets (`ZC_QUEST_DIALOG` 0x0BA6, `ZC_QUEST_DIALOG_MENU_LIST` 0x0BA7, `ZC_MONOLOG_DIALOG` 0x0BA9) have empty SKIP stubs.

### Homunculus and mercenary packets

Generated decode stubs exist for homunculus packets but have known field-type truncation bugs. Mercenary packets are absent entirely. Neither is planned.

### No `MapSession.Packetver()` accessor

The `packetver` passed to `NewMapSession` is not exposed as a public getter. Track it separately from `ServerConfig.Packetver`.
