# rathena-client — High-Level Design

**Status**: Draft v10  
**Date**: 2026-03-06  
**Author**: goKore project  
**Supersedes**: HLD v9 (2026-03-06)

---

## 1. Purpose

`rathena-client` is a standalone Go module that implements the Ragnarok Online wire
protocol as spoken by rAthena login, char, and map servers. It is designed to be
imported by bot/client applications (primarily goKore) as a library.

It is **not** a game client, bot, or application. It is a protocol library: it
receives raw TCP bytes and invokes typed, version-agnostic callbacks; it accepts
typed send requests and returns raw TCP bytes.

### Design Goals

- **Correct**: every decode matches the rAthena source, not OpenKore guesses
- **Typesafe**: no `interface{}`, no reflection in the hot path, no `context.Context`
  threading through the library
- **Zero goroutines**: the library never spawns a goroutine internally; this is a hard
  invariant, not a soft goal
- **Single binary supports all packetvers**: `PACKETVER` is a `uint32` set once at
  session creation; no separate builds or snapshot directories
- **Zero heap allocations in the decode hot path**: event structs are stack-allocated
  and passed by value to callbacks; the library does not escape them to the heap
- **Maintainable**: struct layouts are derived from rAthena source via GCC
  preprocessing — human-written YAML only provides semantic names and groupings
- **Separate from application logic**: this module has zero knowledge of bots,
  scripts, or game strategy

### Explicit Non-Goals

This library explicitly does **not**:

- Own or manage TCP connections (`net.Conn`, `net.Dial`, reconnection, timeouts) —
  goKore owns all sockets; the library accepts a `Dialer` func
- Know whether a bot is authenticated or in what phase of the login flow it is
  (when not using `pkg/fsm`)
- Use `context.Context` (context is an application concern; it does not belong in a
  decode function)
- Log anything (callers log inside their callbacks if needed)
- Spawn goroutines (zero `go` keywords in any `pkg/` file — verified by CI)
- Provide goroutine safety within a single session (one session, one goroutine —
  by design; see §8)
- Implement game logic of any kind (combat, movement decisions, inventory management)
- Implement an OpenKore compatibility layer (goKore handles that mapping)
- Encrypt or decrypt payload bytes (rAthena only XORs the 2-byte packet ID on
  outgoing C→S packets, and only on the map server, and only for PACKETVER ≤ 20180307)
- Record, replay, or proxy packets

---

## 2. Problem with the Prior Approach (goKore v1 network layer)

The existing `internal/network/` layer in goKore generates 10 packetver-snapshot
directories (v20030000 through v20200902), each containing ~420 Go struct files.
This yields:

- ~4,634 generated struct files across 10 snapshot directories
- ~429 generated adapter files (typed struct → canonical params)
- ~429 generated params files
- One 4,980-line `action_selector.go`
- A `PacketVersionRegistry` struct with 8+ maps protected by a single `sync.RWMutex`,
  instantiated once per bot
- 2 goroutines per connection (read loop + process loop) with a `chan []byte`
  buffered at 100 between them

Additionally, the generated code contains confirmed correctness bugs:

| Bug | Location | Description |
|-----|----------|-------------|
| `PosDir string` | `v20030000/packet_zc_notify_standentry.go` | 3-byte binary field stored as Go `string`; `[]byte(s)` conversion is lossy for bytes > 0x7F |
| Wrong direction from MoveData | `handlers/actors/handler.go:88` | `direction = (data[5] & 0xF0) >> 4` — that is `sx0`, not direction; 6-byte format has no direction |
| Hardcoded direction sentinel | `send/builders/map_builder.go:90` | `bits |= uint32(0x44) << 24` hardcodes `dir=4` instead of taking direction as parameter |
| uint32 overflow | `handlers/movement/handler.go:523` | `data[4] = byte(coords >> 32)` — always zero; `coords` is a `uint32` |

The root causes:
1. **PACKETVER conditioning solved at code-generation time** instead of at runtime
2. **Struct population via reflection** (`FromPacket` with `map[string]interface{}`)
   instead of direct byte reads
3. **Two goroutines per connection** when one suffices — the second goroutine adds a
   channel roundtrip with no benefit at the call sites

---

## 3. Architecture Overview

The library exposes two independent APIs at different levels of abstraction. goKore
uses both:

```
┌──────────────────────────────────────────────────────────────────┐
│                        rathena-client                            │
│                                                                  │
│  pkg/fsm/          ConnectionFSM — login + reconnect sequencer   │
│  pkg/packing/      WBUFPOS / WBUFPOS2 encode+decode              │
│  pkg/events/       Canonical event structs (S→C)    GENERATED    │
│  pkg/send/         Canonical send request types (C→S) GENERATED  │
│  pkg/decode/       Raw bytes → events               GENERATED    │
│  pkg/encode/       Send requests → raw bytes        GENERATED    │
│  pkg/session/      PACKETVER-aware tokenizer + dispatcher        │
│                    (LoginSession, CharSession, MapSession)        │
│                                                                  │
│  internal/codegen/ Code generator (reads rAthena + YAML)         │
└──────────────────────────────────────────────────────────────────┘
         ↑ imported by
┌──────────────────────────────────────────────────────────────┐
│                          goKore                              │
│  internal/network/rathena/adapter.go   thin glue layer       │
│  internal/network/connection/          owns net.Conn, Dialer │
│  internal/network/handlers/            game-state handlers   │
└──────────────────────────────────────────────────────────────┘
```

**Tier 1 — `pkg/fsm` (ConnectionFSM)**: Used during initial login and reconnection
only. goKore provides a `Dialer` func; the FSM drives the full
login → char → map sequence, surfacing exactly one callback that requires
application input (char selection), then returns a ready `*MapSession` via
`OnReady`. After that it is idle until goKore calls `Connect` again.

**Tier 2 — `pkg/session` (MapSession)**: Used for all steady-state gameplay.
goKore owns the `net.Conn`, calls `session.Feed(buf)` in its read loop,
and calls `session.Encode(...)` to build outbound packets.

### Data flow (initial login / reconnect)

```
goKore calls fsm.Connect(ctx)
  → FSM calls dialer(ctx, loginAddr) → net.Conn    [goKore-provided dialer]
  → FSM creates LoginSession, runs Feed loop internally
  → recv 0x0069: extracts tokens, closes conn
  → FSM calls dialer(ctx, charAddr) → net.Conn
  → FSM creates CharSession, sends 0x0065, runs Feed loop internally
  → recv 0x006B / 0x099D: builds char list
  → FSM calls OnCharList callback → goKore returns chosen slot (uint8)
  → FSM sends 0x0066, continues Feed loop
  → recv 0x0081 (PACKETVER < 20170315) / 0x0AC5 (≥ 20170315): extracts map addr, closes conn
  → FSM calls dialer(ctx, mapAddr) → net.Conn
  → FSM creates MapSession, sends 0x0436, runs Feed loop internally
  → recv 0x0073 / 0x0A18 / 0x02EB: sends 0x007D + 0x007E/0x0360, transitions to Ready
  → FSM calls OnReady(mapSession, mapConn)        [goKore takes over the conn]
  → goKore's read loop takes over: conn.Read → mapSession.Feed(buf)
```

### Data flow (steady-state gameplay, goKore owns the loop)

```
TCP bytes arrive on net.Conn  (goKore read loop)
  → mapSession.Feed(buf[:n])           [rathena-client]
  → frame boundary detection via lengths[65536]int16
    (S→C packets are NOT obfuscated; packet ID read directly)
  → handlers[packetID](data, packetver)   [GENERATED decode fn]
  → decode fn: direct byte reads, stack-allocated event struct
  → registered callback(events.ActorExists{...})   [goKore, inline]
  → Feed() returns to goKore read loop
```

### Data flow (send path, steady-state)

```
goKore calls mapSession.Encode(send.RequestMove{X: 100, Y: 200})
  → encode.EncodeMove(req, packetver)   [GENERATED, returns [5]byte]
  → if obfuscation enabled: XOR packet ID with rolling key (C→S only)
  → goKore calls conn.Write(bytes[:])   (goKore owns the socket)
```

**Key invariant**: `Feed()` is synchronous. Every callback fires and returns before
`Feed()` processes the next frame. Callbacks must return promptly; heavy work is
goKore's responsibility to dispatch to its own goroutine.

---

## 4. ConnectionFSM

`ConnectionFSM` is an optional high-level helper in `pkg/fsm` that drives the
full login → char → map authentication sequence and hands goKore a ready
`*MapSession` and live `net.Conn`. goKore uses it for initial login and every
subsequent reconnect. During steady-state gameplay it is completely idle.

### Design constraints

- **goKore owns connections**: the FSM never calls `net.Dial` directly. It
  receives a `Dialer` function from goKore and calls it to get each `net.Conn`.
  goKore can wrap any dialer it likes (direct, proxy, timeout-controlled, test
  stub).
- **Zero goroutines inside the FSM**: the FSM runs in the goroutine that called
  `Connect`. All blocking I/O is synchronous.
- **One application-choice point**: char selection is the only step that requires
  bot logic. Everything else is pure protocol sequencing and is handled
  automatically.
- **After `OnReady` fires, the FSM releases the conn**: goKore takes ownership of
  the `net.Conn` returned alongside the `*MapSession` and runs its own read loop.
  The FSM no longer touches the conn.

### Public API

```go
// pkg/fsm/fsm.go

// Dialer is provided by goKore. The FSM calls it for each of the three server
// connections. goKore can use net.DialTimeout, a proxy dialer, or a test stub.
type Dialer func(ctx context.Context, addr string) (net.Conn, error)

// ServerConfig holds the fixed properties of the game server.
// Shared across all bot instances connecting to the same server.
// Mirrors the server-config layer of OpenKore's servers.txt entry.
type ServerConfig struct {
    LoginAddr   string        // "host:port" of the rAthena login server
    Packetver   uint32        // YYYYMMDD; selects packet layouts and IDs
    StepTimeout time.Duration // per-step deadline (default: 30s); enforced via
                               // conn.SetDeadline(time.Now().Add(StepTimeout)) before
                               // each blocking read inside Connect(). Returns ErrTimeout
                               // if the server accepts TCP but never responds within the
                               // deadline. The FSM calls SetDeadline before every read,
                               // not once globally, so long-running authentication steps
                               // each get a fresh deadline.
}

// Credentials holds the per-account authentication details.
// One per bot instance. Mirrors OpenKore's config.txt credentials.
type Credentials struct {
    Username string
    Password string
    CharSlot uint8 // default slot; ignored if OnCharList is registered
}

type ConnectionFSM struct { /* unexported */ }

// New creates a ConnectionFSM. server describes the rAthena server;
// creds are the per-account login details. Both are stored for use on
// every Connect call (including reconnects). Changing either requires
// creating a new FSM.
func New(server ServerConfig, creds Credentials, dialer Dialer) *ConnectionFSM

// Callbacks — all optional except OnReady (otherwise Connect is pointless).
// All callbacks are called synchronously in the Connect goroutine.

// OnCharServerList is called after 0x0069/0x0AC4 is received and the char
// server list has been parsed. The callback receives all advertised char
// servers and returns the index to connect to. If not registered: index 0
// is used (appropriate for private rAthena servers which always advertise
// exactly one char server).
func (f *ConnectionFSM) OnCharServerList(fn func([]events.CharServerInfo) int) *ConnectionFSM

// OnCharList is called once the char list is received. The callback receives
// raw CHARACTER_INFO bytes and returns the slot number to select. If not
// registered, the slot from Credentials.CharSlot is used.
//
// Phase 1 implementation: raw []byte because the CHARACTER_INFO decoder
// (struct varies per PACKETVER) is a generated SKIP stub not yet available.
// goKore receives the raw bytes and may parse them with its own codec.
//
// Planned Phase 2 upgrade: func([]events.CharacterInfo) uint8 once the
// CHARACTER_INFO decoder is generated. The current signature is a known
// deviation from the long-term design (HLD §4 originally specified CharacterInfo).
func (f *ConnectionFSM) OnCharList(fn func([]byte) uint8) *ConnectionFSM

// OnReady is called when the map server accepts entry (0x0073/0x0A18/0x02EB) and the
// map-loaded sequence has been sent. The FSM passes the ready MapSession and the
// live net.Conn back to goKore. After this call returns the FSM is idle.
func (f *ConnectionFSM) OnReady(fn func(*session.MapSession, net.Conn)) *ConnectionFSM

// OnFailed is called on any unrecoverable error (login refused, map refused,
// dial failure, auth timeout). goKore decides whether to reconnect.
func (f *ConnectionFSM) OnFailed(fn func(err error)) *ConnectionFSM

// OnServerNotify is called when 0x0081 SC_NOTIFY_BAN is received during auth.
func (f *ConnectionFSM) OnServerNotify(fn func(events.ServerNotify)) *ConnectionFSM

// Connect runs the full login sequence synchronously. It blocks until OnReady
// or OnFailed fires, then returns. goKore typically calls this in a goroutine.
//
// Connect may be called multiple times on the same FSM. If a previous Connect
// left any live state (session object, partial credentials), Connect tears it
// down before starting fresh. This means reconnection is simply calling Connect
// again — there is no separate Reconnect method.
//
// Error handling: If Connect encounters a failure, it calls OnFailed(err) (if
// registered) and then returns that same error. goKore should use ONE of these
// — either the OnFailed callback or the return value — not both, to avoid
// double-handling the same failure. The recommended pattern is to use OnFailed
// for application-level error reporting and ignore the return value, or check
// only the return value and not register OnFailed.
func (f *ConnectionFSM) Connect(ctx context.Context) error
```

