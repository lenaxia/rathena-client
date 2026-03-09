# EPIC-00: Protocol Library Correctness & Phase 1 Completeness

**Status**: Ready for implementation  
**Created**: 2026-03-07  
**Goal**: Deliver a working, tested, end-to-end protocol library capable of connecting
to a real rAthena server (login → char → map) with correct field decoding and zero
heap allocations on the decode hot path.

---

## Context

The codegen pipeline (`internal/codegen`) is fully wired and produces generated output
that compiles and passes all existing tests. However the library cannot yet accept a
single byte of real network data:

- `pkg/session` contains only five generated lookup tables — no `Feed()`, no session
  types, no framing logic
- `pkg/fsm` does not exist
- S→C packet lengths are absent from the generated lengths tables (packetdb only
  contains C→S entries; the pipeline has no source for S→C lengths)
- 2,568 of 3,353 field reads in `pkg/decode/` are silently skipped due to field name
  mismatches between the SemanticDB and the rAthena struct field names:
  - **Category B** (1,787 skips, 626 SemanticDB entries): case mismatch —
    `BodyState` vs `bodyState`, `Speed` vs `speed`, etc.
  - **Category C** (468 skips, ~39 unique wrong names): OpenKore names used instead
    of rAthena names — `HairColor` vs `headpalette`, `ClothesColor` vs `bodypalette`
  - **Category A** (313 skips): flexible array member fields (`TYPE field[]`) that
    the struct parser silently drops

The stories below address these gaps in dependency order.

---

## Story Map

```
US-01  Fix Category B (case mismatches)  ─────────────────────────────────┐
US-02  Fix S→C lengths pipeline          ────────────────┐                 │
                                                          ▼                 ▼
                                                    US-03 pkg/session ──► US-04 pkg/fsm
                                                          │
                                                          ▼
                                                    US-05 golden tests + benchmarks
US-06  Fix flex array parser             ─────────────── (independent, Phase 2 prep)
US-07  Fix Category C wrong names        ─────────────── (independent, Phase 2 prep)
```

**US-01 and US-02 must complete before US-03.**  
**US-03 must complete before US-04.**  
**US-04 must complete before US-05.**  
US-06 and US-07 are independent and can run in parallel with US-03/04/05.

---

## US-01 — Fix Category B: SemanticDB Field Name Case Mismatches

### Problem

626 `field_mapping` entries in `semantics/mappings.yaml` reference rAthena struct
fields using the wrong case. The codegen does a case-sensitive lookup against the
GCC-parsed struct layout, so all 626 entries silently produce:

```
// e.Speed: field Speed not found in layout
```

when the rAthena struct field is `speed`. This causes 1,787 of 2,568 field-read
skips across 186 decode files affecting 190 semantic actions.

The correct rAthena field names are known exactly from GCC preprocessor output.
This is a pure data correction — no architecture changes required.

### Root Cause

The SemanticDB was populated from OpenKore's naming conventions (PascalCase) rather
than rAthena's C struct field names (camelCase/lowercase). The 0x09FF entries were
corrected manually in a prior session but the other 624 entries were not.

### Known mapping (192 unique wrong → correct name pairs, verified against GCC output)

| SemanticDB uses | rAthena field is |
|---|---|
| `BodyState` | `bodyState` |
| `EffectState` | `effectState` |
| `ClothesColor` → covered in US-07 | `bodypalette` |
| `Speed` | `speed` |
| `Job` | `job` |
| `Sex` | `sex` |
| `Clevel` | `clevel` |
| `Head` | `head` |
| `HeadDir` | `headDir` |
| `ObjectType` | `objecttype` |
| `IsPKModeON` | `isPKModeON` |
| `Virtue` | `virtue` |
| `Honor` | `honor` |
| `Shield` | `shield` |
| `Weapon` | `weapon` |
| `Accessory` / `Accessory2` / `Accessory3` | `accessory` / `accessory2` / `accessory3` |
| `PacketType` | `packetType` |
| `PacketLength` | `packetLength` (or `packetLen`) |
| … 174 more, full list derivable programmatically from GCC output |

