# US-10 — Eliminate SetLength Workarounds (Fix S→C Lengths Pipeline)

**Status**: Ready for implementation  
**Created**: 2026-03-09  
**Epic**: EPIC-00 (Protocol Library Correctness & Phase 1 Completeness) — relates to US-02  
**Depends on**: nothing (standalone refactor of the codegen pipeline)  

---

## Problem

`pkg/fsm/fsm.go` contains 34 manual `SetLength` calls that exist solely because the
generated `pkg/session/lengths_map.go` (and `lengths_char.go`) are missing S→C packet
lengths. These calls are scattered across `runCharPhase()` (1 call) and
`runMapPhase()` (33 calls):

| Call site | Packet ID | Name | Length | Struct source |
|-----------|-----------|------|--------|---------------|
| `runCharPhase` | 0x0081 | HC_NOTIFY_ZONESVR | 28 | `common/packets.hpp` |
| `runMapPhase` | 0x0283 | ZC_AID | 6 | `synthetic_structs.hpp` (SYNTH_ZC_AID already exists) |
| `runMapPhase` | 0x0074 | ZC_REFUSE_ENTER | 3 | `packets.hpp` |
| `runMapPhase` | 0x0073 | ZC_ACCEPT_ENTER | 11 | `packets.hpp` |
| `runMapPhase` | 0x02EB | ZC_ACCEPT_ENTER | 13 | `packets.hpp` |
| `runMapPhase` | 0x0A18 | ZC_ACCEPT_ENTER | 14 | `packets.hpp` |
| `runMapPhase` | 0x0B18 | ZC_EXTEND_BODYITEM_SIZE | 4 | `packets_struct.hpp` |
| `runMapPhase` | 0x007F | ZC_NOTIFY_TIME | 6 | `packets.hpp` |
| `runMapPhase` | 0x0091 | ZC_NPCACK_MAPMOVE | 22 | `packets.hpp` |
| `runMapPhase` | 0x00B0 | ZC_PAR_CHANGE | 8 | `packets_struct.hpp` |
| `runMapPhase` | 0x00B1 | ZC_LONGPAR_CHANGE | 8 | `packets_struct.hpp` |
| `runMapPhase` | 0x00BD | ZC_STATUS | 44 | `packets.hpp` |
| `runMapPhase` | 0x008E | ZC_NOTIFY_CHAT echo | -1 (var) | **no struct** — `clif_packetdb.hpp` only |
| `runMapPhase` | 0x010F | ZC_SKILLINFO_LIST | -1 (var) | `packets_struct.hpp` |
| `runMapPhase` | 0x013A | ZC_ATTACK_RANGE | 4 | `packets_struct.hpp` |
| `runMapPhase` | 0x0141 | ZC_COUPLESTATUS | 14 | `packets_struct.hpp` |
| `runMapPhase` | 0x0087 | ZC_NOTIFY_PLAYERMOVE | 12 | `packets.hpp` |
| `runMapPhase` | 0x02C9 | ZC_PARTY_CONFIG | 3 | `packets_struct.hpp` |
| `runMapPhase` | 0x02D9 | ZC_CONFIG | 10 | **no struct** — `clif_packetdb.hpp` only |
| `runMapPhase` | 0x02DA | ZC_CONFIG_NOTIFY | 3 | `packets.hpp` |
| `runMapPhase` | 0x01D7 | ZC_SPRITE_CHANGE | 15 | `packets_struct.hpp` |
| `runMapPhase` | 0x099B | ZC_MAPPROPERTY_R2 | 8 | `packets_struct.hpp` |
| `runMapPhase` | 0x09E7 | ZC_NOTIFY_UNREAD_RODEX | 3 | `packets_struct.hpp` |
| `runMapPhase` | 0x0A24 | ZC_ACH_UPDATE | 66 | **no struct** — `clif_packetdb.hpp` only |
| `runMapPhase` | 0x0ACB | ZC_LONGLONGPAR_CHANGE | 12 | `packets_struct.hpp` |
| `runMapPhase` | 0x0ADE | ZC_OVERWEIGHT_PERCENT | 6 | **no struct** — `clif_packetdb.hpp` only |
| `runMapPhase` | 0x0ADF | ZC_ACK_REQNAMEALL_NPC | 58 | `packets_struct.hpp` |
| `runMapPhase` | 0x0A9B | ZC_EQUIPSWITCH_LIST | -1 (var) | **no struct** — `clif_packetdb.hpp` only |
| `runMapPhase` | 0x0B08 | ZC_INVENTORY_START | -1 (var) | `packets_struct.hpp` |
| `runMapPhase` | 0x0B09 | ZC_INVENTORY_ITEMLIST_NORMAL | -1 (var) | `packets_struct.hpp` |
| `runMapPhase` | 0x0B0A | ZC_INVENTORY_ITEMLIST_EQUIP | -1 (var) | `packets_struct.hpp` |
| `runMapPhase` | 0x0B0B | ZC_INVENTORY_END | 4 | `packets_struct.hpp` |
| `runMapPhase` | 0x0B1B | ZC_NOTIFY_ACTORINIT | 2 | `packets_struct.hpp` |
| `runMapPhase` | 0x0B20 | ZC_SHORTCUT_KEY_LIST | 271 | `packets_struct.hpp` |
| `runMapPhase` | 0x0A23 | ZC_ALL_ACH_LIST | -1 (var) | **no struct** — `clif_packetdb.hpp` only |

