# Worklog 0010 — Phantom Struct Resolution

**Date**: 2026-03-07  
**Status**: Completed

---

## Summary

Resolved all 48 "phantom" struct names in `semantics/mappings.yaml` — struct names that
appeared in SemanticDB but did not correspond to any actual struct definition in rAthena
source. Also improved the codegen `decode.go` to handle `nil`, zero-literal, and plain
field-name expressions in `field_mapping`.

---

## Problem

The SemanticDB `rathena_struct` field had 48 names with no matching struct in rAthena's
`.hpp` files. This caused the VersionTable lookup to fail silently for these packets,
resulting in "struct not found in VersionTable" errors and generated code that couldn't
reference real field layouts.

These phantoms fell into two categories:
- **Category A (25 packets)**: Wrong name in SemanticDB — real rAthena struct exists under
  a different name (e.g. `PACKET_ZC_NOTIFY_STANDENTRY` → `packet_idle_unit`)
- **Category B (23 packets)**: Genuinely structless — rAthena uses raw RFIFOW/WFIFOW macro
  access with `packet_db[cmd].pos[]` offsets; no struct is defined

---

## Investigation Method

1. GCC preprocessing: `g++ -E -P -DPACKETVER=20181002 ... | grep "^struct "` to get all
   real struct names
2. `packets_struct.hpp` enum scan for `idle_unitType`, `spawn_unitType`, `unit_walkingType`
   etc. to map versioned packet IDs to real struct names
3. `clif_packetdb.hpp` analysis to find `parseable_packet(id, length, handler, pos[0], ...)`
   entries — these give the exact byte layout for structless packets
4. `clif.cpp` handler reading to confirm field types and semantics

---

## Files Changed

### `docs/PHANTOM_STRUCTS.md` (new)
Complete reference document with:
- Category A table: 25 packets with real struct name corrections
- Category B table: 23 structless packets with layout evidence

### `internal/codegen/stubs/synthetic_structs.hpp` (new)
Hand-written `__attribute__((packed))` structs for all Category B packets. All struct
sizes verified via `gcc -x c -o /tmp/check_sizes` to match the lengths in clif_packetdb.hpp.

Structs defined:
- `SYNTH_CZ_CONCLUDE_EXCHANGE_ITEM` (2 bytes)
- `SYNTH_CZ_CLOSE_STORE` (2 bytes)
- `SYNTH_CZ_NOTIFY_ACTORINIT` (2 bytes)
- `SYNTH_CZ_ITEM_PICKUP` / `SYNTH_CZ_ITEM_PICKUP2` (6 bytes)
- `SYNTH_CZ_REQ_NEXT_SCRIPT` (6 bytes)
- `SYNTH_CZ_REQUEST_TIME` / `SYNTH_CZ_REQUEST_TIME2` (6 bytes)
- `SYNTH_CZ_REQUEST_MOVE` / `SYNTH_CZ_REQUEST_MOVE2` (5 bytes)
- `SYNTH_CZ_USE_ITEM` (8 bytes)
- `SYNTH_CZ_MOVE_ITEM_FROM_BODY_TO_STORE` / `SYNTH_CZ_MOVE_ITEM_FROM_STORE_TO_BODY` (8 bytes)
- `SYNTH_CZ_ITEM_THROW2` (6 bytes)
- `SYNTH_CZ_USE_SKILL_TOGROUND` (10 bytes)
- `SYNTH_CZ_ENTER` (19 bytes)
- `SYNTH_CZ_REQUEST_CHAT` (variable)
- `SYNTH_CH_ENTER_0x0065` (17 bytes)
- `SYNTH_CH_ENTER` (37 bytes, partially unknown)
- `SYNTH_ZC_AID` (6 bytes)
- `SYNTH_ZC_PC_SELL_RESULT` (3 bytes)
- `SYNTH_ZC_PETEGG_LIST` (variable)

### `semantics/mappings.yaml`
- 48 `rathena_struct` fields corrected using precise string replacement (preserving YAML
  indentation for the custom state-machine loader)
- Added missing `PacketType` field at position 0 for packet 0x0360 (SYNTH_CZ_REQUEST_TIME2)

