# WORKLOG 0042 — EPIC-05: lengths_map.go correctness — four root-cause fixes

**Date**: 2026-03-11  
**Epic**: EPIC-05 — lengths_map.go packet length table audit & fix  
**Status**: COMPLETE (all 5 user stories implemented)

---

## Summary

Fixed four root-cause classes of bugs in `pkg/session/lengths_map.go` that caused
`session.Feed()` to mis-frame incoming packets or use incorrect lengths. Added a
regression test suite to catch regressions.

## Stories completed

### US-05-1: Fix 2D array parsing (`RANKLIST` collapse)

**File**: `internal/codegen/preprocess/parser.go`

**Bug**: `RANKLIST.names[10][(23+1)]` is a 2D array. The parser's `reArrayField`
regex only matched single-bracket arrays. The 2D field was silently skipped,
contributing 0 bytes. `RANKLIST.TotalSize` was computed as 40 (only `uint32 points[10]`)
instead of 280 (240 + 40).

**Fix**: Added `re2DArrayField` regex before `reArrayField` in the field-parsing loop.
When matched: `size = evalExpr(dim1) * evalExpr(dim2) * typeSizes[typ]`.

**Impact**:
- `0x0219` (ZC_BLACKSMITH_RANK): 42 → 282
- `0x021A` (ZC_ALCHEMIST_RANK): 42 → 282
- `0x0226` (ZC_TAEKWON_RANK): 42 → 282
- `0x0238` (ZC_KILLER_RANK / ZC_PK_RANK): 42 → 282

**Tests added**: `TestParseStructBody_2DArray`, `TestParseStructBody_2DArray_UnknownType`

---

### US-05-2: Fix wrong packet ID assignments (`0x0071`, `0x0092`)

**Files**: `semantics/mappings.yaml` (via MCP), `internal/codegen/semantics/loader.go`,
`internal/codegen/main.go`

**Bug A**: Two duplicate actions (`received_character_id_and_map`,
`received_character_ID_and_Map`) had `packet_id=0x0071` with
`struct=PACKET_HC_NOTIFY_ZONESVR`. That struct's actual headers are `0x0081`
(pre-20170315) and `0x0AC5` (post-20170315). The join pass emitted `t[0x0071]=156`
at `pv >= ~20170315`, overriding the correct `28` from `clif_packetdb`.

**Fix A**: Deleted the wrong `0x0071` implementations from both actions.
Added `0x0081` (with `packetver_max=20170315`) and `0x0AC5` (with
`packetver_min=20170315`) to `received_map_server_info`.

**Bug B**: `zc_npcack_servermove` had `0x0092` with `PACKET_ZC_NPCACK_SERVERMOVE`
and no `packetver_max`. The struct moved to `0x0AC7` at pv=20170315. The join pass
emitted `t[0x0092]=156` indefinitely.

**Fix B**: Added `packetver_max=20170315` to the `0x0092` implementation.

**Root cause in join pass**: The `buildMapStocJoinPass` iterated ALL version ranges
of a struct without checking the `PacketverMin`/`PacketverMax` bounds from the
SemanticDB implementation. Added filtering logic to skip struct version ranges
outside the implementation's valid packetver window.

**Loader change**: `PacketMapping` now carries `PacketverMin`/`PacketverMax` from
the SemanticDB. Deduplication key changed from `packet_id` to `(packet_id, struct_name)`
to allow the same ID with different structs (different pv ranges) to be emitted.

**Impact**:
- `0x0071 = 28` at all versions (was 156 at pv≥20170315)
- `0x0092 = 0` at pv≥20170315 (was 156) — correctly marking it as removed
- `0x0AC7 = 156` at pv≥20170315 (correct — new home of NPCACK_SERVERMOVE)

---

### US-05-3: Missing packet IDs