### State machine

```
Disconnected
    │  Connect() called
    ▼
DialingLogin ── dial error ──────────────────────────────────► Failed
    │  dialer(ctx, loginAddr) succeeds
    ▼
LoginAuth    ── recv 0x006A/0x083E/0x0081 ───────────────────► Failed
    │  recv 0x0069/0x0AC4 → extract tokens, close conn
    │  call OnCharServerList(servers) → get index (default: 0)
    ▼
DialingChar  ── dial error ──────────────────────────────────► Failed
    │  dialer(ctx, charAddr) succeeds → send 0x0065
    ▼
CharAuth     ── recv 0x006C/0x0081 ─────────────────────────► Failed
    │  PACKETVER >= 20130000 path:
    │    recv 0x082D → store slot info (auxiliary, stay)
    │    recv 0x006B → accumulate chars (stay)
    │    recv 0x09A0 → send CH_CHARLIST_REQ (0x09A1) × p.total; wait for 0x099D pages
    │    recv 0x099D → accumulate pages until sync complete
    │  PACKETVER < 20130000 path:
    │    recv 0x006B → char list arrives directly, no 0x09A0
    ▼
SelectingChar ── recv other ──────────────────────────────── (stay)
    │  call OnCharList(chars) → get slot
    │  send 0x0066(slot)
    │  recv 0x0081/0x0AC5 → extract mapAddr, close conn
    │    (0x0081 = HC_NOTIFY_ZONESVR for PACKETVER < 20170315;
    │     0x0AC5 = HC_NOTIFY_ZONESVR for PACKETVER >= 20170315.
    │     NOTE: 0x0081 is also SC_NOTIFY_BAN — see §20 for disambiguation.)
    ▼
DialingMap   ── dial error ──────────────────────────────────► Failed
    │  dialer(ctx, mapAddr) succeeds → send 0x0436
    ▼
MapAuth      ── recv 0x0074/0x0081 ─────────────────────────► Failed
    │  recv 0x0283 ZC_AID → store account ID echo (stay)
    │  (note: 0x0283 is sent by rAthena inside clif_parse_WantToConnection,
    │   immediately on receiving 0x0436, before the char-server auth round-trip.
    │   For PACKETVER < 20070521 a raw 4-byte AID is sent instead — no packet header.
    │   This raw 4-byte form CANNOT be detected by standard framing (lengths[0x??] lookup)
    │   because no packet ID precedes it. Pre-20070521 is OUT OF SCOPE for Phase 1.)
    │  recv 0x0073/0x0A18/0x02EB
    │    → send 0x007D (map loaded confirmation)
    │    → send 0x007E/0x0360 (tick sync; CZ_REQUEST_TIME)
    │    → call OnReady(mapSession, conn)
    ▼
Ready (idle — FSM hands off conn to goKore's read loop)
```

### Sequence of automatic protocol steps

Every step below happens with zero application involvement:

| Trigger | Automatic response |
|---------|-------------------|
| TCP connected to login server | Send `0x0064` login request |
| Recv `0x0AC4`/`0x0069` | Parse char server list; call `OnCharServerList` (default: index 0); close login conn, dial chosen char server, send `0x0065` |
| Recv `0x082D` (PACKETVER ≥ 20130000) | Store slot info; stay in CharAuth |
| Recv `0x006B` (PACKETVER ≥ 20130000) | Accumulate chars; stay — wait for `0x09A0` |
| Recv `0x006B` (PACKETVER < 20130000) | Char list is complete; call `OnCharList` immediately |
| Recv `0x09A0` (PACKETVER ≥ 20130000) | Send `CH_CHARLIST_REQ` (0x09A1) × `p.total`; wait for `0x099D` pages |
| Recv `0x099D` page(s) | Accumulate; when all pages received → call `OnCharList` |
| After `OnCharList` returns slot | Send `0x0066` with chosen slot |
| Recv `0x0081` (PACKETVER < 20170315) or `0x0AC5` (≥ 20170315) | Close char conn, dial map server, send `0x0436`. Source: `common/packets.hpp:290–308` (HEADER_HC_NOTIFY_ZONESVR). NOTE: `0x0081` == SC_NOTIFY_BAN on same connection — FSM distinguishes by payload size: SC_NOTIFY_BAN is 4 bytes; HC_NOTIFY_ZONESVR is ≥ 28 bytes. |
| Recv `0x0283` (PACKETVER ≥ 20070521) | Store account ID echo; stay in MapAuth |
| Recv `0x0073`/`0x0A18`/`0x02EB` | Send `0x007D` (map loaded) + `0x007E`/`0x0360` (tick sync) → call `OnReady` |

### goKore usage pattern

```go
// internal/network/rathena/connector.go

server := fsm.ServerConfig{
    LoginAddr:   cfg.LoginAddr,   // e.g. "127.0.0.1:6900"
    Packetver:   cfg.Packetver,
    StepTimeout: 30 * time.Second,
}

creds := fsm.Credentials{
    Username: cfg.Username,
    Password: cfg.Password,
    CharSlot: cfg.DefaultCharSlot,
}

dialer := func(ctx context.Context, addr string) (net.Conn, error) {
    return net.DialTimeout("tcp", addr, 10*time.Second)
}

mapSessionDone := make(chan struct{})

loginFSM := fsm.New(server, creds, dialer).
    OnCharServerList(func(servers []events.CharServerInfo) int {
        // For a private rAthena server there is always exactly one entry.
        // For public servers, pick by name or index from config.
        return 0
    }).
    OnCharList(func(chars []events.CharacterInfo) uint8 {
        // goKore bot logic: pick the first char, or look up by name, etc.
        return cfg.CharSlot
    }).
    OnReady(func(s *session.MapSession, c net.Conn) {
        // Register all gameplay callbacks on the session
        s.OnActorExists(bot.HandleActorExists)
        s.OnActorMoved(bot.HandleActorMoved)
        // ...
        // Hand the conn+session to goKore's read loop
        go bot.RunMapLoop(ctx, s, c, mapSessionDone)
    }).
    OnFailed(func(err error) {
        log.Errorf("login failed: %v", err)
        // goKore decides whether to retry, after what delay, etc.
    })

go func() {
    for {
        if err := loginFSM.Connect(ctx); err != nil {
            // back-off and retry
            time.Sleep(cfg.RetryDelay)
            continue
        }
        // Connect blocks until OnReady fires. Then block here until the map
        // session ends (bot.RunMapLoop closes mapSessionDone when conn closes).
        <-mapSessionDone
        // Reconnect is just calling Connect again; the FSM cleans up prior state.
        loginFSM.Connect(ctx)
    }
}()
```

### What the FSM does NOT do

- Does not call `net.Dial` (uses the provided `Dialer`)
- Does not schedule keepalives (goKore's read loop sends `0x0200`/`0x0187`/`0x0B1C`
  on timers, same as before)
- Does not reconnect automatically on drop — it fires `OnFailed` or returns, and
  goKore calls `Connect` again when it chooses
- Does not hold a reference to the `net.Conn` after `OnReady` fires
- Does not know about bot logic, scripts, or game state

### Relationship to LoginSession / CharSession / MapSession

The FSM uses `LoginSession` and `CharSession` internally during the auth flow, then
returns a `*MapSession` through `OnReady`. After `OnReady`, the `LoginSession` and
`CharSession` are discarded — they were ephemeral. goKore only ever sees
`*MapSession` directly; the lower-level sessions are an implementation detail of
the FSM.

goKore may also bypass the FSM entirely and construct sessions directly if needed
(e.g. for testing, or for a non-standard auth flow). The FSM is a convenience
wrapper, not a gatekeeper.

---

## 5. Three Typed Sessions

The protocol involves three distinct TCP connections to three different rAthena
servers. Each is represented by a separate session type. The `ConnectionFSM` (§4)
uses `LoginSession` and `CharSession` internally and returns a `*MapSession` to
goKore; goKore may also construct sessions directly for testing or non-standard
flows.

```
LoginSession  ←→  rAthena login server   (src/common/packets.hpp, CA_/AC_ packets)
CharSession   ←→  rAthena char server    (src/common/packets.hpp, CH_/HC_ packets)
MapSession    ←→  rAthena map server     (packets.hpp + packets_struct.hpp, CZ_/ZC_)
```

### Why three types, not one reconfigurable session

- **Memory per bot**: `LoginSession` and `CharSession` each handle ~10–15 packets.
  Loading the full map packet table (400+ entries) for all three connections wastes
  memory. At 1000 bots, unnecessary overhead multiplies directly.
- **Type safety**: goKore cannot call `MapSession.Feed()` during char auth — it is a
  compile-time error to use the wrong session type.
- **No reconfigure race**: a single `Reconfigure()` method would require locking to
  swap handler maps while a read loop might be calling `Feed()`. Three typed sessions
  have no such method; there is nothing to race.
- **Matches rAthena source**: `common/packets.hpp` (login+char), `packets.hpp` and
  `packets_struct.hpp` (map) are already separated in rAthena. The codegen mirrors
  this split.

### Session lifecycle

**Via ConnectionFSM (typical goKore usage)**: The FSM constructs and destroys
`LoginSession` and `CharSession` internally. goKore only ever sees `*MapSession`,
received through the `OnReady` callback. See §4 for the full sequence.

**Direct construction (testing / non-standard flows)**:

```
1. Create LoginSession(packetver)
   → dial net.Conn to login server manually
   → call Feed() in a read loop; register OnLoginAccepted callback
   → OnLoginAccepted fires: extract tokens, close conn

2. Create CharSession(packetver)
   → dial net.Conn to char server
   → call Feed() in a read loop; register OnCharListReceived, OnMapServerInfo
   → OnMapServerInfo fires: extract map address, close conn

3. Create MapSession(packetver)
   → optionally call EnableObfuscation(key0, key1, key2)
   → dial net.Conn to map server
   → call Feed() in a read loop; register all gameplay callbacks
   → session lives for the duration of the game connection
```

The connection FSM (`StateDisconnected → StateLoginAuth → ... → StateReady`)
lives in `pkg/fsm`. This is part of the library, not goKore.

### Public API

```go
// LoginSession
func NewLoginSession(packetver uint32) *LoginSession
func (s *LoginSession) Feed(data []byte) error
func (s *LoginSession) OnLoginAccepted(fn func(events.LoginAccepted))
func (s *LoginSession) OnLoginRefused(fn func(events.LoginRefused))
// ... one On* method per login-server receive action
// NOTE: Sessions do NOT expose an Encode(req) method — callers use generated encode
// functions directly (e.g. encode.EncodeLogin(req, packetver) returns [N]byte).
// This avoids a heap-allocating []byte wrapper on the encode path.

// CharSession
func NewCharSession(packetver uint32) *CharSession
func (s *CharSession) Feed(data []byte) error
func (s *CharSession) OnCharListReceived(fn func(events.CharListReceived))
func (s *CharSession) OnMapServerInfo(fn func(events.MapServerInfo))
// ... one On* method per char-server receive action

// MapSession
func NewMapSession(packetver uint32) *MapSession
func (s *MapSession) EnableObfuscation(key0, key1, key2 uint32)
// EnableObfuscation activates C→S packet ID obfuscation for this session.
// Must be called before the first Encode call. key0/key1/key2 = clif_cryptKey[0/1/2].
// The first encoded packet uses the one-step key ((k0*k1+k2)>>16)&0x7FFF.
// Subsequent packets use the rolling LCG key initialized with the two-step formula.
// S→C packets received via Feed() are never obfuscated.
func (s *MapSession) Feed(data []byte) error
// Encode applies C→S packet ID obfuscation (if enabled) to the given fixed-size
// byte slice and returns the obfuscated version. The caller builds the raw packet
// using a generated encode function (e.g. encode.EncodeMove), then passes the
// result to Encode to apply shuffling/obfuscation before writing to the socket.
// Encode does NOT allocate — it operates on the caller-provided array in place.
func (s *MapSession) Encode(pktID *uint16)
func (s *MapSession) OnActorExists(fn func(events.ActorExists))
func (s *MapSession) OnActorMoved(fn func(events.ActorMoved))
func (s *MapSession) OnActorConnected(fn func(events.ActorConnected))
func (s *MapSession) OnActorVanished(fn func(events.ActorVanished))
func (s *MapSession) OnMapEnter(fn func(events.MapEnter))
func (s *MapSession) OnStatUpdate(fn func(events.StatUpdate))
// ... one On* method per map-server receive action
```

