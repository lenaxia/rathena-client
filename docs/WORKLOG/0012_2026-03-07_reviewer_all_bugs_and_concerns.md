# Worklog 0012 — Reviewer Bug Fixes (All 7 Bugs + 5 Concerns)

**Date**: 2026-03-07
**Status**: Completed

---

## Summary

Fixed all 7 bugs and addressed all 5 concerns from the skeptical reviewer's report
(session 0011). The net effect: decode codegen now actually works for the fixed packets —
previously, all `struct_name` renames in `mappings:` were invisible to the codegen because
`semantic_actions:` was not updated and the SYNTH_* structs were not in the pipeline.

---

## Bugs Fixed

### BUG-1: semantic_actions struct_name fields not updated (CRITICAL)

23 `struct_name:` entries in the `semantic_actions:` section still referenced phantom names
after the previous session's `mappings:` renames. Fixed via Python script with exact line
targeting:

| Phantom Name | Correct Name | Line(s) |
|---|---|---|
| `PACKET_ZC_NOTIFY_NEWENTRY*` | `packet_spawn_unit` | 31516, 31557, 31598, 31639 |
| `PACKET_ZC_NOTIFY_STANDENTRY*` | `packet_idle_unit` | 31810, 31852, 31936 |
| `PACKET_ZC_NOTIFY_STANDENTRY` (entity_spawn/0x007C) | `packet_spawn_unit2` | 35240 |
| `PACKET_ZC_NOTIFY_MOVEENTRY*` | `packet_unit_walking` | 31894, 32091, 32134, 32177, 32220 |
| `PACKET_ZC_MSG_STATE_CHANGE` | `packet_sc_notick` | 32277 |
| `PACKET_ZC_STATUS_CHANGE2` | `packet_status_change2` | 32313 |
| `PACKET_ZC_SKILL_ENTRY` | `packet_skill_entry` | 32402 |
| `PACKET_CZ_REQUEST_MOVE2` | `SYNTH_CZ_REQUEST_MOVE2` | 32923, 35953 |
| `PACKET_DROPFLOORITEM` | `packet_dropflooritem` | 35440 |
| `PACKET_ZC_HP_INFO` | `packet_monster_hp` | 35872 |
| `PACKET_CZ_REQUEST_MOVE` | `SYNTH_CZ_REQUEST_MOVE` | 35946 |
| `PACKET_ZC_PETEGG_LIST` | `SYNTH_ZC_PETEGG_LIST` | 36203 |
| `PACKET_ZC_AID` | `SYNTH_ZC_AID` | 31279 |

Note: `PACKET_ZC_CLOSE_STORE` at line 39040 was **not changed** — it is correctly defined
in `packets.hpp:1024` and is a real rAthena struct.

Note: `PACKET_ZC_NOTIFY_STANDENTRY` at line 35240 maps to packet `0x007C`. This is
`spawn_unit2Type = 0x7c` (from `packets_struct.hpp:49`), so the correct struct is
`packet_spawn_unit2` (not `packet_idle_unit`).

### BUG-2: Generated pkg/decode/ files were stale (CRITICAL)

Resolved by re-running codegen after BUG-1 and BUG-3 fixes. decode: 443 → 442 files,
1 intentional skip (`quest_update_mission_hunt: no implementations`). All previously-failing
structs now resolve correctly.

### BUG-3: SYNTH_* structs not in VersionTable pipeline (CRITICAL)

Added `SourceSynthetic` case to `internal/codegen/preprocess/runner.go`:
- New `Config.SyntheticHPP` field stores path to `synthetic_structs.hpp`
- New `InjectSyntheticStructs(cfg, vt)` function preprocesses the file using plain `g++`
  (no rAthena includes needed — only `stdint.h`) and injects each `SYNTH_*` struct into
  the VersionTable as `MinVer=20030000, MaxVer=0` (covers all versions)
- Only structs with `SYNTH_` prefix are injected; system structs from `stdint.h` are skipped

Added `Config.SyntheticHPP` initialization in `main.go` and Step 5b call:
```
VersionTable has 459 structs
→ VersionTable now has 481 structs (after synthetic injection)
```
22 SYNTH_ structs injected and verified.

### BUG-4: validate_test.go tested against old phantom names (MAJOR)

Updated `internal/codegen/semantics/validate_test.go`:
- Line 31: `"PACKET_ZC_AID"` → `"SYNTH_ZC_AID"`
- Line 32: `"PACKET_ZC_NOTIFY_STANDENTRY"` → `"packet_idle_unit"`
- Line 33: `"PACKET_CZ_REQUEST_MOVE"` → `"SYNTH_CZ_REQUEST_MOVE"`
- Line 67: `"PACKET_ZC_NOTIFY_STANDENTRY11"` → `"packet_idle_unit"`
- Line 76: `"PACKET_CZ_REQUEST_MOVE2"` → `"SYNTH_CZ_REQUEST_MOVE2"`
- `totalImpls` count: 476 → 475 (one implementation removed for BUG-5b)

### BUG-5: PACKET_CZ_REQUEST_MOVE (0x0085) phantom in both sections (MAJOR)

- Fixed `rathena_struct: PACKET_CZ_REQUEST_MOVE` → `rathena_struct: SYNTH_CZ_REQUEST_MOVE`
  at line 16561 in the `mappings:` section.