### `internal/codegen/gen/decode.go`
Updated `generateFieldReads` and `extractFieldName`:
- **New**: `isZeroLiteral()` function — detects "0", "false", "uint32(0)" etc. and emits a
  comment instead of "complex expression — implement manually"
- **New**: Handles `nil`/`null`/`""` field_mapping values as "absent in this version" markers
  instead of complex expression comments
- **Improved**: `extractFieldName` now supports:
  - Plain field names (`"FieldName"` without `packet.` prefix) — for future new-style
    field_mapping entries
  - `"packet.X[:]"` slice form (was broken before — regex matched wrong end)
  - `"[]byte(packet.X)"` byte-slice cast form
- Variable ordering fix in `generateFieldReads`: `goFieldName` and `goType` now computed
  before the nil/zero checks (previously declared after, causing "field not found" fallthrough)

---

## Key Findings

### Category A Corrections

| Old Name | Real Struct | Reason |
|---|---|---|
| `PACKET_ZC_NOTIFY_STANDENTRY*` | `packet_idle_unit` | Versioned via `idle_unitType` enum |
| `PACKET_ZC_NOTIFY_NEWENTRY*` | `packet_spawn_unit` | Versioned via `spawn_unitType` enum |
| `PACKET_ZC_NOTIFY_MOVEENTRY*` | `packet_unit_walking` | Versioned via `unit_walkingType` enum |
| `PACKET_DROPFLOORITEM` / `PACKET_ZC_ITEM_FALL_ENTRY3` | `packet_dropflooritem` | `dropflooritemType` |
| `PACKET_ZC_NORMAL_ITEMLIST` / `INVENTORY_ITEMLIST_NORMAL` | `packet_itemlist_normal` | `inventorylistnormalType` |
| `PACKET_ZC_EQUIPMENT_ITEMLIST` / `INVENTORY_ITEMLIST_EQUIP` | `packet_itemlist_equip` | `inventorylistequipType` |
| `PACKET_ZC_MSG_STATE_CHANGE` | `packet_sc_notick` | `status_change_endType = 0x196` |
| `PACKET_ZC_NOTIFY_ACT2` / `NOTIFY_ACT_DAMAGE` | `packet_damage` | `damageType` enum |
| `PACKET_ZC_SKILL_ENTRY` | `packet_skill_entry` | `skill_entryType` enum |
| `PACKET_ZC_STATUS_CHANGE2` | `packet_status_change2` | `status_change2Type = 0x43f` |
| `PACKET_ZC_HP_INFO` | `packet_monster_hp` | `clif.cpp:19947` comment confirms |
| `PACKET_ZC_USE_ITEM_ACK2` | `PACKET_ZC_USE_ITEM_ACK` | `useItemAckType = 0x1c8` for PACKETVER<=3 |

### Notes on Unused/Legacy Packets

- `PACKET_ZC_PAR_4JOB_CHANGE` (0x0B25): Only a `DEFINE_PACKET_ID` constant. No struct,
  no sending function. Not in `clif_packetdb.hpp`. **No action needed.**
- `PACKET_ZC_QUEST_NOTIFY_EFFECT` (0x02B3): Registered in clif_packetdb but no function
  sends it. The actual quest notify function sends 0x0446 instead. **Legacy/deprecated.**

---

## Gate Status

**76 PASS / 1 FAIL** — unchanged (the 1 expected failure is CH_MAKE_CHAR shuffle).

---

## Next Steps

1. **Extend VersionTable pipeline** to process `synthetic_structs.hpp` — currently the
   VersionTable only covers `packets_struct.hpp`. The SYNTH_ structs need to be included
   so the codegen can look them up.

2. **Extend VersionTable pipeline** to also cover `packets.hpp` (for PACKET_ZC_* structs
   defined there, not just in packets_struct.hpp).

3. **Re-run codegen** to regenerate `pkg/decode/` and `pkg/encode/` with improved
   extractFieldName and isZeroLiteral handling — expect reduction in "implement manually"
   comments from 185 files.

4. **Implement new field_mapping format migration** — eventually migrate all `packet.X`
   field_mapping values to plain field names, enabling full auto-derivation of Go expressions
   from C-type + canonical-Go-type pairs.
