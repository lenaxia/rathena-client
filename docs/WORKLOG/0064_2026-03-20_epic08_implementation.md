# WORKLOG 0064 — EPIC-08 Implementation: 55 Missing Packet Coverage

**Date**: 2026-03-20
**Status**: COMPLETE
**Follows**: EPIC-08 implementation plan (`docs/BACKLOG/EPIC-08_implementation_plan.md`),
  worklog 0062 (open questions resolved)

---

## Summary

Implemented all 55 missing packet implementations identified in the EPIC-08 audit,
plus fixed one pre-existing DB classification bug (`0x02EC`), added 8 SYNTH_ struct
definitions, created 8 new semantic actions, and corrected packetver ranges on 20+
existing entries.

**Approach**: strict TDD — three layers of failing tests written first, then all DB
changes, SYNTH_ structs, and codegen runs drove them to green.

---

## Tests Written (all written BEFORE implementation)

### `internal/codegen/semantics/epic08_test.go` (6 tests)

| Test | What it asserts |
|---|---|
| `TestEPIC08_ActorPIDClassification` | All 18 actor PIDs under correct action per rAthena enum |
| `TestEPIC08_AllGapPIDsPresent` | All 55 gap PIDs in DB under correct action + struct |
| `TestEPIC08_PacketverRangesCorrect` | 22 critical range spot-checks |
| `TestEPIC08_NewActionsExist` | 8 new actions present with minimum impl counts |
| `TestEPIC08_NoDuplicatePIDs` | No two actions claim the same PID |
| `TestEPIC08_ImplCount` | Total impls in range [530, 600] |

### `pkg/session/epic08_dispatch_test.go` (5 tests)

| Test | What it asserts |
|---|---|
| `TestEPIC08_NewPIDsInDispatch` | Every new PID in receiveDispatch under correct SemanticAction |
| `TestEPIC08_ActorMoved_0x02EC_Dispatches` | Bug-fix: 0x02EC fires ActionActorMoved not ActionActorExists |
| `TestEPIC08_ZcAcceptEnter_0x0073_Dispatches` | Pre-2008 map-enter PID dispatches correctly |
| `TestEPIC08_Exp_Dispatches` | Both EXP PIDs (0x07F6 and 0x0ACC) fire ActionExp |
| `TestEPIC08_ItemPickup_AllVariants` | All 7 item_pickup PIDs dispatch across packetver ranges |

### `pkg/decode/epic08_golden_test.go` (9 tests + 3 benchmarks)

Golden byte tests verifying field-level decode correctness for:
`ZcAcceptEnter_0x0073`, `ZcAcceptEnter_0x02EB`, `ItemPickup_0x029A`,
`ZcReqTakeoffEquipAck_0x00AC`, `ZcReqWearEquipAck_0x0999`, `Exp_0x07F6`,
`Exp_0x0ACC`, `ActorMoved_0x02EC`, `CharCreated_0x006D` (panic test),
`ZcHoParChange_0x07DB`, `ZcElParChange_0x081E`.

All benchmarks confirm **0 allocs/op**.

---

## Step 1: Pre-existing Bug Fix — `0x02EC` Classification

**Bug**: `0x02EC` was under `actor_exists` in the DB. rAthena `packets_struct.hpp` enum
shows `unit_walkingType = 0x2ec [PACKETVER < 20091103]` — it is a walking unit packet.

**Fix**: Deleted `0x02EC` from `actor_exists`, added to `actor_moved` with range
`[20080102, 20091102]` via MCP.

---

## Steps 2–9: All 55 Gap PIDs Added

### New actions created (8)

| Action | PIDs | Description |
|---|---|---|
| `char_created` | `0x006D`, `0x0B6F` | HC_ACCEPT_MAKECHAR: char creation success |
| `exp` | `0x07F6`, `0x0ACC` | EXP gain (SYNTH_) |
| `zc_req_takeoff_equip_ack` | `0x00AC`, `0x08D1`, `0x099A` | Unequip acknowledgement |
| `zc_ho_par_change` | `0x07DB` | Homunculus stat change |
| `zc_el_par_change` | `0x081E` | Elemental stat change |
| `inventory_items_stackable` | `0x00A3`, `0x01EE`, `0x02E8`, `0x0991` | Bulk stackable items |
| `inventory_items_equip` | `0x00A4`, `0x0992`, `0x0A0D`, `0x0B0A`, `0x0B39` | Bulk equip items |
| `mail_receive` | `0x0274` | Mail notification (SYNTH_) |

### Existing actions extended

