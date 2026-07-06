# Work Log 0086 — Fix EncodeRepairItem PACKETVER-conditional wire layout (issue #7)

**Date**: 2026-07-06
**Type**: Bug fix — PACKETVER-conditional encoder emitted wrong wire layout
**Scope**: `pkg/send/repair_item.go`, `pkg/encode/repair_item.go`,
          `pkg/encode/register.go`, `pkg/encode/repair_item_test.go` (new)
**Severity**: BLOCKING (goKore) — sending a 15-byte packet at production
              PACKETVER (20200401) would be rejected by the rAthena map
              server (server expects 26 bytes for `PACKET_CZ_REQ_ITEMREPAIR2`)
              or worse, trigger the anti-exploit guard in
              `clif_parse_RepairItem` (`sd->menuskill_id != BS_REPAIRWEAPON`).
**Reference**: GitHub issue #7 —
  "EncodeRepairItem emits wrong wire layout at PACKETVER >= 20181121
   (15 bytes, server expects 25)"

---

## Problem

`EncodeRepairItem` (`pkg/encode/repair_item.go`, codegen output) always emitted
a fixed `[15]byte` packet:

```go
func EncodeRepairItem(req send.RepairItem, packetver uint32) [15]byte {
    var p [15]byte
    p[0] = 0xfd; p[1] = 0x01
    leU16Put(p[2:], uint16(req.Index))   // rAthena: index
    leU16Put(p[4:], req.ItemId)          // rAthena: itemId
    p[6] = req.Refine                    // rAthena: refine
    copy(p[7:], req.Card)                // rAthena: card
    _ = packetver                        // ← packetver discarded
    return p
}
```

`packetver` was discarded (`_ = packetver`). No PACKETVER branching existed.
This is only correct for PACKETVER < 20181121.

`send.RepairItem` used `uint16 ItemId` and `[]byte Card`, which cannot even
represent the wide (uint32) field values used at production packetvers.

---

## Pre-Implementation Gate (MANDATORY)

### GCC verification — struct ground truth

Source files in `~/personal/rathena` (resolved to `/workspace/rathena` in this
environment):

- `src/map/packets_struct.hpp:410-416` — `EQUIPSLOTINFO`
- `src/map/packets_struct.hpp:2901-2934` — `REPAIRITEM_INFO1` / `REPAIRITEM_INFO2`
- `src/map/packets_struct.hpp:2937-2948` — `PACKET_CZ_REQ_ITEMREPAIR1` / `PACKET_CZ_REQ_ITEMREPAIR2`
- `src/map/clif_packetdb.hpp:256` — registration of `HEADER_CZ_REQ_ITEMREPAIR1`
- `src/map/clif_packetdb.hpp:1975-1978` — registration of `HEADER_CZ_REQ_ITEMREPAIR2` (gated)
- `src/map/clif.cpp:13265-13287` — `clif_parse_RepairItem` dispatcher

Command run (preprocess_check.sh at multiple packetvers):

```bash
for PV in 20180307 20181120 20181121 20191223 20191224 20200401 20200916; do
  g++ -E -P -DPACKETVER=$PV -DPACKETVER_MAIN_NUM=$PV \
      -I$RATHENA/src -I$RATHENA/src/map -I$RATHENA/src/common \
      $RATHENA/src/map/packets_struct.hpp
done
```

Result (selected; full table below):

```
PV=20180307  EQUIPSLOTINFO { uint16 card[4]; }      (8 bytes)
             REPAIRITEM_INFO1 { int16 index; uint16 itemId; uint8 refine; EQUIPSLOTINFO slot; }
             PACKET_CZ_REQ_ITEMREPAIR1 present; PACKET_CZ_REQ_ITEMREPAIR2 NOT present

PV=20181121  EQUIPSLOTINFO { uint32 card[4]; }      (16 bytes)  ← WIDE
             REPAIRITEM_INFO1 { int16 index; uint32 itemId; uint8 refine; EQUIPSLOTINFO slot; }
             PACKET_CZ_REQ_ITEMREPAIR1 present; PACKET_CZ_REQ_ITEMREPAIR2 NOT present

PV=20191223  same wide structs as 20181121
             PACKET_CZ_REQ_ITEMREPAIR1 present; PACKET_CZ_REQ_ITEMREPAIR2 NOT present

PV=20191224  REPAIRITEM_INFO2 { int16 index; uint32 itemId; EQUIPSLOTINFO slot; uint8 refine; uint8 grade; }
             PACKET_CZ_REQ_ITEMREPAIR1 present; PACKET_CZ_REQ_ITEMREPAIR2 present (NEW)

PV=20200401  same as 20191224 (goKore production target)
```