Complete authoritative mapping: run the classification script in `validation/` at
`packets_struct.hpp` PACKETVER=20181121 and cross-reference the SemanticDB.

### Implementation

Write a standalone Go program `internal/tools/fix_fieldnames/main.go` that:

1. GCC-preprocesses `packets_struct.hpp` at PACKETVER=20181121 to build a
   case-insensitive → canonical field name map (same logic as the classification
   script above).
2. Reads `semantics/mappings.yaml` directly (read-only).
3. For every `field_mapping` value that contains `packet.X` where `X` is not in
   the rAthena field set but `strings.ToLower(X)` is, replaces `packet.X` with
   `packet.<correct>` in the expression.
4. Writes all corrections to the SemanticDB via the `gokore-semantics` MCP server
   using `semantics_update_field_mapping` calls — one call per (action, packet_id,
   param_name) triple.
5. Prints a summary: N corrections applied, M actions affected, K expressions
   unchanged (already correct or Category C).

**Do not use a static alias table.** The canonical field names come from GCC output
only. The tool re-derives the mapping at runtime.

After running the tool, regenerate `pkg/decode/` and `pkg/encode/` by re-running
`go run ./internal/codegen/main.go`.

### Acceptance Criteria

- [ ] Tool runs to completion without errors
- [ ] Exactly 626 SemanticDB entries updated (verify with `semantics_validate`)
- [ ] After regeneration: `grep -c "not found in layout" pkg/decode/*.go | awk -F: '{sum+=$2} END{print sum}'`
  drops from 2,568 to ≤ 781 (Category A + C skips only)
- [ ] `go build ./...` passes
- [ ] `go test ./internal/codegen/...` passes — `TestLoaderImplCounts` still passes
  (impl/param counts unchanged; only field_mapping values change)
- [ ] `validation/phase1_gate.sh` still shows 76 PASS / 1 FAIL (expected)
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us01_fix_category_b.md` written

---

## US-02 — Fix S→C Packet Lengths Pipeline

### Problem

`clif_packetdb.hpp` only registers C→S (client-to-server) packets. The generated
`pkg/session/lengths_map.go` therefore has no entries for any S→C packet. Eight
critical Phase 1 S→C packets are missing:

| Packet ID | Name | Missing length |
|---|---|---|
| `0x0078` | `ZC_NOTIFY_STANDENTRY` (actor_exists, old) | 56–108 (varies by PACKETVER) |
| `0x0080` | `ZC_NOTIFY_VANISH` (actor_vanished) | 7 |
| `0x00B0` | `ZC_PAR_CHANGE` (stat_update) | 8 |
| `0x00B1` | `ZC_LONGPAR_CHANGE` (stat_update long) | 8 |
| `0x00BE` | `ZC_STATUS_CHANGE` (stat_update sub) | 5 |
| `0x0073` | `ZC_ACCEPT_ENTER` (accept_enter) | 11–14 (varies) |
| `0x0B1D` | `ZC_PING_LIVE` (ping_live) | to be verified |
| `0x0B1C` | `CZ_PING_LIVE` — note: C→S but absent from packetdb | to be verified |

Without these lengths `session.Feed()` cannot frame any S→C packet and will fault
on the first byte received from the server.

### Source of Truth

S→C packet lengths are derivable from **GCC struct sizes** (our primary oracle):
- Fixed-length packets: `TotalSize` from the VersionTable (already computed)
- Variable-length packets: length = -1 (read bytes [2:4] at runtime)

The VersionTable already contains the correct `TotalSize` for every struct in
`packets_struct.hpp`. For structs in `packets.hpp` and `common/packets.hpp`
(not yet in the VersionTable), we use OpenKore's `recvpackets.txt` (1,056 entries)
as a **bootstrap cross-reference only** — every length it provides must be verified
against GCC struct size where the struct exists.

### Implementation

**Step 1 — Extend codegen to emit S→C lengths from VersionTable.**

In `internal/codegen/main.go`, after the VersionTable is built, add a new pass
that walks the SemanticDB `mappings:` section. For each packet entry:

- Look up the `rathena_struct` name in the VersionTable.
- If found: emit a `LengthBreakpoint` for each version range where `TotalSize`
  changes. Fixed-size structs get their `TotalSize`; variable-length structs
  (those whose VersionTable entry contains a flex array field) get `-1`.
- Merge these entries into `mapBreakpoints` alongside the existing C→S entries.

**Step 2 — Add recvpackets.txt as a fallback for structs not yet in VersionTable.**

For packet IDs whose struct is in `packets.hpp` or `common/packets.hpp` (not yet
processed by the pipeline), parse `recvpackets.txt` from the OpenKore tables
directory (path configurable via `--openkore` flag) and emit baseline length
entries. These entries are superseded the moment the full `packets.hpp` pipeline
is added.

Mark fallback entries with a `// source: recvpackets.txt` comment in the generated
file so they are visually distinct from GCC-derived entries.