**Note on obfuscation**: `EnableObfuscation` exists only on `MapSession` and affects
the **send path only** (C→S packet IDs). The login and char servers do not use
PACKET_OBFUSCATION — verified by inspecting `clif_obfuscation.hpp` (map server only)
and finding no key setup in `loginclif.cpp` or `char_clif.cpp`. S→C packets received
via `Feed()` are never obfuscated on any server type.

---

## 6. Source of Truth: Two-Source Hybrid

### Source 1: GCC Preprocessor (struct layout authority)

`packets_struct.hpp` can be preprocessed cleanly with no external dependencies:

```bash
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I src -I src/map -I src/common \
    src/map/packets_struct.hpp
```

Running this at each of the ~223 PACKETVER breakpoints in the file (212 in
`packets_struct.hpp` plus 31 in `packets.hpp`, union = 223 — verified by counting
unique dates after preprocessing) and diffing
adjacent outputs yields a complete, lossless table of every struct layout change
across all supported packetvers. This is mechanically correct — the compiler
resolves every `#if PACKETVER >= X` exactly as rAthena does.

**Note on PACKETVER build flavors**: Some conditions use `PACKETVER_MAIN_NUM`,
`PACKETVER_RE_NUM`, and `PACKETVER_ZERO_NUM` — separate version axes for the Main,
Renewal (RE), and Zero branches of the official RO client. These cannot be collapsed
to a single `PACKETVER` date. The codegen runs three preprocessing passes per
breakpoint (MAIN, RE, ZERO) and merges the results.

`packets.hpp` (newer ZC_/CZ_ structs, ~279 additional structs) requires stub headers
for the `map.hpp → script.hpp → ryml_std.hpp` include chain. These stubs are maintained
in `validation/stubs/packets_hpp_stub.h` and `internal/codegen/stubs/packets_hpp_stub.h`.
**Note (M13 correction):** Prior HLD versions incorrectly stated that `mysql.h` and
`libconfig.h` stubs were needed for `packets.hpp`. The actual dependency chain is
`map.hpp → script.hpp → ryml_std.hpp` — there is no mysql or libconfig in this path.

`common/packets.hpp` (login/char server structs, ~131 structs when preprocessed at
PACKETVER=20180307) requires stubs for `common/mmo.hpp`, `common/socket.hpp`,
`common/showmsg.hpp`, and `common/utilities.hpp`. These are maintained in
`validation/stubs/common_hpp_stub.h` and `internal/codegen/stubs/common_hpp_stub.h`.
**Note:** `common/packets.hpp` does NOT need `mysql.h` or `libconfig.h` stubs.

**What the preprocessor gives us**:
- Exact field names, types, and order for every struct at every packetver
- `UNAVAILABLE_STRUCT` sentinel detection (struct has only `int8 _____unavailable`)
- Packet ID assignment via the `packet_headers` enum
- `DEFINE_PACKET_HEADER` macro resolution for packet ID ↔ struct name mapping

**What the preprocessor cannot give us**:
- Semantic names (`AID` maps to OpenKore `ID`; `effectState` maps to `option`)
- Canonical action grouping (four packet IDs all implement `actor_exists`)
- Decode hints for packed binary fields (PosDir, MoveData — described in §7)
- OpenKore compatibility names

### Source 2: Semantic Manifest (`semantics/mappings.yaml`, 42,751 lines)

A YAML file that provides only what the preprocessor cannot. Despite its size
(42,751 lines as of 2026-03-06), it is machine-readable and machine-writeable via
the `gokore-semantics` MCP server. Do not edit directly; use MCP only.

**Known quality issues**: As of 2026-03-06, running `semantics_validate_all_quality`
returns 306 errors and 1000+ quality warnings. Treat the DB as a starting point —
always cross-check against GCC preprocessor output before implementing.

```yaml
# semantics.yaml — human-maintained semantic layer
# NO field types  — those come from the preprocessor
# NO packetver conditions — those come from the preprocessor

actions:
  actor_exists:
    description: Stationary entity visible in viewport
    openkore_name: actor_exists
    canonical_params:
      - { name: ID,         rathena_field: AID,        type: uint32 }
      - { name: CharID,     rathena_field: GID,        type: uint32 }
      - { name: X,          rathena_field: PosDir,     decode: pos_x }
      - { name: Y,          rathena_field: PosDir,     decode: pos_y }
      - { name: Dir,        rathena_field: PosDir,     decode: pos_dir }
      - { name: Speed,      rathena_field: speed,      type: uint16 }
      - { name: ObjectType, rathena_field: objecttype, type: uint8 }
    implementations:
      - { packet_id: "0x0078", struct: packet_idle_unit }
      - { packet_id: "0x01D8", struct: packet_idle_unit }
      - { packet_id: "0x09FF", struct: packet_idle_unit }
```

This file is human-readable, human-maintainable, and small enough to review in a
single sitting. It is **not** the source of field types or struct layouts.

### How They Combine

```
GCC preprocess at each breakpoint
    → struct_db: map[struct_name]map[packetver_range]StructLayout
    ↓
semantics.yaml
    → action_db: map[action_name]ActionDef (with field name mappings)
    ↓
codegen joins struct_db + action_db
    → pkg/decode/*.go      (one file per action, inline packetver switches)
    → pkg/encode/*.go      (one file per send action)
    → pkg/events/*.go      (one file per canonical event type)
    → pkg/session/lengths.go  (LengthTableFor per server per packetver)
```

The codegen reads rAthena source from a local checkout. The path is a codegen
argument. Generated files are committed to the repository (analogous to `.pb.go`
files). Regeneration is triggered manually when rAthena is updated.

The semantic manifest is at `semantics/mappings.yaml` (accessed via MCP only —
see README-LLM.md §9 for the correct workflow).

---

## 7. goKore Integration Contract

This section precisely documents the boundary between goKore and rathena-client.
It is normative — the library is designed around these constraints.

### What goKore owns

- `net.Conn` — goKore dials (via the `Dialer` it provides), reads, writes, and
  closes every TCP socket. The library never calls `net.Dial`.
- A `Dialer` function passed to `ConnectionFSM` — goKore decides whether this is
  `net.DialTimeout`, a proxy, a test stub, etc.
- Reconnect policy, back-off timing, and credential storage
- Session token storage after `OnReady` fires (`AccountID`, `SessionID1`, etc.) —
  the FSM stores these only for the duration of the auth sequence; goKore is free
  to persist them for diagnostics
- The `hook.Dispatcher` event bus
- Domain handlers (`actors/`, `movement/`, `combat/`, etc.)

### What goKore deletes (migration)

| Deleted | Replaced by |
|---------|-------------|
| `internal/network/packets/generated/` (4,634 files) | `pkg/decode/` + `pkg/events/` in rathena-client |
| `internal/network/packetver/` (`PacketVersionRegistry`, `action_selector.go`) | `pkg/session/` session types |
| `internal/network/adapters/` (~500 files) | Deleted; no replacement needed |
| `internal/network/receive/Receiver` (2-goroutine design) | goKore read loop calls `mapSession.Feed()` directly |
| `internal/network/params/` | `pkg/events/` types |
| `internal/network/connection/fsm.go` | `pkg/fsm/ConnectionFSM` |
| `internal/network/connection/session.go` (`SessionData`) | Tokens stored inside FSM during auth; not needed after `OnReady` |

### What goKore keeps (unchanged)

- `internal/network/tcp/TCPConnection` — socket ownership; still used to provide
  the `Dialer` function to `ConnectionFSM`
- `internal/network/handlers/` — domain handler logic, updated to accept
  `events.*` types from rathena-client
- `internal/hook/` — event bus; goKore callbacks fire the dispatcher

### New goKore adapter layer

```go
// internal/network/rathena/connector.go
// Thin glue: wires rathena-client FSM to goKore's lifecycle.

func Start(ctx context.Context, cfg Config, dispatcher *hook.Dispatcher) {
    dialer := func(ctx context.Context, addr string) (net.Conn, error) {
        return net.DialTimeout("tcp", addr, cfg.DialTimeout)
    }

    server := fsm.ServerConfig{
        LoginAddr:   cfg.LoginAddr,
        Packetver:   cfg.Packetver,
        StepTimeout: 30 * time.Second,
    }

    creds := fsm.Credentials{
        Username: cfg.Username,
        Password: cfg.Password,
        CharSlot: cfg.CharSlot,
    }

    mapDone := make(chan struct{}, 1)

    loginFSM := fsm.New(server, creds, dialer).
        OnCharServerList(func(servers []events.CharServerInfo) int {
            return 0 // private rAthena always has one entry
        }).
        OnCharList(func(chars []events.CharacterInfo) uint8 {
            return cfg.CharSlot  // or bot logic to pick by name
        }).
        OnReady(func(s *session.MapSession, conn net.Conn) {
            // Register all gameplay callbacks
            s.OnActorExists(func(e events.ActorExists) {
                dispatcher.Trigger(ctx, hook.EventActorSpawned, e)
            })
            s.OnActorMoved(func(e events.ActorMoved) {
                dispatcher.Trigger(ctx, hook.EventActorMoved, e)
            })
            // ... remaining callbacks ...

            // Start goKore's read loop — it now owns the conn
            go runMapLoop(ctx, s, conn, dispatcher, mapDone)
        }).
        OnFailed(func(err error) {
            dispatcher.Trigger(ctx, hook.EventConnectionFailed,
                hook.ConnectionFailedEvent{Err: err})
        })

    for {
        if err := loginFSM.Connect(ctx); err != nil {
            time.Sleep(cfg.RetryDelay)
            continue
        }
        // Connect blocks until OnReady fires.
        // runMapLoop blocks until the server disconnects, then signals mapDone.
        <-mapDone
        // Connect again — the FSM tears down prior state automatically.
        loginFSM.Connect(ctx)
    }
}

// runMapLoop is goKore's read loop for steady-state gameplay.
// It owns the net.Conn after OnReady.
func runMapLoop(
    ctx context.Context,
    s *session.MapSession,
    conn net.Conn,
    dispatcher *hook.Dispatcher,
    done chan<- struct{},
) {
    buf := make([]byte, 65536)
    defer func() { done <- struct{}{} }()
    for {
        n, err := conn.Read(buf)
        if err != nil {
            dispatcher.Trigger(ctx, hook.EventDisconnected, err)
            return
        }
        // Feed is synchronous; all callbacks fire and return before next Read.
        // Feed returns error on stream desync (unknown packet ID). Caller closes conn.
        if err := s.Feed(buf[:n]); err != nil {
            dispatcher.Trigger(ctx, hook.EventDisconnected, err)
            return
        }
    }
}
```

### Send pattern (steady-state)

```go
// goKore builds a raw packet using generated encode function, then obfuscates.
raw := encode.EncodeMove(send.RequestMove{X: 150, Y: 200, Dir: 0}, mapSession.Packetver())
mapSession.Encode(&raw[0])  // applies C→S packet ID obfuscation in place if enabled
conn.Write(raw[:])
```

### Keepalive pattern (steady-state)

```go
// goKore schedules keepalives on its own timer goroutine.
// The library provides the encode functions; goKore owns the schedule.
ticker := time.NewTicker(cfg.KeepaliveInterval)
for range ticker.C {
    raw := encode.EncodeMapKeepalive(send.MapKeepalive{}, mapSession.Packetver())
    mapSession.Encode(&raw[0])
    conn.Write(raw[:])
}
```

---

## 8. Performance Contract