### Empirical size verification (C compiler, ground truth)

Mirrored the rAthena structs in a standalone `.c` file and compiled with
`gcc -Wall` using `__attribute__((packed))` (matches rAthena exactly):

```
EQUIPSLOTINFO_NARROW  = 8
EQUIPSLOTINFO_WIDE    = 16
REPAIRITEM_INFO1_NARROW = 13
REPAIRITEM_INFO1_WIDE   = 23
REPAIRITEM_INFO2        = 24

PACKET_CZ_REQ_ITEMREPAIR1 (narrow, pv<20181121) = 15  ✓
PACKET_CZ_REQ_ITEMREPAIR1 (wide, 20181121<=pv<20191224) = 25  ✓
PACKET_CZ_REQ_ITEMREPAIR2 (pv>=20191224) = 26  ✓

REPAIR1_WIDE field offsets (relative to REPAIRITEM_INFO1, not packet):
  index=0 itemId=2 refine=6 slot=7
REPAIR2 field offsets:
  index=0 itemId=2 slot=6 refine=22 grade=23   ← slot BEFORE refine
```

### PACKETVER boundary reconciliation (the issue's open question)

The issue flagged that the struct definition and the clif.cpp dispatcher use
different boundaries:

- Struct definition (`packets_struct.hpp:2936,2942`): `#if PACKETVER >= 20191224`
- Dispatcher cast (`clif.cpp:13271`): `#if PACKETVER_MAIN_NUM >= 20200916 || PACKETVER_RE_NUM >= 20200724`

The **authoritative** boundary for the *client* is the packet registration in
`clif_packetdb.hpp` (the binding contract for which packet IDs the server
accepts at boot):

- `clif_packetdb.hpp:256` registers `HEADER_CZ_REQ_ITEMREPAIR1` (0x01FD) with
  **NO PACKETVER guard** → always accepted.
- `clif_packetdb.hpp:1975-1978` registers `HEADER_CZ_REQ_ITEMREPAIR2` (0x0B66)
  **only when `PACKETVER >= 20191224`** → matches the struct definition.

The dispatcher's `#if >= 20200916` only affects which struct pointer the
server uses to *cast* the bytes — but the only field actually read is
`p->item.index` (at offset 2 in BOTH structs), so both packets work at all
packetvers in practice. The dispatcher boundary is a server-side ambiguity
that does NOT affect the client's wire-format choice.