**Step 3 — Verify Phase 1 lengths against GCC.**

Run `validation/struct_layout.sh` for every Phase 1 S→C packet to confirm the
generated length matches the struct's `TotalSize` at each PACKETVER breakpoint.
Failures are blocking — do not proceed until all pass.

### Acceptance Criteria

- [ ] All 8 missing Phase 1 S→C packet IDs present in `lengths_map.go`
- [ ] `0x0078` has PACKETVER-conditional entries matching the 8 struct breakpoints
  (56, 60, 63, 65, 74, 102, 104, 108 bytes at the correct PACKETVER thresholds)
- [ ] `0x0080` = 7 bytes (verified: `PACKET_ZC_VANISH` via recvpackets.txt; struct
  in `packets.hpp` — annotate as fallback)
- [ ] `0x00B0` = 8, `0x00B1` = 8, `0x00BE` = 5 (verified against GCC)
- [ ] `0x0B1D` and `0x0B1C` lengths verified against GCC or recvpackets.txt
- [ ] `go build ./...` passes
- [ ] `go test ./internal/codegen/...` passes
- [ ] `validation/phase1_gate.sh` still 76 PASS / 1 FAIL
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us02_stoc_lengths.md` written

---

## US-03 — Implement pkg/session

### Problem

`pkg/session` currently contains only five generated files:
`lengths_map.go`, `lengths_login.go`, `lengths_char.go`, `shuffle_map.go`,
`obfuscation_keys.go`. There is no `Feed()`, no session types, no framing logic,
no handler dispatch. The library cannot process a single incoming byte.

### Design (from HLD §5, §8, §9)

Three session types: `LoginSession`, `CharSession`, `MapSession`. All share a
`sessionCore` with:

```go
type sessionCore struct {
    packetver uint32
    buf       []byte          // backing array (grows once to high-water mark)
    active    []byte          // sub-slice of buf holding unprocessed bytes
    faulted   bool
    lengths   [65536]int16    // populated from generated populateXxxLengths()
    handlers  [65536]func([]byte)
}
```

`Feed(data []byte) error` algorithm:

1. If `faulted`, return `ErrFaulted` immediately.
2. Append `data` to `active` (copy-to-front if needed, no allocation in steady state).
3. Loop:
   a. If `len(active) < 2`, break (need more bytes).
   b. Read `packetID = leU16(active, 0)`.
   c. For `MapSession` with obfuscation enabled: deobfuscate `packetID` using the
      rolling LCG key before the lookup.
   d. `length = lengths[packetID]`
   e. If `length == 0`: set `faulted = true`, return `ErrUnknownPacket`.
   f. If `length == -1`: if `len(active) < 4`, break; else
      `length = int16(leU16(active, 2))`.
   g. If `len(active) < int(length)`, break (partial frame).
   h. Call `handlers[packetID](active[:length])` if non-nil.
   i. `active = active[length:]`.
4. Copy remaining `active` bytes to front of `buf`; reset `active = buf[:remaining]`.
5. Return nil.

`MapSession` additionally exposes:
```go
func (s *MapSession) EnableObfuscation(k0, k1, k2 uint32)
func (s *MapSession) Register(packetID uint16, fn func([]byte))
```

Handler registration wires generated decode functions to canonical callbacks:
```go
s.Register(0x09FF, func(data []byte) {
    e := decode.ActorExists_0x09FF(data, s.packetver)
    if s.onActorExists != nil { s.onActorExists(e) }
})
```

### Acceptance Criteria

- [ ] `pkg/session/core.go` — `sessionCore`, `Feed()`, `ErrUnknownPacket`, `ErrFaulted`
- [ ] `pkg/session/login.go` — `NewLoginSession`, `LoginSession`, `Feed`, `Register`
- [ ] `pkg/session/char.go` — `NewCharSession`, `CharSession`, `Feed`, `Register`
- [ ] `pkg/session/map.go` — `NewMapSession`, `MapSession`, `Feed`, `Register`,
  `EnableObfuscation`
- [ ] `pkg/session/obfuscation.go` — LCG key state + XOR logic (from HLD §10)
- [ ] All session types have table-driven unit tests using `net.Pipe` or
  hand-crafted byte slices; at minimum:
  - Feed with a single complete frame → handler fires exactly once
  - Feed with a split frame (two calls) → handler fires once on second call
  - Feed with two concatenated frames → handler fires twice
  - Feed with unknown packet ID → returns `ErrUnknownPacket`, subsequent calls
    return `ErrFaulted`
  - Feed with a variable-length frame → correct length read from bytes [2:4]
- [ ] `go test -bench=. -benchmem ./pkg/session/` → 0 allocs/op for
  `BenchmarkFeed_SmallFixedPacket`
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us03_pkg_session.md` written