These calls are a maintenance liability. The FSM should not know about packet lengths;
that belongs to the generated session layer.

---

## Root Cause Analysis

Auditing the 34 packet IDs against rAthena source reveals three distinct gaps:

### Gap A — `packets_struct.hpp` structs present but not fed to lengths generation (20 packets)

Structs exist in `packets_struct.hpp` and the VersionTable pipeline already computes
their `TotalSize` at every PACKETVER breakpoint. However `genLengths` (Step 3 in
`main.go`) runs **before** the VersionTable is built (Step 5), so the S→C struct sizes
are never joined into the lengths output. No new parsing work required — only pipeline
reordering and a join pass.

Affected IDs: `0x0B18, 0x00B0, 0x00B1, 0x010F, 0x013A, 0x0141, 0x02C9, 0x0ACB,
0x0ADF, 0x0B08, 0x0B0B, 0x0B1B, 0x0B20, 0x01D7, 0x099B, 0x09E7, 0x0B09, 0x0B0A,
0x0B0A, 0x0B0B`

### Gap B — `packets.hpp` and `common/packets.hpp` structs not yet fed to map lengths (8 packets)

`buildLoginCharLengthBreakpoints` already processes `common/packets.hpp` for login/char
lengths using `SourceCommonPackets` and `ParseCommonPacketHeaders`. The exact same
mechanism exists for `SourcePackets` (map `packets.hpp`) but is **not wired into map
lengths generation**. The pipeline already has `SourcePackets` in `runner.go` and
`PacketsHPPStub` in `Config` — connecting them is the remaining work.

Affected IDs: `0x0074, 0x0073, 0x02EB, 0x0A18, 0x007F, 0x0091, 0x00BD, 0x0087,
0x02DA, 0x0081` (plus `0x0081` from `common/packets.hpp` for the char server)

### Gap C — 7 structless packets (raw `WFIFOW` only, no rAthena struct definition)

These packets are sent via raw `WFIFOW(fd, offset) = value` macros in `clif.cpp`.
No `DEFINE_PACKET_HEADER` or struct exists in any rAthena header. The pipeline already
handles this class of packet via `synthetic_structs.hpp` + `InjectSyntheticStructs` —
`SYNTH_ZC_AID` (0x0283) is already there as a proven example.

The fix is to add synthetic struct definitions for the remaining 6:

| Packet ID | Name | Length | Evidence |
|-----------|------|--------|----------|
| 0x0283 | ZC_AID | 6 | `SYNTH_ZC_AID` **already exists** in `synthetic_structs.hpp` |
| 0x008E | ZC_NOTIFY_CHAT echo | -1 (var) | `clif_packetdb.hpp:42` `packet(0x008e,-1)` |
| 0x02D9 | ZC_CONFIG | 10 | `clif_packetdb.hpp:920` `packet(0x02d9,10)`; `clif.cpp:10294` `WFIFOW(fd,0)=0x2d9` |
| 0x0A24 | ZC_ACH_UPDATE | 66 | `clif_packetdb.hpp:1767` `packet(0x0A24,66)`; `clif.cpp:21849` `WFIFOW(fd,0)=0xa24` |
| 0x0ADE | ZC_OVERWEIGHT_PERCENT | 6 | `clif_packetdb.hpp:1891` `packet(0x0ADE,6)` (pv >= 20171025) |
| 0x0A9B | ZC_EQUIPSWITCH_LIST | -1 (var) | `clif_packetdb.hpp:1860` `packet(0x0A9B,-1)`; `clif.cpp:22232` |
| 0x0A23 | ZC_ALL_ACH_LIST | -1 (var) | `clif_packetdb.hpp:1766` `packet(0x0A23,-1)`; `clif.cpp:21836` |

