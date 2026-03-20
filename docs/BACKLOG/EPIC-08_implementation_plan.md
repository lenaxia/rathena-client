# EPIC-08 Implementation Plan — All 55 Missing Packets

**Date**: 2026-03-20  
**Supersedes**: EPIC-08 story descriptions (these are the actionable, step-by-step instructions)

---

## Corrections to EPIC-08 Story Assignments

Two errors discovered during detailed investigation:

1. **`0x0274` is NOT `ac_accept_login`**. Both `kRO/Sakexe_0.pm` and `ServerType0.pm` map it to `mail_return`. It is `ZC_MAIL_RECEIVE` (mail notification, 8 bytes). It belongs to a new `mail_receive` action, not to `ac_accept_login`. Reclassified from CRITICAL to MEDIUM.

2. **`0x02EE` is `actor_exists` (idle), NOT `actor_moved`**. The `idle_unitType = 0x2ee` enum in `packets_struct.hpp` confirms rAthena uses `packet_idle_unit` for this PID. OpenKore's classification as `actor_moved` is incorrect. Struct is `packet_idle_unit`, action is `actor_exists`.

**Revised total: 55 PIDs, 27 actions, 7 new actions to create, 9 SYNTH_ structs needed.**

---

## How the codegen pipeline works (reference)

1. Edit `semantics/mappings.yaml` — add/modify `semantic_actions` entries
2. For Cat B only: add SYNTH_ struct to `internal/codegen/stubs/synthetic_structs.hpp`
3. Run `go generate ./internal/codegen/...` (or `go run ./internal/codegen/`)
4. Verify generated files: `pkg/decode/`, `pkg/events/`, `pkg/session/receive_dispatch.go`
5. `go test ./...`

---

## GROUP 1: Pure YAML — add implementations to existing actions (32 PIDs)

All Cat A packets. **Zero C++ work.** Just add `implementations` entries to existing actions in `semantics/mappings.yaml`, then run codegen.

---

### 1a. `zc_accept_enter` — add `0x0073`, `0x02EB`

**Current**: 1 impl (`0x0A18`)  
**Add 2 impls**:

```yaml
# In semantic_actions.zc_accept_enter.implementations, add:
- packet_id: "0x0073"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range:
    - null          # from the beginning
    - 20080101      # < 20080102
  field_mapping: {}

- packet_id: "0x02EB"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range:
    - null          # from the beginning (this reappears)
    - 20141021      # < 20141022 (0x0A18 takes over; 0x02EB reappears at 20160330)
  field_mapping: {}
```

**rAthena structs** (`map/packets.hpp`):
```c
// 0x0073: PACKETVER < 20080102
struct PACKET_ZC_ACCEPT_ENTER {
    int16 packetType; uint32 startTime; uint8 posDir[3]; uint8 xSize; uint8 ySize;
}; // 11 bytes

// 0x02EB: PACKETVER < 20141022 || PACKETVER >= 20160330
struct PACKET_ZC_ACCEPT_ENTER {
    int16 packetType; uint32 startTime; uint8 posDir[3]; uint8 xSize; uint8 ySize; uint16 font;
}; // 13 bytes
```

**Codegen behavior**: The same `events.ZcAcceptEnter` struct covers all variants (union of fields). The decoder for each PID will read the fields present in its struct version.

**Note on `0x02EB` reappearance**: At `>= 20160330`, `0x02EB` is used again (rAthena switched back). Add a second impl:
```yaml
- packet_id: "0x02EB"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range:
    - 20160330
    - null
  field_mapping: {}
```
Or collapse them with `packetver_range: [null, null]` and let the decoder handle the field differences — but two separate entries with correct ranges is cleaner.

---

### 1b. `item_pickup` — add `0x029A`, `0x02D4`, `0x0990`, `0x0A0C`, `0x0B41`

**Current**: 2 impls (`0x00A0` pv=[20030000,20181120], `0x0A37` pv=[20150226,null])  
**Problem**: `0x0A37` has range starting at 20150226 but `0x0A0C` also covers 20150226. These are `#elif` branches in rAthena — exactly one is active per PACKETVER. The range assignment must be exclusive. The correct split from rAthena `packets_struct.hpp`:

```
0x00A0:  else (< 20061218)          — classic, no HireExpireDate
0x029A:  >= 20061218 && < 20071002  — adds HireExpireDate
0x02D4:  >= 20071002 && < 20120925  — adds bindOnEquipType
0x0990:  >= 20120925 && < 20150226  — adds location as uint32 (was uint16)
0x0A0C:  >= 20150226 && < 20200916  — adds option_data
0x0A37:  >= 20150226 (we have this, same range as 0x0A0C — check!)
0x0B41:  >= 20200916 MAIN, >= 20200723 RE
```

**Wait — `0x0A37` and `0x0A0C` overlap!** Check the rAthena `#elif` chain:

From `packets_struct.hpp`:
```c
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x0b41)  // main/re >= 2020
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x0a37)  // (between 0x0a0c and 0x0b41?)
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x0a0c)  // >= 20150226
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x0990)  // >= 20120925
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x02d4)  // >= 20071002
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x029a)  // >= 20061218
DEFINE_PACKET_HEADER(ZC_ITEM_PICKUP_ACK, 0x00a0)  // else
```

The `0x0A37` is listed between `0x0A0C` and `0x0B41`. Its exact packetver condition must be read from the struct. The current impl in our DB has `pv=[20150226, null]` which overlaps with `0x0B41`. This needs a fix — give `0x0A37` a `packetver_max`.

**Correct ranges to set**:
```yaml
- packet_id: "0x00A0"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [null, 20061217]

- packet_id: "0x029A"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20061218, 20071001]

- packet_id: "0x02D4"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20071002, 20120924]

- packet_id: "0x0990"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20120925, 20150225]

- packet_id: "0x0A0C"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20150226, 20160920]  # approximate: before 0x0A37

- packet_id: "0x0A37"  # EXISTING — update packetver_max
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20160921, 20200915]  # check exact 0x0a37 condition from packets_struct.hpp

- packet_id: "0x0B41"
  struct_name: PACKET_ZC_ITEM_PICKUP_ACK
  packetver_range: [20200916, null]  # MAIN >= 20200916
```