This section documents hard performance requirements, not aspirational targets.
At 1000 concurrent bots, every per-packet overhead multiplies by 1000 × the packet
rate. These constraints are verified by benchmarks in CI.

### Zero goroutines

The library contains no `go` statement in any non-test `pkg/` file. This is enforced by a
CI lint check (`grep -r --include='*.go' --exclude='*_test.go' "^\s*go " pkg/`).
Goroutines are the caller's concern. Test files (`_test.go`) are excluded because
test helpers legitimately use goroutines to simulate concurrent callers.

### Zero heap allocations in the decode hot path

`session.Feed()` must produce zero heap allocations per decoded packet in steady
state. "Steady state" means after the initial `recvBuf` has grown to its high-water
mark (once per session, amortized to zero).

Event structs are stack-allocated inside the generated decode function and passed
by value to the registered callback. The Go compiler must not escape them. This is
verified with `go test -bench=. -benchmem` (0 allocs/op) and
`go build -gcflags="-m" 2>&1 | grep "does not escape"` in CI.

### Handler lookup: O(1) array index

Each session type maintains a `[65536]HandlerFunc` array indexed by packet ID.
No map, no hash, no interface dispatch. Lookup is a single array dereference.

**Acknowledged cost**: at 1000 MapSession instances, this is
1000 × 65536 × 8 bytes = **500 MB** for handler arrays alone.
The length tables add 1000 × 65536 × 2 bytes = **~128 MB**.
Total acknowledged memory: **~628 MB** for a dedicated bot machine. If memory
becomes a constraint, the lookup can be changed to a sorted slice + binary search
(~6 MB total for lengths + handlers) without changing any public API.

### Packet framing: O(1) length lookup

The length table is also a `[65536]int16` array. For a variable-length packet
(length = -1), `Feed()` reads bytes 2:4 of the frame to get the length. No map
lookup.

### `Feed()` not goroutine-safe — by design

A single session must be used from exactly one goroutine. goKore's architecture
already guarantees this: one read goroutine per TCP connection. Adding a mutex to
`Feed()` would add ~20 ns per call × all packets × 1000 bots for no benefit.

### Benchmarks (targets, verified in CI)

| Benchmark | Target |
|-----------|--------|
| `BenchmarkFeed_SmallFixedPacket` | 0 allocs/op, < 200 ns/op |
| `BenchmarkFeed_ActorExists_0x09FF` | 0 allocs/op, < 500 ns/op |
| `BenchmarkEncode_RequestMove` | 0 allocs/op, < 100 ns/op |
| `BenchmarkFeed_1000Sessions_Parallel` | linear scaling with goroutine count |

---

## 9. Key Package Descriptions

### `pkg/packing`

Pure, dependency-free bit-packing for the two packed binary formats. Validated
against rAthena `clif.cpp:173–249`.

```go
func DecodePosDir(data []byte) (x, y uint16, dir uint8)
func EncodePosDir(x, y uint16, dir uint8) [3]byte
func DecodeMoveData(data []byte) (fromX, fromY, toX, toY uint16, sx0, sy0 uint8)
func EncodeMoveData(fromX, fromY, toX, toY uint16, sx0, sy0 uint8) [6]byte
```

**Critical invariants** (from rAthena source):
- `DecodePosDir` is correct for all `uint8 PosDir[3]` fields across all packetvers
- `DecodeMoveData` byte 5 is `sx0` (high nibble) and `sy0` (low nibble) — **NOT
  direction**. There is no direction in the 6-byte format.
- Both functions are only called from `clif.cpp` in rAthena — no other rAthena
  file uses them.

### `pkg/events`

One struct per canonical receive action. These are the types that goKore's callbacks
accept — no knowledge of packet IDs or packetvers.

```go
// events/actor_exists.go — GENERATED
type ActorExists struct {
    ID         uint32  // rAthena AID (bl->id)
    CharID     uint32  // rAthena GID (char_id; 0 for monsters)
    X, Y       uint16  // decoded from PosDir[3] via DecodePosDir
    Dir        uint8   // 0=N,1=NW,2=W,3=SW,4=S,5=SE,6=E,7=NE
    Speed      uint16  // movement speed (ms per cell)
    ObjectType uint8   // 0=PC, 5=MOB, 6=NPC, etc.
    HP, MaxHP  int32
    Name       string
    // ... full field list from semantics.yaml
}
```

All packed binary fields (`PosDir`, `MoveData`) are decoded before the event struct
is populated. No `string`-typed binary fields. No `[]byte` fields that require
caller-side decoding.

### `pkg/send`

One struct per canonical send action (C→S). Value types, no pointers.

```go
// send/move.go
type RequestMove struct {
    X, Y uint16
    Dir  uint8  // encoded into PosDir[3] via EncodePosDir
                // rAthena discards dir on receive (RBUFPOS called with nullptr)
                // but the wire format requires a valid 4-bit value
}
```

### `pkg/decode`

GENERATED. One file per canonical action. Each file contains one decode function
per packet ID variant. All functions are pure (no side effects, no state).

```go
// GENERATED: decode/actor_exists.go
func ActorExists_0x09FF(data []byte, packetver uint32) events.ActorExists {
    var e events.ActorExists
    off := 4 // skip PacketType(2) + PacketLength(2)
    e.ObjectType = data[off]; off++
    e.ID  = leU32(data, off); off += 4
    e.CharID = leU32(data, off); off += 4
    e.Speed  = leU16(data, off); off += 2
    // ...
    if packetver >= 20080102 {
        off += 4 // effectState is int32 post-20080102
    } else {
        off += 2 // effectState is int16 pre-20080102
    }
    // ...
    e.X, e.Y, e.Dir = packing.DecodePosDir(data[off:])
    off += 3
    // ...
    return e  // returned by value; does not escape to heap
}
```

### `pkg/encode`

GENERATED. One file per send action. Functions return fixed-size byte arrays
(`[N]byte`) where possible to avoid heap allocation on the return value.

```go
// GENERATED: encode/move.go
func EncodeMove(req send.RequestMove, packetver uint32) [5]byte {
    var p [5]byte
    leU16Put(p[0:], 0x0085) // packetID; version-conditional handled via packetver switch
    copy(p[2:], packing.EncodePosDir(req.X, req.Y, req.Dir)[:])
    return p
}
```

### Error types (`pkg/session/errors.go`)

```go
// ErrUnknownPacket is returned by Feed() when an unrecognized packet ID is
// encountered. The stream is irrecoverably desynced; the caller must close the conn.
type ErrUnknownPacket struct {
    ID uint16
}
func (e ErrUnknownPacket) Error() string

// ErrTimeout is returned by ConnectionFSM.Connect() when a server step exceeds
// ServerConfig.StepTimeout. The underlying cause is a net.Error with Timeout()==true.
type ErrTimeout struct {
    Step string  // e.g. "login_auth", "char_auth", "map_auth"
    Err  error   // the underlying net.Error
}
func (e ErrTimeout) Error() string
func (e ErrTimeout) Unwrap() error

// HandlerFunc is the type of a registered packet handler callback.
// Called synchronously inside Feed() for each decoded frame.
// data is the full frame bytes (including the 2-byte packet ID header).
// packetver is the packetver the session was created with.
type HandlerFunc func(data []byte, packetver uint32)
```

### `pkg/session` — sessionCore

```go
// Internal structure shared by all three session types
type sessionCore struct {
    packetver uint32
    buf       []byte            // full backing array; never re-sliced; owned by sessionCore
    recvBuf   []byte            // active sub-slice of buf; advances forward as frames are consumed
    lengths   [65536]int16      // GENERATED: length table for this server type
    handlers  [65536]HandlerFunc
    faulted   bool              // set true on ErrUnknownPacket; Feed() becomes a no-op
}

// MapSession also has send-side obfuscation state:
type obfuscationState struct {
    enabled    bool
    firstSent  bool    // false until the first C→S packet has been sent
    rollingKey uint32  // rolling key for C→S packet ID XOR (advances after each send)
    key0       uint32  // clif_cryptKey[0] — used only for first-packet key
    key1       uint32  // clif_cryptKey[1] — LCG multiplier
    key2       uint32  // clif_cryptKey[2] — LCG addend
}
```

**What obfuscation does — direction clarified**: PACKET_OBFUSCATION applies only to
**C→S packet IDs**. The client XORs the 2-byte packet type of every C→S packet
before sending. The server deobfuscates them in `clif_parse`. S→C packets are
**not obfuscated** — `Feed()` reads them as plain unmodified bytes.

Source: `clif.cpp:25692–25764` — `clif_parse` deobfuscates the received `cmd`
(C→S). There is no corresponding obfuscation on the outgoing (S→C) path.

**Client-side obfuscation sequence** (must match the inverse of `clif_parse`):

```
// Source: clif.cpp:25702 — first packet (0x0436) uses one-step key
firstKey = ((clif_cryptKey[0] * clif_cryptKey[1] + clif_cryptKey[2]) >> 16) & 0x7FFF

// Source: clif.cpp:10721 — rolling key initialized after server receives first packet
// The client initializes this at the same time (EnableObfuscation call)
rollingKey = (((clif_cryptKey[0]*clif_cryptKey[1]+clif_cryptKey[2]) & 0xFFFFFFFF)
               * clif_cryptKey[1] + clif_cryptKey[2]) & 0xFFFFFFFF
// i.e.: step1 = (k0*k1+k2)&0xFFFFFFFF; rollingKey = (step1*k1+k2)&0xFFFFFFFF
```

To encode a C→S packet with obfuscation:

```
if !firstSent:
    obfuscatedID = rawPacketID XOR firstKey
    firstSent = true
else:
    obfuscatedID = rawPacketID XOR ((rollingKey >> 16) & 0x7FFF)
    rollingKey = (rollingKey * key1 + key2) & 0xFFFFFFFF
```

`EnableObfuscation(key0, key1, key2 uint32)` stores all three keys and computes
`firstKey` and `rollingKey` from the formulas above.

**Obfuscation discontinuation**: `clif_obfuscation.hpp` sets all three keys to
`0x00000000` for `PACKETVER > 20180307`. When all keys are zero, the XOR is a no-op.
The client should simply not call `EnableObfuscation` for modern servers.

**`ObfuscationKeysFor` API** (GENERATED: `pkg/session/obfuscation_keys.go`):

```go
// GENERATED: pkg/session/obfuscation_keys.go
// Source: src/map/clif_obfuscation.hpp (read with -DPACKET_OBFUSCATION defined).
// Returns the three obfuscation keys for the given packetver.
// Returns (0, 0, 0) for packetver > 20180307 (obfuscation disabled).
// Returns (0, 0, 0) for any packetver not in the key table.
// Callers should check: if k0|k1|k2 != 0 { session.EnableObfuscation(k0, k1, k2) }
func ObfuscationKeysFor(packetver uint32) (k0, k1, k2 uint32)
```

**Codegen requirement**: `clif_obfuscation.hpp` must be preprocessed with
`-DPACKET_OBFUSCATION` defined or it produces zero output. This flag was absent from
prior HLD GCC commands — a BLOCKER corrected here. See `validation/preprocess_check.sh`
for the correct command.

`Feed(data []byte) error` algorithm:

```
1. Append data to recvBuf (no alloc if capacity sufficient)
2. Loop while recvBuf has enough bytes for a complete frame:
   a. Read packetID = leU16(recvBuf[0:2])
      (S→C packets are NOT obfuscated — read the ID directly)
   b. frameLen = lengths[packetID]
      if frameLen == -1: frameLen = leU16(recvBuf[2:4])  (variable-length)
      if frameLen == 0:
          // Unknown packet: the stream is now desynced; we cannot determine
          // the frame length. Set internal faulted flag; all future calls are
          // no-ops. Return ErrUnknownPacket{ID: packetID}.
          // Caller is responsible for closing the connection.
          s.faulted = true
          return ErrUnknownPacket{ID: packetID}
   c. if len(recvBuf) < int(frameLen): break (incomplete frame)
   d. fn := handlers[packetID]
      if fn != nil: fn(recvBuf[:frameLen], packetver)
   e. consumed += int(frameLen)
      recvBuf = recvBuf[frameLen:]  // advance slice header
3. If consumed > 0:
      // Copy unconsumed bytes to the front of the backing array and re-slice.
      // Without this, the backing array fills and append allocates a new one.
      // The full backing array is accessible via s.buf (the underlying array,
      // never re-sliced). recvBuf is a sub-slice of s.buf.
      n := copy(s.buf, recvBuf)
      recvBuf = s.buf[:n]
4. Return nil
```

