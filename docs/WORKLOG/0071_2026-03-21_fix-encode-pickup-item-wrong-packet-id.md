# 0071 — Fix: EncodePickupItem sends wrong packet ID at pv >= 20101124

**Date**: 2026-03-21
**Scope**: `pkg/encode/pickup_item.go`, `pkg/encode/pickup_item_test.go`, `semantics/mappings.yaml`
**Severity**: BLOCKING — server disconnects on every item pickup attempt at pv >= 20101124

---

## Problem

`EncodePickupItem` hardcoded `0x009F` for all packetvers and ignored the `packetver`
argument. At `pv >= 20101124`, rAthena reassigns `0x009F` to a different handler and
moves `clif_parse_TakeItem` to new IDs. Sending `0x009F` at modern pv caused an
immediate server disconnect.

The packet ID for TakeItem changes frequently across versions:
- Pre-shuffle era: 7 explicit reassignments between 20101124 and 20130514
- Shuffle era (>= 20130515): per-week via `shuffledCtoSID(pv, 0x009F)`
- Post-20180307: stable `0x0362`

Production pv=20200401 requires `0x0362`. The old encoder sent `0x009F`.

---

## Cross-Validation (worklog 0070)

Full cross-validation performed against rAthena `clif_packetdb.hpp`,
`clif_shuffle.hpp`, and OpenKore kRO Send modules for every packetver boundary
and all 152 weekly shuffle-era entries.

Results:
- All 7 explicit pre-shuffle boundaries: rAthena and OpenKore agree
- Shuffle era: 57 direct OpenKore module matches, **0 mismatches**
- pv=20111005: OpenKore `RagexeRE_2011_10_05a.pm` has `0x08A7` (OpenKore bug — rAthena
  is authoritative, correct ID is `0x0815` as confirmed by rAthena `clif_packetdb.hpp:1402`
  and OpenKore `RagexeRE_2011_11_02a.pm`)

---

## Why This is a Code Fix, Not a Semantics DB Fix

The semantics DB drives codegen which emits simple per-struct encoders. The shuffle
era requires calling `shuffledCtoSID(pv, 0x009F)` at runtime — codegen cannot emit
this. Pattern is identical to `move_to.go` (hand-written for the same reason).

---

## Changes

### `pkg/encode/pickup_item.go`

Replaced generated file with hand-written implementation following `move_to.go`
pattern exactly:

```go
func EncodePickupItem(req send.PickupItem, packetver uint32) [6]byte {
    var id uint16
    switch {
    case packetver < 20101124: id = 0x009F   // clif_packetdb.hpp:50
    case packetver < 20111005: id = 0x0362   // clif_packetdb.hpp:1384
    case packetver < 20120307: id = 0x0815   // clif_packetdb.hpp:1402
    case packetver < 20120410: id = 0x0865   // clif_packetdb.hpp:1441
    case packetver < 20120418: id = 0x0938   // clif_packetdb.hpp:1494
    case packetver < 20120702: id = 0x07E4   // clif_packetdb.hpp:1560
    case packetver < 20130320: id = 0x089F   // clif_packetdb.hpp:1587
    case packetver < 20130515: id = 0x0933   // clif_packetdb.hpp:1631
    default: id = shuffledCtoSID(packetver, 0x009F) // stable 0x0362 post-20180307
    }
    ...
}
```

### `pkg/encode/pickup_item_test.go` (new)

TDD — written before implementation (red phase: 19 failures, 5 passes).
After implementation: 24/24 pass.

Tests cover:
- All 7 explicit boundary transitions (both sides of each boundary)
- Shuffle era entry (pv=20130515 → 0x08A1, verified vs OpenKore)
- A mid-shuffle weekly pv (pv=20130522 → 0x095E)
- Post-shuffle stable era (pv=20200401 → 0x0362, pv=20180308 → 0x0362)
- Wire length always 6 bytes
- ITID at bytes [2:6] (little-endian uint32)
- Zero ITID encodes correctly
- ITID preserved across all packetvers

### `semantics/mappings.yaml` (via MCP)

Updated `pickup_item` action:
- `0x009F` implementation: constrained `packetver_range` from `[null, null]`
  to `[null, 20101123]` (accurate upper bound)
- Added `0x0362` implementation: `packetver_range: [20101124, null]`,
  `struct_name: SYNTH_CZ_ITEM_PICKUP2` (stable post-shuffle modern wire ID)

---

## rAthena Source References

| Boundary | Packet ID | File | Line |
|---|---|---|---|
| baseline | 0x009F | clif_packetdb.hpp | 50 |
| pv >= 20101124 | 0x0362 | clif_packetdb.hpp | 1384 |
| pv >= 20111005 | 0x0815 | clif_packetdb.hpp | 1402 |
| pv >= 20120307 | 0x0865 | clif_packetdb.hpp | 1441 |
| pv >= 20120410 | 0x0938 | clif_packetdb.hpp | 1494 |
| pv >= 20120418 | 0x07E4 | clif_packetdb.hpp | 1560 |
| pv >= 20120702 | 0x089F | clif_packetdb.hpp | 1587 |
| pv >= 20130320 | 0x0933 | clif_packetdb.hpp | 1631 |
| pv >= 20130515 | shuffledCtoSID(pv, 0x009F) | clif_shuffle.hpp | all blocks |
| pv > 20180307 | 0x0362 (stable) | clif_shuffle.hpp | 4723+ |

---

## Test Results

```
--- PASS: TestEncodePickupItem_PacketID_Table (24/24 sub-tests)
--- PASS: TestEncodePickupItem_Length
--- PASS: TestEncodePickupItem_ITID
--- PASS: TestEncodePickupItem_ZeroITID
--- PASS: TestEncodePickupItem_ITIDPreservedAcrossPacketvers

BenchmarkEncodePickupItem: 151225860 ops, 8.366 ns/op, 0 B/op, 0 allocs/op ✓
```

`go test ./...` — all packages pass.
`grep -r "^\s*go " pkg/` — empty (no goroutines in pkg/).
