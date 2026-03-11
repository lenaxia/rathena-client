# rathena-client Documentation - Complete LLM Starting Point

**This is the ONLY document you need to read to start development on rathena-client.**

All essential information is consolidated here. The HLD is referenced for deep dives only.

---

## Project Overview

**rathena-client** is a standalone Go library (`github.com/lenaxia/rathena-client`, Go 1.24.0) that implements the Ragnarok Online wire protocol as spoken by rAthena login, char, and map servers.

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
module github.com/lenaxia/rathena-client
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

### 7. HLD.md is the Design Authority — but the Semantic DB is the Packet Authority (MANDATORY)

**`docs/DESIGN/HLD.md` is the authoritative design document for architecture and algorithms.**

**`semantics/mappings.yaml` (accessed via MCP) is the authoritative source for packet field definitions.**

The HLD is prose. Prose drifts. The semantic DB is machine-checkable. When they conflict, run the GCC preprocessor and check rAthena source — that is the ground truth. Update both the HLD and the DB to match.

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

**Every HLD claim about a packet ID, field type, or struct layout MUST cite a specific rAthena file:line.**

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

### 9. Semantic DB via MCP Server Only — But Verify Before Trusting (CRITICAL)

**ALWAYS use the `gokore-semantics` MCP server to READ and WRITE the semantic DB. NEVER edit `semantics/mappings.yaml` directly.**

**However: the DB contains known errors and must not be trusted without GCC verification.**

As of 2026-03-06, the DB has 306 validation errors and 1000+ quality issues (run `semantics_validate` and `validate_all_quality` to see the current state). Common error classes:
- Wrong struct names in action implementations
- Invalid canonical param types (`*uint32`, `[4]byte`, etc.)
- Missing fields and metadata

**The correct workflow is:**
1. Query DB via MCP (`semantics_get`, `semantics_list_fields`) — treat as a starting point
2. Run GCC preprocessor to get ground truth
3. Compare and fix DB via MCP if they differ
4. Only then implement

```bash
# CORRECT: Use MCP to query, then verify with GCC
# 1. semantics_get("0x09FF")  ← starting point, may have errors
# 2. g++ -E -P ... packets_struct.hpp | grep -A 30 "struct packet_idle_unit " ← ground truth
# 3. Fix DB via MCP if they differ
# 4. Implement

# WRONG: Direct file edit
vim semantics/mappings.yaml  # NEVER DO THIS

# WRONG: Trust the DB without GCC verification
# (the DB has 306 known validation errors as of 2026-03-06)
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

Code should be self-documenting. Exception: **package-level doc comments** (the `// Package foo ...` comment) are required on every package. Function signatures with non-obvious parameters may have a brief doc comment. Field reads in decode functions **must** cite the rAthena field name as a comment (e.g., `// rAthena: AID`).

Do NOT add inline comments like `// increment offset`, `// parse field`, etc. The code should read clearly without them.

### 12. No Unverified Claims — Every Assertion Must Be Backed by Code (CRITICAL)

**Never state that something exists without showing it. Never state that something does not exist without proving its absence.**

This applies to ALL claims: struct definitions, field names, macro usages, function implementations, file locations, naming conventions, anything.

**If you claim X exists:**
- Show the file path and line number where it is defined
- Show the actual source text, obtained by reading the file or running grep/GCC

**If you claim X does not exist:**
- Show the grep/search command and its output (including the fact that it returned nothing)
- Search all plausible locations: every relevant `.hpp`, `.cpp`, and generated file
- Absence from one file does not mean absence from all files

**Forbidden patterns:**
```
# These are all violations — never say these without proof:
"There is no struct for this packet"
"This field doesn't exist in rAthena"
"rAthena handles this with raw macros, not a struct"
"The struct name was invented by SemanticDB"
"This packet has no definition anywhere"
```

**Required pattern:**
```bash
# Before claiming X doesn't exist, run ALL of:
grep -rn "X" ~/personal/rathena/src/ --include="*.hpp"
grep -rn "X" ~/personal/rathena/src/ --include="*.cpp"
# Show the output. Empty output proves absence. Non-empty output is the definition.
```

**Why this rule exists:** In session 9, the claim was made that `PACKET_CZ_REQUEST_MOVE` and `PACKET_ZC_NOTIFY_STANDENTRY` "do not exist as C struct definitions in rAthena" and were "phantom names invented by SemanticDB." This was stated confidently without exhaustive proof. The actual situation requires full grep across all source files and reading clif.cpp to understand the real naming conventions (`packet_idle_unit` vs `PACKET_ZC_NOTIFY_STANDENTRY`). Confident false claims about code are worse than admitting uncertainty — they get recorded as facts and propagate into design decisions.

**When uncertain: say so and search, don't guess.**

---

## Defense-in-Depth: How We Prevent and Catch Errors

The HLD audit identified 30 issues (10 blockers, 15 majors, 5 minors) in Draft v9 — most caused by prose describing data structures without machine verification. This section defines the process that prevents this class of error from reaching implementation.

### The Core Problem