Once synthetic structs exist, `InjectSyntheticStructs` automatically makes them
available to the VersionTable, and the Gap A join pass handles the rest.

---

## Design

No fallbacks. No `recvpackets.txt`. Every length is derived from either a real rAthena
struct (GCC-preprocessed) or a hand-authored synthetic struct whose layout is verified
against `clif_packetdb.hpp` `packet()` registrations and `clif.cpp` `WFIFOW` call sites.

### Step 1 — Add 6 synthetic struct definitions to `synthetic_structs.hpp`

Add one `SYNTH_ZC_*` struct for each of the 6 remaining structless packets, following
the existing `SYNTH_ZC_AID` pattern. Each must include:
- Layout comment citing `clif_packetdb.hpp` line and `clif.cpp` call site
- `__attribute__((packed))`
- PACKETVER guard note where applicable (e.g. 0x0ADE requires `pv >= 20171025`)

The synthetic structs are version-invariant: they are injected with `MinVer=20030000,
MaxVer=0` by `InjectSyntheticStructs`. PACKETVER-conditionality (i.e. 0x0ADE appears
only at pv >= 20171025) is enforced by the SemanticDB `mappings:` entry, not the struct.

Variable-length structs (0x008E, 0x0A9B, 0x0A23) get a flex array member so the
parser marks them with `IsFlex=true` and the VersionTable records `TotalSize=-1`:

```cpp
struct SYNTH_ZC_EQUIPSWITCH_LIST {
    int16  PacketType;
    uint16 PacketLength;
    uint8  items[]; // variable-length, -1 sentinel
} __attribute__((packed));
```

### Step 2 — Wire `packets.hpp` into map lengths generation

Add `buildMapStocLengthBreakpoints(cfg)` to `internal/codegen/main.go` using the
existing `SourcePackets` source and the same `ParseCommonPacketHeaders` logic already
used for `buildLoginCharLengthBreakpoints`. Extract ZC_* entries (and any other
S→C prefixes as needed) and merge the resulting breakpoints into the map lengths output.

This reuses: `preprocess.Preprocess(cfg, SourcePackets, pv)`, `preprocess.ExtractStructs`,
`preprocess.ParseCommonPacketHeaders`, `preprocess.ExtractBreakpointsFromFile` against
`src/map/packets.hpp`.

### Step 3 — Reorder codegen steps and add S→C join pass

`genLengths` currently runs as Step 3, before the VersionTable exists (Step 5). Reorder:

```
Step 1: genShuffle     (unchanged)
Step 2: genObfuscation (unchanged)
Step 3: buildVersionTable + injectSynthetic + injectCommonPacketStructs
Step 4: genLengths (map C→S from clif_packetdb.hpp)
         + S→C join pass (map from VersionTable)
         + map packets.hpp ZC_* pass
Step 5: genEvents, genSend, genDecode, genEncode (unchanged)
```

The S→C join pass in Step 4 walks `semantics/mappings.yaml` entries with
`direction: receive` and for each:
1. Looks up `rathena_struct` in the VersionTable (which now includes `packets_struct.hpp`
   structs, synthetic structs, and `common/packets.hpp` structs).
2. For each PACKETVER range in the VersionTable entry: emits a `LengthBreakpoint` with
   the struct's `TotalSize` (or -1 for flex-array structs).
3. Merges into the map breakpoints slice before calling `GenerateLengthsFile`.

For `packets.hpp` ZC_* packets not yet in the SemanticDB (i.e. no `direction: receive`
entry), their lengths are populated via Step 2 (`buildMapStocLengthBreakpoints`) which
processes `packets.hpp` directly without requiring SemanticDB entries.

### Step 4 — Delete all SetLength calls from `pkg/fsm/fsm.go`

Once the regenerated `lengths_map.go` and `lengths_char.go` contain all 34 entries:
1. Remove all 34 `SetLength` calls from `pkg/fsm/fsm.go`.
2. Run `go build ./...` and `go test ./...`.
3. Run the live integration test.

---

## Acceptance Criteria

- [ ] All 34 packet IDs appear in the generated lengths files with GCC-verified sizes
      (or -1 for variable-length). Verify:
      ```
      grep -iE "0x(0283|0074|0073|02eb|0a18|0b18|007f|0091|00b0|00b1|00bd|008e|010f|013a|0141|0087|02c9|02d9|02da|01d7|099b|09e7|0a24|0acb|0ade|0adf|0a9b|0b08|0b09|0b0a|0b0b|0b1b|0b20|0a23|0081)" pkg/session/lengths_*.go
      ```