**recvBuf memory management note**: advancing `recvBuf = recvBuf[frameLen:]` only
moves the slice header — it does not free the consumed prefix. If the backing array
is never reclaimed, it grows indefinitely until the next `append` allocates a new
one. The copy-to-front step (step 3) ensures the backing array is reused across
calls. After the first few frames grow `recvBuf` to its high-water mark, no further
allocations occur during steady-state operation.

### `internal/codegen`

The code generator. Not importable — `go run`-only tool.

```
internal/codegen/
    main.go              entry point: orchestrates all generation steps
    preprocess/
        runner.go        runs GCC -E -P for each PACKETVER breakpoint
        parser.go        parses flat preprocessed C into StructDB
        differ.go        diffs adjacent packetver outputs → VersionTable
    semantics/
        loader.go        reads semantics/mappings.yaml
    gen/
        decode.go        generates pkg/decode/*.go
        encode.go        generates pkg/encode/*.go
        events.go        generates pkg/events/*.go
        lengths.go       generates pkg/session/lengths_*.go (per server type)
        shuffle.go       generates pkg/session/shuffle_map.go
        obfuscation.go   generates pkg/session/obfuscation_keys.go
    stubs/
        packets_hpp_stub.h   stubs for packets.hpp → map.hpp → script.hpp → ryml chain
        common_hpp_stub.h    stubs for common/packets.hpp → mmo.hpp etc.
```

**`clif_shuffle.hpp` codegen** (`gen/shuffle.go`):

`clif_shuffle.hpp` has 1 `#if PACKETVER ==` block and 152 `#elif PACKETVER ==` blocks
(153 PACKETVER-exact sections — verified by `grep -c "^#elif"` = 152).
Each section lists `parseable_packet(shuffled_id, ...)` entries. The codegen maps
each `parseable_packet(shuffledID, len, handler, ...)` to the base C→S packet ID
for that handler (the unshuffled ID defined in `clif_packetdb.hpp`).

The generated output is `pkg/session/shuffle_map.go`:

```go
// GENERATED: pkg/session/shuffle_map.go
// ShuffledCtoSID returns the shuffled C→S wire packet ID for a given base packet ID
// at the given packetver. Returns baseID unchanged if no shuffle exists for this
// packetver (i.e. packetver is not one of the 153 exact PACKETVER == values in
// clif_shuffle.hpp).
//
// Source: src/map/clif_shuffle.hpp (153 exact PACKETVER sections).
func ShuffledCtoSID(packetver uint32, baseID uint16) uint16
```

**`clif_packetdb.hpp` codegen** (`gen/lengths.go` + `gen/shuffle.go`):

`clif_packetdb.hpp` defines base (unshuffled) C→S packet registrations:
`parseable_packet(packetID, length, handler, ...)`. It provides the base packet ID
→ handler name mapping needed to reverse-map shuffled IDs to base IDs. The codegen
reads this file to build the base ID table, then cross-references `clif_shuffle.hpp`
to produce the shuffle table. It also produces the C→S length table for sessions
that need it.

---

## 10. Non-Trivial Wire Formats

### 3-byte packed position (WBUFPOS)

Used in all `PosDir[3]` fields. See `pkg/packing`.

```
Byte 0: [x9 x8 x7 x6 x5 x4 x3 x2]
Byte 1: [x1 x0 y9 y8 y7 y6 y5 y4]
Byte 2: [y3 y2 y1 y0 d3 d2 d1 d0]
```

- x: 10-bit coordinate, bits [23:14]
- y: 10-bit coordinate, bits [13:4]
- dir: 4-bit direction, bits [3:0]

### 6-byte packed movement (WBUFPOS2)

Used in all `MoveData[6]` fields. **Byte 5 is NOT direction** — it is sub-cell
interpolation offsets `sx0` (high nibble) and `sy0` (low nibble).

```
Bytes 0-4: fromX(10b) fromY(10b) toX(10b) toY(10b)
Byte 5:    [sx0_3 sx0_2 sx0_1 sx0_0 sy0_3 sy0_2 sy0_1 sy0_0]
```

For bot purposes `sx0`/`sy0` can be ignored — they are cosmetic interpolation hints
for the visual client. The library decodes and exposes them but callers need not use
them.

---

## 11. PACKETVER Handling Strategy

`PACKETVER` is a `uint32` (format `YYYYMMDD`) set once when a session is created.
It never changes for the lifetime of a connection. This matches rAthena's compile-
time `#define PACKETVER` — the condition moves from compile time to the start of the
decode function.

**Packet ID assignment** is also PACKETVER-dependent. For example, `packet_idle_unit`
maps to (from `packets_struct.hpp` `idle_unitType` enum):

| PACKETVER range                    | Packet ID |
|------------------------------------|-----------|
| < 20040000                         | 0x0078    |
| 20040000 – 20071112                | 0x01D8    |
| 20071113 – 20091102                | 0x022A    |
| 20091103 – (next breakpoint)       | 0x02EE    |
| ...                                | 0x07F9    |
| ...                                | 0x0857    |
| ...                                | 0x0915    |
| ...                                | 0x09DD    |
| >= 20150513                        | 0x09FF    |

`LengthTableFor(server, packetver)` returns the correct ID→length mapping for the
given server type and packetver. Each session populates its `lengths[65536]` array
from this table at construction time.

`LengthTableFor` is **not** a runtime function — it is a codegen artifact. The
codegen produces `lengths_login.go`, `lengths_char.go`, and `lengths_map.go`, each
containing a function `func populateLengths(pv uint32, t *[65536]int16)` that is
called from `NewLoginSession`, `NewCharSession`, `NewMapSession` respectively. This
function is an inline `switch pv { case ...: t[id] = len; ... }` with no allocations.
The session constructor calls it once; the array is never modified after construction.

**Same packet ID, different layout** (Pattern B): packet `0x0206`
(`PACKET_ZC_FRIENDS_STATE`) has a 9-byte body pre-20180221 and a 33-byte body
post-20180221. The decode function handles this with a single `if packetver >=
20180221` branch. The tokenizer uses the correct length from `lengths[]`, so framing
is always correct before decode is called.

---

## 12. Known Bugs Fixed by This Library

The following bugs exist in the current goKore network stack and are eliminated by
this library. All are documented in
`docs/WORKLOG/0000_2026-03-05_wire_protocol_byte_packing_rathena.md`.

| # | Bug | Old location | Fix in rathena-client |
|---|-----|-------------|----------------------|
| 1 | `PosDir` stored as `string` | `v20030000/packet_zc_notify_standentry.go: PosDir string` | All packed binary fields use `[3]byte` or `[6]byte`; `DecodePosDir`/`DecodeMoveData` called by generated decode fn before event struct is populated |
| 2 | `direction = (data[5] & 0xF0) >> 4` from MoveData | `handlers/actors/handler.go:88` | 6-byte format has no direction; `DecodeMoveData` returns `(fromX, fromY, toX, toY, sx0, sy0)`; events.ActorMoved has no `Dir` field |
| 3 | `bits |= uint32(0x44) << 24` hardcoded direction | `send/builders/map_builder.go:90` | `EncodePosDir(x, y, dir)` takes explicit direction parameter; rAthena discards it server-side but the wire format is correct |
| 4 | `byte(coords >> 32)` uint32 overflow | `handlers/movement/handler.go:523` | Dead code; `EncodeMoveData` is the replacement and uses `[6]byte` directly with no integer overflow |

---

## 13. Initial Implementation Scope (Phase 1 in README-LLM.md)

The first working slice implements the full login → char → map connect flow plus
core actor visibility — enough to authenticate against a rAthena server, receive
the character list, enter the map, and see nearby actors.

**Phase note**: README-LLM.md uses a different phase numbering (Phase 0 = validation
infrastructure, Phase 1 = fix HLD, Phase 2 = packing, Phase 3 = codegen, etc.).
This §13 describes the *packet scope* for Phase 3 (codegen) and Phase 4 (generated
packages) + Phase 5 (session hand-written parts) in README terms.

### Packets handled by the FSM internally (transparent to goKore)

| Step | Packet IDs | Direction | Notes |
|------|-----------|-----------|-------|
| Send login request | 0x0064 | C→S | Sent automatically on login server connect; plain-text password; rAthena accepts all login variants simultaneously but 0x0064 is the universal baseline |
| Recv login accepted | 0x0069, 0x0AC4 | S→C | Tokens extracted; char server list parsed; OnCharServerList called; conn closed |
| Recv login refused | 0x006A, 0x083E | S→C | → `OnFailed` |
| Send char connect | 0x0065 (raw) | C→S | Sent automatically on char server connect |
| Recv char slot info | 0x082D | S→C | PACKETVER ≥ 20130000 only; auxiliary; stored |
| Recv char list (initial) | 0x006B | S→C | Always sent. PACKETVER < 20130000: char list is complete here, no 0x09A0 follows. PACKETVER ≥ 20130000: partial — wait for 0x09A0 |
| Recv charlist notify | 0x09A0 | S→C | PACKETVER ≥ 20130000 only; sends `CH_CHARLIST_REQ` (0x09A1) × `p.total`. NOTE: for PACKETVER_RE_NUM >= 20151001 AND < 20180103 the packet has an extra `uint32 slots` field (total 10 bytes instead of 6). Source: `common/packets.hpp:617–624`. The FSM reads `p.total` from offset 2; offset of `total` field is unchanged (always at byte 2). |
| Recv char pages | 0x099D | S→C | PACKETVER ≥ 20130000; accumulate until all pages received; then call `OnCharList` |
| Send char select | 0x0066 | C→S | Sent after `OnCharList` returns slot |
| Recv map server info | 0x0081 (PACKETVER < 20170315), 0x0AC5 (≥ 20170315) | S→C | HC_NOTIFY_ZONESVR. Map addr extracted, conn closed. Source: `common/packets.hpp:290–308`. NOTE: 0x0081 is also SC_NOTIFY_BAN — FSM distinguishes by frame length (HC_NOTIFY_ZONESVR ≥ 28 bytes; SC_NOTIFY_BAN = 4 bytes). |
| Send map connect | 0x0436 (raw) | C→S | Sent automatically on map server connect |
| Recv ZC_AID | 0x0283 | S→C | PACKETVER ≥ 20070521; sent by rAthena in `clif_parse_WantToConnection` immediately on receiving 0x0436, before char-server auth round-trip; account ID echo; stored. PACKETVER < 20070521: raw 4-byte AID with no packet header — not relevant for Phase 1. |
| Recv map enter | 0x0073, 0x0A18, 0x02EB | S→C | ZC_ACCEPT_ENTER variants by PACKETVER; triggers map-loaded sequence |
| Send map loaded | 0x007D | C→S | Required; sent before `OnReady` |
| Send tick sync | 0x007E / 0x0360 | C→S | Required; sent before `OnReady`. 0x007E = CZ_REQUEST_TIME (default); 0x0360 = CZ_REQUEST_TIME2 (PACKETVER ≥ 20101124). Packet-shuffled servers use alternate IDs that map to the same rAthena handler. |
| Recv map enter refused | 0x0074 | S→C | → `OnFailed` |
| Recv server notify | 0x0081 | S→C | → `OnFailed` / `OnServerNotify` |

### Packets goKore handles directly (steady-state, via MapSession.Feed)

| Action | Packet IDs | Direction | Notes |
|--------|-----------|-----------|-------|
| `ping_live` | 0x0B1D (recv) / 0x0B1C (send) | S→C / C→S | Modern ping; caller replies in callback |
| `actor_exists` | 0x0078, 0x01D8, 0x09FF | S→C | Phase 1 subset; full set has 9 IDs (see §11) |
| `actor_moved` | 0x007B, 0x01DA, 0x022C, 0x09FD | S→C | Phase 1 subset; full set has 9 IDs |
| `actor_connected` | 0x007C, 0x01D9, 0x09FE | S→C | Phase 1 subset; full set has 9 IDs |
| `actor_vanished` | 0x0080 | S→C | |
| `stat_update` | 0x00B0, 0x00B1, 0x00BE | S→C | |
| `request_move` | 0x0085 (send) | C→S | |
| `map_keepalive` | scheduled by caller timer | C→S | `0x007E` or `0x0B1C` |

Phase 2 adds the remaining ~400+ actions via full codegen from the preprocessor +
`semantics/mappings.yaml`.

---

## 14. Repository Structure

