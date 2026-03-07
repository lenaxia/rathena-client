# rathena-client Documentation - Complete LLM Starting Point

**This is the ONLY document you need to read to start development on rathena-client.**

All essential information is consolidated here. The HLD is referenced for deep dives only.

---

## Project Overview

**rathena-client** is a standalone Go library (`github.com/lenaxia/ragnarok-go-client`, Go 1.24.0) that implements the Ragnarok Online wire protocol as spoken by rAthena login, char, and map servers.

It is **not** a game client, bot, or application. It is a **pure protocol library**:
- Receives raw TCP bytes → invokes typed, version-agnostic callbacks
- Accepts typed send-request structs → returns raw TCP bytes

Primary consumer: **goKore** (`~/personal/goKore`) — the bot framework that imports this library as its network layer.

**NOTE:** ALWAYS USE THE SEMANTICDB MCP SERVER (`gokore-semantics`) TO ACCESS THE SEMANTICDB. NEVER GREP OR DIRECTLY MODIFY `semantics/mappings.yaml`. IF YOU NEED SPECIFIC FUNCTIONALITY, SPIN UP A SUBTASK TO ADD IT TO THE MCP SERVER THEN NOTIFY THE USER.

### External References

```
RATHENA_ROOT = ~/personal/rathena
GOKORE_ROOT  = ~/personal/goKore
```

---

## CRITICAL RULES - READ BEFORE CODING

### 0. MANDATORY WORK LOGS (ALWAYS REQUIRED)

**EVERY task, story, or significant work session MUST create a work log before completion.**

**A task is NOT complete without a work log. No exceptions.**

- Create work log at end of EVERY task
- Create work log for delegated subtasks
- Create work log for package implementations
- Create work log for bug fixes
- Document what was done, test results, any issues
- Commit work log with code changes

**Format**: `NNNN_YYYY-MM-DD_description.md` in `docs/WORKLOG/`

Get the next sequence number:
```bash
cd docs/WORKLOG
NEXT=$(printf "%04d" $(($(ls -1 [0-9][0-9][0-9][0-9]_*.md 2>/dev/null | sed 's/_.*//' | sort -n | tail -1) + 1)))
```

### 1. Test-Driven Development (MANDATORY)

**Write tests BEFORE code, ALWAYS. No exceptions.**

```go
// 1. Write test FIRST (must fail initially)
func TestDecodePosDir_RoundTrip(t *testing.T) { /* test */ }

// 2. Then implement to make test pass
func DecodePosDir(data []byte) (x, y uint16, dir uint8) { /* implementation */ }
```

**Requirements:**
- Multiple happy path tests (3-5 scenarios)
- Multiple unhappy path tests (error cases, edge cases)
- All tests must pass before task is complete
- Fuzz tests for all bit-packing functions
- Benchmark tests verifying 0 allocs/op on all decode/encode functions

**At the end of EVERY task:**
```bash
go build ./...   # ALL packages must build
go test ./...    # ALL tests must pass
go test -bench=. -benchmem ./pkg/...  # benchmarks must meet targets
```

### 2. NEVER PERFORM DESTRUCTIVE GIT OPERATIONS (CRITICAL)

**Multiple agents may work in this repository simultaneously.**

**FORBIDDEN:**
- `git checkout .` — discards ALL uncommitted changes
- `git reset --hard` — destroys work from other agents
- `git clean -fd` — deletes untracked files indiscriminately

**REQUIRED:**
- Revert files ONE AT A TIME with explicit confirmation
- Always check `git status` before any revert
- Ask user for confirmation before reverting any file

### 3. Zero Goroutines in `pkg/` (HARD INVARIANT)

**No `go` statement may appear anywhere in any `pkg/` file. Ever.**

This is enforced by CI:
```bash
grep -r "^\s*go " pkg/   # must produce zero output
```

If you need concurrency, it belongs in the caller (goKore), not the library. The library is a pure synchronous transformation: bytes in, typed events out.

**Why**: At 1000 concurrent bots, goroutines spawned by the library multiply by 1000. The library's contract is zero internal goroutines — the caller decides concurrency.

### 4. Zero Heap Allocations in the Decode Hot Path (MANDATORY)

**`session.Feed()` must produce 0 allocs/op in steady state.**

Event structs are stack-allocated inside generated decode functions and passed by value to callbacks. The Go compiler must not escape them to the heap.

Enforced by:
```bash
go test -bench=. -benchmem ./pkg/...     # 0 allocs/op required
go build -gcflags="-m" 2>&1 | grep "does not escape"  # CI escape analysis
```

