# Work Log 0036 — v0.2.1: Fix Enum-Assigned Packet ID Gap (Gap D)

**Date**: 2026-03-10
**Tag**: v0.2.1
**Status**: Complete

---

## Summary

Fixed the root cause of `TestReplay_FullAuth_20200401` and `TestReplay_Movement_20200401`
failing: the codegen was missing packet lengths for three IDs that are assigned via C++
enum values in `packets_struct.hpp` rather than `DEFINE_PACKET_HEADER` constants. This
made them invisible to the existing `reHeaderConst` regex parser.

---

## Diagnosis

### Symptom
Both replay tests reported `feedErrors=1` — `Feed()` encountered an unknown packet ID
mid-stream, desyncing the TCP framing.

### Investigation method
1. Parsed the `.fixture` binary files directly using a Go program that simulated the
   framing engine with the kRO recvpackets.txt as ground truth.
2. Identified the exact byte offset where desync occurred by tracking consumed bytes
   per packet.
3. Traced the desync: `0x01D7` was consuming 11 bytes instead of 15, causing the next
   packet ID to land mid-payload.

### Root causes found

**Gap D: enum-assigned packet IDs** — Three packet IDs in `packets_struct.hpp` are
assigned via C++ enum values, not `DEFINE_PACKET_HEADER`:

```cpp
// packets_struct.hpp
inventorylistnormalType = 0xb09,  // PACKETVER_MAIN_NUM >= 20181002
inventorylistequipType  = 0xb0a,  // PACKETVER_MAIN_NUM >= 20181002
sendLookType            = 0x1d7,  // PACKETVER >= 4 (always)
```

The codegen's `ParseCommonPacketHeaders` uses `reHeaderConst` which only matches
`const int16 HEADER_* = ...` — enum values never match, so these IDs were never
emitted into `lengths_map.go`.

Additionally, `clif_packetdb.hpp` still declares `packet(0x01d7, 11)` but
`PACKET_ZC_SPRITE_CHANGE` changed its `val`/`val2` fields from `uint16` to `uint32`
at `PACKETVER_MAIN_NUM >= 20181121`, making the wire size 15 bytes. The packetdb was
never updated. The struct-derived size via `structDB` is correct (15), but nothing
connected `sendLookType = 0x1d7` to `PACKET_ZC_SPRITE_CHANGE`.

**Verification**: Running `ParseCommonPacketHeaders` at pv=20200401 confirmed `0x0B09`
and `0x0B0A` were NOT FOUND, and `0x01D7` was found but with length 11 (from the stale
clif_packetdb entry, via Part 1 which wins over the missing Part 2 result).

---

## Fix

### New `buildMapEnumPacketBreakpoints` function (Part 4 in `genLengths`)

Added `internal/codegen/main.go:buildMapEnumPacketBreakpoints()` which:

1. Preprocesses `packets_struct.hpp` at each of its PACKETVER breakpoints (155 total)
2. Extracts enum assignments matching `reEnumAssign` regex for known enum names
3. Resolves sizes from `structDB` (for fixed-size packets) or marks as variable-length
4. Emits `LengthBreakpoint` diffs that are merged into the final `lengths_map.go`
   via `mergeBreakpoints` (Part 4 wins over Parts 1–3 on conflict)

The known enum table (`knownEnumPackets`) covers:

| Enum name | Struct | Length |
|-----------|--------|--------|
| `inventorylistnormalType` | `packet_itemlist_normal` (has `packetLength`) | -1 (variable) |
| `inventorylistequipType` | `packet_itemlist_equip` (has `packetLength`) | -1 (variable) |
| `sendLookType` | `PACKET_ZC_SPRITE_CHANGE` | structDB-derived (11 → 15 at pv≥20181121) |

### Collateral fixes

The enum pass also correctly tracks the full lineage of inventory list packet IDs as
they were superseded across packetvers, properly retiring old IDs to `0` (unknown) when
replaced:

| Era | Normal list ID | Equip list ID |
|-----|---------------|---------------|
| base | `0x00A3` (-1) | `0x00A4` (-1) |
| pv≥20071002 | `0x01EE` (-1) | `0x0295` (-1) |
| pv≥20080102 | `0x02E8` (-1) | `0x02D0` (-1) |
| pv≥20120925 | `0x0991` (-1) | `0x0992` (-1) |
| pv≥20150226 | `0x0991` (-1) | `0x0A0D` (-1) |
| pv≥20181002 | `0x0B09` (-1) | `0x0B0A` (-1) |
| pv≥20200916 | `0x0B09` (-1) | `0x0B39` (-1) |

Other incidental fixes from the reprocessing:
- `0x008A` (Actor Action): was missing at pv≥20071113 (33 bytes) and pv≥20131223 (34 bytes)
- `0x02E1` (Actor Action mid-variant): correctly retired to `0` at pv≥20131223
- `0x007C` (Actor Spawned): correctly retired to `0` at pv≥20131223
- `0x0A37` (Inventory Item Added): fixed size progression across breakpoints
- `0x0ADD` (Item Exists): spurious entries at intermediate breakpoints removed
- `0x0A51` (Rodex Char Name): added 10 bytes at pv≥20141119
- `0x0B25` (ZC_PAR_4JOB_CHANGE): added 12 bytes at pv≥20170830
- `0x00A8`, `0x01C8`, `0x00AC`: removed stale C→S entries from S→C table

---

## Test results

```
ok  github.com/lenaxia/rathena-client/pkg/fsm     0.132s
ok  github.com/lenaxia/rathena-client/pkg/session 0.012s
ok  github.com/lenaxia/rathena-client/pkg/decode  (cached)
ok  github.com/lenaxia/rathena-client/pkg/encode  (cached)
```

Encode benchmarks: 0 allocs/op on all three benchmarks.

---

## Files changed

| File | Change |
|------|--------|
| `internal/codegen/main.go` | Added `reEnumAssign`, `enumPacketEntry`, `knownEnumPackets`, `buildMapEnumPacketBreakpoints()`, wired as Part 4 in `genLengths()` |
| `pkg/session/lengths_map.go` | Regenerated — 22 packet IDs affected across multiple packetver breakpoints |
| `pkg/decode/actor_action.go` | Regenerated (SKIP stub filled in) |
| `pkg/decode/actor_died_or_disappeared.go` | Regenerated (SKIP stub filled in) |
| `pkg/decode/actor_moved.go` | Regenerated (0x09FD added) |
| `pkg/decode/zc_status.go` | Regenerated (SKIP stub filled in) |
| `pkg/events/actor_action.go` | Regenerated |
| `pkg/events/actor_died_or_disappeared.go` | Regenerated |
| `pkg/events/zc_status.go` | Regenerated |
| `pkg/encode/send_chat.go` | Hand-written implementation |
| `pkg/encode/public_chat.go` | Hand-written implementation |
| `pkg/send/send_chat.go` | Added `Name`, `Message` fields |
| `pkg/send/public_chat.go` | Added `Name`, `Message` fields |
| `semantics/mappings.yaml` | Added `0x09FD` under `actor_moved` |
| `pkg/fsm/replay_test.go` | Added `0x07FB` SetLength override (25 bytes) |

---

## Key insight

The codegen has four passes for map server lengths:
- **Part 1**: C→S lengths from `clif_packetdb.hpp`
- **Part 2**: S→C fixed-size from `packets.hpp` `HEADER_*` constants
- **Part 3**: S→C from SemanticDB + VersionTable join
- **Part 4** *(new)*: S→C from enum-assigned IDs in `packets_struct.hpp`

Part 4 was the missing gap. The merge order means Part 4 wins on conflict, which is
correct: `structDB`-derived sizes are more accurate than the stale `clif_packetdb.hpp`
entry for `0x01D7`.