```
rathena-client/
    go.mod                              module github.com/lenaxia/ragnarok-go-client
    semantics/
        mappings.yaml                   human-maintained semantic layer (~42,751 lines; edit via MCP only)

    docs/
        DESIGN/
            HLD.md                      this document
        WORKLOG/
            0000_2026-03-05_wire_protocol_byte_packing_rathena.md

    pkg/
        packing/
            packing.go                  DecodePosDir, EncodePosDir, DecodeMoveData, EncodeMoveData
            packing_test.go             table-driven tests + fuzz tests

        fsm/
            fsm.go                      ConnectionFSM, Credentials, Dialer type
            fsm_test.go                 unit tests; uses net.Pipe stubs, no real network
            states.go                   State enum (Disconnected → Ready/Failed)

        events/                         GENERATED — one file per canonical receive action
            actor_exists.go             type ActorExists struct { ... }
            actor_moved.go
            actor_connected.go
            actor_vanished.go
            login_accepted.go
            char_list.go
            map_enter.go
            stat_update.go
            ...

        send/                           GENERATED — one file per canonical send action
            move.go                     type RequestMove struct { X, Y uint16; Dir uint8 }
            login.go                    type LoginRequest struct { ... }
            char.go
            ...

        decode/                         GENERATED — one file per canonical receive action
            actor_exists.go             func ActorExists_0x09FF(data []byte, pv uint32) events.ActorExists
            actor_moved.go
            login_accepted.go
            ...

        encode/                         GENERATED — one file per canonical send action
            move.go                     func EncodeMove(req send.RequestMove, pv uint32) [5]byte
            login.go
            ...

        session/
            login.go                    LoginSession
            char.go                     CharSession
            map.go                      MapSession
            lengths_login.go            GENERATED — login server length table
            lengths_char.go             GENERATED — char server length table
            lengths_map.go              GENERATED — map server length table
            obfuscation.go              LCG key state, XOR logic (MapSession only)
            session_bench_test.go       performance benchmarks

    internal/
        codegen/
            main.go                     entry point
            preprocess/
                 runner.go               runs GCC -E -P at each PACKETVER breakpoint
                 parser.go               parses preprocessed C into StructDB
                 differ.go               diffs adjacent outputs → VersionTable
            semantics/
                loader.go               reads semantics/mappings.yaml
            gen/
                decode.go               generates pkg/decode/*.go
                encode.go               generates pkg/encode/*.go
                events.go               generates pkg/events/*.go
                lengths.go              generates pkg/session/lengths_*.go
                shuffle.go              generates pkg/session/shuffle_map.go
                obfuscation.go          generates pkg/session/obfuscation_keys.go
            stubs/
                packets_hpp_stub.h      stubs for map.hpp → script.hpp → ryml chain
                common_hpp_stub.h       stubs for common/mmo.hpp etc.
```

---

## 15. Key Design Decisions and Rationale

| Decision | Rationale |
|---|---|
| GCC `-E -P` as struct source of truth | Eliminates manual transcription errors for field types/sizes. Compiler resolves all `#if PACKETVER` correctly. Validated at the compiler level. |
| Runtime `packetver uint32` in each decode fn | Single binary supports all servers. No snapshot directories. Packetver switch cost is immeasurable (~1 integer comparison per packet). |
| `semantics/mappings.yaml` for names only | Keeps human-maintained data minimal. Only semantic names, groupings, and decode hints — not the hundreds of type/size facts the compiler already knows. |
| Zero runtime deps | Protocol library must be embeddable. No transitive dependency surprises. |
| Separate Go module from goKore | Protocol correctness is independent of bot strategy. Other Go projects can import this library without importing goKore. |
| No `context.Context` in library | Context is an application concern. Threading it through every decode call at 1000 bots × N packets/sec is pure overhead with no cancelable operations to offer. |
| No goroutines in library | Goroutines are a concurrency primitive for the application layer. The library is a pure transformation: bytes in, typed events out. Zero goroutines means zero coordination overhead. |
| Callbacks not channels | A channel between the library and goKore adds a goroutine + synchronization + allocation at every event. Inline synchronous callbacks cost nothing and let goKore decide whether to dispatch asynchronously. |
| Three typed sessions | Type safety, memory efficiency (load only the packet table for the active server), no reconfigure race, mirrors rAthena source structure. |
| `[65536]HandlerFunc` array | O(1) lookup, no hash computation, no map overhead. 500 MB at 1000 bots — acknowledged and accepted for a dedicated machine. |
| `[N]byte` return from encode functions | Prevents heap escape on encode path. goKore calls `conn.Write(raw[:])` directly. |
| No reflection in hot path | `FromPacket(packet *types.Packet)` + reflection was a bug surface (wrong type assertions) and a performance cost. Direct byte reads with offset arithmetic are simpler and faster. |
| `pkg/packing` as authoritative byte-packing | Four separate incorrect implementations existed in goKore v1. One canonical implementation, one test suite, no ambiguity. |
| `ConnectionFSM` takes a `Dialer`, not a `net.Conn` | goKore owns the socket; the FSM needs to dial three separate servers sequentially. A `Dialer` func keeps the interface minimal: goKore can pass `net.DialTimeout`, a proxy, or a `net.Pipe` stub for tests. |
| `ConnectionFSM` is optional — sessions usable directly | Bot authors who want finer control (custom auth flows, replay testing, injection) can bypass the FSM and construct sessions directly. The FSM is a convenience, not a requirement. |
| FSM blocks in the caller's goroutine | Consistent with zero-goroutines-in-library. The caller (`Connect`) decides whether to run the FSM in a goroutine. goKore calls `go fsm.Connect(ctx)` to avoid blocking its main loop. |
| Map-loaded sequence sent inside FSM before `OnReady` | `0x007D` (map loaded ACK) and `0x007E`/`0x0360` (CZ_REQUEST_TIME tick sync) are required by rAthena before it sends game state. Sending them inside the FSM means `OnReady` fires on a fully ready session — goKore cannot accidentally forget them. |

---

## 16. Known Constraints and Risks

| Risk | Mitigation |
|---|---|
| `packets.hpp` requires stub headers (ryml, script) | Stubs are maintained in `validation/stubs/` and `internal/codegen/stubs/`. The actual dependency chain is `map.hpp → script.hpp → ryml_std.hpp` — NOT mysql/libconfig (common misconception). Breakage is obvious — codegen fails loudly. |
| rAthena upstream changes break the preprocessor input | Codegen is re-run when rAthena is updated. The structs it produces are the ground truth — no human review needed for type changes. |
| `semantics/mappings.yaml` field name mappings can be wrong | The DB contains 306 known validation errors as of 2026-03-06. Always cross-check against GCC preprocessor output before implementing. Errors surface as test failures in action integration tests. |
| PACKETVER_RE / PACKETVER_ZERO variants | `config/packets.hpp` defines `PACKETVER_RE_NUM` when `PACKETVER_RE` is set. The codegen runs separate preprocessing passes for MAIN, RE, and ZERO variants and merges the results. |
| 500 MB handler array cost at 1000 bots | Accepted for a dedicated bot machine. If memory becomes a constraint, the public API is unchanged — the internal lookup can be replaced with a sorted slice + binary search (~6 MB total) without any API break. |
| `Feed()` not goroutine-safe | By design. goKore's architecture already guarantees one read goroutine per connection. Documenting this explicitly prevents misuse. |
| Stack-allocated event structs may escape to heap | Verified by `go build -gcflags="-m"` in CI. If the compiler escapes an event struct, it is treated as a benchmark regression and fixed in the generated code. |
| Obfuscation key table changes per rAthena release | `clif_obfuscation.hpp` is read by codegen and regenerated into `pkg/session/obfuscation_keys.go`. Re-running codegen picks up new keys automatically. |
| Obfuscation rolling-key formula must exactly match rAthena | The two-step initialization (`step1 = (k0*k1+k2); key = (step1*k1+k2)`) and per-packet advance (`key = key*k1+k2`) must be bit-for-bit identical to `clif.cpp`. A unit test replays a captured packet stream and verifies the decoded packet IDs match expected values. |
| `0x0065` char-server connect is not in PacketDatabase | The initial char-server auth packet is sent with raw byte offsets, no struct. It is hardcoded in `pkg/session/char.go:SendConnect()` — not generated. |
| Doubly-nested variable-length quests | `packet_quest_list_header` + `packet_quest_list_info[]` + `packet_mission_info_sub[]` cannot be decoded with a flat struct. The generated decode fn for `quest_list` calls a handwritten two-pass parser in `pkg/decode/quest_list_parse.go`. |
| `CHARACTER_INFO` HP/SP/EXP type changes | `int32` → `int64` at PACKETVER breakpoints. Handled by packetver branching in the generated decode fn, same as any other versioned field. |
| `0x0b1d` map-server ping requires immediate reply | If goKore fails to register `OnPingLive` or fails to send the reply within the server timeout, the connection is dropped. This is documented in §16; it is a goKore responsibility, not a library bug. |
| `login_id2` not in `0x0436` | The client sends only `login_id1` in the map-connect packet. `login_id2` reaches the map server via the char server's internal auth channel (`0x2afd`). The map server then validates `login_id1` from the client against what the char server provided. No action required from the library — this is transparent. |
| Events with `[]T` fields allocate | `make([]T, n)` inside decode functions for variable-count arrays. The zero-alloc benchmark target applies only to fixed-size events; slice-bearing events are benchmarked separately with explicit alloc counts documented. Known instances: `PetEggList_0x01A6` (`InventoryIndices []int16`) and `ZcSkillSelectRequest_0x0442` (`SkillIds []int16`) each produce one heap allocation per call — confirmed by `go build -gcflags="-m=2"`. These are accepted as the packet type inherently carries a variable-length list. |
| FSM `Connect` is synchronous and blocking | goKore must call it in a goroutine (`go fsm.Connect(ctx)`). If called on the main goroutine it will block until `OnReady` or `OnFailed`. This is documented and verified by the integration test, which always calls it in a goroutine. |
| FSM does not auto-reconnect | By design. goKore calls `Connect` again when it decides reconnection is appropriate (respecting rate limits, max-attempts config, etc.). The FSM has no opinion on retry policy. |
| Charlist pagination depends on PACKETVER | `PACKETVER >= 20130000`: server sends `0x082D`, then `0x006B`, then `0x09A0` (with `p.total >= 1`; never 0 in rAthena). FSM sends `0x09A1` × `p.total`, waits for all `0x099D` pages. `PACKETVER < 20130000`: server sends only `0x006B` — no `0x09A0`, no `0x099D`. FSM proceeds immediately on `0x006B`. The test suite covers both paths. |
| `0x0283` timing | rAthena sends `0x0283` in `clif_parse_WantToConnection`, immediately on receipt of `0x0436` — before the char-server auth round-trip completes. `ZC_ACCEPT_ENTER` arrives much later (after `pc_authok`). The FSM must be prepared to receive `0x0283` in the gap before any other map-server packet. |
| Multiple char server entries in 0x0069/0x0AC4 | Private rAthena always advertises one char server. Public servers may advertise several. The `OnCharServerList` callback handles this; the default (index 0) is correct for single-server setups. |
| `AID` field absent pre-20131223 in actor structs | `packet_idle_unit`, `packet_spawn_unit`, `packet_unit_walking` have no `AID` field before PACKETVER 20131223. The canonical `ActorExists.ID` field must be set to 0 for those older packet versions. The codegen must handle this: pre-20131223 `ID` is sourced from `GID` (the only identifier in old packets), not from an absent `AID` field. |
| `effectState` type changes | `packet_idle_unit` and `packet_spawn_unit`: `int16` pre-20080102, `int32` post. `packet_unit_walking`: `int16` for PACKETVER < 7 (pre-2006), `int32` otherwise. Both breakpoints must appear in the generated decode functions. |
| `GID` → `CharID` naming is a misnomer for non-player entities | `GID` is `char_id` for PCs but a general unit GID for monsters/NPCs. The canonical name `CharID` is kept for OpenKore compatibility but the comment in `events.ActorExists` must note that it is 0 for monsters and NPCs. |
| 225 PACKETVER breakpoints, plus PACKETVER_MAIN_NUM/RE_NUM/ZERO_NUM second dimension | The union of breakpoints in `packets_struct.hpp` (212) and `packets.hpp` (31) is 223 unique values (corrected from earlier prose claiming 225). Additionally, some conditions use `PACKETVER_MAIN_NUM`, `PACKETVER_RE_NUM`, and `PACKETVER_ZERO_NUM` — three independent build-flavor axes that cannot be expressed as a single `PACKETVER` date. The codegen must handle these as separate preprocessing passes. |