The HLD is prose written by reasoning about rAthena. Reasoning produces plausible-sounding but unverified claims. An LLM writing code against unverified prose will produce plausible-sounding but incorrect code. Errors compound silently until runtime.

**The solution is three layers of machine-checkable verification between rAthena source and Go code.**

### Layer 1: GCC Preprocessor as Ground Truth

Every struct field claim in the HLD or DB must be verifiable by running:

```bash
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I ~/personal/rathena/src \
    -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null | grep -A 20 "struct packet_idle_unit "
```

`packets_struct.hpp` preprocesses cleanly with no stubs needed. `packets.hpp` requires stubs (see `validation/stubs/`) because it includes `map.hpp` → `script.hpp` → `ryml_std.hpp`. `common/packets.hpp` also requires stubs for `common/mmo.hpp` etc.

The `validation/` directory contains scripts to automate this:

```
validation/
    preprocess_check.sh     run GCC -E on all three headers; output to validation/output/
    length_check.sh         for Phase 1 packets: verify field sizes sum to expected total
    db_validate.sh          run MCP validate_all_quality + validate_lengths
    stubs/
        packets_hpp_stub.h  stubs for map.hpp include chain (ryml, script, sql)
        common_hpp_stub.h   stubs for common/mmo.hpp, socket.hpp, showmsg.hpp include chain
```

**Run these scripts before implementing any package. If a script fails, fix the DB and HLD first.**

### Layer 2: Semantic DB as a Starting Point — Not a Source of Truth

The DB (`semantics/mappings.yaml`, accessed via MCP only) has 446 packet mappings and 1995 field definitions. It is a useful starting point for understanding what packets exist and their approximate structure.

**However, the DB is known to contain errors.** As of 2026-03-06, running `semantics_validate` produces 306 validation errors and `validate_all_quality` produces 1000+ quality issues. Examples of confirmed errors include:

- Wrong struct names in semantic action implementations (e.g. `actor_action` using `PACKET_ZC_NOTIFY_ACT` for a packet that is actually `PACKET_ZC_NOTIFY_ACT_DAMAGE`)
- Struct name mismatches between action implementations and packet definitions (e.g. `send_chat`, `reply_party_invite`, `market_purchase`)
- Invalid canonical param types (e.g. `*uint32`, `[4]byte`, `[10]uint32` — pointer and array types that are not valid param types)
- Missing fields, missing OpenKore names, missing semantic descriptions across hundreds of packets
- Semantic actions referencing packet IDs that don't exist in the DB

**The DB must be treated as unverified until validated against GCC preprocessor output.**

The workflow for using the DB:

1. Query the DB via MCP to get the starting-point field list
2. Run the GCC preprocessor for the same packet
3. Compare field names, types, and order between DB and preprocessor output
4. Fix any discrepancies in the DB via MCP before writing any code
5. Only after DB entry matches GCC output is it safe to implement

**The DB is a cross-reference, not an oracle. GCC output is the oracle.**

When a delegate agent is asked to implement a decode function, the prompt is:
> "Query the DB for packet `0x09FF` using MCP `semantics_get` as a starting point. Then run the GCC preprocessor to verify. Fix the DB via MCP if they differ. Only then implement."

### Layer 3: Tiered Tests that Catch What the DB Cannot

**Tier A — Byte-level golden tests (no network):**
Every decode function has a test that feeds known bytes and asserts specific field values. Golden bytes are synthesized directly from the rAthena struct definition by constructing the packet manually from the C field layout. These catch codegen bugs and DB errors simultaneously.

**Tier B — Integration tests against real rAthena (Docker):**
A full `FSM.Connect()` against a real rAthena server. This is the only way to verify packet IDs, obfuscation keys, framing, and the full auth sequence. rAthena Docker is at `127.0.0.1:6900` (see goKore README-LLM.md for credentials).

**Tier C — Regression tests from captured traffic:**
Real packet captures saved as binary fixtures in `testdata/captures/`. Replayed through `Feed()` and the callback sequence asserted. These are immune to golden tests written from the same wrong source as the code.

### Pre-Implementation Gate (MANDATORY for every package)

Before any delegate agent writes a single line of implementation code:

1. **Run `validation/preprocess_check.sh`** for every packet the package touches. Paste the relevant struct output into the work log.

2. **Query the DB** (`semantics_get`, `semantics_list_fields`) for every packet. Verify field names, types, and positions match the preprocessor output. If they differ, fix the DB via MCP.

3. **Run `validation/db_validate.sh`** — all DB quality checks must pass.

4. **Run `validation/length_check.sh`** for every fixed-length packet — field sizes must sum to the expected total.

5. **Document the GCC command used** and its key output in the work log. Example:
   ```
   Verified: packet_idle_unit at PACKETVER=20180307
   Command: g++ -E -P -DPACKETVER=20180307 ... packets_struct.hpp | grep -A 30 "struct packet_idle_unit {"
   Result: objecttype(1) + AID(4) + GID(4) + speed(2) + ... + PosDir[3](3) = 175 bytes total
   ```

**No implementation proceeds without passing this gate.**

### When the HLD, DB, and rAthena Source Conflict

The hierarchy of authority is fixed:

```
rAthena source (GCC preprocessor output)  ← ALWAYS WINS
        ↓
semantics/mappings.yaml (via MCP)          ← fix to match GCC output
        ↓
docs/DESIGN/HLD.md                         ← fix to match GCC output
        ↓
Go implementation                          ← implement against corrected DB
```

When any conflict is found:
1. Run the GCC preprocessor. That output is authoritative.
2. Fix the DB entry via MCP to match.
3. Fix the HLD claim to match, citing `rAthena src/file:line`.
4. Document the correction in the work log.
5. Then implement.

---

## Architecture

### Package Map

```
rathena-client/
    semantics/
        mappings.yaml          human-maintained semantic layer (edit via MCP only)
                               446 packet mappings, 1995 fields — already populated

    validation/                pre-implementation verification scripts
        preprocess_check.sh    run GCC -E on rAthena headers
        length_check.sh        verify field sizes vs expected packet lengths
        db_validate.sh         run MCP quality checks on semantic DB
        stubs/
            packets_hpp_stub.h   stubs for map.hpp → script.hpp → ryml chain
            common_hpp_stub.h    stubs for common/mmo.hpp etc.

    pkg/
        packing/               COMPLETE — packing.go + packing_test.go
        fsm/                   NOT STARTED — Phase 6 ← CURRENT
        events/                COMPLETE — 417 generated event structs
        send/                  COMPLETE — 163 generated send request structs
        decode/                COMPLETE — 442 generated decode functions
        encode/                COMPLETE — 80 generated encode functions
        session/               COMPLETE — hand-written (session.go, login.go, char.go, map.go,
                                          obfuscation.go) + generated (lengths_*.go, shuffle_map.go,
                                          obfuscation_keys.go)

    internal/
        codegen/               COMPLETE — GCC+semantics pipeline, 481 structs in VersionTable
```

### Data Flow Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                        rathena-client                            │
│                                                                  │
│  pkg/fsm/          ConnectionFSM — login + reconnect sequencer   │
│  pkg/packing/      WBUFPOS / WBUFPOS2 encode+decode    PARTIAL   │
│  pkg/events/       Canonical event structs (S→C)    GENERATED    │
│  pkg/send/         Canonical send request types (C→S) GENERATED  │
│  pkg/decode/       Raw bytes → events               GENERATED    │
│  pkg/encode/       Send requests → raw bytes        GENERATED    │
│  pkg/session/      PACKETVER-aware tokenizer + dispatcher        │
│                    (LoginSession, CharSession, MapSession)        │
│                                                                  │
│  internal/codegen/ Code generator (reads rAthena + mappings.yaml)│
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
  → receives 0x0081 / 0x0AC5 with map addr, closes conn
  → FSM calls dialer(ctx, mapAddr) → net.Conn
  → FSM creates MapSession, sends 0x0436
  → receives 0x0073/0x0A18/0x02EB, sends 0x007D + shuffled(0x007E/0x0360)
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
  → encode.EncodeMove(req, packetver)   [GENERATED, returns [N]byte]
  → look up shuffled C→S packet ID for this packetver
  → optionally XOR packet ID (obfuscation, PACKETVER ≤ 20180307 only)
  → goKore calls conn.Write(bytes[:])  [goKore owns the socket]
