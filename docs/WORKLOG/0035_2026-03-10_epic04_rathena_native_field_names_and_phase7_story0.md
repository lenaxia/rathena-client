# Work Log 0035 — EPIC-04: rAthena-Native Field Names + Phase 7 Story 0

**Date**: 2026-03-10  
**Stories**: EPIC-04 Stories 1–6, Phase 7 Story 0  
**Status**: Complete

---

## Summary

Completed EPIC-04 in full and prepared rathena-client for Phase 7 goKore integration.

---

## EPIC-04: What Was Done

### Story 1 — Strip `mappings.yaml` to groupings only
- Reduced `semantics/mappings.yaml` from 42,783 → 2,307 lines
- Kept only `semantic_actions` groupings (packet ID lists + struct names)
- All field definitions, pack codes, and OpenKore mappings removed

### Story 2 — Rewrite codegen to derive from rAthena structs
- `internal/codegen/semantics/loader.go`: minimal loader; `Action` struct has only `PacketIDs` + `Implementations`
- `internal/codegen/gen/decode.go`: derives field reads from `VersionTable` directly (no yaml field metadata)
- `internal/codegen/gen/events.go`: `mergedEventFields` + `buildActionEventFieldTypes` — all field names from rAthena structs
- `internal/codegen/gen/encode.go`: derives from struct layouts
- `internal/codegen/gen/send.go`: derives from struct layouts

### Story 3 — Regenerate all packages
- `pkg/decode/`, `pkg/encode/`, `pkg/events/`, `pkg/send/` all regenerated with rAthena-native field names
- ~150 CZ_ decode stubs deleted (S→C only; CZ_ packets are C→S)
- ~18 new event files added (`zc_notify_mapinfo`, `zc_restart_ack`, `zc_resurrection`, etc.)
- `pkg/session/lengths_map.go` explicitly excluded from codegen (hand-maintained)
- `go build ./...` compiles clean

### Story 4 — Update hand-written tests
All test files updated to rAthena-native field names:

| File | Key changes |
|---|---|
| `pkg/decode/phase1_golden_test.go` | `e.ID`→`e.GID`, `e.WalkSpeed`→`e.Speed`, `e.HairStyle`→`e.Head`, etc. |
| `pkg/decode/gaps_test.go` | `e.ID`→`e.GID`, buffer size fix for `ZcPositionIdNameInfo` |
| `pkg/decode/character_moves_test.go` | `e.Time`→`e.MoveStartTime`; CZ_035F test rewritten as encode test |
| `pkg/fsm/replay_test.go` | `e.StatType`→`e.VarID`, `e.ID`→`e.GID` |
| `pkg/encode/actor_action_test.go` | `TargetID`→`TargetGID`, `Type`→`Action` |
| `pkg/encode/skill_use_test.go` | `Lv`→`SkillLv` |
| `pkg/session/lengths_map.go` | Restored to pre-EPIC-04 state (codegen had incorrectly modified it) |

### Story 5 — Verify 0 allocs/op encode benchmarks
```
BenchmarkEncodeActorAction-14    0 B/op   0 allocs/op
BenchmarkEncodeMoveTo-14         0 B/op   0 allocs/op
BenchmarkEncodeSkillUse-14       0 B/op   0 allocs/op
```
All pass.

### Story 6 — Commit EPIC-04
Committed as `b37a5b1` with 1023 files changed.

---

## Phase 7 Story 0: rathena-client dependency setup

### Tagged v0.2.0
```
git tag v0.2.0
```

### Added to goKore `go.mod`
```
require github.com/lenaxia/rathena-client v0.2.0
replace github.com/lenaxia/rathena-client => ../rathena-client
```

### Verified
- `go mod tidy` in goKore: clean
- `go build ./...` in goKore: passes with rathena-client dependency

---

## Key Discoveries

### `pkg/session/lengths_map.go` must NOT be regenerated
The EPIC-04 codegen incorrectly modified `pkg/session/lengths_map.go`, removing valid entries
(`0x09FD`, `0x00A3`, `0x00A4`, `0x00A8`, `0x01C8`, `0x02E1`, `0x08C8`) used in replay fixtures.
This caused `fsm` replay tests to fail with `feedErrors=1`. Fixed by restoring from pre-EPIC-04 state.
The codegen `main.go` must never write this file.

### `CharacterMove_0x035F` is CZ_ (C→S only)
No decode function generated. Test rewritten to use `EncodeCharacterMove` instead.

### `ZcPositionIdNameInfo` decode requires ≥32 bytes
The decode function does `data[8:32]`. Test fixture must pad to at least 32 bytes.

---

## Test Results
```
ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/fsm
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session
```
All pass. Zero allocs on encode benchmarks.