---

## 17. Keepalive / Ping Mechanisms

rAthena uses different keepalive mechanisms for each server type. The library must
expose send types for each; goKore is responsible for scheduling them.

### Login server keepalive

- **Packet:** `0x0200` `PACKET_CA_CONNECT_INFO_CHANGED` — C→S, 26 bytes
- **Fields:** `int16 packetType`, `char name[24]` (name is ignored by server)
- **Schedule:** goKore sends this on a timer. The login server handler does nothing
  except acknowledge receipt (returns `true`). It is a repurposed heartbeat.
- **Library type:** `send.LoginKeepalive{Name string}` (name unused but required for
  correct wire length)

### Char server keepalive

- **Packet:** `0x0187` `PACKET_PING` — C→S, 6 bytes
- **Fields:** `int16 packetType`, `uint32 AID`
- **Schedule:** goKore sends this periodically with the account ID. The char server
  validates `p->AID == sd.account_id` and does nothing else.
- **Library type:** `send.CharKeepalive{AccountID uint32}`

### Map server ping (server-initiated)

Two mechanisms exist depending on PACKETVER:

**Modern (PACKETVER ≥ 20190213):**
- Server sends `0x0b1d` `PACKET_ZC_PING_LIVE` — S→C, 2 bytes (type only)
- Client must respond with `0x0b1c` `PACKET_CZ_PING_LIVE` — C→S, 2 bytes (type only)
- Failure to respond causes the map server to disconnect the client

**Legacy (all PACKETVERs):**
- Server may send `0x0187` `PACKET_PING` — S→C, 6 bytes
- This is one-way; no reply expected

**Library handling:**
- `events.PingLive{}` fired when `0x0b1d` is received (modern map server ping)
- `send.PingLiveResponse{}` for `0x0b1c` — goKore sends this in the `OnPingLive`
  callback
- `events.Ping{AID uint32}` fired when `0x0187` is received on map session (legacy)
  — goKore may ignore this; no reply needed

**goKore responsibility:** goKore must register `OnPingLive` and immediately call
`session.Encode(send.PingLiveResponse{})` + `conn.Write(...)`. The library does not
auto-reply — auto-reply would require the library to call `conn.Write`, which
violates the "library never owns the socket" invariant.

---

## 18. Authentication Token Flow

This section documents exactly which fields flow between session types during
authentication, and who is responsible for each forwarding step.

**When using `ConnectionFSM` (§4)**: the FSM handles all token forwarding
internally. goKore sees none of this; it just receives a ready `*MapSession`
via `OnReady`.

**When constructing sessions directly** (testing or non-standard flows): goKore
must forward the tokens as described below.

### Phase 1: Login server → goKore

`events.LoginAccepted` contains:

```go
type LoginAccepted struct {
    AccountID   uint32   // AID — used in all subsequent sessions
    LoginID1    uint32   // primary session token — forwarded to char and map servers
    LoginID2    uint32   // secondary session token — forwarded to char server
    Sex         uint8    // 0=female, 1=male
    Token       string   // WEB_AUTH_TOKEN (17 bytes, PACKETVER >= 20170315 only; "" otherwise).
                         // WEB_AUTH_TOKEN_LENGTH = 16+1 = 17: 16 random bytes + 1 null terminator.
                         // Source: common/mmo.hpp:120: #define WEB_AUTH_TOKEN_LENGTH 16+1
    CharServers []CharServerInfo
}

// CharServerInfo is the entry passed to OnCharServerList.
type CharServerInfo struct {
    Name  string // 20 bytes, null-terminated
    Addr  string // "host:port", converted from wire IP+port via Addr()
    Users uint16
    Type  uint16
    New   uint16
}

// Internal decode helper for building Addr() from the raw wire fields:
// IP is written htonl(host_ip) on the wire; byte order reconstruction:
//   net.IP{byte(rawIP>>24), byte(rawIP>>16), byte(rawIP>>8), byte(rawIP)}
```

After `OnLoginAccepted` fires, the FSM calls `OnCharServerList(charServers)` to
obtain the index, then dials `charServers[index].Addr`.

### Phase 2: goKore → char server (raw send, no struct)

The initial char-server packet (`0x0065`) is **not** in the PacketDatabase. It is
17 bytes, sent immediately after TCP connect:

```
uint16 0x0065
uint32 AccountID    (from LoginAccepted.AccountID)
uint32 LoginID1     (from LoginAccepted.LoginID1)
uint32 LoginID2     (from LoginAccepted.LoginID2)
uint16 ClientType   (0 — unused by server)
uint8  Sex          (from LoginAccepted.Sex)
```

**Library exposure:** `CharSession.SendConnect(req send.CharConnect) [17]byte` where
`CharConnect` carries the four fields above. goKore calls this immediately after
creating the `CharSession` and writes the result to its socket.

### Phase 3: Char server → goKore

The char server first sends `PACKET_HC_ACCEPT_ENTER2` (`0x082d`) with slot counts,
then `PACKET_HC_ACCEPT_ENTER` (`0x006b` / `0x099d`) with the character list.
These are separate events:

```go
type CharSlotInfo struct {
    Normal     uint8
    Premium    uint8
    Billing    uint8
    Producible uint8
    Total      uint8
}

type CharListReceived struct {
    Characters []CharacterInfo
}

type CharacterInfo struct {
    CharID     uint32
    Name       string  // 24 bytes (NAME_LENGTH)
    Class      uint16
    BaseLevel  uint16
    // ... full CHARACTER_INFO fields
    BaseExp    int64   // int32 for PACKETVER < 20170830, int64 after
    JobExp     int64   // int32 for PACKETVER < 20170830, int64 after
    HP, MaxHP  int64   // int32 for PACKETVER_RE_NUM < 20211103 AND PACKETVER_MAIN_NUM < 20220330
    SP, MaxSP  int64   // int16 for PACKETVER_RE_NUM < 20211103 AND PACKETVER_MAIN_NUM < 20220330
                       // (pre-modern SP is int16, not int32 — source: common/packets.hpp:53–57)
}
```

### Phase 4: goKore selects a character

goKore sends `0x0066` `PACKET_CH_SELECT_CHAR` with the chosen slot:

```go
type SelectChar struct {
    Slot uint8
}
```

### Phase 5: Char server → goKore (map server address)

`events.MapServerInfo` contains:

```go
type MapServerInfo struct {
    CharID  uint32
    MapName string  // 16 bytes (MAP_NAME_LENGTH_EXT), e.g. "prontera.gat"
    IP      uint32
    Port    uint16
    Domain  string  // 128 bytes, PACKETVER >= 20170315 only; "" otherwise.
                    // NOTE: current rAthena always sets domain to "" even for
                    // PACKETVER >= 20170315 (source: char_clif.cpp:913:
                    // safestrncpy(p.domain, "", sizeof(p.domain))).
                    // Addr() falls back to IP when Domain is "".
}

// Addr returns a "host:port" string suitable for net.Dial.
// If Domain is non-empty (PACKETVER >= 20170315) it is used instead of IP.
func (m MapServerInfo) Addr() string {
    if m.Domain != "" {
        return net.JoinHostPort(m.Domain, strconv.Itoa(int(m.Port)))
    }
    ip := net.IP{byte(m.IP >> 24), byte(m.IP >> 16), byte(m.IP >> 8), byte(m.IP)}
    return net.JoinHostPort(ip.String(), strconv.Itoa(int(m.Port)))
}
```

goKore opens a TCP connection to `m.Addr()` — using the helper ensures correct IP
byte order and domain precedence.

### Phase 6: goKore → map server (raw send, no struct)

**Source:** `src/map/clif.cpp:10640` — `0x0436` (`CZ_ENTER2`) is parsed by
`clif_parse_WantToConnection`. `src/map/clif_packetdb.hpp:1148` defines its layout:

```
parseable_packet(0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18)
//               id     len  handler                      pos[0..4]
// pos[0]=2  account_id  (L, 4 bytes)
// pos[1]=6  char_id     (L, 4 bytes)
// pos[2]=10 login_id1   (L, 4 bytes) — "auth code" in rAthena comment
// pos[3]=14 client_tick (L, 4 bytes)
// pos[4]=18 sex         (B, 1 byte)
// total: 2 + 4+4+4+4+1 = 19 bytes
```

Wire layout:

```
uint16 0x0436
uint32 AccountID    (from LoginAccepted.AccountID)
uint32 CharID       (from MapServerInfo.CharID)
uint32 LoginID1     (from LoginAccepted.LoginID1)
uint32 ClientTick   (current tick; 0 is acceptable)
uint8  Sex          (from LoginAccepted.Sex)
```

Total: 19 bytes.

**`login_id2` and map server auth**: The client does NOT include `LoginID2` in
`0x0436`. However, `login_id2` is not silently ignored — the char server forwards
it to the map server via the internal `0x2afd` channel (`chrif_authok`), where it
is passed to `pc_authok`. The map server cross-validates `login_id1` from the client
packet (`node->login_id1 == login_id1` at `chrif.cpp:700`) against what the char
server provided. `login_id2` flows char→map internally; the client never needs to
send it to the map server directly.

**Library exposure:** `MapSession.SendConnect(req send.MapConnect) [19]byte`

---

## 19. Variable-Length and Nested Struct Framing

The `Feed()` framing loop (§8 `pkg/session`) already handles variable-length packets
correctly: when `lengths[packetID] == -1` it reads bytes `[2:4)` as `int16` for the
total frame length. This section documents the additional complexity of nested struct
arrays inside those variable-length packets.

### Category A: Flat variable-length (trailing `T[]`)

The packet body contains a fixed header followed by a homogeneous array of
fixed-size sub-structs. The element count is `(totalLength - headerSize) / elementSize`.

Examples:
- `0x0069` / `0x0ac4` `AC_ACCEPT_LOGIN` → `PACKET_AC_ACCEPT_LOGIN_sub char_servers[]`
  - PACKETVER < 20170315: sub-struct is 32 bytes (`uint32 ip` + `uint16 port` + `char name[20]` + `uint16 users` + `uint16 type` + `uint16 new_`). Source: `common/packets.hpp:200–207`.
  - PACKETVER >= 20170315: sub-struct is 160 bytes (same fields + `uint8 unknown[128]`). Source: `common/packets.hpp:176–184`.
  - Token field: `0x0AC4` variant (PACKETVER >= 20170315) includes `char token[WEB_AUTH_TOKEN_LENGTH]` (17 bytes) in the outer struct before `char_servers[]`. `0x0069` variant does not.
- `0x006b` `HC_ACCEPT_ENTER` → `CHARACTER_INFO characters[]`
- `0x099d` / `0x0b72` `HC_ACK_CHARINFO_PER_PAGE` → `CHARACTER_INFO characters[]`

Generated decode functions handle this with a simple loop:

```go
n := (totalLen - headerSize) / elementSize
e.Characters = make([]events.CharacterInfo, n)
for i := range e.Characters {
    decodeCharacterInfo(&e.Characters[i], data[off:], packetver)
    off += elementSize(packetver)  // elementSize may vary by packetver
}
```

**Allocation note:** `make([]T, n)` in the decode function does allocate. This is
unavoidable for variable-count arrays. The zero-alloc invariant applies only to
fixed-size structs; events containing slices are explicitly excluded from that
benchmark target.

### Category B: Doubly-nested variable-length

The packet body contains entries that are themselves variable-length. The only known
example is the quest list:

- `packet_quest_list_header` → `packet_quest_list_info[]` where each entry contains
  `packet_mission_info_sub objectives[]`

Each `packet_quest_list_info` entry has a variable number of objectives, so the entry
size is not constant. **The codegen cannot handle this case automatically.** The
generated decode function for `quest_list` delegates to a handwritten two-pass parser
in `pkg/decode/quest_list_parse.go` (handwritten, not generated, clearly annotated).

### Category C: Fixed-size nested arrays

Some packets use `T list[MAX_ITEMLIST]` with a compile-time constant size. These are
not variable-length at the framing level — the packet has a fixed total length. The
generated decode function reads exactly `MAX_ITEMLIST` elements.

Examples:
- `packet_itemlist_normal` → `NORMALITEM_INFO list[MAX_ITEMLIST]`
- `packet_itemlist_equip` → `EQUIPITEM_INFO list[MAX_ITEMLIST]`

### Sub-struct element size and PACKETVER