```

---

## Implementation Phases

### Current State

| Package | Status | Notes |
|---|---|---|
| `pkg/packing` | **Complete** | packing.go + packing_test.go; all tests pass (worklog 0001) |
| `validation/` | **Complete** | preprocess_check.sh, phase1_gate.sh, struct_layout.sh (worklogs 0002-0007) |
| `internal/codegen` | **Complete** | Full GCC+semantics pipeline; 770 structs in VersionTable after PACKET_CZ_ injection from packets.hpp (worklog 0039) |
| `pkg/events` | **Complete** | 281 generated event structs; `[3]byte`/`[6]byte` for PosDir/MoveData (worklog 0013) |
| `pkg/send` | **Complete** | 152 generated send request structs (worklog 0039: 13 duplicate aliases removed) |
| `pkg/decode` | **Complete** | 282 generated decode functions; zero allocs on all benchmarks; zero "complex expression" gaps remaining (worklogs 0036-0037) |
| `pkg/encode` | **Complete** | 115 generated encode functions including gameplay CZ packets (worklog 0039: PACKET_CZ_ injection fix) |
| `pkg/session` (generated) | **Complete** | lengths_login.go (13 entries), lengths_char.go (37+ entries), lengths_map.go, shuffle_map.go, obfuscation_keys.go — lengths generated from GCC sizeof via common/packets.hpp (worklog 0014) |
| `pkg/session` (hand-written) | **Complete** | session.go, login.go, char.go, map.go, obfuscation.go; 12 tests pass; 0 allocs/op benchmarks (worklog 0013) |
| `pkg/fsm` | **Complete** | ConnectionFSM: full login→char→map auth sequence; 21 tests pass; zero goroutines; net.Pipe stubs (worklog 0015) |

**Gate status**: 76 PASS / 1 FAIL (expected; CH_MAKE_CHAR 0x0065 shuffle — documented). `go build ./...` and `go test ./...` are clean.

**Known open issues** (non-blocking for Phase 7):
- SemanticDB has validation errors (run `semantics_validate`). Not blockers for Phase 7.
- lengths_char.go: HC_ACCEPT_MAKECHAR (0x006D/0x0B6F) sizes may still be wrong — nested struct CHARACTER_INFO not fully resolved by codegen.
- `lengths_map.go` partially populated; FSM uses `SetLength` for auth-phase packets where needed.
- 3 encode functions always panic (`EncodeGameLogin`, `EncodeMapLoaded`, `EncodeTimeSyncResponse`) — **eliminated**; these are FSM-owned actions added to the skip list in `GenerateEncodeDirFiles` (worklog 0040); no generated files exist for them.
- `EncodeDealFinalize` always panics — **fixed**; hand-written in `pkg/encode/deal_finalize.go`; `0x00EB` is a 2-byte header-only packet with no rAthena struct (worklog 0040).
- `EncodeSkillUse` and `EncodeActorAction` have a trailing panic that is unreachable (`case packetver >= 0` is always true).

**Out of scope — not planned:**
- **Homunculus packets** (`ZC_PROPERTY_HOMUN`, `ZC_FEED_MER`, `ZC_PROPERTY_HOMUN_*`, `PACKET_CZ_*_HOMUN`, etc.) — homunculus support is not planned for the initial goKore integration. The generated decode stubs exist but the known type truncation bugs (`hp`/`maxHp` `uint32→uint16`, `exp`/`expNext` `int64→uint32`) in those stubs will not be fixed.
- **Mercenary packets** (`ZC_MER_*`, `CZ_MER_*`) — mercenary support is not planned for the initial goKore integration.

### Phase 0 — Validation Infrastructure ✅ COMPLETE (worklogs 0002-0007)

Validation scripts are built and working.

**Deliverables (done):**
- `validation/preprocess_check.sh` — runs GCC -E on all three headers at a given PACKETVER
- `validation/phase1_gate.sh` — 76-check gate (76 PASS / 1 FAIL expected)
- `validation/struct_layout.sh` — struct layout verification
- `validation/stubs/` — stub headers for packets.hpp and common/packets.hpp include chains

### Phase 1 — Fix HLD and DB ✅ COMPLETE (worklogs 0001-0007)

The HLD audit blockers and majors were fixed. Struct layouts verified against GCC preprocessor.

### Phase 2 — pkg/packing completion ✅ COMPLETE (worklog 0001)

`pkg/packing/packing_test.go` — table-driven, round-trip, and benchmark tests. All pass.

### Phase 3 — internal/codegen ✅ COMPLETE (worklog 0008)

Code generator built. Inputs: `packets_struct.hpp`, `packets.hpp` (with stubs), `common/packets.hpp` (with stubs), `clif_packetdb.hpp`, `clif_shuffle.hpp`, `clif_obfuscation.hpp`, and `semantics/mappings.yaml` via MCP. VersionTable has 481 structs (459 from rAthena + 22 SYNTH_*).

### Phase 4 — Generated packages ✅ COMPLETE (worklogs 0009-0013, 0036-0037, 0039)

Codegen output:
- `pkg/events/` — 281 event structs; PosDir/MoveData use `[3]byte`/`[6]byte` (CONCERN-2 resolved, worklog 0013)
- `pkg/send/` — 152 send request structs (13 duplicate aliases removed, worklog 0039)
- `pkg/decode/` — 282 decode functions (1 skipped intentionally: quest_update_mission_hunt); zero `make([]byte)` calls; zero "complex expression" gaps
- `pkg/encode/` — 115 encode functions including gameplay CZ packets (PACKET_CZ_ injection fix, worklog 0039)
- `pkg/session/lengths_login.go` — 13 entries; CA_/AC_/CT_/TC_/SC_ packets; generated from GCC sizeof via common/packets.hpp
- `pkg/session/lengths_char.go` — 37+ entries; CH_/HC_/SC_/PING packets; nested struct sizes resolved
- `pkg/session/lengths_map.go` — full map server table (4 codegen passes + post-merge dedup)
- `pkg/session/shuffle_map.go` — `ShuffledCtoSID(packetver uint32, baseID uint16) uint16`
- `pkg/session/obfuscation_keys.go` — `ObfuscationKeysFor(packetver uint32) (k0, k1, k2 uint32)`

VersionTable now has 770 structs (up from 482) after PACKET_CZ_ injection from packets.hpp.

### Phase 5 — pkg/session (hand-written parts) ✅ COMPLETE (worklog 0013)

Implemented:
- `pkg/session/session.go` — `sessionCore`, `Feed()`, `ErrUnknownPacket`
- `pkg/session/login.go` — `LoginSession`
- `pkg/session/char.go` — `CharSession`
- `pkg/session/map.go` — `MapSession`
- `pkg/session/obfuscation.go` — LCG key state + XOR logic (clif.cpp:25702)
- 12 tests pass; benchmarks: 1.79 ns/op fixed, 10.78 ns/op variable, 0 allocs/op

### Phase 6 — pkg/fsm ✅ COMPLETE (worklog 0015)

Implemented `ConnectionFSM`. Full state machine, public API, all protocol steps.
- Zero goroutines; `Connect()` is fully synchronous in caller's goroutine
- Dialer-based: FSM never calls `net.Dial` directly
- Pre-20170315 path: 0x0069 login accept, 0x0081 zone server (28-byte disambiguation)
- Post-20170315 path: 0x0AC4 login accept, 0x0AC5 zone server
- Paged char list (pv >= 20130000): 0x09A0 → 0x09A1 × N → 0x099D pages
- Map auth: 0x0283 ZC_AID, 0x007D + 0x007E/0x0360 sequence, then `OnReady`
- C→S obfuscation applied to 0x0436, 0x007D, 0x007E/0x0360 via `encodePacketID`
- 21 tests pass using `net.Pipe` stubs

### Phase 7 — Integration with goKore

Replace goKore's `internal/network/` layer. See HLD §7.

---

## Code Generation Pipeline

The code generator (`internal/codegen`) is **complete** (Phase 3, worklog 0008).

### Inputs

1. **rAthena C++ headers** (from `RATHENA_ROOT`):
   - `src/map/packets_struct.hpp` — map server packet structs (no stubs needed — verified)
   - `src/map/packets.hpp` — newer ZC_/CZ_ structs (needs stubs — ryml chain)
   - `src/common/packets.hpp` — login/char server structs (needs stubs — mmo/socket chain)
   - `src/map/clif_packetdb.hpp` — base C→S packet registration (lengths + handler names)
   - `src/map/clif_shuffle.hpp` — per-PACKETVER C→S packet ID shuffle table
   - `src/map/clif_obfuscation.hpp` — PACKET_OBFUSCATION key table (needs `-DPACKET_OBFUSCATION`)

2. **`semantics/mappings.yaml`** — accessed via MCP server only. Provides semantic field names, canonical action groupings, decode hints for packed binary fields.

### Processing

```bash
# packets_struct.hpp — no stubs needed
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I RATHENA_ROOT/src -I RATHENA_ROOT/src/map -I RATHENA_ROOT/src/common \
    RATHENA_ROOT/src/map/packets_struct.hpp