---

## US-04 — Implement pkg/fsm

### Problem

`pkg/fsm` does not exist. Without it there is no way to drive the rAthena
login → char select → map entry sequence. goKore cannot connect to a server.

### Design (from HLD §4)

```go
package fsm

type Dialer func(ctx context.Context, addr string) (net.Conn, error)

type Config struct {
    ServerConfig  ServerConfig
    Packetver     uint32
    StepTimeout   time.Duration  // applied via conn.SetDeadline before each read
}

type FSM struct { cfg Config; dialer Dialer }

func New(cfg Config, dialer Dialer) *FSM

// Connect drives the full login→char→map sequence synchronously in the caller's
// goroutine. Calls OnReady when the MapSession is ready; calls OnFailed on any error.
func (f *FSM) Connect(ctx context.Context, callbacks Callbacks) error
```

The FSM drives three sequential sub-sessions internally. It uses the session types
from US-03 with `net.Pipe`-compatible `io.ReadWriter` inputs. Each step:

1. **Login**: send `CA_LOGIN` (0x0064) or `CA_LOGIN2` depending on packetver;
   wait for `0x0069` / `0x0AC4` (accept) or `0x006A` / `0x083E` (refuse).
2. **Char**: send `CH_ENTER` (0x0065 or 0x0275); wait for `0x006B` / `0x099D`
   (char list) and `0x0081` / `0x0AC5` (map server info). Call
   `OnCharList(chars)` → wait for `uint8` slot. Send `CH_SELECT_CHAR` (0x0066).
3. **Map**: send `CZ_ENTER` (0x0436); wait for `0x0073` / `0x0A18` / `0x02EB`
   (map enter accept). Send `CZ_NOTIFY_ACTORINIT` (0x007D) + shuffled ping.
   Call `OnReady(mapSession, conn)`.