**Action**: Read `packets_struct.hpp` lines 540–594 precisely to get exact `#elif` dates for `0x0A37`.

---

### 1c. `received_characters_page` — add `0x0B72`

**Current**: 1 impl (`0x099D` pv=[20100803, null])  
**Add**:
```yaml
- packet_id: "0x0B72"
  struct_name: PACKET_HC_ACK_CHARINFO_PER_PAGE
  packetver_range: [20201007, null]  # MAIN >= 20201007 || RE >= 20211103
  field_mapping: {}
```
Also update `0x099D` packetver_max to `20201006`.

---

### 1d. `add_exchange_item` — add `0x080F`, `0x0A09`, `0x0A96`

**Current**: 1 impl (`0x00E9` pv=[null, null])  
**Struct** (`PACKET_ZC_ADD_EXCHANGE_ITEM` from `packets_struct.hpp`): same struct, fields vary by packetver.

```yaml
- packet_id: "0x00E9"   # UPDATE packetver_max
  struct_name: PACKET_ZC_ADD_EXCHANGE_ITEM
  packetver_range: [null, 20100222]

- packet_id: "0x080F"
  struct_name: PACKET_ZC_ADD_EXCHANGE_ITEM
  packetver_range: [20100223, 20150225]

- packet_id: "0x0A09"
  struct_name: PACKET_ZC_ADD_EXCHANGE_ITEM
  packetver_range: [20150226, 20161101]  # < MAIN 20161102 / RE 20161026

- packet_id: "0x0A96"
  struct_name: PACKET_ZC_ADD_EXCHANGE_ITEM
  packetver_range: [20161026, null]  # MAIN >= 20161102 || RE >= 20161026 || ZERO
```

**PACKET_ZC_ADD_EXCHANGE_ITEM struct** (relevant field changes):
- `< 20100223`: `amount(i32) + itemId(u16)` order
- `>= 20100223`: `itemId(u16) + itemType(u8) + amount(i32)` order  
- `>= 20181121`: `itemId(u32)` (widened)
- `>= 20150226`: adds `option_data[MAX_ITEM_OPTIONS]`
- `>= 20161026 MAIN`: adds `location(u32) + look(u16)`
- `>= 20200916 MAIN`: `refine` moves to end, adds `grade`

---

### 1e. `zc_shortcut_key_list` — add `0x07D9`, `0x0A00`, `0x0B20`

**Current**: 1 impl (`0x02B9` pv=[null, null])

**`hotkey_data` struct**: `isSkill(i8) + id(u32) + count(i16)` = 7 bytes each.

```
0x02B9: MAX_HOTKEYS_PACKET=27, no rotate, no tab  — 2 + 27*7 = 191 bytes
0x07D9: MAX_HOTKEYS_PACKET=36 (>= 20090603) or 38 (>= 20090617), no rotate, no tab
0x0A00: MAX_HOTKEYS_PACKET=38, with rotate(i8) + tab(i16)
0x0B20: MAX_HOTKEYS_PACKET=38, with rotate(i8) + tab(i16)
```

```yaml
- packet_id: "0x02B9"   # UPDATE packetver_max
  struct_name: PACKET_ZC_SHORTCUT_KEY_LIST
  packetver_range: [null, 20090616]

- packet_id: "0x07D9"
  struct_name: PACKET_ZC_SHORTCUT_KEY_LIST
  packetver_range: [20090617, 20141021]  # MAIN/RE/SAK >= 20090617

- packet_id: "0x0A00"
  struct_name: PACKET_ZC_SHORTCUT_KEY_LIST
  packetver_range: [20141022, 20190521]  # MAIN >= 20141022 / RE >= 20141015

- packet_id: "0x0B20"
  struct_name: PACKET_ZC_SHORTCUT_KEY_LIST
  packetver_range: [20190522, null]  # MAIN >= 20190522
```

---

### 1f. `zc_guild_info` — add `0x01B6`, `0x0B7B`

**Current**: 1 impl (`0x0A84` pv=[null, null])

**Struct differences** (`PACKET_ZC_GUILD_INFO` from `packets_struct.hpp`):
- `else` (pre-2016): has `guildname + masterName + manageLand + zeny` (field order A)
- `>= 20161019 MAIN / >= 20160921 RE` incl. `0x0A84`: same fields but `masterGID` instead of `masterName` (field order B)
- `>= 20200902` incl. `0x0B7B`: adds `masterGID` AND `masterName` together

```yaml
- packet_id: "0x01B6"
  struct_name: PACKET_ZC_GUILD_INFO
  packetver_range: [null, 20160920]  # else branch

- packet_id: "0x0A84"   # UPDATE packetver_max
  struct_name: PACKET_ZC_GUILD_INFO
  packetver_range: [20160921, 20200901]

- packet_id: "0x0B7B"
  struct_name: PACKET_ZC_GUILD_INFO
  packetver_range: [20200902, null]
```

---

### 1g. `zc_equipwin_microscope` — add `0x0859`, `0x0906`, `0x0997`, `0x0A2D`, `0x0B03`

**Current**: 1 impl (`0x02D7` pv=[null, null])  
**All use `PACKET_ZC_EQUIPWIN_MICROSCOPE`**. The struct gains fields progressively:

| PID | Added fields vs prev | Condition |
|---|---|---|
| `0x02D7` | baseline (no robe) | `< 20071211` (we have) |
| `0x0859` | +`robe` | `>= 20101123` |
| `0x0906` | same as 0x0859 | MAIN `>= 20111207` / RE `>= 20111122` |
| `0x0997` | same | MAIN `>= 20121205` / RE `>= 20121107` |
| `0x0A2D` | +`sex` | `>= 20140820` |
| `0x0B03` | +`body2` | MAIN/RE `>= 20180801` |

