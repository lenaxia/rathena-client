# EPIC-06: pkg/sequence — Mandatory Protocol Response Helpers

**Status**: Ready for implementation
**Created**: 2026-03-12
**Goal**: Introduce `pkg/sequence` — a zero-alloc, zero-goroutine package of pure
functions that return the wire bytes for each mandatory server→client response
sequence, and refactor `pkg/fsm` to use them so there is one implementation shared
by both the FSM auth path and goKore's steady-state handlers.

---

## Context

Several rAthena server→client packets require the client to send one or more
client→server packets as an unconditional protocol obligation — not optional game
logic. The rAthena server will not proceed (or will disconnect the client) if the
response is absent.

Three such sequences recur in steady-state gameplay (i.e. after `OnReady` hands
the connection to goKore):

| Trigger (S→C) | ID | Required response(s) (C→S) | Recurrence |
|---|---|---|---|
| `ZC_ACCEPT_ENTER` / `ZC_ACCEPT_ENTER2` / `ZC_ACCEPT_ENTER3` | 0x0073 / 0x02EB / 0x0A18 | `CZ_NOTIFY_ACTORINIT` (0x007D) then `CZ_REQUEST_TIME` / `CZ_REQUEST_TIME2` (0x007E / 0x0360) | Every warp (same-server warp via `ZC_NPCACK_MAPMOVE` 0x0091) |
| `ZC_PING_LIVE` | 0x0B1D | `CZ_PING_LIVE` (0x0B1C) | Periodic keep-alive (pv ≥ 20190213) |
| `ZC_NOTIFY_ACTORINIT` | 0x0B1B | `CZ_BLOCKING_PLAY_CANCEL` (0x0447) | Every warp (pv ≥ 20190403) |

The `ZC_ACCEPT_ENTER` sequence is already implemented in `pkg/fsm` for the initial
login (`buildMapLoadedPacket` / `buildTickSyncPacket` in `pkg/fsm/packets.go`), but
as unexported FSM-internal functions. goKore would have to duplicate this logic for
the post-warp case. The other two sequences have no implementation anywhere in the
library.

**Sources (rAthena):**
- Sequence 1: `clif.cpp:10742` (`CZ_NOTIFY_ACTORINIT`), `clif.cpp:11196-11197`
  (`CZ_REQUEST_TIME` / `CZ_REQUEST_TIME2`); struct sources `packets_struct.hpp`
  (0x007D = 2 bytes, 0x007E/0x0360 = 6 bytes).
- Sequence 2: `clif.cpp:22469-22509` (`clif_ping()`); `packets_struct.hpp:4004-4008`
  (`CZ_PING_LIVE` / `ZC_PING_LIVE` both 2 bytes); `clif_packetdb.hpp:1945`
  (`parseable_packet(HEADER_CZ_PING_LIVE, ...)`).
- Sequence 3: `clif.cpp:19990-20014` (`clif_loadConfirm()`); `packets_struct.hpp:4032`
  (`ZC_NOTIFY_ACTORINIT` 0x0B1B); `clif_packetdb.hpp:1244`
  (`parseable_packet(0x0447, 2, clif_parse_blocking_playcancel, 0)`).

**Design constraints** (from README-LLM.md):
- Zero goroutines anywhere in `pkg/` — enforced by CI.
- Zero heap allocations in the decode/encode hot path — functions must return
  fixed-size `[N]byte` value types.
- `net.Conn` never touches library code — functions return bytes; goKore writes them.
- No external runtime dependencies.
- rAthena is the only source of truth for packet structure and field sizes.

---

## Pre-Implementation Gate (mandatory before any code)

For each packet involved, run the GCC preprocessor and verify struct sizes match
what is documented here. Update the SemanticDB via MCP if discrepancies are found.