`StepTimeout` is applied before each blocking read via `conn.SetDeadline`.
The FSM never spawns goroutines. All I/O is synchronous in the caller's goroutine.

### Acceptance Criteria

- [ ] `pkg/fsm/fsm.go` — `FSM`, `New`, `Connect`, `Config`, `Callbacks`
- [ ] `pkg/fsm/fsm_test.go` — full login→char→map sequence tested with `net.Pipe`
  stubs; the stub writes scripted server responses on the pipe and asserts the FSM
  sends the correct client packets in the correct order
- [ ] Step timeout tested: stub delays past `StepTimeout` → `Connect` returns error
- [ ] Login refused tested: stub sends `0x006A` → `OnFailed` fires
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] `go test -race ./pkg/fsm/` passes (no data races)
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us04_pkg_fsm.md` written

---

## US-05 — Byte-Level Golden Tests and 0-Alloc Benchmarks for Phase 1 Decode

### Problem

No `pkg/decode/` or `pkg/events/` file has a single test. The 0 allocs/op invariant
is unverified for all generated decode functions. There is no evidence that the
generated field reads produce correct values for real rAthena packet bytes.

### Design

**Golden tests**: for each Phase 1 decode function, construct a byte slice that
exactly matches the rAthena struct layout (field by field, using the GCC-verified
offsets from `validation/struct_layout.sh dump`), then assert every decoded field
has the expected value.

Example for `ActorExists_0x09FF` at PACKETVER=20181121 (struct size 108 bytes):

```go
func TestActorExists_0x09FF_GoldenDecode(t *testing.T) {
    // Construct 108 bytes matching packet_idle_unit layout at 20181121
    // (verified: GID at offset 9 size 4, PosDir at offset 63 size 3, etc.)
    data := make([]byte, 108)
    binary.LittleEndian.PutUint16(data[0:], 0x09FF) // PacketType
    binary.LittleEndian.PutUint16(data[2:], 108)    // PacketLength
    data[4] = 5                                       // objecttype = MOB
    binary.LittleEndian.PutUint32(data[5:], 1001)    // AID
    binary.LittleEndian.PutUint32(data[9:], 2002)    // GID
    // ... all fields at their GCC-verified offsets ...
    
    e := decode.ActorExists_0x09FF(data, 20181121)
    assert.Equal(t, uint32(2002), e.ID)    // GID maps to ID
    assert.Equal(t, uint32(1001), e.CharID) // AID maps to CharID (after US-01)
    // ...
}
```

Offsets come from `struct_layout.sh dump`, not from the generated code itself —
this ensures we catch codegen bugs.

**Benchmarks** (in `pkg/decode/benchmarks_test.go`):

```go
func BenchmarkActorExists_0x09FF(b *testing.B) {
    data := makeActorExists0x09FF()
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _ = decode.ActorExists_0x09FF(data, 20181121)
    }
}
```

Must show 0 allocs/op. If any field produces an allocation (e.g. `[]byte` slice
returned from a flex array, `string` conversion), it must be noted in `KNOWN_ISSUES.md`
with a resolution plan.

### Phase 1 decode functions to cover

| Decode function | Packet ID | Struct | Priority |
|---|---|---|---|
| `ActorExists_0x09FF` | 0x09FF | `packet_idle_unit` | P0 |
| `ActorExists_0x0078` | 0x0078 | `packet_idle_unit` | P0 |
| `ActorMoved_0x09DB` | 0x09DB | `packet_unit_walking` | P0 |
| `ActorMoved_0x007B` | 0x007B | `packet_unit_walking` | P0 |
| `ActorConnected_0x09FE` | 0x09FE | `packet_spawn_unit` | P0 |
| `ActorConnected_0x0079` | 0x0079 | `packet_spawn_unit` | P0 |
| `StatUpdate_0x00B0` | 0x00B0 | `PACKET_ZC_PAR_CHANGE` | P0 |
| `AcAcceptLogin_0x0069` | 0x0069 | `PACKET_AC_ACCEPT_LOGIN` | P0 |

### Acceptance Criteria

- [ ] Golden test for every P0 decode function at all PACKETVER breakpoints
- [ ] Every golden test constructs bytes from GCC-verified offsets, not from the
  generated code
- [ ] `go test ./pkg/decode/` — all tests pass
- [ ] `go test -bench=. -benchmem ./pkg/decode/` — 0 allocs/op for all P0 benchmarks
- [ ] `go test -bench=. -benchmem ./pkg/session/` — 0 allocs/op for
  `BenchmarkFeed_SmallFixedPacket` and `BenchmarkFeed_ActorExists_0x09FF`
- [ ] Any non-zero alloc identified, explained in `docs/KNOWN_ISSUES.md`, and
  given a resolution plan
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us05_golden_tests.md` written