Note: `0x0859`/`0906`/`0997` have same struct layout (robe added). Codegen handles the packetver-conditional field decode.

```yaml
- packet_id: "0x02D7"   # UPDATE packetver_max
  packetver_range: [null, 20101122]

- packet_id: "0x0859"
  struct_name: PACKET_ZC_EQUIPWIN_MICROSCOPE
  packetver_range: [20101123, 20111206]

- packet_id: "0x0906"
  struct_name: PACKET_ZC_EQUIPWIN_MICROSCOPE
  packetver_range: [20111122, 20121204]  # RE 20111122 / MAIN 20111207

- packet_id: "0x0997"
  struct_name: PACKET_ZC_EQUIPWIN_MICROSCOPE
  packetver_range: [20121107, 20140819]

- packet_id: "0x0A2D"
  struct_name: PACKET_ZC_EQUIPWIN_MICROSCOPE
  packetver_range: [20140820, 20180800]

- packet_id: "0x0B03"
  struct_name: PACKET_ZC_EQUIPWIN_MICROSCOPE
  packetver_range: [20180801, null]
```

---

### 1h. `skill_add` — add `0x0B31`

**Current**: 1 impl (`0x0111`)  
**Add**:
```yaml
- packet_id: "0x0B31"
  struct_name: PACKET_ZC_ADD_SKILL
  packetver_range: [20190807, null]  # RE >= 20190807 / ZERO >= 20190918
```
`PACKET_ZC_ADD_SKILL` embeds `struct SKILLDATA`. `SKILLDATA` has a `level2` field at `>= 20190807`. The existing `0x0111` impl covers all packetvers; the decoder already handles this conditionally. Adding `0x0B31` just registers the new PID.

---

### 1i. `skills_list` — add `0x0B32`

**Current**: 1 impl (`0x010F`)  
**Add**:
```yaml
- packet_id: "0x0B32"
  struct_name: PACKET_ZC_SKILLINFO_LIST
  packetver_range: [20190807, null]
```
`PACKET_ZC_SKILLINFO_LIST` = header + length + `SKILLDATA[]`. Same struct, new PID.

---

### 1j. `skill_update` — add `0x0B33`

**Current**: 1 impl (`0x010E`)  
`0x07E1` (`ZC_SKILLINFO_UPDATE2`) is already in our DB under `zc_skillinfo_update2`. But we also need `0x0B33` under `skill_update`.

**Decision**: Both `zc_hoskillinfo_update` (0x0239) and `zc_skillinfo_update2` (0x07E1) fire under `skill_update`-related actions. Check which action `0x0B33` belongs to:
- `0x0B33` = `PACKET_ZC_SKILLINFO_UPDATE2` (RE/ZERO 2019+) → should go in `zc_skillinfo_update2`

```yaml
# Add to zc_skillinfo_update2:
- packet_id: "0x0B33"
  struct_name: PACKET_ZC_SKILLINFO_UPDATE2
  packetver_range: [20190807, null]
```

`PACKET_ZC_SKILLINFO_UPDATE2` (from `packets_struct.hpp`): adds `level2` field at this packetver. The existing `0x07E1` decoder handles it conditionally.

---

### 1k. `zc_ho_par_change` and `zc_el_par_change` — new actions + PIDs