If you change a decode function and a benchmark suddenly shows allocs, the event struct is escaping — fix the generated code.

### 5. No External Runtime Dependencies (MANDATORY)

**`go.mod` must have zero `require` entries. Keep it that way.**

```
module github.com/lenaxia/ragnarok-go-client
go 1.24.0
// NO require block — zero external deps
```

This library must be embeddable with no transitive dependency surprises. Use only the Go standard library.

### 6. rAthena is the ONLY Source of Truth for Packet Structure (CRITICAL)

**rAthena is PRIMARY and ONLY source of truth for packet structure, field names, and field types.**

**rAthena is used for:**
- Packet struct definitions (`src/map/packets_struct.hpp`, `src/map/packets.hpp`, `src/common/packets.hpp`)
- Field names (`AID`, `GID`, `speed`, `objecttype` — exact C field names)
- Field types (`uint16`, `int32`, `uint8[3]`)
- Field order and sizes
- `#if PACKETVER` conditionals

**OpenKore (`~/personal/openkore`) is ONLY used for:**
- Semantic understanding of what a field means
- Bot behavior and AI logic (not relevant to this library)

**OpenKore is NEVER used for:**
- Packet structure
- Field names or types

**Example:**
```go
// In generated decode functions, use rAthena field names as comments:
e.ID = leU32(data, off)    // rAthena: AID
e.CharID = leU32(data, off) // rAthena: GID
e.Speed = leU16(data, off)  // rAthena: speed
```

### 7. HLD.md is the Design Authority (MANDATORY)

**`docs/DESIGN/HLD.md` is the authoritative design document for this library.**

Before implementing any package, read the relevant HLD section:
- §3 Architecture + data flows
- §4 ConnectionFSM public API + state machine
- §5 Three typed sessions (Login/Char/Map)
- §6 Code generation pipeline (GCC + semantics)
- §8 Performance contract
- §9 Package descriptions with code examples
- §13 Phase 1 implementation scope (what packets/actions to implement first)
- §14 Repository structure

**If you need to deviate from the HLD, update the HLD FIRST, then implement.**

### 8. Type Safety — No `interface{}` or Reflection in the Hot Path (MANDATORY)

**NEVER use `interface{}`, `any`, or reflection in any decode/encode path.**

```go
// FORBIDDEN in pkg/decode, pkg/encode, pkg/session:
func decode(pkt interface{}) { ... }

// REQUIRED: direct byte reads, typed returns
func ActorExists_0x09FF(data []byte, packetver uint32) events.ActorExists {
    var e events.ActorExists
    e.ID = leU32(data, 4)
    // ...
    return e  // by value, no heap escape
}
```

Reflection was a bug surface and performance cost in goKore v1. Direct byte reads with offset arithmetic are both simpler and faster.

### 9. Semantic DB via MCP Server Only (CRITICAL)

**ALWAYS use the `gokore-semantics` MCP server. NEVER edit `semantics/mappings.yaml` directly.**

The MCP server provides validation, error checking, and maintains database consistency. The YAML file is the persistence layer — edit it only through the MCP server.

```bash
# CORRECT: Use the gokore-semantics MCP server tools
# (semantics_add, semantics_update, semantics_add_field, etc.)

# WRONG: Direct file edit
vim semantics/mappings.yaml  # NEVER DO THIS
```

### 10. Ask Before Deciding (MANDATORY)

**Never assume architectural choices. State assumptions with confidence level (LOW/MEDIUM/HIGH) and ask.**

When uncertain about:
- Whether a packet ID should be added to Phase 1 or Phase 2
- Whether a field needs special decode handling
- Whether a struct layout matches the rAthena source
- Any deviation from the HLD

...cite specific evidence and ask.

### 11. No Comments in Code

Code should be self-documenting. Exception: **package-level doc comments** (the `// Package foo ...` comment) are required on every package. Function signatures with non-obvious parameters may have a brief doc comment.

Do NOT add inline comments like `// increment offset`, `// parse field`, etc. The code should read clearly without them.

---

## Architecture

### Package Map

```
rathena-client/
    semantics/
        mappings.yaml          human-maintained semantic layer (edit via MCP only)

    pkg/
        packing/               DONE — WBUFPOS/WBUFPOS2 bit-packing
        fsm/                   BUILD THIS — ConnectionFSM login sequencer
        events/                GENERATED — canonical S→C event structs
        send/                  GENERATED — canonical C→S request structs
        decode/                GENERATED — raw bytes → event structs
        encode/                GENERATED — request structs → raw bytes
        session/               HAND-WRITTEN + GENERATED — PACKETVER-aware tokenizer + dispatcher

    internal/
        codegen/               BUILD THIS — code generator (reads rAthena + mappings.yaml)
```

### Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        rathena-client                            │
│                                                                  │
│  pkg/fsm/          ConnectionFSM — login + reconnect sequencer   │
│  pkg/packing/      WBUFPOS / WBUFPOS2 encode+decode         DONE │
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
┌──────────────────────────────────────────────────────────────────┐
│                          goKore                                  │
│  internal/network/rathena/connector.go  thin glue layer          │
│  internal/network/connection/           owns net.Conn, Dialer    │
│  internal/network/handlers/             game-state handlers      │
└──────────────────────────────────────────────────────────────────┘
```

### Login / Reconnect Flow

```
goKore calls fsm.Connect(ctx)
  → FSM calls dialer(ctx, loginAddr) → net.Conn    [goKore-provided]
  → FSM creates LoginSession, feeds it until 0x0AC4/0x0069 received
  → extracts tokens, calls OnCharServerList (default: index 0), closes conn
  → FSM calls dialer(ctx, charAddr) → net.Conn
  → FSM creates CharSession, sends 0x0065, feeds it
  → receives char list (0x006B / 0x099D), calls OnCharList callback
  → sends 0x0066 with chosen slot
  → receives 0x0071 / 0x0AC5 with map addr, closes conn
  → FSM calls dialer(ctx, mapAddr) → net.Conn
  → FSM creates MapSession, sends 0x0436
  → receives 0x0073/0x0A18/0x02EB, sends 0x007D + 0x007E/0x0360
  → calls OnReady(mapSession, conn)   [goKore takes over the conn]
```

### Steady-State Gameplay Flow (goKore owns the loop)

```
TCP bytes arrive on net.Conn  (goKore read loop)
  → mapSession.Feed(buf[:n])                         [rathena-client]
  → frame boundary detection via lengths[65536]int16 [GENERATED]
  → handlers[packetID](data, packetver)               [GENERATED decode fn]
  → decode fn: direct byte reads, stack-allocated event struct
  → registered callback(events.ActorExists{...})      [goKore, inline]
  → Feed() returns to goKore read loop

goKore calls mapSession.Encode(send.RequestMove{X: 100, Y: 200})
  → encode.EncodeMove(req, packetver)   [GENERATED, returns [5]byte]
  → optionally XOR packet ID (C→S obfuscation, PACKETVER ≤ 20180307 only)
  → goKore calls conn.Write(bytes[:])  [goKore owns the socket]
