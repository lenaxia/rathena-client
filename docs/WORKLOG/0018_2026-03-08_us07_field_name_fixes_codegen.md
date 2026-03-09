# Worklog 0018 — US-07: Category B/C Field Name Fixes + Codegen Run

**Date**: 2026-03-08  
**Session focus**: Regenerate `pkg/decode/` from the SemanticDB changes accumulated in the
previous session, then fix all remaining wrong-name field mappings down to the Category A
(flex array) floor.

---

## Context

The previous session had updated many SemanticDB field mappings (actor packets, skill packets,
account_server_info, move_to, etc.) but had **not run codegen**. The generated files in
`pkg/decode/` still reflected the old broken mappings. Skip count was 779.

---

## Step 1 — Run Codegen

```
go run ./internal/codegen/main.go --rathena ~/personal/rathena --out .
```

Result: **779 → 133 skips** (646 fixed in one pass).

The bulk of the improvement came from field renames applied in the previous session:
`headpalette`, `bodypalette`, `head`, `accessory`, `accessory2`, `accessory3`, `honor`,
`moveStartTime`, `sex`, `login_id1`, `login_id2`, `last_ip`, `last_login`, `srcId`, `dstId`,
`targetAID`, `srcAID`, `index`, `amount`, `dest`, `PosDir`, `grade`, `tab`, `packetType`,
`result`, etc.

---

## Step 2 — Audit Remaining 133 Skips

Categorised by field name:

| Field name(s) | Count | Category |
|---|---|---|
| `slot`, `list`/`List`, `items`, `message`, `details`, `char_servers`, `rgInfo`, `skills`, `skillInfo`, `skillIds`, `guildMemberInfo`, `posInfo`, `emblem_data`, `Castle_list`, `ItemList`, `Eggs`, `material_info`, `req`, `imageData`, `AID[]`, `chatMsg`, `effects`, `Textcontent`, `Message` | ~85 | **A — flex arrays** (need US-06) |
| `ClothesColor` | 18 | **C — wrong field name** (actor_moved 0x01DA/0x022C) |
| `ITID`, `ITID2`, `IsIdentified`, `XPos`, `YPos` | 10 | **C — wrong field name** (item_exists 0x009D) |
| `StatusID`, `Flag` | 2 | **C — wrong field name** (actor_status_active 0x0196) |
| `Job` (capital) | 2 | **C — wrong field name** (entity_spawn 0x007C) |
| `Result`, `MenuIndex`, `Num`, `MasterGID`, `Unused1`, `Unused2`, `Align`, `MsgId`, `PpId`, `Data`, `Own_char` | 11 | **B — wrong case** (various packets) |
| `shield`, `grade`, `location`, `look`, `remainingUses`, `masterGID`, `name`, `Name`, `itemId` | ~15 | **Expected** — field genuinely absent in older PACKETVER branch |

---

## Step 3 — Fixes Applied

### actor_moved (0x01DA, 0x022C) — ClothesColor → bodypalette

Both packets use `packet_unit_walking` which has `bodypalette`, not `ClothesColor`.
The `0x007B` implementation already had the correct mapping; `0x01DA` and `0x022C`
were left with the old OpenKore name.

```yaml
# before
Clothes_color: uint16(packet.ClothesColor)
# after
Clothes_color: uint16(packet.bodypalette)
```

### item_exists (0x009D) — ITID/ITID2/IsIdentified/XPos/YPos

`PACKET_ZC_ITEM_ENTRY` struct:
```c
uint32 itemId;   // was ITID
uint8  identify; // was IsIdentified
uint16 x;        // was XPos
uint16 y;        // was YPos
// no ITID2 field at all
```