**Decision**: emit `0x0B66` starting at `PACKETVER >= 20191224` to match the
packetdb registration boundary (matches the issue's recommended fix and the
official kRO client's behavior).

### Shuffle and obfuscation check

```
grep -n "0x01fd\|0x01FD\|0x0b66\|0x0B66\|ITEMREPAIR" \
     src/map/clif_shuffle.hpp src/map/clif_packetdb.hpp
```

Result: NOT in `clif_shuffle.hpp` (no C→S shuffle applied — these IDs use
literal `HEADER_*` constants). C→S packets are not subject to
`PACKET_OBFUSCATION` (S→C only). The existing encoder correctly applied
neither.

### DB query (semantics/mappings.yaml — read for verification only)

The semantics DB has only sparse entries for these packets:

```yaml
# semantics/mappings.yaml:3637
repair_item:
    name: repair_item
    description: Request weapon repair at a repair NPC
    canonical_params: []          # ← empty
    implementations:
        - packet_id: "0x01FD"
          packetver_range: [null, null]   # ← unbounded
          struct_name: SYNTH_CZ_REQ_ITEMREPAIR1
          field_mapping: {}       # ← empty

# semantics/mappings.yaml:2059
cz_req_itemrepair2:
    implementations:
        - packet_id: "0x0B66"
          packetver_range: [null, null]
          struct_name: PACKET_CZ_REQ_ITEMREPAIR2
          field_mapping: {}
```

`canonical_params` and `field_mapping` are empty — the DB has no field-level
metadata to conflict with GCC output. The DB also has no PACKETVER branching
for `repair_item` (single implementation with unbounded range), which is why
the codegen produced a single-layout encoder. The DB struct name
`SYNTH_CZ_REQ_ITEMREPAIR1` (synthesized) suggests the DB was authored when
rAthena had no real `PACKET_CZ_REQ_ITEMREPAIR1` definition; rAthena now has
one at `packets_struct.hpp:2944`, but the DB has not been reconciled.

**DB cleanup is a separate non-blocking task.** Per README rule 9, DB edits
go through the `gokore-semantics` MCP server, which is not available in this
environment. No DB field-level conflicts existed to reconcile; GCC output is
the authoritative ground truth used for implementation.

`./validation/db_validate.sh` was not run — it requires the MCP server.
Noted as a known environment limitation.

---

## Fix (TDD)

### 1. Tests FIRST (`pkg/encode/repair_item_test.go`)

Tests written before implementation; all initially failed to compile against
the old `[15]byte` return type and old struct shape (TDD red):

- `TestEncodeRepairItem_PacketIDAndLength_AllVariants` — 9 subtests covering
  narrow, wide, and REPAIR2 regimes at boundary packetvers (20180307, 20181120,
  20181121, 20190000, 20191223, 20191224, 20200401, 20200916)
- `TestEncodeRepairItem_GoldenBytes_Narrow` — hand-synthesized 15-byte wire
  layout byte-by-byte from the rAthena struct definition
- `TestEncodeRepairItem_GoldenBytes_Wide` — hand-synthesized 25-byte wire
  layout (the bug scenario from the issue)
- `TestEncodeRepairItem_GoldenBytes_Repair2` — hand-synthesized 26-byte wire
  layout (the production 20200401 case)
- `TestEncodeRepairItem_FieldOrder_Repair2` — confirms the critical layout
  difference: in REPAIR2, slot comes BEFORE refine, and grade is appended
- `TestEncodeRepairItem_CardNarrowing` — uint32 cards truncate to uint16 on
  narrow wire (matches rAthena EQUIPSLOTINFO behavior)
- `TestEncodeRepairItem_ItemIdNarrowing` — itemId truncation on narrow wire
- `TestEncodeRepairItem_Boundaries` — adjacent-day boundary tests
  (20181120→20181121, 20191223→20191224) to catch off-by-one errors
- `TestEncodeRepairItem_PacketIDVaries` — catches the original `_ = packetver`
  bug by asserting REPAIR1 and REPAIR2 produce different wire IDs
- `TestEncodeRepairItem_IndexAlwaysAtOffset2` — confirms `index` survives at
  offset [2..3] in all three layouts (the only field the server reads)
- Three benchmarks: `_Narrow`, `_Wide`, `_Repair2`

### 2. Updated `send.RepairItem` (`pkg/send/repair_item.go`)

```go
type RepairItem struct {
    Index  int16
    ItemId uint32       // narrowed to uint16 on wire at pv < 20181121
    Refine uint8
    Card   [4]uint32    // narrowed to uint16[4] on wire at pv < 20181121
    Grade  uint8        // REPAIR2 only (pv >= 20191224); ignored earlier
}
```

Field types hold the widest representation needed; the encoder narrows on
the wire for the narrow variant (matches rAthena uint16 encoding). `Grade`
is only emitted for REPAIR2.

### 3. Reimplemented `EncodeRepairItem` (`pkg/encode/repair_item.go`)

Marked as "Manually implemented" (no longer codegen output). Returns `[]byte`
(variable-length, since total size is 15 / 25 / 26). Three branches:

```go
const (
    repairItemWideFieldsPV uint32 = 20181121 // itemId uint32, card uint32[4]
    repairItemRepair2PV    uint32 = 20191224 // emit 0x0B66 (REPAIRITEM_INFO2)
)

func EncodeRepairItem(req send.RepairItem, packetver uint32) []byte {
    switch {
    case packetver >= repairItemRepair2PV:
        // PACKET_CZ_REQ_ITEMREPAIR2 (0x0B66), 26 bytes
        // Layout: id(2) index(2) itemId(4) slot(16) refine(1) grade(1)
        // NOTE: slot BEFORE refine; grade appended
        var p [26]byte
        ...
    default:
        // PACKET_CZ_REQ_ITEMREPAIR1 (0x01FD), 15 or 25 bytes
        if packetver >= repairItemWideFieldsPV {
            // Wide: 25 bytes
        } else {
            // Narrow: 15 bytes (itemId/card truncate to uint16)
        }
    }
}
```

Each field write cites the rAthena field name per README rule 11
(e.g., `// rAthena: item.index`).

### 4. Updated `register.go`

Wrapper simplified (no more array slicing — `EncodeRepairItem` already returns
`[]byte`):

```go
session.RegisterSendEncoder(session.ActionRepairItem,
    func(req interface{}, pv uint32) ([]byte, error) {
        r, ok := req.(send.RepairItem)
        if !ok {
            return nil, session.ErrWrongSendType{}
        }
        return EncodeRepairItem(r, pv), nil
    },
)
```

---

## Test Results

```
$ go test ./pkg/encode/ -run TestEncodeRepairItem -count=1 -v
=== RUN   TestEncodeRepairItem_PacketIDAndLength_AllVariants
--- PASS (9 subtests)
=== RUN   TestEncodeRepairItem_GoldenBytes_Narrow          --- PASS
=== RUN   TestEncodeRepairItem_GoldenBytes_Wide            --- PASS
=== RUN   TestEncodeRepairItem_GoldenBytes_Repair2         --- PASS
=== RUN   TestEncodeRepairItem_FieldOrder_Repair2          --- PASS
=== RUN   TestEncodeRepairItem_CardNarrowing               --- PASS
=== RUN   TestEncodeRepairItem_ItemIdNarrowing             --- PASS
=== RUN   TestEncodeRepairItem_Boundaries                  --- PASS
=== RUN   TestEncodeRepairItem_PacketIDVaries              --- PASS
=== RUN   TestEncodeRepairItem_IndexAlwaysAtOffset2        --- PASS
PASS
ok      github.com/lenaxia/rathena-client/pkg/encode    0.005s
```

> **Note**: A later iteration added `TestEncodeRepairItem_NarrowingMatchesRathenaTruncation`
> (6 subtests) in response to AI review feedback — see "v0.6.8 Shipped Lifecycle"
> at the end of this log. Final count: 11 tests + 3 benchmarks.

## Benchmark

```
$ go test ./pkg/encode/ -bench=BenchmarkEncodeRepairItem -benchmem -count=3 -run='^$'
BenchmarkEncodeRepairItem_Narrow-8    42153172    28.75 ns/op    16 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Narrow-8    46202257    28.69 ns/op    16 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Narrow-8    45295533    28.36 ns/op    16 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Wide-8      39171086    32.08 ns/op    32 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Wide-8      33758932    31.23 ns/op    32 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Wide-8      34135578    30.35 ns/op    32 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Repair2-8   36805125    33.44 ns/op    32 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Repair2-8   41372316    30.47 ns/op    32 B/op    1 allocs/op
BenchmarkEncodeRepairItem_Repair2-8   38170507    30.87 ns/op    32 B/op    1 allocs/op
```

**1 alloc/op** — variable-length return (`[]byte` of 15/25/26 bytes). This
matches the established pattern for variable-length encoders
(`EncodeBattleChat`, `EncodeWhisper` at 1 alloc/op). The fixed-size
`register.go` wrapper also allocates 1/op when slicing a fixed array to
`[]byte` (escape on slice), so going through `session.Send` allocates once
regardless of encode signature.

Encode cost 24–33 ns/op is well within the variable-length encode range
(Whisper: 60 ns/op, BattleChat: 37 ns/op).

## Full Test Suite

```
$ go build ./...              # BUILD OK
$ go vet ./...                # VET OK
$ go test ./... -count=1
ok      github.com/lenaxia/rathena-client/internal/codegen/gen       0.012s
ok      github.com/lenaxia/rathena-client/internal/codegen/preprocess 0.004s
ok      github.com/lenaxia/rathena-client/internal/codegen/semantics  0.038s
ok      github.com/lenaxia/rathena-client/pkg/decode                  0.005s
ok      github.com/lenaxia/rathena-client/pkg/encode                  0.005s
ok      github.com/lenaxia/rathena-client/pkg/packing                 0.003s
ok      github.com/lenaxia/rathena-client/pkg/session                 0.151s

$ go test -race ./... -count=1    # ALL CLEAN (no races)
$ grep -rn '^\s*go ' pkg/ --include='*.go' --exclude='*_test.go'
# (empty — zero goroutines in production pkg/ code)
```

## Validation Scripts

```
$ RATHENA_ROOT=/workspace/rathena ./validation/preprocess_check.sh 20180307
=== preprocess_check.sh PACKETVER=20180307 ===
packets_struct.hpp ... OK (393 structs)
packets.hpp ... OK (642 structs)
common/packets.hpp ... OK (131 structs)
clif_obfuscation.hpp ... OK (1 key definitions)
EXIT=0

$ RATHENA_ROOT=/workspace/rathena ./validation/preprocess_check.sh 20200401
packets_struct.hpp ... OK (434 structs)
packets.hpp ... OK (686 structs)
common/packets.hpp ... OK (131 structs)
clif_obfuscation.hpp ... OK (1 key definitions)
EXIT=0
```

`./validation/db_validate.sh` — NOT RUN (requires `gokore-semantics` MCP
server, unavailable in this environment). DB was read directly from
`semantics/mappings.yaml` for verification; no field-level conflicts existed
to reconcile.

`./validation/phase1_gate.sh` — 2 FAILURES (Char/0x0065 CH_MAKE_CHAR,
Char/0x0081 HC_NOTIFY_ZONESVR). **Pre-existing**, unrelated to RepairItem.
Confirmed by running the gate both with and without these changes (via
`git stash`) — same 2 failures on a clean tree. Documented in README-LLM.md
line 528.

---

## Design Notes

### Why `[]byte` return instead of `[N]byte`?

The total wire size varies (15 / 25 / 26 bytes depending on PACKETVER). The
existing convention for variable-length encoders is `[]byte` (see
`pkg/encode/battle_chat.go`). Three options were considered:

1. **`[]byte` (chosen)** — clean, matches `EncodeBattleChat` pattern. 1
   alloc/op inherent to variable-length return.
2. `[26]byte` (max) with caller-side length tracking — awkward API; caller
   must know length based on packetver.
3. Split into separate `EncodeRepairItem1` / `EncodeRepairItem2` functions
   with fixed arrays — defeats the semantic-action API goal (caller would
   need to pick based on packetver).

The `register.go` wrapper for fixed-size encoders also allocates 1/op when
slicing `[N]byte` to `[]byte` for the `session.Send` contract, so going
through `session.Send` allocates once regardless. Variable-length return is
honest and consistent.

### Why one `send.RepairItem` struct instead of `RepairItem1`/`RepairItem2`?

REPAIR1 and REPAIR2 share the same semantic action ("repair this item") —
only the wire layout differs. From the user's perspective, they call
`session.Send(ms, conn, session.ActionRepairItem, send.RepairItem{...})` and
the library picks the right wire format. `Grade` is only emitted for REPAIR2;
it is silently ignored at earlier packetvers (zero value stays zero on the
wire either way).

### Why emit `0x0B66` starting at `20191224` (not `20200916`)?

See the "PACKETVER boundary reconciliation" section above. The packetdb
registration is the binding contract — `0x0B66` is only registered when
`PACKETVER >= 20191224`. The clif.cpp dispatcher's `>= 20200916` boundary
only affects which struct pointer the server uses for casting; it does not
affect which packet IDs are accepted. The only field the server reads is
`p->item.index` at offset 2 in both structs, so both packets work at all
packetvers regardless of the dispatcher's cast choice. Emitting `0x0B66` at
`20191224` matches the packetdb registration boundary, the struct definition
boundary, and the official kRO client's behavior.

### Why not regenerate via codegen?

The codegen produced the wrong output because the semantics DB entry for
`repair_item` (`mappings.yaml:3637`) has empty `field_mapping` and an
unbounded `packetver_range` — so the generator had no PACKETVER-conditional
field information and produced a single-layout encoder using the synthesized
`SYNTH_CZ_REQ_ITEMREPAIR1` struct. This is the same class of DB-driven
codegen blind spot documented in README-LLM.md (line 534):
"`clif_packetdb.hpp` hardcodes sizes as integer literals, not `sizeof()`."

The DB cleanup (adding field-level metadata and PACKETVER ranges for
`repair_item`, possibly splitting into separate `repair_item_v1`/`repair_item_v2`
implementations) is a separate non-blocking task that requires the
`gokore-semantics` MCP server (unavailable in this environment). The
long-term fix (mentioned in README) is a codegen Part 5 cross-check pass
that compares Part 1 output against VersionTable `TotalSize` at each
breakpoint. For now, the surgical manual fix to the encoder matches the
existing convention (see `actor_action.go`, `battle_chat.go` — both marked
"Manually implemented").

### Impact on existing `EncodeCzReqItemrepair2`

`EncodeCzReqItemrepair2` (for `ActionCzReqItemrepair2`) remains untouched.
It already emits the correct 26-byte `0x0B66` layout. After this fix,
`ActionRepairItem` and `ActionCzReqItemrepair2` both emit `0x0B66` at
pv >= 20191224 — but they're registered as separate semantic actions in
`pkg/session/actions.go`. This overlap is a known codegen artifact (the DB
models them as separate actions). Users should prefer `ActionRepairItem`
(the typed, packetver-abstracted API); `ActionCzReqItemrepair2` exists as a
low-level escape hatch.

---

## Files Changed

| File | Change |
|---|---|
| `pkg/send/repair_item.go` | Reimplemented as manually-maintained. New field types: `ItemId uint32`, `Card [4]uint32`, added `Grade uint8`. |
| `pkg/encode/repair_item.go` | Reimplemented as manually-maintained. Three-way PACKETVER branch; returns `[]byte` of 15/25/26 bytes. |
| `pkg/encode/register.go` | Wrapper updated — `EncodeRepairItem` now returns `[]byte` directly (no array slicing). |
| `pkg/encode/repair_item_test.go` | NEW — 11 tests + 3 benchmarks, golden bytes hand-synthesized from GCC-verified rAthena struct layouts. (10 initial; +1 from review iteration.) |
| `.github/workflows/ci.yml` | Drive-by: added `BattleChat`, `PartyChat`, `Whisper`, `RepairItem` to the 0-allocs benchmark allowlist. CI had been failing on `main` since 2026-07-04 because three legitimate variable-length encoders weren't allowlisted. |
| `CHANGELOG.md` | Added `[v0.6.8]` entry with breaking-change callout for `send.RepairItem` field types. |
| `README-LLM.md` | Updated "Last Updated" stamp to reference worklog 0086 / issue #7. |

No production-code outside `pkg/encode` and `pkg/send` was modified.
`send.RepairItem` is not referenced by any other production code in this
repo (verified via `grep -rn "send.RepairItem" --include="*.go"`), and is
not used by goKore's Go code yet (verified via grep in `gokore/`).

---

## Validation Sources

- `rathena/src/map/packets_struct.hpp:410-416` (EQUIPSLOTINFO)
- `rathena/src/map/packets_struct.hpp:2901-2934` (REPAIRITEM_INFO1/2)
- `rathena/src/map/packets_struct.hpp:2937-2948` (PACKET_CZ_REQ_ITEMREPAIR1/2)
- `rathena/src/map/clif_packetdb.hpp:256` (REPAIR1 registration, no guard)
- `rathena/src/map/clif_packetdb.hpp:1975-1978` (REPAIR2 registration, pv >= 20191224)
- `rathena/src/map/clif.cpp:13265-13287` (clif_parse_RepairItem dispatcher)
- Empirical C-compiler sizeof/offsetof verification of all three wire layouts
- GCC preprocessor output at 7 PACKETVERs spanning all three regimes

---

## v0.6.8 Shipped Lifecycle

This section documents events after the initial fix write-up above, up to and
including the v0.6.8 release. Added as an addendum so the original TDD/GCC
narrative stays intact.

### Drive-by: CI benchmark allowlist fix

While preparing the PR, I discovered CI had been **failing on every `main` push
since 2026-07-04** (4 commits, 2 days). Root cause: the 0-allocs/op benchmark
check in `.github/workflows/ci.yml` had an allowlist that predated several
variable-length encoders. The allowlist covered `GuildChat`, `NpcTalkText`,
`ShopBuy`, `ShopSell`, `PublicChat` but was missing:

- `BenchmarkEncodeBattleChat` — `EncodeBattleChat` returns `[]byte`, 1 alloc/op
- `BenchmarkEncodePartyChat` — `EncodePartyChat` returns `[]byte`, 1 alloc/op
- `BenchmarkEncodeWhisper` — `EncodeWhisper` returns `[]byte`, 1 alloc/op

All three legitimately allocate 1/op: Go's escape analysis cannot stack-allocate
the returned slice when the length depends on runtime input (text length). This
is the same pattern documented in v0.5.13 / v0.5.14 of the CHANGELOG.

Reproduced locally with the exact CI grep before fixing:

```
$ go test -bench=. -benchmem ./pkg/... | grep -P "\s[1-9]\d* allocs/op" | \
    grep -vP "<old allowlist>"
BenchmarkEncodeWhisper-8    ...    48 B/op    1 allocs/op   ← unallowlisted
BenchmarkEncodeBattleChat-8 ...    24 B/op    1 allocs/op   ← unallowlisted
BenchmarkEncodePartyChat-8  ...    24 B/op    1 allocs/op   ← unallowlisted
```

Fix: extended the allowlist regex in `ci.yml` to cover
`(BattleChat|PartyChat|Whisper|RepairItem)` and clarified the comment to make
the allowlist a forcing function (new variable-length encoder benchmarks must
be added here or CI fails loudly — desired behavior). Without this fix, the
PR's own CI would have failed on the new `BenchmarkEncodeRepairItem_*` tests.

Verified post-fix by running the exact CI grep locally: zero unexpected
non-zero-alloc lines. CI `Test` check passed on the PR after this fix.

### PR #8 — review and iteration

- **PR opened**: `fix/issue-7-encode-repair-item-packetver` → `main`, with
  comprehensive body citing rAthena file:line for every claim and the boundary
  reconciliation reasoning.
- **CI `Test` check**: passed first try (build, test, race, 0-goroutine
  invariant, benchmark allowlist with the drive-by fix).
- **AI PR review #1** (OpenCode workflow `.github/workflows/pr-review.yml`,
  run 28770981093, ~11 min): independently cloned `rathena/rathena`, traced
  every byte write against the actual C structs, **verdict APPROVE**.
  - One non-blocking suggestion: add `TestEncodeRepairItem_NarrowingMatchesRathenaTruncation`
    to confirm uint32→uint16 narrowing matches C's `uint32_t→uint16_t` truncation
    for boundary values like `0xFFFF0000` and `0x0000FFFF`.
  - One out-of-scope pre-existing note: `pkg/send` missing package-level doc
    comment (Rule 11). Reviewer explicitly said "not a blocker for this fix,
    worth a follow-up cleanup PR" — deferred.