| Action | New PIDs added |
|---|---|
| `zc_accept_enter` | `0x0073` (< 20080102), `0x02EB` (20080102–20141021) |
| `received_characters_page` | `0x0B72` (MAIN >= 20201007) |
| `item_pickup` | `0x029A`, `0x02D4`, `0x0990`, `0x0A0C`, `0x0B41` |
| `zc_req_wear_equip_ack` | `0x0999` (MAIN >= 20121205) |
| `actor_exists` | `0x022A`, `0x02EE`, `0x09DD` |
| `actor_connected` | `0x022B`, `0x09DC` |
| `skill_add` | `0x0B31` (RE >= 20190807) |
| `skills_list` | `0x0B32` (RE >= 20190807) |
| `zc_skillinfo_update2` | `0x0B33` (RE >= 20190807) |
| `add_exchange_item` | `0x080F`, `0x0A09`, `0x0A96` |
| `zc_shortcut_key_list` | `0x07D9`, `0x0A00`, `0x0B20` |
| `zc_guild_info` | `0x01B6`, `0x0B7B` |
| `zc_equipwin_microscope` | `0x0859`, `0x0906`, `0x0997`, `0x0A2D`, `0x0B03` |
| `received_map_server_info` | `0x0071` (SYNTH_) |
| `stat_update` | `0x02A2` (SYNTH_) |
| `pin_code_request` | `0x02AD` (SYNTH_) |
| `character_server_refused` | `0x02CA` (SYNTH_) |
| `zc_hoskillinfo_list` | `0x029D` |
| `area_spell` | `0x08C7` (SYNTH_) |

### Range fixes on existing entries

| Action | PID | Old range | New range |
|---|---|---|---|
| `item_pickup` | `0x00A0` | `[null, null]` | `[20030000, 20061217]` |
| `item_pickup` | `0x0A37` | `[20150226, null]` | `[20160921, 20200915]` |
| `zc_accept_enter` | `0x0A18` | `[null, null]` | `[20141022, 20160329]` |
| `zc_req_wear_equip_ack` | `0x00AA` | `[null, null]` | `[20030000, 20101122]` |
| `add_exchange_item` | `0x00E9` | `[null, null]` | `[20030000, 20100222]` |
| `zc_shortcut_key_list` | `0x02B9` | `[null, null]` | `[20030000, 20090616]` |
| `zc_guild_info` | `0x0A84` | `[null, null]` | `[20160921, 20200901]` |
| `zc_equipwin_microscope` | `0x02D7` | `[null, null]` | `[20030000, 20101122]` |
| `actor_exists` | `0x0078` | `[null, null]` | `[20030000, 20030000]` |
| `actor_exists` | `0x01D8` | `[null, null]` | `[20030001, 20050410]` |
| `actor_connected` | `0x0079` | `[null, null]` | `[20030000, 20030000]` |
| `actor_connected` | `0x01D9` | `[null, null]` | `[20030001, 20050410]` |
| `actor_connected` | `0x02ED` | `[null, null]` | `[20080102, 20091102]` |
| `actor_moved` | `0x007B` | `[null, null]` | `[20030000, 20030000]` |
| `actor_moved` | `0x01DA` | `[null, null]` | `[20030001, 20050410]` |
| `actor_moved` | `0x022C` | `[null, null]` | `[20050411, 20080101]` |
| `zc_hoskillinfo_list` | `0x0235` | `[null, null]` | `[20030000, 20060423]` |
| `skill_add` | `0x0111` | `[null, null]` | `[20030000, 20190806]` |
| `skills_list` | `0x010F` | `[null, null]` | `[20030000, 20190806]` |
| `zc_skillinfo_update2` | `0x07E1` | `[null, null]` | `[20030000, 20190806]` |
| `add_exchange_item` | `0x0A09` | `[20150226, 20161101]` | `[20150226, 20161025]` |
| `zc_accept_enter` | `0x02EB` | `[20030000, 20141021]` | `[20080102, 20141021]` |

---

## SYNTH_ Structs Added to `synthetic_structs.hpp`

8 new SYNTH_ struct definitions added and verified against rAthena source:

| Struct | PID | Size | Source |
|---|---|---|---|
| `SYNTH_HC_NOTIFY_ZONESVR_OLD` | `0x0071` | 28 bytes | `char_clif.cpp chclif_send_map_data()` |
| `SYNTH_ZC_LONG_PAR_CHANGE` | `0x07F6` | 14 bytes | `clif.cpp clif_displayexp()` |
| `SYNTH_ZC_LONG_PAR_CHANGE2` | `0x0ACC` | 18 bytes | `clif.cpp clif_displayexp()` |
| `SYNTH_ZC_PAR_CHANGE2` | `0x02A2` | 8 bytes | `clif_packetdb.hpp` + OpenKore pack |
| `SYNTH_HC_SECOND_PASSWD_LOGIN_OLD` | `0x02AD` | 8 bytes | OpenKore unpack |
| `SYNTH_HC_REFUSE_ENTER_OLD` | `0x02CA` | 3 bytes | `clif_packetdb.hpp` |
| `SYNTH_ZC_SKILL_ENTRY3` | `0x08C7` | 20 bytes | `clif.cpp case 0x08c7` |
| `SYNTH_ZC_MAIL_RECEIVE` | `0x0274` | 8 bytes | `clif_packetdb.hpp` + OpenKore pack |