---

## US-06 — Fix Flexible Array Member Parsing (Category A)

### Problem

313 field-read skips are caused by `TYPE field[]` (C flexible array member) fields
that the struct parser silently drops. The parser's `reArrayField` regex requires
`[EXPR]` to be non-empty; `[]` causes the line to fall through and be discarded.
This truncates the parsed struct at the last fixed field, making the flex field
unreachable in the codegen.

Affected structs include: `PACKET_ZC_NOTIFY_CHAT` (`.Message[]`),
`PACKET_ZC_ACK_PLAYER_AID_IN_RANGE` (`.AID[]`), `PACKET_ZC_SKILL_SELECT_REQUEST`
(`.skillIds[]`), `PACKET_ZC_EQUIPMENT_EFFECT` (`.effects[]`), and 21 others.

Three sub-patterns exist:
- **Pattern 1** — `char field[]`: variable-length string payload
- **Pattern 2** — `T field[]` (primitive): variable-length array of `uint16`,
  `int16`, or `uint32` elements
- **Pattern 3** — `struct SubType field[]`: variable-length array of sub-structs

### Implementation

1. Add `IsFlex bool` and `ElementSize int` to `Field` in
   `internal/codegen/preprocess/types.go`. No breaking change — existing fixed-array
   fields keep `IsFlex=false`.

2. Add `reFlexField` regex to `internal/codegen/preprocess/parser.go`:
   ```
   ^(?:struct\s+)?(\w+)\s+(\w+)\[\]\s*$
   ```
   When matched: set `IsFlex=true`, `ArrayLen=-1`,
   `Offset=currentOffset` (= total fixed-prefix size),
   `Size=0` (unknown until runtime).
   For `char`: `ElementSize=1`.
   For primitive types: `ElementSize=typeSizes[typ]`.
   For `struct SubType`: `ElementSize=0` initially; resolved in a second pass
   once all struct sizes are known (or left as 0 for manual SemanticDB annotation).

3. Update `fieldReadExpr` in `internal/codegen/gen/decode.go`: when `f.IsFlex`,
   emit `data[f.Offset:]` unconditionally. The SemanticDB `field_mapping` expression
   determines how the caller interprets the slice (`string(data[off:])`,
   `[]uint16`, etc.).

4. Update `pkg/packing` with typed helpers:
   - `DecodeFlexString(payload []byte) string`
   - `DecodeFlexUint16Array(payload []byte) []uint16`
   - `DecodeFlexUint32Array(payload []byte) []uint32`

   Note: these helpers allocate. They are not in the zero-alloc hot path — flex-array
   packets (chat, skill lists, item lists) are inherently variable-length and cannot
   be stack-allocated. Document this in `KNOWN_ISSUES.md`.

### Acceptance Criteria

- [ ] `TestParseStructBody_FlexArray` passes: a struct with `char msg[]` parses
  correctly with `IsFlex=true`, `Offset=<fixed prefix size>`, `ElementSize=1`