Some sub-structs are themselves version-conditional. For example, `CHARACTER_INFO`
switches SP/MaxSP from `int16` to `int64` and HP/MaxHP from `int32` to `int64`
at `PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330`. The element
size function is generated alongside the decode loop:

```go
// GENERATED: elementSize is a private function within each decode file.
// Signature form: func characterInfoSize(packetver uint32) int
func characterInfoSize(packetver uint32) int {
    if isRE(packetver) && packetver >= 20211103 || isMain(packetver) && packetver >= 20220330 {
        return /* sizeof CHARACTER_INFO with int64 hp/sp/maxhp/maxsp */ 156
    }
    return /* sizeof CHARACTER_INFO with int32/int16 fields */ 144
}
```

The exact constant sizes are verified by running the GCC preprocessor and summing
field sizes (use `validation/length_check.sh`).

### `CHARACTER_INFO` struct (key nested struct)

`CHARACTER_INFO` is defined in `src/common/packets.hpp` lines 31–105. It is used in
four char-server packets. Key version-conditional fields:

| Field | Condition | Type | Notes |
|-------|-----------|------|-------|
| `exp` | PACKETVER < 20170830 | `int32` | Source: `common/packets.hpp:33–37` |
| `exp` | PACKETVER >= 20170830 | `int64` | |
| `jobexp` | PACKETVER < 20170830 | `int32` | |
| `jobexp` | PACKETVER >= 20170830 | `int64` | |
| `hp` | PACKETVER_RE_NUM < 20211103 AND PACKETVER_MAIN_NUM < 20220330 | `int32` | Source: `common/packets.hpp:51–59` |
| `hp` | PACKETVER_RE_NUM >= 20211103 OR PACKETVER_MAIN_NUM >= 20220330 | `int64` | |
| `maxhp` | same condition as `hp` | `int32` / `int64` | |
| `sp` | PACKETVER_RE_NUM < 20211103 AND PACKETVER_MAIN_NUM < 20220330 | **`int16`** (NOT `int32`) | |
| `sp` | PACKETVER_RE_NUM >= 20211103 OR PACKETVER_MAIN_NUM >= 20220330 | `int64` | |
| `maxsp` | same condition as `sp` | **`int16`** / `int64` | |

**Critical correction (M7):** The earlier HLD prose stated "int32 → int64 at PACKETVER 20170830"
for HP/SP. This is WRONG:
- `exp`/`jobexp` breakpoint is `PACKETVER >= 20170830` (a plain PACKETVER date).
- `hp`/`maxhp` breakpoint is `PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330` (RE/MAIN axes).
- Pre-modern `sp`/`maxsp` are `int16` — not `int32` — a distinct bug.

The Go event struct uses `int64` for all; decode functions sign-extend from the
narrower types when reading older packets.

---

## 20. Error and Refusal Packets

These packets are required for correct session lifecycle management. goKore must
handle them to know when a connection has been definitively rejected vs. when it
should reconnect.

### Login server errors

| Event | Packet ID | Key fields | Notes |
|-------|-----------|-----------|-------|
| `LoginRefused` | `0x006a` (PACKETVER < 20120000) / `0x083e` (≥ 20120000) | `ErrorCode uint8` (< 20120000) or `ErrorCode uint32` (≥ 20120000), `UnblockTime string` | Source: `common/packets.hpp:224–238`. For PACKETVER >= 20120000, `error` field is `uint32` — the Go event uses `uint32`; pre-20120000 values are zero-extended. |
| `ServerNotify` | `0x0081` | `Result uint8` | Generic disconnect (also used by char+map) |

`LoginRefused.ErrorCode` values (validated from `loginclif.cpp:185–207`):
- 0 = Unregistered ID
- 1 = Incorrect password
- 2 = Account expired
- 3 = Rejected from server
- 4 = Blocked by GM
- 5 = EXE file not latest version
- 6 = Prohibited until `UnblockTime`
- 7 = Server overpopulated
- 99 = ID erased
- 100 = Login info remains at `UnblockTime`
- 101–104 = Investigation / bug lock / deletion in progress

### Char server errors

| Event | Packet ID | Key fields | Notes |
|-------|-----------|-----------|-------|
| `CharEnterRefused` | `0x006c` | `ErrorCode uint8` | Always 0 in current rAthena |
| `CharMakeRefused` | `0x006e` | `ErrorCode uint8` | 0=name taken, 1=over limit |
| `CharDeleteRefused` | `0x0070` | `ErrorCode uint8` | 0=wrong email/birthdate |

### Map server errors

| Event | Packet ID | Key fields | Notes |
|-------|-----------|-----------|-------|
| `MapEnterRefused` | `0x0074` | `ErrorCode uint8` | Client type/ID mismatch |
| `ServerNotify` | `0x0081` | `Result uint8` | Forced disconnect |

`ServerNotify.Result` values when sent by map server (`clif.cpp:794–836`):
- 0 = Unfair disconnection
- 1 = Server closed
- 2 = ID already logged in
- 3 = Timeout / too much lag
- 4 = Server full
- 5 = Underaged
- 8 = Server still recognizes last connection
- 9 = Too many connections from this IP
- 10 = Out of paid time
- 15 = Disconnected by GM

**Note on 0x0081 disambiguation on CharSession (M1):** `PACKET_SC_NOTIFY_BAN`
(`SC_NOTIFY_BAN`, 4 bytes) and `PACKET_HC_NOTIFY_ZONESVR` (`HC_NOTIFY_ZONESVR`,
≥ 28 bytes for PACKETVER < 20170315) both use packet ID `0x0081` (source:
`common/packets.hpp:308,315`). The CharSession length table must register `0x0081`
with length `-1` (variable), then the FSM distinguishes them by the length field
(bytes 2:4 of the frame): SC_NOTIFY_BAN is always exactly 4 bytes (`int16 packetType`
+ `uint8 result` + padding? — actually SC_NOTIFY_BAN is 4 bytes total with just
packetType+result); HC_NOTIFY_ZONESVR is 28 bytes (packetType+CID+mapname+ip+port).
The FSM checks frame length to decide which event to fire.

**Note:** `0x0081` `PACKET_SC_NOTIFY_BAN` is the same packet ID across login, char,
and map servers. The `ServerNotify` event is defined once in `pkg/events` and
registered in all three session types. The error code interpretation differs by
server context — goKore is responsible for distinguishing based on which session
received it.

**Design implication for goKore:** When `events.ServerNotify` fires during map
session, goKore must stop the read loop, close `net.Conn`, and decide whether to
reconnect based on `Result`. The library has no opinion on reconnect policy.

---

## 21. Package Statefulness

This section explicitly classifies every package as stateful or stateless. This
distinction matters for testing, concurrency reasoning, and understanding which
components can be shared across goroutines vs. which must be per-connection.

### Stateless packages (pure functions, no internal state)

| Package | What it contains |
|---------|-----------------|
| `pkg/packing` | `DecodePosDir`, `EncodePosDir`, `DecodeMoveData`, `EncodeMoveData` — pure byte transformations, no state |
| `pkg/decode` | GENERATED decode functions — pure `([]byte, uint32) → events.T` transforms |
| `pkg/encode` | GENERATED encode functions — pure `(send.T, uint32) → [N]byte` transforms |
| `pkg/events` | Plain struct type definitions only — no state |
| `pkg/send` | Plain struct type definitions only — no state |

All functions in these packages are safe to call concurrently from any goroutine.
They hold no mutable state between calls.

### Stateful packages (per-connection state, not goroutine-safe)

| Package | State it holds |
|---------|---------------|
| `pkg/session` | `recvBuf []byte` (reassembly buffer), `handlers [65536]HandlerFunc` (registered callbacks), `lengths [65536]int16` (packet length table), `packetver uint32`. `MapSession` additionally holds `obfuscationState` (send-side rolling LCG key for C→S packet ID obfuscation). |
| `pkg/fsm` | `ServerConfig` (LoginAddr, Packetver, StepTimeout), `Credentials` (Username, Password, CharSlot), current `FSMState`, accumulated token fields (`AccountID`, `LoginID1`, `LoginID2`, `Sex`, `CharID`), registered callbacks. |

**Consequence**: Each TCP connection requires its own `LoginSession`, `CharSession`,
or `MapSession` instance. At 1000 bots that means 1000 `MapSession` instances, each
with its own independent state. Sessions must not be shared between goroutines.

**`pkg/fsm` thread safety**: The FSM runs entirely in the goroutine that called
`Connect`. Its callbacks fire synchronously in that same goroutine. There is no
concurrent access to FSM state.

**`pkg/session` thread safety**: Sessions are not goroutine-safe. `Feed()` must be
called from exactly one goroutine — the one that owns the TCP read loop for that
connection. `Encode()` must also be called from the same goroutine, or the caller
must provide external synchronization. (goKore typically sends from the same
goroutine or a single writer goroutine per connection.)

---

## 22. Field Name Reference for `semantics/mappings.yaml`

This section documents the exact rAthena struct field names for Phase 1 packets,
validated against `packets_struct.hpp` and `packets.hpp`. The `semantics/mappings.yaml`
author must use these exact names as `rathena_field` values.

### Actor visibility structs (`packet_idle_unit`, `packet_spawn_unit`, `packet_unit_walking`)

| Canonical name | rAthena field | Type | Notes |
|---|---|---|---|
| `ID` | `AID` | `uint32` | **Only present for PACKETVER >= 20131223.** Pre-20131223: field absent; set `ID = 0` in the decoded event for old packets. |
| `CharID` | `GID` | `uint32` | Present unconditionally. For monsters/NPCs this is a generic unit GID, not a character ID — the name `CharID` is kept for OpenKore compatibility; document as "0 for monsters" in the event struct. |
| `ObjectType` | `objecttype` | `uint8` | Present in `idle_unit` from PACKETVER ≥ 20091103, in `unit_walking` from ≥ 20071106. Absent in `idle_unit2`/`spawn_unit2` for PACKETVER < 20071106. |
| `Speed` | `speed` | `int16` | Unconditional in all variants. |
| `BodyState` | `bodyState` | `int16` | Unconditional. |
| `HealthState` | `healthState` | `int16` | Unconditional. |
| `EffectState` | `effectState` | `int16` (< 20080102) / `int32` (≥ 20080102) | In `unit_walking`: `int16` for PACKETVER < 7 (pre-2006), `int32` otherwise. Breakpoint is version integer 7, not a date. |
| `X, Y, Dir` | `PosDir[3]` | `uint8[3]` | Decoded via `DecodePosDir`. Present in `idle_unit`, `spawn_unit`, `idle_unit2`, `spawn_unit2`. |
| `FromX, FromY, ToX, ToY, Sx0, Sy0` | `MoveData[6]` | `uint8[6]` | Decoded via `DecodeMoveData`. Present only in `unit_walking`. |

### Map entry struct (`packet_authok` / `PACKET_ZC_ACCEPT_ENTER`)

Source: `packets_struct.hpp:509–522` and `packets.hpp:545–574`.

| Canonical name | rAthena field | Type | Condition |
|---|---|---|---|
| `StartTime` | `startTime` | `uint32` | Unconditional |
| `X, Y, Dir` | `PosDir[3]` | `uint8[3]` | Unconditional; decoded via `DecodePosDir` |
| `XSize` | `xSize` | `uint8` | Unconditional; always 5, ignored |
| `YSize` | `ySize` | `uint8` | Unconditional; always 5, ignored |
| `Font` | `font` | `int16` | PACKETVER ≥ 20080102 |
| `Sex` | `sex` | `uint8` | PACKETVER ≥ 20141022 AND < 20160330 only |

### Stat update structs (0x00B0, 0x00B1)

Source: `packets_struct.hpp:354–366`.

| Packet | Struct name | Fields | Notes |
|---|---|---|---|
| `0x00B0` | `PACKET_ZC_PAR_CHANGE` | `PacketType int16`, `varID uint16`, `count int32` | 8 bytes, no PACKETVER conditions |
| `0x00B1` | `PACKET_ZC_LONGPAR_CHANGE` | `PacketType int16`, `varID uint16`, `amount int32` | 8 bytes, no PACKETVER conditions; note macro difference: `DEFINE_PACKET_ID` vs `DEFINE_PACKET_HEADER` |

### Actor vanished struct (0x0080)

Source: `packets.hpp:596–601`.

| Struct | Fields | Notes |
|---|---|---|
| `PACKET_ZC_NOTIFY_VANISH` | `packetType int16`, `gid uint32`, `type uint8` | 7 bytes, no PACKETVER conditions. Note lowercase field names (newer struct style vs. `packets_struct.hpp`). |