**Also added**: `typedef uint64_t uint64;` to synthetic_structs.hpp typedefs block (required for `SYNTH_ZC_LONG_PAR_CHANGE2.exp` field).

### Bug caught during revalidation: `SYNTH_ZC_SKILL_ENTRY3` range field

Initial implementation had `uint8 range` (following OpenKore's `C` unpack code). Cross-checking against `clif.cpp case 0x08c7` revealed `WBUFW(buf, pos+15)` — a 16-bit write. OpenKore's unpack string is wrong; rAthena uses `uint16`. Fixed before codegen was finalised.

---

## Codegen Output

| File | Change |
|---|---|
| `pkg/session/actions.go` | +8 new `SemanticAction` constants |
| `pkg/session/receive_dispatch.go` | 294 → 349 entries (+55) |
| `pkg/decode/` | +55 new decode functions across 23 files |
| `pkg/events/` | +8 new event structs |
| `pkg/session/lengths_map.go` | Updated with new PID lengths |

---

## Revalidation Findings

### Issues found and fixed during revalidation

**1. `SYNTH_ZC_SKILL_ENTRY3.range` field type (uint8 → uint16)**
Caught by cross-checking `clif.cpp case 0x08c7` line `WBUFW(buf,pos+15) = unit->range`.
OpenKore unpack `C` (uint8) is incorrect. Fixed in SYNTH_ struct before finalizing.

**2. 12 new packetver range overlaps from EPIC-08**
All were metadata precision issues (old `[null, null]` entries without bounds, and
missing min/max on new bounded entries). Resolved by adding correct packetver bounds
to all affected existing entries via MCP. Zero functional impact on dispatch — each PID
has exactly one decode function and one dispatch entry regardless.

**3. Actor `0x02ED` range was wrong**
Had `min=20050411` (same as `0x022B`). Correct range is `[20080102, 20091102]`
(spawn_unitType switches from `0x022B` to `0x02ED` at `PACKETVER >= 20080102`).

**4. `zc_accept_enter 0x02EB` min was wrong**
Had `min=20030000`. Correct: `0x02EB` is the second-era map-enter PID starting after
`0x0073` ends. First span: `[20080102, 20141021]`.

### Issues confirmed as pre-existing (TECH-DEBT-01)

2 remaining overlaps in `zc_equipwin_microscope` (0x0859/0x0906 and 0x0906/0x0997)
are RE/MAIN split-date artifacts. `0x0906` has condition
`PACKETVER_MAIN_NUM >= 20111207 || PACKETVER_RE_NUM >= 20111122` — the 15-day window
20111122–20111206 is ambiguous without PACKETVER_RE_NUM tracking.
Same class as 10 other pre-existing overlaps in the DB. Not fixable without
TECH-DEBT-01 RE support.

---

## Final Metrics

| Metric | Before | After |
|---|---|---|
| DB implementations | 476 | 531 |
| Dispatch entries | ~294 | 349 |
| New actions | — | +8 |
| New SYNTH_ structs | — | +8 |
| New overlaps (EPIC-08) | 0 | 2 (RE/MAIN only, same as pre-existing) |
| `go test ./...` | PASS | PASS |
| `go test -race ./...` | PASS | PASS |
| Hot-path allocs/op | 0 | 0 |

---

## Files Changed

| File | Type | Notes |
|---|---|---|
| `semantics/mappings.yaml` | Modified | +55 new impls, 8 new actions, 20+ range fixes |
| `internal/codegen/stubs/synthetic_structs.hpp` | Modified | +8 SYNTH_ structs, +uint64 typedef |
| `pkg/decode/` | Generated | +55 new functions across 23 files |
| `pkg/events/` | Generated | +8 new event structs |
| `pkg/session/actions.go` | Generated | +8 new SemanticAction constants |
| `pkg/session/receive_dispatch.go` | Generated | 349 total entries |
| `pkg/session/lengths_map.go` | Generated | Updated |
| `internal/codegen/semantics/epic08_test.go` | New | 6 semantic DB tests |
| `pkg/session/epic08_dispatch_test.go` | New | 5 dispatch routing tests |
| `pkg/decode/epic08_golden_test.go` | New | 9 golden decode tests + 3 benchmarks |
| `internal/codegen/semantics/validate_test.go` | Modified | Updated actor_exists impl count assertion |
| `pkg/decode/gaps_test.go` | Modified | Updated AddExchangeItem test to use correct PID |