# packets.hpp — needs stubs for ryml/script/sql chain
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I RATHENA_ROOT/src -I RATHENA_ROOT/src/map -I RATHENA_ROOT/src/common \
    -include internal/codegen/stubs/packets_hpp_stub.h \
    RATHENA_ROOT/src/map/packets.hpp

# common/packets.hpp — needs stubs for mmo/socket/showmsg/utilities chain
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I RATHENA_ROOT/src -I RATHENA_ROOT/src/common \
    -include internal/codegen/stubs/common_hpp_stub.h \
    RATHENA_ROOT/src/common/packets.hpp

# clif_obfuscation.hpp — needs -DPACKET_OBFUSCATION
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKET_OBFUSCATION \
    -I RATHENA_ROOT/src \
    RATHENA_ROOT/src/map/clif_obfuscation.hpp
```

Three passes per struct breakpoint (MAIN, RE, ZERO build flavors) to handle `PACKETVER_RE_NUM` and `PACKETVER_ZERO_NUM` variants.

**Diffing adjacent outputs** produces a `VersionTable`: a map of struct name → list of (packetver_range, StructLayout) entries.

`lengths_map.go` is generated from **four passes** merged via `mergeBreakpoints` (with post-merge deduplication):
- **Part 1**: C→S lengths from `clif_packetdb.hpp`
- **Part 2**: S→C fixed-size from `packets.hpp` `HEADER_*` constants
- **Part 3**: S→C from SemanticDB + VersionTable join
- **Part 4**: S→C from `enum packet_headers` in `packets_struct.hpp` (covers IDs invisible to HEADER_* parser)

Part 4 uses `knownEnumPackets` table in `internal/codegen/main.go` which maps enum names
(e.g. `skillscale`, `guildLeave`, `partymemberinfo`) to their struct names/variability.
The table covers all 15 active enum-assigned packet IDs as of v0.2.2.

### Combination

```
GCC preprocess at each breakpoint
    → StructDB: map[struct_name][]VersionedLayout
    ↓
semantics/mappings.yaml  (via MCP server — DO NOT EDIT DIRECTLY)
    → ActionDB: map[action_name]ActionDef (canonical names, decode hints)
    ↓