- **Iteration (commit 2)**: added the suggested test (6 subtests covering
  `0x0000FFFF`, `0xFFFF0000`, `0xDEADBEEF`, `0x12345678`, `0x00000000`,
  `0xFFFF0001` across both `itemId` and `card[0]` truncation paths). All pass.
- **AI PR review #2** (run 28771516925): re-verified, **verdict APPROVE**
  again, noting the iteration was "good response to the prior review's
  non-blocking suggestion" and that the narrowing truncation was now
  "well-covered after commit 2".
- **Merge state**: `CLEAN` (both checks SUCCESS, mergeable).
- **Merged** via squash merge as `659f7f5` on 2026-07-06T06:22:28Z.
- **Issue #7 auto-closed** as COMPLETED via the `Fixes #7` trailer in the
  squash-merge commit body.

### Release v0.6.8

- Annotated tag `v0.6.8` created on merge commit `659f7f5` and pushed.
- Push triggered `.github/workflows/release.yml` (run 28772199573): ran
  build / test / race detector, then created the GitHub release via
  `softprops/action-gh-release@v2`.
- Release published 2026-07-06T06:25:31Z at
  https://github.com/lenaxia/rathena-client/releases/tag/v0.6.8
  (not draft, not prerelease).

### Outcome

| Item | State |
|---|---|
| Issue #7 | CLOSED (COMPLETED) |
| PR #8 | MERGED (`659f7f5`) |
| Release v0.6.8 | PUBLISHED |
| CI on `main` | Fixed (was failing since 2026-07-04) |
| Tests | 11 repair_item tests + 3 benchmarks, all pass |
| Architecture invariants | All hold (0 goroutines in `pkg/`, 0 external deps, no reflection in encode path) |
| Known follow-ups (non-blocking) | (1) `semantics/mappings.yaml` cleanup for `repair_item` via MCP; (2) codegen Part 5 cross-check pass (README line 534); (3) `pkg/send` package doc comment (reviewer note); (4) CHANGELOG gap v0.6.5–v0.6.7 (reviewer note) |