Both actions are **new** (don't exist in `semantic_actions`).

**`zc_ho_par_change`**:
```yaml
zc_ho_par_change:
  description: "Homunculus stat change"
  openkore_name: ""
  canonical_params: []
  implementations:
    - packet_id: "0x07DB"
      struct_name: PACKET_ZC_HO_PAR_CHANGE
      packetver_range: [null, 20209999]  # else branch (before 0xBA5)
      field_mapping: {}
```

`PACKET_ZC_HO_PAR_CHANGE` (two variants):
```c
// < 20210000 MAIN / < 20211103 RE:
{ int16 packetType; uint16 type; int32 value; }  // 8 bytes

// >= 20210000 MAIN (0xBA5, already handled separately if in DB):
{ int16 packetType; uint16 type; uint64 value; }  // 12 bytes
```
Note: `0xBA5` is a separate PID we may or may not have. Check and add if needed.

**`zc_el_par_change`**:
```yaml
zc_el_par_change:
  description: "Elemental stat change"
  openkore_name: ""
  canonical_params: []
  implementations:
    - packet_id: "0x081E"
      struct_name: PACKET_ZC_EL_PAR_CHANGE
      packetver_range: [null, null]
      field_mapping: {}
```

`PACKET_ZC_EL_PAR_CHANGE`: `{ int16 packetType; uint16 type; uint32 value; }` — unconditional, 8 bytes.

---

### 1l. New actions for equip acks: `zc_req_takeoff_equip_ack` and `zc_req_wear_equip_ack`

**`zc_req_takeoff_equip_ack`** (new action):
```yaml
zc_req_takeoff_equip_ack:
  description: "Server acknowledges unequip request"
  openkore_name: "unequip_item"
  implementations:
    - packet_id: "0x00AC"
      struct_name: PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK
      packetver_range: [null, 20110823]
    - packet_id: "0x08D1"
      struct_name: PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK
      packetver_range: [20110824, 20129999]
    - packet_id: "0x099A"
      struct_name: PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK
      packetver_range: [20130000, null]
```

**rAthena struct** (`map/packets.hpp`):
```c
// else (< 20110824):
{ uint16 packetType; uint16 index; uint16 wearLocation; bool flag; }  // 8 bytes

// >= 20110824 && < 20130000:
{ uint16 packetType; uint16 index; uint16 wearLocation; uint8 flag; }  // 8 bytes (bool→uint8)

// >= 20130000:
{ uint16 packetType; uint16 index; uint32 wearLocation; uint8 flag; }  // 10 bytes (location widened to u32)
```

**`zc_req_wear_equip_ack`** (new action):
```yaml
zc_req_wear_equip_ack:
  description: "Server acknowledges equip request"
  openkore_name: "equip_item"
  implementations:
    - packet_id: "0x00AA"    # PRE-existing PID — check if already in DB
      struct_name: PACKET_ZC_REQ_WEAR_EQUIP_ACK
      packetver_range: [null, 20101122]
    - packet_id: "0x0999"
      struct_name: PACKET_ZC_REQ_WEAR_EQUIP_ACK
      packetver_range: [20121107, null]  # MAIN >= 20121205 / RE >= 20121107
```

**Action**: Check if `0x00AA` is already in DB under a different action (`zc_req_wear_equip_ack` doesn't exist yet). The struct:
```c
// < 20101123:
{ int16 PacketType; uint16 index; uint16 wearLocation; uint8 result; }  // 8 bytes

// >= 20101123 && < 20121205 MAIN:
{ int16 PacketType; uint16 index; uint16 wearLocation; uint16 wItemSpriteNumber; uint8 result; }  // 10 bytes

// >= 20121205 MAIN (0x0999):
{ int16 PacketType; uint16 index; uint32 wearLocation; uint16 wItemSpriteNumber; uint8 result; }  // 12 bytes
```

---

### 1m. Auth: `received_characters_page` `0x0B72` and new `char_created` action

**`received_characters_page`**: see 1c above.

**`char_created`** (new action):
```yaml
char_created:
  description: "Char server accepts character creation"
  openkore_name: "character_creation_successful"
  implementations:
    - packet_id: "0x006D"
      struct_name: PACKET_HC_ACCEPT_MAKECHAR
      packetver_range: [null, 20201006]
    - packet_id: "0x0B6F"
      struct_name: PACKET_HC_ACCEPT_MAKECHAR
      packetver_range: [20201007, null]
```

Both use identical `PACKET_HC_ACCEPT_MAKECHAR`:
```c
struct PACKET_HC_ACCEPT_MAKECHAR {
    int16 packetType;
    CHARACTER_INFO character;
};
```
`CHARACTER_INFO` is a large struct with many `#if PACKETVER` fields (already handled by existing codegen for `received_characters` since it uses the same type).

---

## GROUP 2: Actor packets — existing structs, just add PIDs (5 PIDs)

**Key finding**: `0x022A`, `0x022B`, `0x02EE`, `0x09DC`, `0x09DD` all use structs that **already exist** in rAthena and are **already used** by other packet IDs in our DB. No SYNTH_ required.

| PID | Action | Struct | Packetver range | Why struct exists |
|---|---|---|---|---|
| `0x022A` | `actor_exists` | `packet_idle_unit` | `[20050411, 20080101]` | `idle_unitType=0x022a` |
| `0x022B` | `actor_connected` | `packet_spawn_unit` | `[20050411, 20080101]` | `new_unitType=0x022b` |
| `0x02EE` | `actor_exists` | `packet_idle_unit` | `[20080102, 20091102]` | `idle_unitType=0x02ee` |
| `0x09DC` | `actor_connected` | `packet_spawn_unit` | `[20131223, 20150512]` | `new_unitType=0x09dc` |
| `0x09DD` | `actor_exists` | `packet_idle_unit` | `[20131223, 20150512]` | `idle_unitType=0x09dd` |

**Clarification on `0x02EE`**: Despite OpenKore calling it `actor_moved`, rAthena's `idle_unitType = 0x02ee` definitively places it as an idle/standing-unit notification using `packet_idle_unit`. The 60-byte length at that packetver matches `sizeof(packet_idle_unit)` with `PACKETVER >= 20080102` fields (effectState widens from i16 to i32, weapon widens from u16 to u32 = +4 bytes over pre-2008). OpenKore's naming is wrong.

**YAML additions**:
```yaml
# actor_exists — add:
- packet_id: "0x022A"
  struct_name: packet_idle_unit
  packetver_range: [20050411, 20080101]
- packet_id: "0x02EE"
  struct_name: packet_idle_unit
  packetver_range: [20080102, 20091102]
- packet_id: "0x09DD"
  struct_name: packet_idle_unit
  packetver_range: [20131223, 20150512]

# actor_connected — add:
- packet_id: "0x022B"
  struct_name: packet_spawn_unit
  packetver_range: [20050411, 20080101]
- packet_id: "0x09DC"
  struct_name: packet_spawn_unit
  packetver_range: [20131223, 20150512]
```

Also update existing packetver_max for neighboring PIDs to avoid gaps:
- `actor_exists` `0x01D8`: add `packetver_max: 20050410`
- `actor_exists` `0x02EC`: This is listed under `actor_exists` but is `packet_unit_walking` — check if this is a data error.
- `actor_connected` `0x01D9`: add `packetver_max: 20050410`

---

## GROUP 3: Cat D — Inventory list packets (9 PIDs, 2 new actions)

These use `packet_itemlist_normal` and `packet_itemlist_equip` — structs that already exist in rAthena and are already used by codegen (for the 2018+ versions via `ZC_INVENTORY_START`).

**Key complexity**: For `>= 20181002` (MAIN), rAthena wraps inventory inside `ZC_INVENTORY_START`/`ZC_INVENTORY_END` frames. The `0xB09` PID is currently mapped to `zc_inventory_start` (the session-boundary marker). The actual item list packets (`0x0991` through `0x0B39`) are **separate** from `ZC_INVENTORY_START`.

**Investigation step required**: Before implementing, confirm that `0xB09` in our current `zc_inventory_start` action is correct and that `packet_itemlist_normal` at `>= 20181002` uses a DIFFERENT PID for the actual items (it should, since the inventory items are sent item-by-item inside the `START`/`END` frame envelope using the same per-item packets).

**Assuming the above is correct**:

**New action `inventory_items_stackable`**:
```yaml
inventory_items_stackable:
  description: "Bulk stackable inventory items (on login or map change)"
  openkore_name: "inventory_items_stackable"
  implementations:
    - packet_id: "0x00A3"
      struct_name: packet_itemlist_normal
      packetver_range: [null, 20071001]         # else (< 20071002)
    - packet_id: "0x01EE"
      struct_name: packet_itemlist_normal
      packetver_range: [20071002, 20080101]     # >= 20071002 && < 20080102
    - packet_id: "0x02E8"
      struct_name: packet_itemlist_normal
      packetver_range: [20080102, 20120924]     # >= 20080102 && < 20120925
    - packet_id: "0x0991"
      struct_name: packet_itemlist_normal
      packetver_range: [20120925, 20181001]     # >= 20120925 (pre-2018 RE/ZERO wrap)
```

**New action `inventory_items_equip`**:
```yaml
inventory_items_equip:
  description: "Bulk equippable inventory items (on login or map change)"
  openkore_name: "inventory_items_equip"
  implementations:
    - packet_id: "0x00A4"
      struct_name: packet_itemlist_equip
      packetver_range: [null, 20071001]
    - packet_id: "0x0992"
      struct_name: packet_itemlist_equip
      packetver_range: [20120925, 20150225]
    - packet_id: "0x0A0D"
      struct_name: packet_itemlist_equip
      packetver_range: [20150226, 20181001]
    - packet_id: "0x0B0A"
      struct_name: packet_itemlist_equip
      packetver_range: [20181002, 20200915]     # MAIN >= 20181002
    - packet_id: "0x0B39"
      struct_name: packet_itemlist_equip
      packetver_range: [20200916, null]         # MAIN >= 20200916
```

**Note on missing 2007–2012 equip PIDs**: `0x0295` (`>= 20071002 && < 20080102`) and `0x02D0` (`>= 20080102 && < 20120925`) are also missing from our 55-gap list but are genuine gaps. Add them too:
```yaml
    - packet_id: "0x0295"
      struct_name: packet_itemlist_equip
      packetver_range: [20071002, 20080101]
    - packet_id: "0x02D0"
      struct_name: packet_itemlist_equip
      packetver_range: [20080102, 20120924]
```

---

## GROUP 4: Cat B — SYNTH_ structs required (9 packets, 9 SYNTH_ structs)

For each, the steps are:
1. Add SYNTH_ struct to `internal/codegen/stubs/synthetic_structs.hpp`
2. Add impl to `semantic_actions`
3. Run codegen

---

### 4a. `received_map_server_info` — `0x0071` (28 bytes)

**SYNTH_ struct** (`synthetic_structs.hpp`):
```cpp
// 0x0071 HC_NOTIFY_ZONESVR (pre-0x0081 format, 28 bytes)
// Used before rAthena introduced 0x0081. Layout identical to PACKET_HC_NOTIFY_ZONESVR
// without the domain field. Source: char_clif.cpp chclif_send_map_data() + OpenKore 
// Sakexe_0.pm 0x0071 pack='a4 Z16 a4 v'
// MAP_NAME_LENGTH_EXT = 16 (MAP_NAME_LENGTH(12) + 4)
struct SYNTH_HC_NOTIFY_ZONESVR_OLD {
    int16  packetType;   // = 0x0071
    uint32 CID;          // character ID
    char   mapname[16];  // MAP_NAME_LENGTH_EXT = 16
    uint32 ip;           // map server IP (network byte order)
    uint16 port;         // map server port
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_HC_NOTIFY_ZONESVR_OLD, 0x0071);
```

**YAML**:
```yaml
# received_map_server_info — add:
- packet_id: "0x0071"
  struct_name: SYNTH_HC_NOTIFY_ZONESVR_OLD
  packetver_range: [null, 20170314]   # before 0x0081 took over at ~20170315
```

Also update `0x0081` packetver_min to `20170315` if not already set, and add `0x0081` if it's missing:
```yaml
- packet_id: "0x0081"
  struct_name: PACKET_HC_NOTIFY_ZONESVR
  packetver_range: [null, 20170314]    # < 20170315
```
(Check: `0x0081` is `PACKET_HC_NOTIFY_ZONESVR` without domain, same layout as `SYNTH_HC_NOTIFY_ZONESVR_OLD`. They may share the struct if the #else branch of `PACKET_HC_NOTIFY_ZONESVR` matches.)

---

### 4b. `exp` — `0x07F6` (14 bytes) + `0x0ACC` (18 bytes), **new action**

**SYNTH_ structs** (`synthetic_structs.hpp`):
```cpp
// 0x07F6 EXP gain notification (pre-20170830, 14 bytes)
// Source: clif.cpp clif_displayexp()
// WFIFOW(fd,0)=cmd, WFIFOL(fd,2)=sd->id, WFIFOL(fd,6)=exp, WFIFOW(fd,10)=type, WFIFOW(fd,12)=quest
struct SYNTH_ZC_LONG_PAR_CHANGE {
    int16  packetType;   // = 0x07F6
    uint32 aid;          // actor ID
    uint32 exp;          // experience value
    uint16 type;         // SP_BASEEXP=1, SP_JOBEXP=2
    uint16 quest;        // 1 if quest exp, 0 otherwise
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_LONG_PAR_CHANGE, 0x07f6);

// 0x0ACC EXP gain notification (>= 20170830, 18 bytes)
// Source: clif.cpp clif_displayexp()  
// WFIFOW(fd,0)=cmd, WFIFOL(fd,2)=sd->id, WFIFOQ(fd,6)=exp(int64), WFIFOW(fd,14)=type, WFIFOW(fd,16)=quest
struct SYNTH_ZC_LONG_PAR_CHANGE2 {
    int16  packetType;   // = 0x0ACC
    uint32 aid;          // actor ID
    uint64 exp;          // experience value (int64 in modern format)
    uint16 type;         // SP_BASEEXP=1, SP_JOBEXP=2
    uint16 quest;        // 1 if quest exp, 0 otherwise
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_LONG_PAR_CHANGE2, 0x0acc);
```

**New action**:
```yaml
exp:
  description: "Base or job experience gain notification"
  openkore_name: "exp"
  canonical_params: []
  implementations:
    - packet_id: "0x07F6"
      struct_name: SYNTH_ZC_LONG_PAR_CHANGE
      packetver_range: [null, 20170829]
      field_mapping: {}
    - packet_id: "0x0ACC"
      struct_name: SYNTH_ZC_LONG_PAR_CHANGE2
      packetver_range: [20170830, null]
      field_mapping: {}
```

---

### 4c. `stat_update` — `0x02A2` (8 bytes)

**Struct**: same layout as `PACKET_ZC_PAR_CHANGE` (`0x00B0`): `packetType(i16) + type(i16) + value(u32)` = 8 bytes. But `0x02A2` was introduced in 2006 as a variant with no gid field (unlike `PACKET_ZC_PAR_CHANGE_USER` which has gid).

```cpp
// 0x02A2 — stat update (alternate PID, 8 bytes)
// Source: clif_packetdb.hpp packet(0x02a2, 8), OpenKore pack='v V' fields=[type, val]
// Same layout as PACKET_ZC_PAR_CHANGE
struct SYNTH_ZC_PAR_CHANGE2 {
    int16  packetType;   // = 0x02A2
    int16  type;         // stat type (SP_*)
    uint32 value;        // new value
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_PAR_CHANGE2, 0x02a2);
```

**YAML**:
```yaml
# stat_update — add:
- packet_id: "0x02A2"
  struct_name: SYNTH_ZC_PAR_CHANGE2
  packetver_range: [20060424, null]
```

---

### 4d. `pin_code_request` — `0x02AD` (8 bytes)

**Struct**:
```cpp
// 0x02AD — PIN code / second password login request
// Source: clif_packetdb.hpp packet(0x02ad, 8), OpenKore pack='v V' fields=[flag, key]
struct SYNTH_HC_SECOND_PASSWD_LOGIN_OLD {
    int16  packetType;   // = 0x02AD
    uint16 flag;         // 0=correct, 1=incorrect, 2=no pin set, 3=need to create
    uint32 key;          // random key used for PIN encoding
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_HC_SECOND_PASSWD_LOGIN_OLD, 0x02ad);
```

Note: We already have `0x08B9` (`PACKET_HC_SECOND_PASSWD_LOGIN`) for newer packetvers.

**YAML**:
```yaml
# pin_code_request — add:
- packet_id: "0x02AD"
  struct_name: SYNTH_HC_SECOND_PASSWD_LOGIN_OLD
  packetver_range: [20070227, 20110308]  # before PACKET_HC_SECOND_PASSWD_LOGIN
```

---

### 4e. `character_server_refused` — `0x02CA` (3 bytes)

**Struct**:
```cpp
// 0x02CA — char server refuses entry (3 bytes)
// Source: clif_packetdb.hpp packet(0x02ca, 3), OpenKore pack='C' fields=[type]
// Comment in clif.cpp: "Notifies the client, that it's connection attempt was refused."
struct SYNTH_HC_REFUSE_ENTER_OLD {
    int16 packetType;   // = 0x02CA
    uint8 errorCode;    // error code (same semantics as 0x006C)
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_HC_REFUSE_ENTER_OLD, 0x02ca);
```

We already have `0x006C` (`PACKET_HC_REFUSE_ENTER`).

**YAML**:
```yaml
# character_server_refused — add:
- packet_id: "0x02CA"
  struct_name: SYNTH_HC_REFUSE_ENTER_OLD
  packetver_range: [20070227, null]
```

---

### 4f. `skills_list` — `0x029D` (variable length)

**Struct**: `PACKET_ZC_HOSKILLINFO_LIST` is actually a **named struct** in rAthena (`packets.hpp`). We already have `0x0235` under `zc_hoskillinfo_list` using this struct. `0x029D` is a different PID for the same packet.

```cpp
// packets.hpp already has:
struct PACKET_ZC_HOSKILLINFO_LIST_sub { ... };
struct PACKET_ZC_HOSKILLINFO_LIST {
    int16 packetType;
    int16 packetLength;
    PACKET_ZC_HOSKILLINFO_LIST_sub skillList[];
};
```

**`0x029D` is Cat B only because it has no `DEFINE_PACKET_HEADER`** — but the struct exists. We can reference it directly.

**YAML** (add to `zc_hoskillinfo_list`):
```yaml
- packet_id: "0x029D"
  struct_name: PACKET_ZC_HOSKILLINFO_LIST
  packetver_range: [20060424, null]
```

Wait — do we need a SYNTH_ here? No. `PACKET_ZC_HOSKILLINFO_LIST` exists with a struct definition. The only missing piece is the `DEFINE_PACKET_HEADER` for `0x029D`. We can add it to `synthetic_structs.hpp`:

```cpp
// 0x029D — additional homunculus skill list PID
// PACKET_ZC_HOSKILLINFO_LIST struct exists in packets.hpp but has no DEFINE_PACKET_HEADER for this PID
DEFINE_PACKET_HEADER(ZC_HOSKILLINFO_LIST_2, 0x029d);  // NOT a new struct, just a PID alias
```

Or more cleanly, add it directly to the struct's `DEFINE_PACKET_HEADER` section in `packets.hpp` — but that would modify rAthena source. Better to add a standalone `DEFINE_PACKET_HEADER` in `synthetic_structs.hpp` referencing the same struct name:

```cpp
// Additional PID for ZC_HOSKILLINFO_LIST (struct defined in map/packets.hpp)
// 0x029D is an earlier PID for the same packet
const int16 HEADER_ZC_HOSKILLINFO_LIST_029D = 0x029d;
```

Actually the cleanest approach: add to `semantic_actions` using `struct_name: PACKET_ZC_HOSKILLINFO_LIST` — the codegen will find it in the regular rAthena headers.

**YAML** (add to `zc_hoskillinfo_list`):
```yaml
- packet_id: "0x029D"
  struct_name: PACKET_ZC_HOSKILLINFO_LIST
  packetver_range: [20060424, null]
```
No SYNTH_ needed — the struct is defined in `packets.hpp`. The codegen will generate the decoder using the existing struct. This is equivalent to Cat A.

---

### 4g. `area_spell` — `0x08C7` (20 bytes)

**Need to read the full clif.cpp case**:
From the OpenKore unpack: `x2 a4 a4 v2 C3` = skip2 + a4 + a4 + v2 + C3 = 4+4+4+2+3 = 17 bytes of data + 2 header = but wait that's 19 not 20.

Actually: `x2` = skip 2 bytes (after header), `a4`=4, `a4`=4, `v`=2, `v`=2, `C`=1, `C`=1, `C`=1 = 2+4+4+2+2+1+1+1 = 17 data + header 2 = 19. But clif_packetdb says 20 bytes.

Fields: `[ID, sourceID, x, y, type, range, isVisible]`

```cpp
// 0x08C7 — area spell / skill ground effect entry (20 bytes)
// Source: clif.cpp case 0x08c7, OpenKore pack='x2 a4 a4 v2 C3'
// The x2 likely means 2 padding bytes after the header
struct SYNTH_ZC_SKILL_ENTRY3 {
    int16  packetType;   // = 0x08C7
    uint8  _pad[2];      // padding (the x2 in OpenKore's pack)
    uint32 id;           // area spell ID
    uint32 creatorId;    // source unit ID  
    uint16 x;            // x position
    uint16 y;            // y position
    uint8  type;         // skill type
    uint8  range;        // effect range
    uint8  isVisible;    // visibility flag
} __attribute__((packed));
// Total: 2+2+4+4+2+2+1+1+1 = 19 bytes -- need to verify to match 20
DEFINE_PACKET_HEADER(SYNTH_ZC_SKILL_ENTRY3, 0x08c7);
```

**YAML** (add to `area_spell`):
```yaml
- packet_id: "0x08C7"
  struct_name: SYNTH_ZC_SKILL_ENTRY3
  packetver_range: [20121212, null]
```

**Pre-implementation check required**: Compare against existing `packet_skill_entry` struct and against clif.cpp case 0x08c7 body to confirm exact field layout and size.

---

### 4h. `mail_receive` — `0x0274` (8 bytes), **new action**

This was incorrectly listed as `ac_accept_login` in EPIC-08. Corrected here.

```cpp
// 0x0274 — Mail received notification (8 bytes)  
// Source: clif_packetdb.hpp packet(0x0274, 8), OpenKore handler=mail_return, pack='V v'
struct SYNTH_ZC_MAIL_RECEIVE {
    int16  packetType;   // = 0x0274
    uint32 mailId;       // mail ID
    uint16 fail;         // 0=success, nonzero=failure code
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_MAIL_RECEIVE, 0x0274);
```

**New action**:
```yaml
mail_receive:
  description: "Mail received notification"
  openkore_name: "mail_return"
  implementations:
    - packet_id: "0x0274"
      struct_name: SYNTH_ZC_MAIL_RECEIVE
      packetver_range: [20060306, null]
```

---

## GROUP 5: Already-present-but-wrong-range fixes

Several existing implementations have `packetver_range: [null, null]` (covers all packetvers) when they should have bounds. These need fixing to avoid the codegen generating "choose first match" ambiguity when multiple PIDs for the same struct exist.

| Action | PID | Fix |
|---|---|---|
| `item_pickup` | `0x00A0` | Add `packetver_max: 20061217` |
| `item_pickup` | `0x0A37` | Add correct `packetver_min`/`max` from packets_struct.hpp |
| `add_exchange_item` | `0x00E9` | Add `packetver_max: 20100222` |
| `received_characters_page` | `0x099D` | Add `packetver_max: 20201006` |
| `zc_equipwin_microscope` | `0x02D7` | Add `packetver_max: 20101122` |
| `zc_shortcut_key_list` | `0x02B9` | Add `packetver_max: 20090616` |
| `zc_guild_info` | `0x0A84` | Add `packetver_max: 20200901` |

---

## Summary Table: All 55 PIDs

| PID | Action | Group | Method | New SYNTH? | New action? |
|---|---|---|---|---|---|
| `0x0073` | `zc_accept_enter` | 1a | YAML | — | — |
| `0x02EB` | `zc_accept_enter` | 1a | YAML | — | — |
| `0x029A` | `item_pickup` | 1b | YAML | — | — |
| `0x02D4` | `item_pickup` | 1b | YAML | — | — |
| `0x0990` | `item_pickup` | 1b | YAML | — | — |
| `0x0A0C` | `item_pickup` | 1b | YAML | — | — |
| `0x0B41` | `item_pickup` | 1b | YAML | — | — |
| `0x0B72` | `received_characters_page` | 1c | YAML | — | — |
| `0x080F` | `add_exchange_item` | 1d | YAML | — | — |
| `0x0A09` | `add_exchange_item` | 1d | YAML | — | — |
| `0x0A96` | `add_exchange_item` | 1d | YAML | — | — |
| `0x07D9` | `zc_shortcut_key_list` | 1e | YAML | — | — |
| `0x0A00` | `zc_shortcut_key_list` | 1e | YAML | — | — |
| `0x0B20` | `zc_shortcut_key_list` | 1e | YAML | — | — |
| `0x01B6` | `zc_guild_info` | 1f | YAML | — | — |
| `0x0B7B` | `zc_guild_info` | 1f | YAML | — | — |
| `0x0859` | `zc_equipwin_microscope` | 1g | YAML | — | — |
| `0x0906` | `zc_equipwin_microscope` | 1g | YAML | — | — |
| `0x0997` | `zc_equipwin_microscope` | 1g | YAML | — | — |
| `0x0A2D` | `zc_equipwin_microscope` | 1g | YAML | — | — |
| `0x0B03` | `zc_equipwin_microscope` | 1g | YAML | — | — |
| `0x0B31` | `skill_add` | 1h | YAML | — | — |
| `0x0B32` | `skills_list` | 1i | YAML | — | — |
| `0x0B33` | `zc_skillinfo_update2` | 1j | YAML | — | — |
| `0x07DB` | `zc_ho_par_change` | 1k | YAML | — | NEW |
| `0x081E` | `zc_el_par_change` | 1k | YAML | — | NEW |
| `0x00AC` | `zc_req_takeoff_equip_ack` | 1l | YAML | — | NEW |
| `0x08D1` | `zc_req_takeoff_equip_ack` | 1l | YAML | — | NEW |
| `0x099A` | `zc_req_takeoff_equip_ack` | 1l | YAML | — | NEW |
| `0x0999` | `zc_req_wear_equip_ack` | 1l | YAML | — | NEW |
| `0x006D` | `char_created` | 1m | YAML | — | NEW |
| `0x0B6F` | `char_created` | 1m | YAML | — | NEW |
| `0x022A` | `actor_exists` | 2 | YAML | — | — |
| `0x022B` | `actor_connected` | 2 | YAML | — | — |
| `0x02EE` | `actor_exists` | 2 | YAML | — | — |
| `0x09DC` | `actor_connected` | 2 | YAML | — | — |
| `0x09DD` | `actor_exists` | 2 | YAML | — | — |
| `0x00A3` | `inventory_items_stackable` | 3 | YAML | — | NEW |
| `0x01EE` | `inventory_items_stackable` | 3 | YAML | — | NEW |
| `0x02E8` | `inventory_items_stackable` | 3 | YAML | — | NEW |
| `0x0991` | `inventory_items_stackable` | 3 | YAML | — | NEW |
| `0x00A4` | `inventory_items_equip` | 3 | YAML | — | NEW |
| `0x0992` | `inventory_items_equip` | 3 | YAML | — | NEW |
| `0x0A0D` | `inventory_items_equip` | 3 | YAML | — | NEW |
| `0x0B0A` | `inventory_items_equip` | 3 | YAML | — | NEW |
| `0x0B39` | `inventory_items_equip` | 3 | YAML | — | NEW |
| `0x0071` | `received_map_server_info` | 4a | YAML+SYNTH | YES | — |
| `0x07F6` | `exp` | 4b | YAML+SYNTH | YES | NEW |
| `0x0ACC` | `exp` | 4b | YAML+SYNTH | YES | NEW |
| `0x02A2` | `stat_update` | 4c | YAML+SYNTH | YES | — |
| `0x02AD` | `pin_code_request` | 4d | YAML+SYNTH | YES | — |
| `0x02CA` | `character_server_refused` | 4e | YAML+SYNTH | YES | — |
| `0x029D` | `zc_hoskillinfo_list` | 4f | YAML only | — | — |
| `0x08C7` | `area_spell` | 4g | YAML+SYNTH | YES | — |
| `0x0274` | `mail_receive` | 4h | YAML+SYNTH | YES | NEW |

**SYNTH_ structs to write: 8**  
**New actions to create: 9** (`char_created`, `exp`, `zc_req_takeoff_equip_ack`, `zc_req_wear_equip_ack`, `zc_ho_par_change`, `zc_el_par_change`, `inventory_items_stackable`, `inventory_items_equip`, `mail_receive`)  
**Range fixes on existing impls: 7**

---

## Open Questions (must resolve before implementing)

1. **`item_pickup` `0x0A37` exact range**: Read `packets_struct.hpp` lines 580–595 to get the precise `#elif` condition for `0x0A37`.

2. **`zc_accept_enter` `0x02EB` reappearance**: Confirm the exact packetver range. rAthena says `PACKETVER < 20141022 || PACKETVER >= 20160330`. This means it was used, then replaced, then brought back. Should be TWO separate impls with a gap.

3. **`zc_req_wear_equip_ack` `0x00AA`**: Check if already in DB under another action. If not, add it as the pre-2010 variant.

4. **`actor_exists` `0x02EC`**: Currently listed as `packet_unit_walking` — this seems wrong for a "exists/idle" packet. Investigate whether `0x02EC` should be in `actor_exists` or `actor_moved`. Check the rAthena enum.

5. **Inventory 2018+ framing**: Confirm that the `ZC_INVENTORY_START`/`END` framing at `>= 20181002` means the item list PIDs (`0x0991`, `0x0992` etc.) are sent inside that frame with the SAME PID values, and that our `zc_inventory_start` action correctly handles this vs the standalone item list.

6. **`0x0B09` overlap**: `zc_inventory_start` currently uses `0x0B09`. But `inventorylistnormalType = 0xb09` at `>= 20181002`. These are different packets sharing the same PID at the same packetver. This is a collision that requires investigation.

---

## Recommended implementation order

1. **Group 1a, 1c, 1m** first (login/char-creation path) — highest value, simplest  
2. **Group 1b** (item_pickup 5 variants) — after resolving `0x0A37` range  
3. **Group 1l** (equip acks — 2 new actions) — important for gameplay  
4. **Group 2** (actor packets — just YAML) — 5 simple adds  
5. **Group 4b** (EXP — new action + 2 SYNTH_) — important, manageable  
6. **Group 4a** (`0x0071` — SYNTH_) — straightforward  
7. **Group 1d–1k remaining** (trade, hotkeys, guild, skills, stats) — medium priority  
8. **Group 4c–4h remaining** (other SYNTH_) — lower priority  
9. **Group 3** (inventory lists — investigate framing first) — most complex, do last