```

---

## Implementation Phases

### Phase 0 — Complete (pkg/packing)

`pkg/packing/packing.go` is fully implemented:
- `DecodePosDir(data []byte) (x, y uint16, dir uint8)` — 3-byte RBUFPOS
- `EncodePosDir(x, y uint16, dir uint8) [3]byte`
- `DecodeMoveData(data []byte) (fromX, fromY, toX, toY uint16, sx0, sy0 uint8)` — 6-byte RBUFPOS2
- `EncodeMoveData(fromX, fromY, toX, toY uint16, sx0, sy0 uint8) [6]byte`

Still needed: `packing_test.go` (table-driven tests + fuzz tests).

### Phase 1 — Build internal/codegen

Build the code generator that reads rAthena C++ headers and `semantics/mappings.yaml` to produce Go source files. See "Code Generation Pipeline" section below.

**Deliverables:**
- `internal/codegen/preprocess/` — GCC runner + C parser + VersionTable differ
- `internal/codegen/semantics/` — mappings.yaml loader
- `internal/codegen/gen/` — Go source generators for decode, encode, events, session lengths
- `internal/codegen/stubs/` — mysql_stub.h, libconfig_stub.h

### Phase 2 — Generated packages (pkg/events, pkg/send, pkg/decode, pkg/encode, pkg/session/lengths_*)

Run the codegen against the rAthena source to produce:
- `pkg/events/*.go` — one file per canonical receive action
- `pkg/send/*.go` — one file per canonical send action
- `pkg/decode/*.go` — one file per canonical action, with per-packetver byte-reading logic
- `pkg/encode/*.go` — one file per send action, returns `[N]byte`
- `pkg/session/lengths_login.go`, `lengths_char.go`, `lengths_map.go`

Phase 1 (login → char → map connect + core actor visibility) targets these actions:
- `actor_exists` (0x0078, 0x01D8, 0x09FF)
- `actor_moved` (0x007B, 0x01DA, 0x022C, 0x09FD)
- `actor_connected` (0x007C, 0x01D9, 0x09FE)
- `actor_vanished` (0x0080)
- `stat_update` (0x00B0, 0x00B1, 0x00BE)
- `request_move` send (0x0085)
- All FSM packets (login/char/map auth sequence — see HLD §13)

Phase 2 adds the remaining ~400+ actions.

### Phase 3 — pkg/session (hand-written parts)

Implement the three session types:
- `pkg/session/login.go` — `LoginSession`
- `pkg/session/char.go` — `CharSession`
- `pkg/session/map.go` — `MapSession` (includes `EnableObfuscation`)
- `pkg/session/obfuscation.go` — LCG key state + XOR logic

The `Feed()` algorithm is specified in HLD §9. Key points:
- O(1) length lookup via `[65536]int16` array (generated)
- O(1) handler dispatch via `[65536]HandlerFunc` array
- Zero allocs in steady state (copy-to-front on the recvBuf)
- Not goroutine-safe by design (one goroutine per connection — caller's responsibility)

### Phase 4 — pkg/fsm

Implement `ConnectionFSM`. Full state machine, public API, and automatic protocol steps are specified in HLD §4. Key constraints:
- Zero goroutines inside the FSM
- Receives a `Dialer func(ctx context.Context, addr string) (net.Conn, error)` — never calls `net.Dial` directly
- Blocks in the caller's goroutine until `OnReady` or `OnFailed` fires
- After `OnReady` fires, releases all references to the `net.Conn`

Test with `net.Pipe` stubs — no real rAthena server needed for unit tests.

### Phase 5 — Integration with goKore

Replace goKore's `internal/network/` layer:

| Deleted from goKore | Replaced by |
|---|---|
| `internal/network/packets/generated/` (~4,634 files) | `pkg/decode/` + `pkg/events/` |
| `internal/network/packetver/` (`PacketVersionRegistry`) | `pkg/session/` |
| `internal/network/adapters/` (~500 files) | Deleted; not needed |
| `internal/network/receive/Receiver` (2-goroutine) | goKore read loop calls `mapSession.Feed()` directly |
| `internal/network/params/` | `pkg/events/` types |
| `internal/network/connection/fsm.go` | `pkg/fsm/ConnectionFSM` |

New goKore adapter: `internal/network/rathena/connector.go` (thin glue layer, see HLD §7).

---

## Code Generation Pipeline

The code generator (`internal/codegen`) is the most complex part of this project. It is a `go run`-only tool — not importable as a library.

### Inputs

1. **rAthena C++ headers** (from `RATHENA_ROOT`):
   - `src/map/packets_struct.hpp` — map server packet structs (~214 PACKETVER breakpoints)
   - `src/map/packets.hpp` — newer ZC_/CZ_ structs (~279 additional structs, requires stubs)
   - `src/common/packets.hpp` — login/char server structs (~66 structs, requires stubs)
   - `src/map/clif_packetdb.hpp` — base packet length/handler registration table
   - `src/map/clif_shuffle.hpp` — per-PACKETVER packet ID shuffling table
   - `src/map/clif_obfuscation.hpp` — PACKET_OBFUSCATION key table

2. **`semantics/mappings.yaml`** — human-maintained, accessed via MCP server only. Provides what the preprocessor cannot: semantic field names, canonical action groupings, decode hints for packed binary fields.

### Processing

```bash
# The codegen runs GCC at each of ~225 PACKETVER breakpoints:
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I RATHENA_ROOT/src -I RATHENA_ROOT/src/map -I RATHENA_ROOT/src/common \
    -include internal/codegen/stubs/mysql_stub.h \
    -include internal/codegen/stubs/libconfig_stub.h \
    RATHENA_ROOT/src/map/packets_struct.hpp
```

Three passes per breakpoint (MAIN, RE, ZERO build flavors) to handle `PACKETVER_RE_NUM` and `PACKETVER_ZERO_NUM` variants.

**Diffing adjacent outputs** produces a `VersionTable`: a map of struct name → list of (packetver_range, StructLayout) entries. This is the authoritative source for all field names, types, sizes, and PACKETVER conditionals.

### Combination

```
GCC preprocess at each breakpoint
    → StructDB: map[struct_name][]VersionedLayout
    ↓
semantics/mappings.yaml  (via MCP server — DO NOT EDIT DIRECTLY)
    → ActionDB: map[action_name]ActionDef (canonical names, decode hints)
    ↓
codegen joins StructDB + ActionDB
    → pkg/decode/*.go      (one file per action, inline packetver switches)
    → pkg/encode/*.go      (one file per send action)
    → pkg/events/*.go      (one file per canonical event type)
    → pkg/session/lengths_login.go
    → pkg/session/lengths_char.go
    → pkg/session/lengths_map.go
```

### Running the codegen

```bash
go run ./internal/codegen/main.go \
    --rathena RATHENA_ROOT \
    --semantics semantics/mappings.yaml \
    --out .
```

Generated files are committed to the repository (analogous to `.pb.go` files). Regeneration is triggered manually when rAthena is updated or when `semantics/mappings.yaml` changes.

### What semantics/mappings.yaml provides

The mappings.yaml (edit via MCP server only) provides:
- **Action groupings**: multiple packet IDs that all implement the same logical action (e.g., `actor_exists` covers 0x0078, 0x01D8, 0x09FF, and others)
- **Canonical field names**: `AID` → `ID`, `GID` → `CharID`, `speed` → `Speed`
- **Decode hints**: which fields use `DecodePosDir` (3-byte packed) vs `DecodeMoveData` (6-byte packed)
- **OpenKore compatibility names** (for goKore's handler layer)

It does NOT provide field types, sizes, or PACKETVER conditions — those come exclusively from the GCC preprocessor output.

---

## Non-Trivial Wire Formats

These are the two packed binary formats used throughout the protocol. They are fully implemented in `pkg/packing`. All generated decode functions call `packing.DecodePosDir` and `packing.DecodeMoveData` — never reimplement this logic inline.

### 3-byte packed position (WBUFPOS / PosDir[3])

```
Byte 0: [x9 x8 x7 x6 x5 x4 x3 x2]
Byte 1: [x1 x0 y9 y8 y7 y6 y5 y4]
Byte 2: [y3 y2 y1 y0 d3 d2 d1 d0]
```

- x: 10-bit map coordinate (bits 23:14)
- y: 10-bit map coordinate (bits 13:4)
- dir: 4-bit direction (bits 3:0), 0=N/1=NW/2=W/3=SW/4=S/5=SE/6=E/7=NE

Used in all `PosDir[3]` fields across all PACKETVER values.

### 6-byte packed movement (WBUFPOS2 / MoveData[6])

```
Bytes 0-4: fromX(10b) fromY(10b) toX(10b) toY(10b)  [packed]
Byte 5:    [sx0_3 sx0_2 sx0_1 sx0_0 sy0_3 sy0_2 sy0_1 sy0_0]
```

**CRITICAL**: Byte 5 is `sx0` (high nibble) and `sy0` (low nibble) — sub-cell interpolation offsets. It is **NOT a direction value**. There is no direction field in the 6-byte format.

This was a confirmed bug in goKore v1 (`handlers/actors/handler.go:88`: `direction = (data[5] & 0xF0) >> 4`). This library fixes it: `events.ActorMoved` has no `Dir` field.

`sx0`/`sy0` are cosmetic interpolation hints for the visual client. Bot code can ignore them.

---

## Performance Contract

These are hard requirements verified by benchmarks in CI, not aspirational targets.

### Benchmark Targets

| Benchmark | Target |
|---|---|
| `BenchmarkFeed_SmallFixedPacket` | 0 allocs/op, < 200 ns/op |
| `BenchmarkFeed_ActorExists_0x09FF` | 0 allocs/op, < 500 ns/op |
| `BenchmarkEncode_RequestMove` | 0 allocs/op, < 100 ns/op |
| `BenchmarkFeed_1000Sessions_Parallel` | linear scaling with goroutine count |

### CI Checks

```bash
# Zero goroutines in pkg/ — must produce empty output
grep -r "^\s*go " pkg/

# Zero allocs in benchmarks
go test -bench=. -benchmem ./pkg/...

# Event structs must not escape to heap
go build -gcflags="-m" 2>&1 | grep "does not escape"
```

### Handler Lookup: O(1) Array Index

Each session type uses a `[65536]HandlerFunc` array indexed by packet ID — no map, no hash. Single array dereference. Length table is a `[65536]int16` array — same O(1) lookup.

Memory cost: 1000 MapSession instances × 65536 entries × 8 bytes = ~500 MB for handler arrays. This is accepted for a dedicated bot machine. The public API is unchanged if this needs to be optimized later.

### `Feed()` is Not Goroutine-Safe — By Design

One session per goroutine. goKore's architecture already guarantees one read goroutine per TCP connection. Adding a mutex to `Feed()` would cost ~20 ns per call × all packets × 1000 bots for zero benefit.

---

## Testing Requirements

### pkg/packing

- Table-driven tests covering known bit patterns from rAthena `clif.cpp:173–249`
- Round-trip tests: `encode(decode(bytes)) == bytes`
- Fuzz tests:
  ```bash
  go test -fuzz=FuzzDecodePosDir ./pkg/packing/
  go test -fuzz=FuzzDecodeMoveData ./pkg/packing/
  ```

### pkg/fsm

- Unit tests using `net.Pipe` stubs — no real rAthena server needed
- Test every state transition (see HLD §4 state machine)
- Test PACKETVER-conditional paths (< 20130000 vs ≥ 20130000 char list flow)
- Test `OnFailed` paths (login refused, dial error, timeout)

### pkg/session

- Benchmark tests in `session_bench_test.go`
- Feed a pre-captured packet stream; verify all callbacks fire in correct order
- Verify 0 allocs/op after initial recvBuf warmup

### pkg/decode, pkg/encode (generated)

- Generated tests verify byte-level correctness against known rAthena packet captures
- Round-trip: `encode(decode(bytes)) ≈ bytes` (for fields that survive round-trip)

### Full Repository Validation (MANDATORY after every task)

```bash
go build ./...
go test ./...
go test -race ./...
go test -bench=. -benchmem ./pkg/...
grep -r "^\s*go " pkg/   # must be empty
```

---

## Development Workflow Guide

### Agent Role 1: Orchestrator Agent

Use when coordinating multi-package implementations, phase-level work, or any task spanning multiple files.

#### Orchestrator Workflow (11-Step Process)

```
1. Context Setup
   → Delegate: "Read README-LLM.md, relevant HLD sections, existing code"
   → Define: Clear scope, ownership boundaries, expected deliverables
   → Include: Design constraints, architectural invariants (zero goroutines, zero allocs)

2. Implementation Delegation
   → Delegate: Package implementation with TDD requirements
   → Prompt Detail Level: "Fresh college grad seeing codebase for first time"
   → Include: Specific HLD section references, pattern examples, testing requirements

3. Code Review Delegation
   → Delegate: Skeptical reviewer to validate implementation
   → Focus: Zero-goroutine invariant, zero-alloc requirement, correctness vs rAthena source
   → Requirement: Only code + benchmarks count as proof (NOT status updates)
   → Output: Detailed gap report with code references and fix recommendations

4. Gap Remediation
   → Delegate: Fix ALL gaps identified in review (no matter how minor)
   → Validate: Each fix with targeted benchmarks or tests

5. Iterative Validation
   → Repeat Steps 2-4 until ZERO gaps remain
   → No Compromises: All benchmarks meeting targets, all tests passing

6. End-to-End Validation
   → Test against a live rAthena server when implementing session/fsm packages
   → Reference: rAthena Docker containers at 127.0.0.1:6900 (see goKore README-LLM.md for credentials)
   → For codegen: verify generated structs match rAthena source field-for-field

7. Build and Test Validation
   → go build ./...       ALL packages must build
   → go test ./...        ALL tests must pass
   → go test -race ./...  No race conditions
   → grep -r "^\s*go " pkg/  Must be empty
   → go test -bench=. -benchmem ./pkg/...  Benchmarks must meet targets

8. Commit and Push
   → git add .
   → git commit -m "Descriptive message referencing phase/package"
   → git push origin HEAD

9. Work Log Creation
   → Create work log in docs/WORKLOG/
   → Format: NNNN_YYYY-MM-DD_description.md
   → Content: What was implemented, test results, benchmark results, next steps
   → Commit work log with code changes

10. Move to Next Phase/Package
    → Validate no integration gaps between previous and current work
    → Repeat workflow from Step 1

11. Integration Gap Check
    → Ask: "Was previous package's code actually integrated/wired correctly?"
    → Check: Import paths, function signatures match HLD spec
    → Test: End-to-end flow through the new package
```

#### Delegation Prompt Template

```
CONTEXT:
- Primary Doc: README-LLM.md (your bible)
- HLD Sections: [List relevant sections from docs/DESIGN/HLD.md]
- Design Invariants: Zero goroutines in pkg/, zero allocs in decode hot path,
                     no external deps, rAthena is source of truth

SCOPE:
- Objective: [Clear, specific goal]
- Boundaries: [Which files/packages this delegation owns]
- Integration Points: [How this connects to other packages]

REQUIREMENTS:
- MUST read README-LLM.md
- MUST read listed HLD sections
- MUST follow TDD (tests first)
- MUST benchmark decode/encode functions (0 allocs/op target)
- MUST create work log when done

DELIVERABLES:
1. [Specific package or file with acceptance criteria]
2. [Tests with required coverage]
3. [Benchmarks meeting targets in HLD §8]

SUCCESS CRITERIA:
- go build ./... exits 0
- go test ./... exits 0
- go test -bench=. -benchmem shows 0 allocs/op on hot path
- grep -r "^\s*go " pkg/ produces empty output
- Work log created
```

### Agent Role 2: Delegation Agent

Use when implementing a specific package, running codegen, or fixing a specific bug.

#### Delegation Agent Workflow

```
1. Read Required Documentation
   - README-LLM.md (MANDATORY)
   - All listed HLD sections
   - Existing code in the package being extended

2. Understand Constraints
   - Zero goroutines in pkg/ — hard invariant
   - Zero allocs in decode hot path — verified by benchmarks
   - rAthena source is the only packet structure authority
   - No external deps

3. Plan Implementation
   - Break down into sub-tasks
   - Identify test scenarios (happy + edge cases + error paths)
   - Identify which rAthena source files to reference

4. Write Tests FIRST (TDD)
   - Tests MUST fail initially
   - Include benchmark tests for all decode/encode functions

5. Implement
   - Follow package descriptions in HLD §9
   - Direct byte reads (no reflection, no interface{})
   - Value types, no pointers in event structs
   - [N]byte returns from encode functions where possible

6. Validate
   - go build ./...
   - go test ./...
   - go test -bench=. -benchmem ./pkg/...  (0 allocs/op)
   - grep -r "^\s*go " pkg/  (must be empty)

7. Create Work Log
   - MANDATORY — task is not done without it

8. Report Back
   - Clear completion status
   - Benchmark results
   - Any gaps or questions
```

#### Critical Principles

**READ FIRST, ASK LATER**: Read README-LLM.md and all HLD sections before any work.

**VERIFY AGAINST RATHENA SOURCE**: Before declaring a field correctly decoded, cross-reference with `RATHENA_ROOT/src/map/packets_struct.hpp` or `clif.cpp`.

**BENCHMARK EVERYTHING**: Every decode function needs a benchmark. 0 allocs/op is not optional.

**NO GOROUTINES IN PKG/**: Before writing `go`, stop and ask if it belongs in the caller instead.

---

## Build & Validation Commands

```bash
# Standard build
go build ./...

# Run all tests
go test ./...

# Run tests with race detector
go test -race ./...

# Benchmarks with memory stats
go test -bench=. -benchmem ./pkg/...

# Fuzz tests (run for extended periods to find edge cases)
go test -fuzz=FuzzDecodePosDir -fuzztime=60s ./pkg/packing/
go test -fuzz=FuzzDecodeMoveData -fuzztime=60s ./pkg/packing/

# CI: verify zero goroutines in pkg/ — must produce empty output
grep -r "^\s*go " pkg/

# CI: escape analysis — event structs must not heap-escape
go build -gcflags="-m" 2>&1 | grep "does not escape"

# Run codegen (Phase 1+)
go run ./internal/codegen/main.go \
    --rathena ~/personal/rathena \
    --semantics semantics/mappings.yaml \
    --out .
```

---

## Common Mistakes to Avoid

### 1. Adding a goroutine to any `pkg/` package

```go
// FORBIDDEN anywhere in pkg/
go someFunc()

// Why: The library contract is zero goroutines.
// If you need concurrency, put it in the caller (goKore).
```

### 2. Extracting direction from MoveData byte 5

```go
// WRONG — this is a confirmed bug from goKore v1
direction := (data[5] & 0xF0) >> 4

// CORRECT — byte 5 is sx0/sy0, NOT direction
fromX, fromY, toX, toY, sx0, sy0 := packing.DecodeMoveData(data)
// events.ActorMoved has no Dir field — the 6-byte format has no direction
```

### 3. Using a Go `string` for binary fields

```go
// WRONG — causes UTF-8 corruption for bytes > 0x7F (goKore v1 bug)
type Foo struct {
    PosDir string  // NEVER
}

// CORRECT
type Foo struct {
    PosDir [3]byte  // or decoded inline by the decode function
}
```

### 4. Editing semantics/mappings.yaml directly

```bash
# WRONG
vim semantics/mappings.yaml

# CORRECT — use the gokore-semantics MCP server tools
# (semantics_add, semantics_add_field, semantics_update, etc.)
```

### 5. Adding external dependencies

```go
// WRONG — adding a require entry to go.mod
require github.com/some/library v1.0.0

// This library must have zero external runtime dependencies.
// Use only the Go standard library.
```

### 6. Using reflection or interface{} in decode/encode

```go
// WRONG
func decode(pkt interface{}) interface{} { reflect.ValueOf(pkt)... }

// CORRECT — direct byte reads, typed returns
func ActorExists_0x09FF(data []byte, pv uint32) events.ActorExists {
    var e events.ActorExists
    e.ID = leU32(data, 4)
    ...
    return e
}
```

### 7. Deviating from HLD without updating it

If you discover the HLD is wrong or incomplete, update `docs/DESIGN/HLD.md` to reflect the correct design BEFORE implementing. The HLD is the design authority.

### 8. Using OpenKore field names in Go code

```go
// WRONG — OpenKore names
e.ObjectID = ...   // OpenKore calls it objectID
e.x = ...          // OpenKore calls it x

// CORRECT — use rAthena names as comments, semantic names in Go
e.ID = leU32(data, off)   // rAthena: AID
e.X = ...                  // decoded from PosDir, rAthena: xPos
```

### 9. Calling `net.Dial` from inside `pkg/fsm`

```go
// WRONG — the library never dials
conn, err := net.Dial("tcp", addr)

// CORRECT — call the provided Dialer
conn, err := dialer(ctx, addr)
```

### 10. Using `context.Context` in decode or encode functions

```go
// WRONG — context is an application concern
func ActorExists_0x09FF(ctx context.Context, data []byte, pv uint32) events.ActorExists

// CORRECT — no context in library internals
func ActorExists_0x09FF(data []byte, pv uint32) events.ActorExists
```

---

## Work Log Directory

**Format**: `docs/WORKLOG/NNNN_YYYY-MM-DD_description.md`

- `NNNN` — 4-digit sequence number, sequential, zero-padded
- `YYYY-MM-DD` — ISO date when work was performed
- `description` — brief snake_case description

**Current logs:**
```bash
ls docs/WORKLOG/
```

**Get next sequence number:**
```bash
cd docs/WORKLOG
NEXT=$(printf "%04d" $(($(ls -1 [0-9][0-9][0-9][0-9]_*.md 2>/dev/null | sed 's/_.*//' | sort -n | tail -1) + 1)))
```

---

## Quick Reference

### Essential Files Before Implementing Any Package

| File | Purpose | When to Check |
|---|---|---|
| `docs/DESIGN/HLD.md` §9 | Package descriptions with code examples | Before implementing any package |
| `docs/DESIGN/HLD.md` §4 | FSM public API + state machine | Before implementing pkg/fsm |
| `docs/DESIGN/HLD.md` §6 | Codegen pipeline spec | Before implementing internal/codegen |
| `docs/DESIGN/HLD.md` §8 | Performance contract + benchmark targets | Before writing any decode/encode |
| `docs/DESIGN/HLD.md` §13 | Phase 1 scope — which packets first | When choosing what to implement next |
| `RATHENA_ROOT/src/map/packets_struct.hpp` | Packet struct definitions | When implementing any decode function |
| `RATHENA_ROOT/src/map/clif.cpp:173–249` | WBUFPOS/WBUFPOS2 C source | When implementing packing tests |
| `semantics/mappings.yaml` (via MCP) | Semantic name mappings | When codegen needs field name info |
| `pkg/packing/packing.go` | Bit-packing reference implementation | When writing decode functions |

### Key Design Decisions Summary

| Decision | Why |
|---|---|
| GCC `-E -P` as struct source of truth | Compiler resolves all `#if PACKETVER` correctly; eliminates manual errors |
| Runtime `packetver uint32` in decode fns | Single binary supports all servers; no snapshot directories |
| Zero goroutines in library | Library is a pure transformation; concurrency belongs in the caller |
| `[N]byte` returns from encode fns | Prevents heap escape on encode path |
| Three typed sessions (Login/Char/Map) | Type safety, memory efficiency, no reconfigure race |
| `[65536]HandlerFunc` array | O(1) lookup, no hash overhead |
| Callbacks not channels | Channel adds goroutine + allocation per event; inline callbacks cost nothing |
| No `context.Context` in library | Context is application-layer; threading through 1000-bot decode calls is pure overhead |
| FSM takes Dialer not net.Conn | goKore owns all sockets; FSM needs to dial three separate servers sequentially |

---

**Last Updated**: 2026-03-06
**Design Authority**: `docs/DESIGN/HLD.md` (Draft v9)