Per prior investigation, 335/338 missing IDs at pv=20180621 are OpenKore-only
(reverse-engineered, no rAthena struct). The remaining 3 are C→S or already
partially covered. **No code change required** — documented as expected and
out-of-scope for GCC-derived codegen.

---

### US-05-4: Fix variable/fixed length overrides

**File**: `internal/codegen/main.go`

**Bug**: Both Part 2 (`buildMapStocLengthBreakpoints`, from `packets.hpp` HEADER_*
constants) and Part 3 (S→C join pass) used `mergeBreakpoints` which let the
**later pass win** on conflict. This caused:
- Packets registered as `-1` (variable-length) in `clif_packetdb.hpp` to be
  overridden with fixed struct sizes from Parts 2 or 3.
- Examples: `0x0166` (ours=32, should be -1), `0x09FD` (ours=110, should be -1),
  `0x09FF` (ours=104, should be -1).

**Fix**: Added `mergeBreakpointsFillOnly` — a new merge function that simulates
the cumulative state from previous passes and only fills in entries that are
currently `0` (unknown). Entries with any nonzero value (including -1) from
`clif_packetdb` are never overridden.

Changed Parts 2 and 3 to use `mergeBreakpointsFillOnly` instead of `mergeBreakpoints`.

**Impact**:
- `0x0166`, `0x09FD`, `0x09FF` and many others correctly remain `-1`
- Variable/fixed mismatches at pv=20180621 reduced from 87 to ~57
  (remaining 57 are pre-existing structural issues outside scope of this epic)

---

### US-05-5: Regression harness

**File**: `pkg/session/lengths_regression_test.go`

Added 6 regression tests that will catch future regressions for all four
root-cause classes:

1. `TestLengthRegression_RankingPackets` — asserts 0x0219/0x021A/0x0226/0x0238 = 282
2. `TestLengthRegression_PacketID0071` — asserts 0x0071 = 28 at all packetvers
3. `TestLengthRegression_PacketID0092` — asserts 0x0092 = 28 pre-20170315, 0 after
4. `TestLengthRegression_PacketID0AC7` — asserts 0x0AC7 = 156 post-20170315
5. `TestLengthRegression_VariableLengthNotOverridden` — asserts 0x0166/0x09FD/0x09FF = -1
6. `TestLengthRegression_CharServerNotInMapTable` — asserts 0x0081 ≠ 156 in map table

All 6 tests pass. Full `go test ./...` passes.

---

## Final metrics

| Metric | Before | After | Δ |
|---|---|---|---|
| Real mismatches at pv=20180621 | 87 | ~57 | -30 |
| `0x0219` (ZC_BLACKSMITH_RANK) | 42 | 282 | ✓ |
| `0x0071` at pv=20180621 | 156 | 28 | ✓ |
| `0x0092` at pv=20180621 | 156 | 0 | ✓ |
| `0x0166` at pv=20180621 | 32 | -1 | ✓ |
| `go test ./...` | pass | pass | ✓ |

## Files changed

- `internal/codegen/preprocess/parser.go` — 2D array regex + handler
- `internal/codegen/preprocess/preprocess_test.go` — 2 new tests
- `internal/codegen/semantics/loader.go` — PacketMapping carries packetver bounds
- `internal/codegen/main.go` — join pass filter + mergeBreakpointsFillOnly + Part 2 fix
- `semantics/mappings.yaml` — SemanticDB fixes via MCP:
  - Removed wrong `0x0071` impls from `received_character_id_and_map` and `received_character_ID_and_Map`
  - Added `0x0081` (max=20170315) and updated `0x0AC5` (min=20170315) in `received_map_server_info`
  - Added `packetver_max=20170315` to `zc_npcack_servermove`'s `0x0092` impl
  - Added `0x0092` (max=20170315) impl to `map_changed`
- `pkg/session/lengths_map.go` — regenerated
- `pkg/session/lengths_regression_test.go` — new regression test file
- `docs/BACKLOG/EPIC-05/README.md` — epic backlog (pre-existing, not committed yet)
