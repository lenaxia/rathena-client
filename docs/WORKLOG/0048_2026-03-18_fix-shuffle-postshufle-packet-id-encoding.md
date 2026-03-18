# 0048 — 2026-03-18 — Fix shuffle era and post-shuffle packet ID encoding

## Problem

Three bugs in C→S packet ID encoding, all rooted in the same gap: the codegen's
`ParseShuffle` only handled `PACKETVER == N` sections in `clif_shuffle.hpp` and
silently dropped the final `PACKETVER > 20180307` block (which also contained
nested `#if`/`#endif` pairs that terminated the section prematurely).

### Bug 1 — `ShuffledCtoSID` returned `baseID` unchanged for `packetver > 20180307`

`clif_shuffle.hpp` comment: *"Clients after 2018-03-07bRagexeRE do not have
shuffled packets anymore"*. That block assigns the stable post-shuffle wire IDs:

```
parseable_packet(0x035F,5,clif_parse_WalkToXY,2);    // base 0x0085
parseable_packet(0x0437,7,clif_parse_ActionRequest,2,6); // base 0x0089
```

Because the codegen never parsed this block, `ShuffledCtoSID(20200401, 0x0085)`
returned `0x0085` (unchanged) instead of `0x035F`.

### Bug 2 — `EncodeActorAction` hardcoded wrong packet IDs

The generated file had two cases:
- `packetver >= 20200401`: hardcoded `0x085A` (UseSkillToPosMoreInfo — wrong handler)
- `packetver >= 20030000`: hardcoded `0x0089` (correct base ID, but not the wire ID)

Neither called `ShuffledCtoSID`, so both were wrong for any packetver.

### Bug 3 — `EncodeMoveTo` hardcoded `0x035F` unconditionally

Correct for `packetver > 20180307` but wrong for all shuffle-era clients
(20130515–20180307), where `0x0085` maps to different wire IDs per version.

## Root cause

`preprocess.ParseShuffle` used a regex matching only `#(?:if|elif)\s+PACKETVER\s*==\s*N`.
The `#elif PACKETVER > 20180307` line was not matched. Additionally, the parser
did not track `#if`/`#endif` nesting depth, so nested conditionals inside the
`> 20180307` block caused `#endif` to terminate the section before all entries
were collected (specifically, `0x0437 = ActionRequest` appears after a nested
`#if PACKETVER_MAIN_NUM` block and was dropped).

## Fix

### Layer 1 — `internal/codegen/preprocess/packetdb.go`

- Added `RangeAbove bool` field to `ShuffleSection`.
- Added `reShuffleGt` regex matching `#elif PACKETVER > N`.
- Added `reIfAny` and `reEndif` regexes + `depth int` counter to track nesting.
- Nested `#if`/`#endif` pairs inside a section increment/decrement depth;
  only a `#endif` at `depth == 0` terminates the section.
- Inner `#if` lines are skipped (not started as new sections); entries inside
  nested blocks are still captured.

### Layer 2 — `internal/codegen/gen/shuffle.go`

- Added `RangeAbove bool` to `ShuffleBreakpoint`.
- `BuildShuffleBreakpoints` propagates `RangeAbove` from `ShuffleSection`.
- `GenerateShuffleFile` separates range-above breakpoints from exact-match ones.
  The range-above block is emitted as an `if packetver > N { switch baseID { ... } }`
  guard *before* the `switch packetver` block, so it short-circuits first.

### Layer 3 — Regenerated `pkg/session/shuffle_map.go`

`ParseShuffle` now captures 154 sections (was 153). The generated function opens with:

```go
if packetver > 20180307 {
    switch baseID {
    case 0x0085:
        return 0x035F
    case 0x0089:
        return 0x0437
    // ... other stable post-shuffle mappings
    }
}
switch packetver {
// ... 153 exact-match shuffle cases
```

### Layer 4 — `pkg/encode/actor_action.go`

Rewritten as manually implemented (removed generated file). Uses
`session.ShuffledCtoSID(packetver, 0x0089)` for the wire ID. Correct for all packetvers.

### Layer 5 — `pkg/encode/move_to.go`

Updated to use `session.ShuffledCtoSID(packetver, 0x0085)`. Correct for all packetvers.

## TDD process

Tests were written first against the baseline (failing), then implementation applied:

**Failing tests added before any implementation:**
- `TestShuffledCtoSID_PostShuffle` — verifies `> 20180307` wire IDs (6 cases)
- `TestShuffledCtoSID_ShuffleEra` — verifies shuffle-era wire IDs still correct
- `TestParseShuffle_RangeAbove` — verifies `RangeAbove` field and nesting depth
- `TestGenerateShuffleFile_RangeAbove` — verifies `if packetver > N` guard emitted first
- `TestEncodeActorAction_PacketID` — table-driven across post-shuffle and shuffle-era
- `TestEncodeMoveTo_PacketID` — table-driven across post-shuffle and shuffle-era
- `TestEncodeMoveTo_PacketverVaries` — different packetvers produce different output

All tests confirmed failing before implementation. All pass after.

## GCC verification

```bash
# Confirmed: clif_shuffle.hpp PACKETVER > 20180307 block
grep -A5 "PACKETVER > 20180307" ~/personal/rathena/src/map/clif_shuffle.hpp
# parseable_packet(0x035F,5,clif_parse_WalkToXY,2)
# parseable_packet(0x0437,7,clif_parse_ActionRequest,2,6)

# Confirmed: base IDs from clif_packetdb.hpp
# 0x0085 line 37: parseable_packet(0x0085,5,clif_parse_WalkToXY,2)
# 0x0089 line 38: parseable_packet(0x0089,7,clif_parse_ActionRequest,2,6)
```

## Test results

```
ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/session
ok  github.com/lenaxia/rathena-client/pkg/fsm
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/packing
```

Benchmarks: 0 allocs/op on all hot paths. Zero goroutines in `pkg/`.

## Related

goKore bug filed separately: `action_selector.go` `request_action` variants use
`0x0437` for all `packetver >= 20080910`, ignoring shuffle-era remapping. The
correct fix is to use `rathena-client`'s `ShuffledCtoSID` at send time.