```bash
# Verify CZ_NOTIFY_ACTORINIT (0x007D) = 2 bytes
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A5 "DEFINE_PACKET_HEADER.*CZ_NOTIFY_ACTORINIT"

# Verify CZ_REQUEST_TIME (0x007E) = 6 bytes
# Verify CZ_REQUEST_TIME2 (0x0360) = 6 bytes
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A5 "DEFINE_PACKET_HEADER.*CZ_REQUEST_TIME"

# Verify the packetver threshold for 0x007E → 0x0360 switchover
# Cross-check fsm/packets.go buildTickSyncPacket threshold (>= 20080102) against
# the actual #if PACKETVER condition in packets_struct.hpp or packets.hpp
g++ -E -P -DPACKETVER=20080101 -DPACKETVER_MAIN_NUM=20080101 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    -include ~/personal/rathena-client/internal/codegen/stubs/packets_hpp_stub.h \
    ~/personal/rathena/src/map/packets.hpp 2>/dev/null \
    | grep -B2 -A5 "0x0360\|CZ_REQUEST_TIME2"

# Verify CZ_PING_LIVE (0x0B1C) = 2 bytes, ZC_PING_LIVE (0x0B1D) = 2 bytes
g++ -E -P -DPACKETVER=20190213 -DPACKETVER_MAIN_NUM=20190213 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A5 "DEFINE_PACKET_HEADER.*CZ_PING_LIVE\|ZC_PING_LIVE"

# Verify CZ_BLOCKING_PLAY_CANCEL (0x0447) = 2 bytes
g++ -E -P -DPACKETVER=20190403 -DPACKETVER_MAIN_NUM=20190403 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A5 "0x0447\|CZ_BLOCKING_PLAY_CANCEL"
```

Document the GCC output in the worklog before writing any code.

---

## Story Map

```
US-20  pkg/sequence — core package (MapEntryResponse, PingResponse,
       BlockingPlayCancelResponse) + tests + benchmarks

US-21  Refactor pkg/fsm to use pkg/sequence
       (delete buildMapLoadedPacket / buildTickSyncPacket,
        wire onMapEnter to sequence.MapEntryResponse)
```

US-21 depends on US-20. The two stories can be reviewed independently but
US-20 must be merged and green before US-21 begins.

---

## US-20 — Implement pkg/sequence

### Problem

goKore needs to respond to three recurring mandatory server packets. There is no
library-provided implementation. Without it, goKore must either embed raw byte
magic or duplicate the FSM's `buildMapLoadedPacket`/`buildTickSyncPacket` logic,
which is unexported and therefore inaccessible.

### API

```go
// Package sequence provides pure functions that return the wire bytes for
// mandatory client→server responses to specific server→client packets.
//
// All functions return fixed-size byte arrays and perform zero heap allocations.
// The caller is responsible for applying C→S packet ID obfuscation (via
// MapSession.Encode) and writing the returned bytes to the network connection.
package sequence
```

#### MapEntryResponse

```go
// MapEntryResponse returns the two packets that the client must send after
// receiving ZC_ACCEPT_ENTER (0x0073), ZC_ACCEPT_ENTER2 (0x02EB), or
// ZC_ACCEPT_ENTER3 (0x0A18).
//
//   p1 — CZ_NOTIFY_ACTORINIT (0x007D): 2-byte map-loaded confirmation.
//        Source: clif.cpp:10742
//
//   p2 — CZ_REQUEST_TIME (0x007E, packetver < 20080102) or
//         CZ_REQUEST_TIME2 (0x0360, packetver >= 20080102): 6-byte tick sync.
//        clientTime should be the StartTime field from the decoded
//        ZcAcceptEnter event.
//        Source: clif.cpp:11196-11197
//
// The caller must apply MapSession.Encode to the packet ID bytes of each
// returned packet before writing to the connection.
func MapEntryResponse(packetver uint32, clientTime uint32) (p1 [2]byte, p2 [6]byte)
```

**Wire layout** (from rAthena structs, GCC-verified):

`p1` — `CZ_NOTIFY_ACTORINIT`:
```
[0:2]  packetType = 0x007D (LE)
```

`p2` — `CZ_REQUEST_TIME` (packetver < 20080102):
```
[0:2]  packetType = 0x007E (LE)
[2:6]  clientTime (uint32 LE)
```

`p2` — `CZ_REQUEST_TIME2` (packetver >= 20080102):
```
[0:2]  packetType = 0x0360 (LE)
[2:6]  clientTime (uint32 LE)
```

**Packetver threshold for 0x007E → 0x0360**: currently documented as >= 20080102
(from `pkg/fsm/packets.go:buildTickSyncPacket`). This must be verified against
the `#if PACKETVER` guard in rAthena source during the pre-implementation gate.
If the threshold differs, update both this document and the FSM (see US-21).

#### PingResponse

```go
// PingResponse returns the CZ_PING_LIVE (0x0B1C) pong packet that must be
// sent in response to ZC_PING_LIVE (0x0B1D).
//
// Only register a ZC_PING_LIVE handler and call this function when
// packetver >= 20190213.
// Source: clif.cpp:22469-22509; packets_struct.hpp:4004-4008
func PingResponse() [2]byte
```