- The `semantic_actions:` entry was fixed in BUG-1 (line 35946).
- Fixed misleading comment in `synthetic_structs.hpp`: removed false claim "already has
  real struct in some PACKETVER" (grep confirms no such struct exists anywhere).

### BUG-5b: PACKET_ZC_QUEST_NOTIFY_EFFECT (0x02B3) phantom with unverifiable fields (MAJOR)

- Removed the implementation block for `0x02B3` from `quest_update_mission_hunt` action.
- `clif_packetdb.hpp:896` registers `packet(0x02b3,107)` but no rAthena function sends it
  (`clif_quest_show_event` sends `0x0446` instead). Fields `Active` and `QuestID` in the
  old field_mapping were unverifiable. Implementation replaced with:
  ```yaml
  implementations: []
  # 0x02B3 removed: registered but no function sends it. Legacy/deprecated.
  ```

### BUG-6: 0x02EC openkore_name was actor_exists instead of actor_moved (MODERATE)

Fixed: `openkore_name: actor_exists` → `openkore_name: actor_moved` at line 17414.
`0x02EC` is `unit_walkingType` — a moving actor, not a standing/idle actor.

### BUG-7: 0x0977 direction was send instead of receive (MINOR)

Fixed: `direction: send` → `direction: receive` at line 19930.
`0x0977` is `ZC_HP_INFO` — server→client (ZC_ prefix = server sends to client = receive).

---

## Concerns Addressed

### CONCERN-1: Empty string literals in isZeroLiteral

Added `""` and `string("")` to `isZeroLiteral()` in `gen/decode.go`. These appear as
field_mapping values for string fields where no value is expected. Previously they fell
through to "complex expression — implement manually" comments; now emit the correct
"zero value (field absent/defaulted)" comment.

### CONCERN-2: PosDir type mismatch documented

Documented in `docs/KNOWN_ISSUES.md`. The `uint32(packet.PosDir)` field_mapping
expressions hit the packing branch and produce `[]byte` at runtime, which mismatches
canonical param type `uint32`. Paths are now active (structs resolve). Needs proper fix
(option: change field_mapping to `[]byte(packet.PosDir)` and update canonical types).

### CONCERN-3: 0x0B09 shared packet ID documented

Documented in `docs/KNOWN_ISSUES.md`. At `PACKETVER >= 20181002`, inventorylistnormalType,
storageListNormalType, and cartlistnormalType all resolve to `0x0B09`. Current mapping is
correct but fragile if storage/cart actions are added.

### CONCERN-4: SYNTH_CH_ENTER (0x0275) unknown layout

Added `WARNING: DO NOT USE IN ENCODE PIPELINE` comment block in `synthetic_structs.hpp`.
Documented in `docs/KNOWN_ISSUES.md`. The struct is in the VersionTable for length
accounting only and is not wired to any send action.

### CONCERN-5: SYNTH_CZ_REQUEST_MOVE misleading comment

Fixed comment in `synthetic_structs.hpp` to state clearly: "No real rAthena struct
exists for this packet at any PACKETVER." The previous comment falsely claimed "already
has real struct in some PACKETVER."

---

## Files Changed

- `semantics/mappings.yaml`: 23 `struct_name:` fixes + BUG-5 rathena_struct fix +
  BUG-5b implementation removal + BUG-6 openkore_name + BUG-7 direction
- `internal/codegen/preprocess/runner.go`: Added `SourceSynthetic`, `Config.SyntheticHPP`,
  `InjectSyntheticStructs()`
- `internal/codegen/main.go`: Added `Config.SyntheticHPP`, Step 5b `injectSynthetic()` call,
  `injectSynthetic()` wrapper function
- `internal/codegen/semantics/validate_test.go`: Updated 5 struct name assertions,
  `totalImpls` count 476 → 475
- `internal/codegen/gen/decode.go`: Added `""` and `string("")` to `isZeroLiteral()`
- `internal/codegen/stubs/synthetic_structs.hpp`: Fixed CONCERN-4 and CONCERN-5 comments
- `docs/KNOWN_ISSUES.md`: New file documenting CONCERN-2, CONCERN-3, CONCERN-4, CONCERN-6
- `pkg/decode/` (generated): Regenerated — 443 → 442 files, 1 intentional skip
- `pkg/encode/` (generated): Regenerated — 81 → 80 files, 83 skipped

---

## Gate Status

**76 PASS / 1 FAIL** — unchanged (expected failure: CH_MAKE_CHAR shuffle).
`go build ./...` — clean. `go test ./...` — all pass.

---

## Codegen Statistics (before → after)

| Metric | Before | After |
|---|---|---|
| VersionTable struct count | 459 | 481 (+22 SYNTH_) |
| decode files generated | 443 | 442 |
| decode files skipped | many | 1 (intentional) |
| encode files generated | 81 | 80 |

---

## Next Steps

1. **Fix CONCERN-2** (PosDir type mismatch): Change `uint32(packet.PosDir)` field_mapping
   entries to `[]byte(packet.PosDir)` and update canonical param types to `[]byte` for
   PosDir fields. Otherwise the newly-active decode paths for idle_unit/spawn_unit/
   unit_walking packets will have type errors in the generated code.

2. **Write worklog 0012** (this file).

3. **Phase next**: Extend VersionTable to cover `packets.hpp` static structs, then Phase 4.