codegen joins StructDB + ActionDB
    → pkg/decode/*.go
    → pkg/encode/*.go
    → pkg/events/*.go
    → pkg/session/lengths_login.go
    → pkg/session/lengths_char.go
    → pkg/session/lengths_map.go
    → pkg/session/shuffle_map.go        ShuffledCtoSID(packetver, baseID)
    → pkg/session/obfuscation_keys.go   ObfuscationKeysFor(packetver) (k0,k1,k2)
```

### Running the codegen

```bash
go run ./internal/codegen/main.go \
    --rathena ~/personal/rathena \
    --semantics semantics/mappings.yaml \
    --out .
```

Generated files are committed to the repository (analogous to `.pb.go` files). Regeneration is triggered manually when rAthena is updated or when `semantics/mappings.yaml` changes.

---

## Non-Trivial Wire Formats

These are the two packed binary formats used throughout the protocol. They are implemented in `pkg/packing`. All generated decode functions call `packing.DecodePosDir` and `packing.DecodeMoveData` — never reimplement this logic inline.

### 3-byte packed position (WBUFPOS / PosDir[3])

Source: `clif.cpp:173–178` (encode), `clif.cpp:197–211` (decode).

```
Byte 0: [x9 x8 x7 x6 x5 x4 x3 x2]
Byte 1: [x1 x0 y9 y8 y7 y6 y5 y4]
Byte 2: [y3 y2 y1 y0 d3 d2 d1 d0]
```

- x: 10-bit map coordinate
- y: 10-bit map coordinate
- dir: 4-bit direction. Values: 0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE
  (Source: `src/map/path.hpp`: `DIR_NORTH=0, DIR_NORTHWEST=1, DIR_WEST=2, ...`)

### 6-byte packed movement (WBUFPOS2 / MoveData[6])

Source: `clif.cpp:182–190` (encode), `clif.cpp:214–240` (decode).

```
Bytes 0-4: fromX(10b) fromY(10b) toX(10b) toY(10b)  [packed]
Byte 5:    [sx0_3 sx0_2 sx0_1 sx0_0 sy0_3 sy0_2 sy0_1 sy0_0]
```

**CRITICAL**: Byte 5 is `sx0` (high nibble) and `sy0` (low nibble) — sub-cell interpolation offsets. It is **NOT a direction value**. There is no direction field in the 6-byte format.

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

Each session type uses a `[65536]HandlerFunc` array indexed by packet ID. Length table is a `[65536]int16` array. Both are per-session-instance. Memory at 1000 bots:
- Handler arrays: 1000 × 65536 × 8 bytes = ~500 MB
- Length arrays: 1000 × 65536 × 2 bytes = ~128 MB
- Total: ~628 MB — accepted for a dedicated bot machine

`type HandlerFunc func(data []byte, packetver uint32)` (defined in `pkg/session`).

`Feed()` returns `error`. Signature: `func (s *MapSession) Feed(data []byte) error`.

`Encode()` does NOT wrap the generated functions in a heap-allocating `[]byte` return. Callers call generated encode functions directly: `encode.EncodeMove(req, packetver)` returns `[5]byte`. Session types may expose typed helpers but must not add allocations.

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
- Integration test against real rAthena Docker (127.0.0.1:6900)

### pkg/session

- Benchmark tests in `session_bench_test.go`
- Feed a pre-captured packet stream; verify all callbacks fire in correct order
- Verify `Feed()` returns `ErrUnknownPacket` on unknown packet ID
- Verify 0 allocs/op after initial recvBuf warmup

### pkg/decode, pkg/encode (generated)

- Generated tests verify byte-level correctness against known rAthena packet captures
- Golden bytes synthesized from rAthena struct definitions (not from intuition)

### Full Repository Validation (MANDATORY after every task)

```bash
go build ./...
go test ./...
go test -race ./...
go test -bench=. -benchmem ./pkg/...
grep -r "^\s*go " pkg/   # must be empty
./validation/preprocess_check.sh 20180307   # must exit 0
./validation/db_validate.sh                 # must exit 0
```

---

## Development Workflow Guide

### Agent Role 1: Orchestrator Agent

Use when coordinating multi-package implementations, phase-level work, or any task spanning multiple files.

#### Orchestrator Workflow (12-Step Process)

```
1. Context Setup
   → Delegate: "Read README-LLM.md, relevant HLD sections, existing code"
   → Define: Clear scope, ownership boundaries, expected deliverables
   → Include: Design constraints, architectural invariants

2. Pre-Implementation Gate
   → BEFORE delegating any implementation:
   → Run ./validation/preprocess_check.sh for every packet the work touches
   → Run ./validation/db_validate.sh — must pass
   → Query MCP for every packet: semantics_get, semantics_list_fields
   → Fix DB via MCP if any field is wrong
   → Fix HLD if any claim is wrong (with rAthena file:line citations)
   → Only proceed when DB and HLD are verified against GCC output

3. Implementation Delegation
   → Delegate: Package implementation with TDD requirements
   → Prompt Detail Level: "Fresh college grad seeing codebase for first time"
   → Include: Specific HLD section references, DB packet IDs to query, testing requirements

4. Code Review Delegation
   → Delegate: Skeptical reviewer to validate implementation
   → Focus: Zero-goroutine invariant, zero-alloc requirement, correctness vs DB fields
   → Requirement: Only code + benchmarks count as proof (NOT status updates)
   → Output: Detailed gap report with code references and fix recommendations

5. Gap Remediation
   → Delegate: Fix ALL gaps identified in review (no matter how minor)
   → Validate: Each fix with targeted benchmarks or tests

6. Iterative Validation
   → Repeat Steps 3-5 until ZERO gaps remain

7. End-to-End Validation
   → Test against a live rAthena server when implementing session/fsm packages
   → Reference: rAthena Docker containers at 127.0.0.1:6900

8. Build and Test Validation
   → go build ./...       ALL packages must build
   → go test ./...        ALL tests must pass
   → go test -race ./...  No race conditions
   → grep -r "^\s*go " pkg/  Must be empty
   → go test -bench=. -benchmem ./pkg/...  Benchmarks must meet targets
   → ./validation/preprocess_check.sh 20180307  Must exit 0
   → ./validation/db_validate.sh  Must exit 0

9. Commit and Push
   → git add .
   → git commit -m "Descriptive message referencing phase/package"
   → git push origin HEAD

10. Work Log Creation
    → Create work log in docs/WORKLOG/
    → Format: NNNN_YYYY-MM-DD_description.md
    → Content: GCC commands run, DB entries checked, test results, benchmark results
    → Commit work log with code changes

11. Move to Next Phase/Package
    → Validate no integration gaps between previous and current work
    → Repeat workflow from Step 1

12. Integration Gap Check
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

PRE-IMPLEMENTATION GATE (do this before writing any code):
- Run: ./validation/preprocess_check.sh YYYYMMDD
- Query MCP: semantics_get for each packet ID you will implement
- Verify: every field name/type/position in the DB matches the GCC output
- Fix: DB via MCP if anything is wrong; document in work log

SCOPE:
- Objective: [Clear, specific goal]
- Boundaries: [Which files/packages this delegation owns]
- Integration Points: [How this connects to other packages]
- Packet IDs to implement: [List DB packet IDs to query]

REQUIREMENTS:
- MUST read README-LLM.md
- MUST read listed HLD sections
- MUST run pre-implementation gate
- MUST follow TDD (tests first, golden bytes from rAthena struct definitions)
- MUST benchmark decode/encode functions (0 allocs/op target)
- MUST create work log when done (include GCC output evidence)

DELIVERABLES:
1. [Specific package or file with acceptance criteria]
2. [Tests with required coverage]
3. [Benchmarks meeting targets in HLD §8]

SUCCESS CRITERIA:
- go build ./... exits 0
- go test ./... exits 0
- go test -bench=. -benchmem shows 0 allocs/op on hot path
- grep -r "^\s*go " pkg/ produces empty output
- ./validation/preprocess_check.sh exits 0
- ./validation/db_validate.sh exits 0
- Work log created with GCC evidence
```

### Agent Role 2: Delegation Agent

#### Delegation Agent Workflow

```
1. Read Required Documentation
   - README-LLM.md (MANDATORY)
   - All listed HLD sections
   - Existing code in the package being extended

2. Run Pre-Implementation Gate (MANDATORY — do before writing a single line)
   - ./validation/preprocess_check.sh YYYYMMDD for all relevant packets
   - semantics_get / semantics_list_fields for all packet IDs in scope
   - Verify DB fields match GCC output; fix via MCP if they differ

3. Understand Constraints
   - Zero goroutines in pkg/ — hard invariant
   - Zero allocs in decode hot path — verified by benchmarks
   - rAthena source is the only packet structure authority
   - No external deps

4. Plan Implementation
   - Break down into sub-tasks
   - Identify test scenarios (golden bytes from rAthena struct definitions)
   - Identify which rAthena source files to reference

5. Write Tests FIRST (TDD)
   - Tests MUST fail initially
   - Golden bytes synthesized from rAthena C struct layout — not from intuition
   - Include benchmark tests for all decode/encode functions

6. Implement
   - Follow package descriptions in HLD §9
   - Direct byte reads (no reflection, no interface{})
   - Value types, no pointers in event structs
   - Field reads must cite rAthena field name: `// rAthena: AID`

7. Validate
   - go build ./...
   - go test ./...
   - go test -bench=. -benchmem ./pkg/...  (0 allocs/op)
   - grep -r "^\s*go " pkg/  (must be empty)
   - ./validation/preprocess_check.sh exits 0
   - ./validation/db_validate.sh exits 0

8. Create Work Log (MANDATORY — task is not done without it)
   - Include GCC command and key output
   - Include DB packet IDs queried
   - Include benchmark results

9. Report Back
   - Clear completion status
   - Benchmark results
   - Any gaps or questions
```

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

# Fuzz tests
go test -fuzz=FuzzDecodePosDir -fuzztime=60s ./pkg/packing/
go test -fuzz=FuzzDecodeMoveData -fuzztime=60s ./pkg/packing/

# CI: verify zero goroutines in pkg/ — must produce empty output
grep -r "^\s*go " pkg/

# CI: escape analysis — event structs must not heap-escape
go build -gcflags="-m" 2>&1 | grep "does not escape"

# Pre-implementation validation gate
./validation/preprocess_check.sh 20180307    # test at a known PACKETVER
./validation/length_check.sh                 # verify Phase 1 packet field sums
./validation/db_validate.sh                  # MCP quality checks

# Run codegen (Phase 3+)
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
```

### 2. Extracting direction from MoveData byte 5

```go
// WRONG — confirmed bug from goKore v1
direction := (data[5] & 0xF0) >> 4

// CORRECT — byte 5 is sx0/sy0, NOT direction
fromX, fromY, toX, toY, sx0, sy0 := packing.DecodeMoveData(data)
```

### 3. Using a Go `string` for binary fields

```go
// WRONG — causes UTF-8 corruption for bytes > 0x7F
type Foo struct { PosDir string }

// CORRECT
type Foo struct { PosDir [3]byte }
```

### 4. Editing semantics/mappings.yaml directly

```bash
# WRONG
vim semantics/mappings.yaml

# CORRECT — use the gokore-semantics MCP server tools
```

### 5. Adding external dependencies

```go
// WRONG — adding a require entry to go.mod
require github.com/some/library v1.0.0
```

### 6. Using reflection or interface{} in decode/encode

```go
// WRONG
func decode(pkt interface{}) interface{} { reflect.ValueOf(pkt)... }
```

### 7. Deviating from HLD without updating it AND citing rAthena source

Every HLD correction must include `rAthena src/file:line` as the authority. No HLD claim is valid without a citation.

### 8. Skipping the pre-implementation gate

Every packet implementation must be preceded by a GCC preprocess run. No exceptions. Document the output in the work log.

### 9. Calling `net.Dial` from inside `pkg/fsm`

```go
// WRONG
conn, err := net.Dial("tcp", addr)

// CORRECT — call the provided Dialer
conn, err := dialer(ctx, addr)
```

### 10. Using `context.Context` in decode or encode functions

```go
// WRONG
func ActorExists_0x09FF(ctx context.Context, data []byte, pv uint32) events.ActorExists

// CORRECT
func ActorExists_0x09FF(data []byte, pv uint32) events.ActorExists
```

### 11. Trusting the DB without GCC verification

The DB is a useful starting point, not a trusted oracle. It has 306 known validation errors. Always verify DB fields against GCC preprocessor output before implementing. The DB is edited via MCP to match GCC output — not the other way around.

### 12. Writing golden test bytes from intuition

Golden bytes must be constructed by manually laying out the C struct field-by-field. Open `packets_struct.hpp`, find the struct, write the bytes according to the field types in order. Cross-check with the GCC preprocessor output.

---

## Work Log Directory

**Format**: `docs/WORKLOG/NNNN_YYYY-MM-DD_description.md`

**Content requirements:**
- GCC commands run and key output (struct names, field types verified)
- DB packet IDs queried and whether they matched GCC output
- Test results (pass/fail counts)
- Benchmark results (ns/op, allocs/op)
- Any discrepancies found and how they were resolved

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
| `docs/USAGE.md` | Integration guide for consumers — how to wire FSM, sessions, decode, encode | Before integrating with goKore or any consumer |
| `docs/ADDING_PACKETS.md` | How to fix decode gaps, implement stub encodes, add new packets/PACKETVERs | Before implementing any packet decode/encode work |
| `docs/DESIGN/HLD.md` §9 | Package descriptions with code examples | Before implementing any package |
| `docs/DESIGN/HLD.md` §4 | FSM public API + state machine | Before implementing pkg/fsm |
| `docs/DESIGN/HLD.md` §6 | Codegen pipeline spec | Before implementing internal/codegen |
| `docs/DESIGN/HLD.md` §8 | Performance contract + benchmark targets | Before writing any decode/encode |
| `docs/DESIGN/HLD.md` §13 | Phase 1 scope — which packets first | When choosing what to implement next |
| `RATHENA_ROOT/src/map/packets_struct.hpp` | Packet struct definitions | Preprocess and diff before every decode fn |
| `RATHENA_ROOT/src/map/clif.cpp:173–249` | WBUFPOS/WBUFPOS2 C source | When implementing packing tests |
| `RATHENA_ROOT/src/map/clif_shuffle.hpp` | C→S packet ID shuffle table | Before implementing any C→S encode fn |
| `RATHENA_ROOT/src/map/clif_obfuscation.hpp` | Obfuscation keys | Before implementing obfuscation |
| `semantics/mappings.yaml` (via MCP) | Starting-point packet field definitions — **verify against GCC before trusting** (306 known errors) | Before every implementation — as a cross-reference only |
| `pkg/packing/packing.go` | Bit-packing reference implementation | When writing decode functions |
| `validation/preprocess_check.sh` | GCC verification gate | Before every implementation |

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
| `Feed()` returns error | Stream desync on unknown packet is unrecoverable; caller must close connection |
| Semantic DB is a cross-reference, not an oracle | DB has 306 known validation errors as of 2026-03-06. GCC preprocessor output is the only trustworthy source. DB is a starting point — verify against GCC before implementing |
| Pre-implementation gate is mandatory | Prevents HLD drift from becoming code bugs; catches errors before compilation |

---

**Last Updated**: 2026-03-09 (session 0024 — Phases 0–6 complete; 25 work logs; US06/US07/US08/US09/US10 complete)
**Design Authority**: `docs/DESIGN/HLD.md` (Draft v9)
**Ground Truth**: GCC preprocessor output against `~/personal/rathena/src/`
**Packet Cross-Reference**: `semantics/mappings.yaml` via `gokore-semantics` MCP (306 known errors — verify against GCC before trusting)
**Consumer Integration Guide**: `docs/USAGE.md`