**Wire layout:**
```
[0:2]  packetType = 0x0B1C (LE)
```

#### BlockingPlayCancelResponse

```go
// BlockingPlayCancelResponse returns the CZ_BLOCKING_PLAY_CANCEL (0x0447)
// packet that must be sent in response to ZC_NOTIFY_ACTORINIT (0x0B1B).
//
// Only register a ZC_NOTIFY_ACTORINIT (0x0B1B) handler and call this
// function when packetver >= 20190403.
// Source: clif.cpp:19990-20014; clif_packetdb.hpp:1244
func BlockingPlayCancelResponse() [2]byte
```

**Wire layout:**
```
[0:2]  packetType = 0x0447 (LE)
```

### File layout

```
pkg/sequence/
    sequence.go         all three functions
    sequence_test.go    all tests and benchmarks
```

### Tests (write first — TDD)

Golden bytes are derived from the rAthena struct layouts above, not from
intuition or the generated encode functions.

**Functional tests:**

| Test | Inputs | Assertions |
|---|---|---|
| `TestMapEntryResponse_ActorInitPacket` | any pv, any clientTime | `p1 == [0x7D, 0x00]` |
| `TestMapEntryResponse_TickID_Pre20080102` | pv=20070521, clientTime=0 | `p2[0:2] == [0x7E, 0x00]` |
| `TestMapEntryResponse_TickID_Post20080102` | pv=20080102, clientTime=0 | `p2[0:2] == [0x60, 0x03]` |
| `TestMapEntryResponse_ClientTimeEncoded` | pv=20180307, clientTime=0x12345678 | `p2[2:6] == [0x78, 0x56, 0x34, 0x12]` |
| `TestMapEntryResponse_TickBoundary` | pv=20080101 (one below threshold) | `p2[0:2] == [0x7E, 0x00]` |
| `TestPingResponse` | (none) | returns `[0x1C, 0x0B]` |
| `TestBlockingPlayCancelResponse` | (none) | returns `[0x47, 0x04]` |

**Benchmark tests** (0 allocs/op required):

```go
func BenchmarkMapEntryResponse(b *testing.B)         // 0 allocs/op
func BenchmarkPingResponse(b *testing.B)             // 0 allocs/op
func BenchmarkBlockingPlayCancelResponse(b *testing.B) // 0 allocs/op
```

### Implementation notes

- Use `encoding/binary` only; no external imports.
- `MapEntryResponse` must not call `make` — the two return values are
  `[2]byte` and `[6]byte`, both stack-allocated value types.
- `binary.LittleEndian.PutUint16` and `binary.LittleEndian.PutUint32` write
  into the fixed-size arrays directly.
- No `init()`, no package-level state.

### Acceptance criteria

- [ ] `pkg/sequence/sequence.go` compiles with `go build ./pkg/sequence/`
- [ ] All 7 functional tests pass
- [ ] `go test -bench=. -benchmem ./pkg/sequence/` — 0 allocs/op for all benchmarks
- [ ] `grep -r "^\s*go " pkg/sequence/` — empty output
- [ ] `go test ./...` — no regressions
- [ ] Pre-implementation gate (GCC verification) documented in worklog
- [ ] Worklog created in `docs/WORKLOG/` before task is marked complete

---

## US-21 — Refactor pkg/fsm to Use pkg/sequence

### Problem

`pkg/fsm/packets.go` contains `buildMapLoadedPacket()` and `buildTickSyncPacket()`
as unexported FSM-internal functions. Now that `pkg/sequence` provides the canonical
implementations, these are dead weight that must be maintained in sync with the
sequence package. The FSM should delegate to `pkg/sequence` instead.

### Changes

**Delete from `pkg/fsm/packets.go`:**
- `buildMapLoadedPacket()` (lines 101–105)
- `buildTickSyncPacket()` (lines 120–129)

**Update `pkg/fsm/fsm.go` `onMapEnter` handler (lines 679–697):**

Before:
```go
loadedPkt := buildMapLoadedPacket()
encodePacketID(mapSess, loadedPkt)
if err := writeDeadline(conn, loadedPkt, f.stepTimeout()); err != nil { ... }

_, tickPkt := buildTickSyncPacket(f.server.Packetver)
encodePacketID(mapSess, tickPkt)
if err := writeDeadline(conn, tickPkt, f.stepTimeout()); err != nil { ... }
```