- [ ] After regeneration: Category A skip count drops from 313 to 0
- [ ] `go test ./internal/codegen/preprocess/` — all existing tests still pass
- [ ] `go build ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us06_flex_array.md` written

---

## US-07 — Fix Category C: Wrong Field Names (OpenKore → rAthena)

### Problem

468 field-read skips are caused by 39 unique field names in the SemanticDB that do
not exist in any rAthena struct under any case — they are OpenKore-derived names
used in older packet implementations:

| SemanticDB uses | rAthena field is | Affected packets |
|---|---|---|
| `HairColor` | `headpalette` | 0x0078, 0x01D8, 0x007B, 0x01DA, 0x022C, 0x0079, 0x01D9, 0x02ED |
| `ClothesColor` | `bodypalette` | same packets |
| `HairStyle` | `head` | same packets |
| `HeadBottom` | `accessory` | same packets |
| `HeadMid` | `accessory3` | same packets |
| `HeadTop` | `accessory2` | same packets |
| `SourceID` | `AID` (check per-struct) | `PACKET_ZC_NOTIFY_SKILL`, `PACKET_ZC_USE_SKILL` |
| `Grade` | field absent pre-20181121 — map to `0` | `PACKET_ZC_ADD_EXCHANGE_ITEM` |
| … 31 more | verify per-struct against GCC output | various |

Each mapping requires GCC verification: run `struct_layout.sh dump` for the
specific struct at the relevant PACKETVER to confirm the correct rAthena field name
before updating the SemanticDB.

### Implementation

For each of the 39 wrong names:
1. Run `struct_layout.sh dump <header> <struct_name> <packetver>` to find the
   correct rAthena field name.
2. Update all affected SemanticDB `field_mapping` entries via the MCP server
   (`semantics_update_field_mapping`).
3. For fields that are genuinely absent in older struct versions (e.g. `Grade`
   before a PACKETVER threshold): set `field_mapping` value to `"0"` (zero literal)
   for those implementations.

**Do not patch the codegen.** All corrections belong in the SemanticDB.

### Acceptance Criteria

- [ ] All 39 wrong field names resolved via GCC verification, not by assumption
- [ ] Each correction documented with the GCC command used and output observed
- [ ] After regeneration: Category C skip count drops from 468 to 0
- [ ] `go test ./internal/codegen/...` passes
- [ ] `go build ./...` passes
- [ ] `validation/phase1_gate.sh` still 76 PASS / 1 FAIL
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us07_category_c.md` written

---

## Exit Criteria for EPIC-00

EPIC-00 is complete when all of the following are true:

1. `go build ./...` — clean
2. `go test ./...` — all pass, including pkg/decode golden tests and pkg/session +
   pkg/fsm unit tests
3. `go test -bench=. -benchmem ./pkg/...` — 0 allocs/op for all Phase 1 decode and
   Feed benchmarks
4. `validation/phase1_gate.sh` — 76 PASS / 1 FAIL (the 1 fail remains intentional)
5. `grep -r "not found in layout" pkg/decode/*.go | wc -l` — ≤ 0 after US-01,
   US-06, US-07 complete (or each remaining skip has an entry in `KNOWN_ISSUES.md`
   with a tracked resolution)
6. The FSM can complete a full login → char select → map entry sequence against a
   `net.Pipe` stub server that replays scripted rAthena responses
7. Worklog written for every story

---

## What This Epic Does NOT Cover

These are explicitly deferred to a later epic:

- `packets.hpp` structs not yet in the VersionTable (e.g. `PACKET_ZC_CLOSE_STORE`)
- `common/packets.hpp` structs fully wired into the VersionTable
- Integration test against a real rAthena Docker instance (HLD §layer Tier B)
- Phase 2 actions (the ~400+ non-FSM packets beyond the Phase 1 scope)
- `pkg/fsm` reconnect / retry logic
- goKore integration (`internal/network/` replacement)