- [ ] 6 new synthetic structs added to `synthetic_structs.hpp`, each with layout comment
      citing `clif_packetdb.hpp` line and `clif.cpp` WFIFOW call site
- [ ] `go test ./internal/codegen/preprocess/` passes (synthetic struct injection test
      added for each new SYNTH_ZC_* struct)
- [ ] All 34 `SetLength` calls removed from `pkg/fsm/fsm.go`; no other `SetLength` calls
      remain in any non-test production file
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -race ./pkg/...` passes
- [ ] `go test -bench=. -benchmem ./pkg/session/` still shows 0 allocs/op for
      `BenchmarkFeed_ActorExists_0x09FF` and `BenchmarkFeed_SmallFixedPacket`
- [ ] `go test -tags integration -timeout 60s ./pkg/fsm/` passes against the live server
      with no `Feed()` faults and at least one event firing
- [ ] `validation/phase1_gate.sh` still shows 76 PASS / 1 FAIL (no regressions)
- [ ] No `recvpackets.txt` path exists anywhere in `internal/codegen/`
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us10_eliminate_setlength.md` written

---

## Implementation Notes

### PACKETVER-conditional IDs in `packets.hpp`

`ZC_ACCEPT_ENTER` maps to three packet IDs across version history:

| Packet ID | Length | PACKETVER condition (in `packets.hpp`) |
|-----------|--------|---------------------------------------|
| 0x0073 | 11 | `PACKETVER < 20080102` |
| 0x02EB | 13 | `20080102 <= pv < 20141022` or `pv >= 20160330` |
| 0x0A18 | 14 | `20141022 <= pv < 20160330` |

`buildMapStocLengthBreakpoints` processes `packets.hpp` at each PACKETVER breakpoint
found in that file — exactly as `buildLoginCharLengthBreakpoints` does for
`common/packets.hpp`. The preprocessor resolves the `#if PACKETVER` guards, so each
breakpoint snapshot naturally produces the correct ID→length mapping for that version.
The diff logic in `diffLenTable` handles the ID-change-at-version-boundary case
correctly: 0x0073 disappears from the table at 20080102 (emits `Length=0`) while
0x02EB appears (emits `Length=13`).

### Structless packet PACKETVER guards

0x0ADE is only registered in `clif_packetdb.hpp` for `pv >= 20171025`. The synthetic
struct (`SYNTH_ZC_OVERWEIGHT_PERCENT`) is version-invariant in the VersionTable
(MinVer=20030000), but the SemanticDB `mappings:` entry for 0x0ADE must have a
`packetver_min: 20171025` constraint so the join pass only emits the length breakpoint
starting at that version. If no SemanticDB entry exists yet for 0x0ADE, add one.

### Structs using enum-based IDs (not `DEFINE_PACKET_HEADER`)

Several `packets_struct.hpp` entries bind packet IDs via typed enums rather than
`DEFINE_PACKET_HEADER`:

- `packet_maptypeproperty2` → `maptypeproperty2Type = 0x99b`
- `packet_itemlist_normal` → `inventorylistnormalType = 0xb09`
- `packet_itemlist_equip` → `inventorylistequipType = 0xb0a`
- `PACKET_ZC_SPRITE_CHANGE` → `sendLookType = 0x1d7`

The VersionTable pipeline resolves these via the preprocessed enum value at each
PACKETVER. The join pass looks them up by struct name (`rathena_struct` in SemanticDB),
not by ID — so the lookup is unaffected by whether the ID comes from an enum or a macro.

### Ordering constraint

The S→C join pass requires the VersionTable to exist. `genLengths` must be called
after `buildVersionTable` + injections complete. The current step ordering in `main.go`
must be changed: move `genLengths` from Step 3 to Step 4 (after Step 5's version table
build). The function signature of `genLengths` must be updated to accept the VersionTable
and the SemanticDB as additional parameters.

### Estimated scope

| File | Change |
|------|--------|
| `internal/codegen/stubs/synthetic_structs.hpp` | Add 6 SYNTH_ZC_* structs (~50 lines) |
| `internal/codegen/main.go` | Reorder steps; add `buildMapStocLengthBreakpoints`; update `genLengths` signature (~60 lines) |
| `pkg/fsm/fsm.go` | Delete 34 `SetLength` calls (~100 lines removed) |
| `pkg/session/lengths_map.go` | Regenerated (adds ~34+ entries) |
| `pkg/session/lengths_char.go` | Regenerated (adds ~1+ entries) |
| New test | Verify 34 IDs in generated output (~30 lines) |