After:
```go
p1, p2 := sequence.MapEntryResponse(f.server.Packetver, res.startTime)
encodePacketID(mapSess, p1[:])
if err := writeDeadline(conn, p1[:], f.stepTimeout()); err != nil { ... }
encodePacketID(mapSess, p2[:])
if err := writeDeadline(conn, p2[:], f.stepTimeout()); err != nil { ... }
```

Note: `res.startTime` is set at line 700 in the current code (after the sends).
The refactor must move the `res.startTime` extraction to before the
`sequence.MapEntryResponse` call, since `clientTime` is now a parameter.
`clientTime` at map entry is the `startTime` field from the `ZC_ACCEPT_ENTER`
payload — this is the correct value to echo back to the server as the initial
client tick.

**Update `pkg/fsm/packets_test.go`:** Remove tests for
`buildMapLoadedPacket` and `buildTickSyncPacket`. The equivalent coverage now
lives in `pkg/sequence/sequence_test.go`.

**Add import:** `"github.com/lenaxia/rathena-client/pkg/sequence"` to `pkg/fsm/fsm.go`.

### Obfuscation note

`encodePacketID` takes a `[]byte` and mutates `[0:2]` in-place. After the
refactor it receives `p1[:]` and `p2[:]` — slices of the fixed-size arrays.
This is valid Go: a slice of a local array is addressable and writable.
The local arrays `p1`, `p2` are stack-allocated and do not escape to the heap
because `encodePacketID` does not store the slice. Verify with
`go build -gcflags="-m" 2>&1 | grep "sequence\|p1\|p2"` — these must show
"does not escape".

### Acceptance criteria

- [ ] `buildMapLoadedPacket` and `buildTickSyncPacket` deleted from `pkg/fsm/packets.go`
- [ ] Their tests deleted from `pkg/fsm/packets_test.go`
- [ ] `pkg/fsm/fsm.go` imports `pkg/sequence` and uses `sequence.MapEntryResponse`
- [ ] `res.startTime` extraction moved before the `sequence.MapEntryResponse` call
- [ ] All existing FSM tests still pass: `go test ./pkg/fsm/` — all pass
- [ ] `go build -gcflags="-m" ./pkg/fsm/` — `p1`, `p2` do not escape to heap
- [ ] `go test ./...` — no regressions
- [ ] Worklog created in `docs/WORKLOG/` before task is marked complete

---

## Exit Criteria for EPIC-05

1. `pkg/sequence` package exists and compiles clean.
2. `go test ./pkg/sequence/` — all 7 functional tests pass.
3. `go test -bench=. -benchmem ./pkg/sequence/` — 0 allocs/op for all benchmarks.
4. `go test ./pkg/fsm/` — all existing FSM tests pass (no regression).
5. `go test ./...` — no regressions across the full repository.
6. `grep -r "^\s*go " pkg/sequence/` — empty (zero goroutines).
7. `buildMapLoadedPacket` and `buildTickSyncPacket` no longer exist in `pkg/fsm/`.
8. GCC verification of all 5 packet structs documented in worklog.
9. Worklogs written for both US-20 and US-21.

---

## What This Epic Does NOT Cover

- **`ZC_NOTIFY_TIME` (0x007F) handling** — this is the server's reply to
  `CZ_REQUEST_TIME`, not a mandatory client response. goKore may choose to use
  it to calibrate its tick offset, but that is bot logic, not a protocol obligation.
- **Periodic tick sync dispatch** — sending `CZ_REQUEST_TIME` on a timer in
  steady-state is goKore's responsibility. `pkg/sequence` provides `MapEntryResponse`
  for the single send at map entry; the periodic sends use `pkg/encode.EncodeRequestTime`
  directly.
- **Char-login sequences** (`HC_CHARLIST_NOTIFY` → `CH_CHARLIST_REQ`,
  `HC_SECOND_PASSWD_LOGIN` → PIN response) — these are one-time auth sequences
  already handled inside `pkg/fsm`. They do not recur in steady-state and do not
  belong in `pkg/sequence`.
- **Private-server sync-ex** — not part of the rAthena canonical protocol.
- **PIN code encode helpers** — PIN encoding (XOR with seed) belongs in goKore's
  char-login flow, which is already handled by the FSM for automated login. A
  manual PIN entry path is an application concern, not a protocol library concern.