Mappings corrected: `ITID→packet.itemId`, `IsIdentified→packet.identify`,
`XPos→packet.x`, `YPos→packet.y`, `ITID2→uint16(0)` (field doesn't exist).

### actor_status_active (0x0196) — StatusID/Flag

`packet_sc_notick` struct has `index` (not `StatusID`) and `state` (not `Flag`).

```yaml
# before
StatusID: packet.StatusID
Active:   packet.Flag
# after
StatusID: packet.index
Active:   packet.state
```

### entity_spawn (0x007C) — Job → job

`packet_spawn_unit2` has `job` (lowercase), not `Job`.

```yaml
# before
EntityType: uint16(packet.Job)
# after
EntityType: uint16(packet.job)
```

### Batch case-sensitivity fixes

| Action | Packet | Param | Before | After |
|---|---|---|---|---|
| `zc_ack_takeoff_equip_all` | 0x0BAE | Result | `packet.Result` | `packet.result` |
| `cz_choose_menu_zero` | 0x0BA8 | MenuIndex | `packet.MenuIndex` | `packet.menuIndex` |
| `zc_soulenergy` | 0x0B73 | Num | `packet.Num` | `packet.num` |
| `cz_approximate_actor` | 0x0BB0 | MasterGID | `packet.MasterGID` | `packet.masterGID` |
| `cz_approximate_actor` | 0x0BB0 | Unused1 | `packet.Unused1` | `packet.unused1` |
| `cz_approximate_actor` | 0x0BB0 | Unused2 | `packet.Unused2` | `packet.unused2` |
| `zc_dialog_text_align` | 0x0BA1 | Align | `packet.Align` | `packet.align` |
| `zc_response_enchant` | 0x0B9F | MsgId | `packet.MsgId` | `packet.msgId` |
| `zc_specialpopup` | 0x0BBE | PpId | `packet.PpId` | `packet.ppId` |
| `zc_ui_open2` | 0x0B9A | Data | `packet.Data` | `packet.data` |
| `cz_checkname2` | 0x0B97 | Own_char | `packet.Own_char` | `packet.own_char` |

---

## Step 4 — Re-run Codegen

**133 → 90 skips** after second codegen pass.

---

## Final Skip Breakdown (90 remaining)

All 90 remaining skips are correct and expected:

**~73 Category A — flex arrays** (will be fixed by US-06):
- `slot` (14) — `struct EQUIPSLOTINFO slot` embedded in exchange/refine item packets
- `list` / `List` (13) — barter market item lists
- `items` (10) — item list arrays in various inventory packets
- `message` (5) — `char message[]` in chat/dialog packets
- `effects` (1), `chatMsg` (1), `Textcontent` (1), `Message` (1) — various flex char/uint16 arrays
- `details` (2), `rgInfo` (1), `skills`/`skillInfo`/`skillIds` (3), `guildMemberInfo` (1) — nested struct flex arrays
- `char_servers`/`Char_servers` (4), `posInfo` (2), `emblem_data` (2), `Castle_list` (1), `ItemList` (1), `Eggs` (1), `material_info` (1), `req` (1), `imageData` (3), `AID[]` (1) — misc flex arrays

**~17 genuinely absent in older PACKETVER branches**:
- `shield` (4) — not in `packet_idle_unit` / `packet_spawn_unit` (old structs)
- `grade` (4) — only in `PACKETVER >= 20200916`
- `location` (2), `look` (2) — only in `PACKETVER >= 20161102`
- `remainingUses` (1) — only in `PACKETVER >= 20100701`
- `masterGID` (1) — absent in `PACKET_ZC_GUILD_INFO` before `0x0a84`
- `itemId` (3) — absent in newer `PACKET_ZC_PROPERTY_HOMUN` variants (field removed)
- `name` (1) — flex in newer `PACKET_ZC_INVENTORY_START`, fixed-length `char name[NAME_LENGTH]` in older (older version branch correctly omits flex)
- `Name` (1) — `PACKET_ZC_CHECKNAME` 0x0A14 genuinely has no Name field
- `itemType` (1) — absent in oldest `PACKET_ZC_ADD_EXCHANGE_ITEM` version

---

## Files Changed

| File | Change |
|---|---|
| `semantics/mappings.yaml` | 22 field mapping corrections across 15 actions |
| `pkg/decode/` (generated) | Full regeneration: 442 files |

---

## Test Results

```
go test -race ./...  →  all PASS (11 packages)
grep "field.*not found" pkg/decode/ | wc -l  →  90
```

---

## Skip Count History

| After | Skips | Delta |
|---|---|---|
| Before this session (codegen not yet run) | 779 | — |
| After first codegen run | 133 | -646 |
| After 22 additional SemanticDB fixes + second codegen | 90 | -43 |

---

## Next Steps

- **US-06**: Flex array parser in codegen — eliminates ~73 remaining Category A skips
- **US-08** (phase 7): goKore integration — wire decoded events into the bot engine
