# EPIC-08: Missing Packet Coverage — 55 Absent Implementations

**Status**: Ready for implementation
**Created**: 2026-03-20
**Priority**: HIGH — connectivity and gameplay correctness across the full packetver range
**Discovered by**: Cross-reference audit (worklogs 0059, 0060)

---

## Problem Statement

A systematic three-layer audit (receive_dispatch.go + pkg/decode/ + semantic_actions)
confirmed that **55 packet IDs are entirely absent** from all generated code — no dispatch
entry, no decode function, no semantic_actions entry. These are not false alarms: every
one was verified against the actual generated files.

The gaps span 27 distinct semantic actions and cover the full packetver range from
2004 through 2022. Several are in the login/map-enter critical path, meaning the
client will silently drop the server's response and never complete the connection
handshake for certain packetver ranges.

The 8 PIDs that appeared as gaps in worklog 0059 but are actually fully present
(`0x006A`, `0x006B`, `0x013E`, `0x01AB`, `0x0235`, `0x0239`, `0x07E1`, `0x083E`)
are **not** part of this epic — they are correctly wired.

---

## Root-Cause Categories

The 55 gaps fall into three categories that require different implementation strategies.

### Category A — 32 PIDs: rAthena has `DEFINE_PACKET_HEADER`, never added to DB

These have fully typed C structs in rAthena. Adding them to `semantic_actions` in
`semantics/mappings.yaml` and running `go generate ./internal/codegen/...` is
sufficient. No C++ work required.

### Category B — 14 PIDs: rAthena has `clif_packetdb.hpp` only, no named struct

rAthena sends these via raw `WFIFOW`/`WFIFOL` macros in `clif.cpp` rather than typed
structs. The struct layout is known from `clif.cpp` source and/or OpenKore unpack
strings. Requires:
1. Add SYNTH_ struct definition to `internal/codegen/stubs/synthetic_structs.hpp`
2. Add implementation to `semantic_actions`
3. Run codegen

### Category D — 9 PIDs: rAthena uses runtime enum PID, no `DEFINE_PACKET_HEADER`

The inventory list packets set their PID via the `inventorylistnormalType` /
`inventorylistequipType` enum in `packets_struct.hpp`. The struct types
`packet_itemlist_normal` and `packet_itemlist_equip` exist and are already used by
the codegen (for `0xB09`). Each missing PID is just another enum value that needs
registering as an implementation pointing at the same existing struct with appropriate
packetver bounds.

---

## Severity Tiers

### CRITICAL — Login and map-enter path (breaks connectivity)

| PID | Action | Category | Packetver condition | rAthena struct |
|---|---|---|---|---|
| `0x006D` | `char_created` | A | `< 20201007` (else) | `HC_ACCEPT_MAKECHAR` |
| `0x0073` | `zc_accept_enter` | A | `< 20080102` | `ZC_ACCEPT_ENTER` |
| `0x02EB` | `zc_accept_enter` | A | `< 20141022 OR >= 20160330` | `ZC_ACCEPT_ENTER` |
| `0x0071` | `received_map_server_info` | B | unconditional pre-2017 | SYNTH needed |
| `0x0B6F` | `char_created` | A | MAIN `>= 20201007` / RE `>= 20211103` | `HC_ACCEPT_MAKECHAR` |
| `0x0B72` | `received_characters_page` | A | MAIN `>= 20201007` / RE `>= 20211103` | `HC_ACK_CHARINFO_PER_PAGE` |

**Note on `0x006D`/`0x0B6F`:** These require creating the `char_created` action from
scratch — it does not yet exist in `semantic_actions`.

**Note on `0x0071`:** This PID was deliberately removed in worklog 0042 because it was
incorrectly mapped to `PACKET_HC_NOTIFY_ZONESVR` (the struct for `0x0081`/`0x0AC5`).
It needs a SYNTH_ struct describing the actual wire layout and a packetver_max cap at
20170315 (when rAthena switched to `0x0081`).

### HIGH — Core gameplay (actors, inventory, skills, stats, EXP)

**Actor packets — 5 PIDs (Category B):**

| PID | Action | Packetver condition | Notes |
|---|---|---|---|
| `0x022A` | `actor_exists` | `>= 20050411 && < 20131223` | SYNTH_ZC_NOTIFY_STANDENTRY5 |
| `0x022B` | `actor_connected` | `>= 20050411 && < 20131223` | SYNTH_ZC_NOTIFY_NEWENTRY5 |
| `0x02EE` | `actor_moved` | `>= 20080102 && < 20131223` | SYNTH_ZC_NOTIFY_MOVEENTRY6 |
| `0x09DC` | `actor_connected` | `>= 20131223` (variable length) | SYNTH_ZC_NOTIFY_NEWENTRY10 |
| `0x09DD` | `actor_exists` | `>= 20131223` (variable length) | SYNTH_ZC_NOTIFY_STANDENTRY10 |

Layout for `0x022A`/`022B`/`02EE` can be derived from `0x022C`/`02ED` (which we
already have and have the same field structure at the same offsets).

**Item pickup — 5 PIDs (Category A, same struct):**

| PID | Action | Packetver condition |
|---|---|---|
| `0x029A` | `item_pickup` | `>= 20061218 && < 20071002` |
| `0x02D4` | `item_pickup` | `>= 20071002 && < 20120925` |
| `0x0990` | `item_pickup` | `>= 20120925 && < 20150226` |
| `0x0A0C` | `item_pickup` | `>= 20150226 && < 20200916` (MAIN) |
| `0x0B41` | `item_pickup` | MAIN `>= 20200916` / RE `>= 20200723` / ZERO `>= 20221024` |

All use `PACKET_ZC_ITEM_PICKUP_ACK` from `map/packets_struct.hpp`. Each is a `#elif`
branch of the same `DEFINE_PACKET_HEADER` — one is active per PACKETVER.

**EXP packets — 2 PIDs (Category B):**

| PID | Action | Packetver condition | Wire layout (from clif_displayexp()) |
|---|---|---|---|
| `0x07F6` | `exp` | `< 20170830` | `uint16 type; uint32 id; uint32 exp; uint16 type; uint16 quest` (14 bytes) |
| `0x0ACC` | `exp` | `>= 20170830` | `uint16 type; uint32 id; uint64 exp; uint16 type; uint16 quest` (18 bytes) |

The `exp` action does not yet exist in `semantic_actions` and must be created.

**Equipment result — 4 PIDs (Category A, two new actions needed):**

`zc_req_takeoff_equip_ack` (unequip result) and `zc_req_wear_equip_ack` (equip result)
actions must be created. Both the send-side CZ actions exist; the ZC ack actions are missing.

| PID | New Action | Packetver condition | rAthena struct |
|---|---|---|---|
| `0x00AC` | `zc_req_takeoff_equip_ack` | `< 20110824` (else) | `ZC_REQ_TAKEOFF_EQUIP_ACK` |
| `0x08D1` | `zc_req_takeoff_equip_ack` | `>= 20110824 && < 20130000` | `ZC_REQ_TAKEOFF_EQUIP_ACK` |
| `0x099A` | `zc_req_takeoff_equip_ack` | `>= 20130000` | `ZC_REQ_TAKEOFF_EQUIP_ACK` |
| `0x0999` | `zc_req_wear_equip_ack` | MAIN `>= 20121205` / RE `>= 20121107` | `ZC_REQ_WEAR_EQUIP_ACK` |

**Stat updates — 3 PIDs (Category A, actions exist):**

| PID | Action | Packetver condition | rAthena struct | Notes |
|---|---|---|---|---|
| `0x07DB` | `zc_ho_par_change` | else (pre-`ZC_HO_PAR_CHANGE` date) | `ZC_HO_PAR_CHANGE` | Homunculus stat |
| `0x081E` | `zc_el_par_change` | unconditional | `ZC_EL_PAR_CHANGE` | Elemental stat |
| `0x02A2` | `stat_update` | `>= 20060424` | SYNTH_ZC_PAR_CHANGE2 | Cat B — 8 bytes |

**Skills — 5 PIDs (mix of A and B):**

| PID | Action | Cat | Packetver | rAthena struct / SYNTH |
|---|---|---|---|---|
| `0x029D` | `skills_list` | B | `>= 20060424` | SYNTH_ZC_SKILLINFO_LIST2 (var len) |
| `0x0B31` | `skill_add` | A | RE `>= 20190807` / ZERO `>= 20190918` | `ZC_ADD_SKILL` |
| `0x0B32` | `skills_list` | A | RE `>= 20190807` / ZERO `>= 20190918` | `ZC_SKILLINFO_LIST` |
| `0x0B33` | `skill_update` | A | RE `>= 20190807` / ZERO `>= 20190918` | `ZC_SKILLINFO_UPDATE2` |

### MEDIUM — Inventory lists, UI, trade, guild, misc

**Inventory lists — 9 PIDs (Category D, two new actions needed):**

`inventory_items_stackable` and `inventory_items_equip` actions must be created. The
structs `packet_itemlist_normal` and `packet_itemlist_equip` already exist in rAthena and
share the same definition across all packetvers (only the PID changes via enum).

*Stackable items (packet_itemlist_normal):*

| PID | Action | Packetver condition |
|---|---|---|
| `0x00A3` | `inventory_items_stackable` | `< 20071002` (else) |
| `0x01EE` | `inventory_items_stackable` | `>= 20071002 && < 20080102` |
| `0x02E8` | `inventory_items_stackable` | `>= 20080102 && < 20120925` |
| `0x0991` | `inventory_items_stackable` | `>= 20120925` (pre-2018 RE/ZERO) |

Note: `0xB09` (`ZC_INVENTORY_START`) covers 2018+; it is a different packet (session
boundary marker), not the item list. The item list for 2018+ is sent via the same
`packet_itemlist_normal` struct but within the `ZC_INVENTORY_START`/`ZC_INVENTORY_END`
framing. Investigate before implementing.

*Non-stackable / equippable items (packet_itemlist_equip):*

| PID | Action | Packetver condition |
|---|---|---|
| `0x00A4` | `inventory_items_equip` | `< 20071002` (else) |
| `0x0992` | `inventory_items_equip` | `>= 20120925 && < 20150226` |
| `0x0A0D` | `inventory_items_equip` | `>= 20150226` (pre-2018) |
| `0x0B0A` | `inventory_items_equip` | MAIN `>= 20181002` / RE `>= 20180912` / ZERO `>= 20180919` |
| `0x0B39` | `inventory_items_equip` | MAIN `>= 20200916` / RE `>= 20200723` / ZERO `>= 20221024` |

**Trade — 3 PIDs (Category A, action exists):**

| PID | Action | Packetver condition | rAthena struct |
|---|---|---|---|
| `0x080F` | `add_exchange_item` | `>= 20100223 && < 20150226` | `ZC_ADD_EXCHANGE_ITEM` |
| `0x0A09` | `add_exchange_item` | `>= 20150226 && < 20161102 MAIN` | `ZC_ADD_EXCHANGE_ITEM` |
| `0x0A96` | `add_exchange_item` | MAIN `>= 20161102` / RE `>= 20161026` / ZERO | `ZC_ADD_EXCHANGE_ITEM` |

**Hotkeys / Shortcuts — 3 PIDs (Category A, action exists):**

| PID | Action | Packetver condition | rAthena struct |
|---|---|---|---|
| `0x07D9` | `zc_shortcut_key_list` | MAIN/RE/SAK `>= 20090617` | `ZC_SHORTCUT_KEY_LIST` |
| `0x0A00` | `zc_shortcut_key_list` | MAIN `>= 20141022` / RE `>= 20141015` / ZERO | `ZC_SHORTCUT_KEY_LIST` |
| `0x0B20` | `zc_shortcut_key_list` | MAIN `>= 20190522` / RE `>= 20190508` / ZERO `>= 20190605` | `ZC_SHORTCUT_KEY_LIST` |

**Guild info — 2 PIDs (Category A, action exists):**

| PID | Action | Packetver condition | rAthena struct |
|---|---|---|---|
| `0x01B6` | `zc_guild_info` | else (pre-2020) | `ZC_GUILD_INFO` |
| `0x0B7B` | `zc_guild_info` | `>= 20200902` | `ZC_GUILD_INFO` |

**Equipment window (view player equip) — 5 PIDs (Category A, action exists):**

| PID | Action | Packetver condition | rAthena struct |
|---|---|---|---|
| `0x0859` | `zc_equipwin_microscope` | `>= 20101123 && < 20111207 MAIN` | `ZC_EQUIPWIN_MICROSCOPE` |
| `0x0906` | `zc_equipwin_microscope` | MAIN `>= 20111207` / RE `>= 20111122` | `ZC_EQUIPWIN_MICROSCOPE` |
| `0x0997` | `zc_equipwin_microscope` | MAIN `>= 20121205` / RE `>= 20121107` | `ZC_EQUIPWIN_MICROSCOPE` |
| `0x0A2D` | `zc_equipwin_microscope` | `>= 20140820` | `ZC_EQUIPWIN_MICROSCOPE` |
| `0x0B03` | `zc_equipwin_microscope` | MAIN/RE `>= 20180801` / ZERO `>= 20180808` | `ZC_EQUIPWIN_MICROSCOPE` |

**Misc — 3 PIDs:**

| PID | Action | Cat | Packetver | rAthena struct / SYNTH |
|---|---|---|---|---|
| `0x02AD` | `pin_code_request` | B | `>= 20070227` | SYNTH_HC_SECOND_PASSWD_LOGIN2 (8 bytes) |
| `0x02CA` | `character_server_refused` | B | `>= 20070227` | SYNTH_HC_REFUSE_ENTER2 (3 bytes) |
| `0x08C7` | `area_spell` | B | `>= 20121212` | SYNTH_ZC_SKILL_ENTRY3 (20 bytes) |
| `0x0274` | `ac_accept_login` | B | `>= 20060306 && < ~20090000` | SYNTH_AC_ACCEPT_LOGIN2 (8 bytes) |

---

## Story Map

The 55 gaps decompose into 7 user stories grouped by implementation effort.

```
US-08-1  Cat A: login/map-enter path (0x006D, 0x0073, 0x02EB, 0x0B6F, 0x0B72)    HIGH / easy
US-08-2  Cat B: login/map-enter path (0x0071) — SYNTH_ struct needed              HIGH / medium
US-08-3  Cat A: item_pickup all 5 variants                                         HIGH / easy
US-08-4  Cat B: EXP packets (0x07F6, 0x0ACC) — new action + 2 SYNTH_              HIGH / medium
US-08-5  Cat A: equipment acks (0x00AC, 0x08D1, 0x099A, 0x0999) — 2 new actions   HIGH / easy
US-08-6  Cat B: actor packets (0x022A, 0x022B, 0x02EE, 0x09DC, 0x09DD)            HIGH / medium
US-08-7  Cat A+B remaining 22 PIDs (stats, skills, trade, hotkeys, guild, etc.)    MED  / easy
US-08-8  Cat D: inventory lists — 9 PIDs, 2 new actions, enum-PID pattern          MED  / complex
```

Dependencies: none between stories except US-08-8 which should be preceded by
investigation of the `ZC_INVENTORY_START`/`ZC_INVENTORY_END` framing (2018+).

---

## US-08-1: Cat A Login/Map-Enter (5 PIDs)

**Effort**: Small — YAML edits only, then codegen.

**New actions required**: `char_created` (create from scratch).

**Implementations to add**:

```yaml
char_created:
  description: "Char server accepts new character creation"
  openkore_name: "character_creation_successful"
  implementations:
    - packet_id: "0x006D"
      struct_name: PACKET_HC_ACCEPT_MAKECHAR
      packetver_range: [null, 20201006]   # else branch → before 20201007
    - packet_id: "0x0B6F"
      struct_name: PACKET_HC_ACCEPT_MAKECHAR
      packetver_range: [20201007, null]   # MAIN >= 20201007 || RE >= 20211103

zc_accept_enter:  # add to existing action
  implementations:
    - packet_id: "0x0073"
      struct_name: PACKET_ZC_ACCEPT_ENTER
      packetver_range: [null, 20080101]   # < 20080102
    - packet_id: "0x02EB"
      struct_name: PACKET_ZC_ACCEPT_ENTER
      packetver_range: [null, 20141021]   # < 20141022 (and again >= 20160330, handled by existing 0x0A18 gap)

received_characters_page:  # add to existing action
  implementations:
    - packet_id: "0x0B72"
      struct_name: PACKET_HC_ACK_CHARINFO_PER_PAGE
      packetver_range: [20201007, null]
```

**Acceptance criteria**:
- `go test ./...` passes
- `receive_dispatch.go` contains entries for `0x006D`, `0x0073`, `0x02EB`, `0x0B6F`, `0x0B72`
- `pkg/decode/char_created.go` is generated
- `pkg/events/char_created.go` is generated

---

## US-08-2: Cat B Map-Enter (0x0071)

**Effort**: Medium — requires SYNTH_ struct design.

**Context**: `0x0071` was deliberately removed in worklog 0042. rAthena sends it via
`clif_packetdb.hpp` as `packet(0x0071, 28)` — 28 bytes, no named struct. It was
the zone-server redirect packet before `0x0081`/`0x0AC5`.

**Wire layout** (28 bytes, from `clif_send_map_login` analysis and OpenKore unpack
`'a4 a4 a4 a4 a4 a4 C'`):
```
offset  0: uint16 packetType    (= 0x0071)
offset  2: uint8  mapName[16]   (map name string)
offset 18: uint32 x             (map X coord) — Note: OpenKore reads IP+port here
offset 22: uint16 port
offset 24: uint32 aid           (account ID)
offset 28: ... (total 28 bytes per recvpackets)
```
**Caution**: The exact layout needs verification against `clif.cpp` `clif_send_map_login`
before writing the SYNTH_ struct.

**Implementations to add**:
```yaml
received_map_server_info:  # add to existing action
  implementations:
    - packet_id: "0x0071"
      struct_name: SYNTH_HC_NOTIFY_ZONESVR2   # new name to avoid confusion with SYNTH_HC_NOTIFY_ZONESVR
      packetver_range: [null, 20170314]        # < 20170315 when 0x0081 took over
```

**Acceptance criteria**:
- `SYNTH_HC_NOTIFY_ZONESVR2` defined in `synthetic_structs.hpp`
- `receive_dispatch.go` entry for `0x0071` under `ActionReceivedMapServerInfo`
- Integration test passes at packetver 20160101

---

## US-08-3: Cat A item_pickup (5 PIDs)

**Effort**: Small — all use `PACKET_ZC_ITEM_PICKUP_ACK`, YAML edits only.

```yaml
item_pickup:  # add to existing action
  implementations:
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
      packetver_range: [20150226, 20200915]
    - packet_id: "0x0B41"
      struct_name: PACKET_ZC_ITEM_PICKUP_ACK
      packetver_range: [20200916, null]
```

**Acceptance criteria**: 6 `item_pickup` entries in `receive_dispatch.go`
(existing `0x00A0` plus these 5). All `item_pickup` decode functions use the same
event type `events.ItemPickup`.

---

## US-08-4: Cat B EXP Packets (2 PIDs + new action)

**Effort**: Medium — new action, 2 SYNTH_ structs.

**Wire layouts** from `clif_displayexp()` in `clif.cpp:18969`:

```c
// Pre-20170830 (0x07F6, 14 bytes):
// WFIFOW(fd,0)  = 0x07F6
// WFIFOL(fd,2)  = sd->id      (uint32 account ID)
// WFIFOL(fd,6)  = exp value   (uint32)
// WFIFOW(fd,10) = type        (uint16: SP_BASEEXP=1 or SP_JOBEXP=2)
// WFIFOW(fd,12) = quest flag  (uint16: 1=quest exp)

// Post-20170830 (0x0ACC, 18 bytes):
// WFIFOW(fd,0)  = 0x0ACC
// WFIFOL(fd,2)  = sd->id      (uint32)
// WFIFOQ(fd,6)  = exp value   (uint64 — client_exp(exp))
// WFIFOW(fd,14) = type        (uint16)
// WFIFOW(fd,16) = quest flag  (uint16)
```

**SYNTH_ structs to add** to `synthetic_structs.hpp`:

```cpp
// EXP gain notification (pre-20170830)
struct SYNTH_ZC_LONG_PAR_CHANGE {
    int16  packetType;
    uint32 aid;
    uint32 exp;
    uint16 type;
    uint16 questFlag;
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_LONG_PAR_CHANGE, 0x07F6);

// EXP gain notification (20170830+, int64 exp)
struct SYNTH_ZC_LONG_PAR_CHANGE2 {
    int16  packetType;
    uint32 aid;
    uint64 exp;
    uint16 type;
    uint16 questFlag;
} __attribute__((packed));
DEFINE_PACKET_HEADER(SYNTH_ZC_LONG_PAR_CHANGE2, 0x0ACC);
```

**New action** `exp`:
```yaml
exp:
  description: "Experience gain notification (base or job EXP)"
  openkore_name: "exp"
  implementations:
    - packet_id: "0x07F6"
      struct_name: SYNTH_ZC_LONG_PAR_CHANGE
      packetver_range: [null, 20170829]
    - packet_id: "0x0ACC"
      struct_name: SYNTH_ZC_LONG_PAR_CHANGE2
      packetver_range: [20170830, null]
```

**Acceptance criteria**: `ActionExp` constant generated; `receive_dispatch.go` has
2 entries; `events.Exp` struct has `Aid`, `Exp`, `Type`, `QuestFlag` fields.

---

## US-08-5: Cat A Equipment Acks (4 PIDs + 2 new actions)

**Effort**: Small — YAML only, no SYNTH_ needed.

**New actions**: `zc_req_takeoff_equip_ack` (unequip result) and `zc_req_wear_equip_ack`
(equip result). Both map to existing rAthena struct definitions.

```yaml
zc_req_takeoff_equip_ack:
  description: "Server acknowledgement for unequip request"
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

zc_req_wear_equip_ack:
  description: "Server acknowledgement for equip request"
  openkore_name: "equip_item"
  implementations:
    - packet_id: "0x0999"
      struct_name: PACKET_ZC_REQ_WEAR_EQUIP_ACK
      packetver_range: [20121107, null]   # RE >= 20121107 / MAIN >= 20121205
```

**Acceptance criteria**: `ActionZcReqTakeoffEquipAck` and `ActionZcReqWearEquipAck`
in `actions.go`; dispatch entries for `0x00AC`, `0x08D1`, `0x099A`, `0x0999`.

---

## US-08-6: Cat B Actor Packets (5 PIDs)

**Effort**: Medium — 5 SYNTH_ structs. Layout derivable from adjacent known structs.

The 2009–2013 actor packets (`0x022A`/`022B`/`02EE`) have the same field structure
as the ones we already have (`0x022C`/`02ED`/`02EC`). Cross-reference the field
layout from `clif.cpp` `clif_set_unit_*` functions and the existing structs to define:

```
SYNTH_ZC_NOTIFY_STANDENTRY5  (0x022A, 58 bytes)
SYNTH_ZC_NOTIFY_NEWENTRY5    (0x022B, 57 bytes)
SYNTH_ZC_NOTIFY_MOVEENTRY6   (0x02EE, 60 bytes)
SYNTH_ZC_NOTIFY_STANDENTRY10 (0x09DD, variable)
SYNTH_ZC_NOTIFY_NEWENTRY10   (0x09DC, variable)
```

For `0x09DC`/`09DD` (variable-length, 2013+): these are the first structs of the
`ZC_NOTIFY_*ENTRY10` series. rAthena's comments in `clif_packetdb.hpp` identify them.
Cross-reference with the `0x09FE`/`09FF` variants (which we do have) to determine
the field delta.

**Acceptance criteria**:
- 5 SYNTH_ structs in `synthetic_structs.hpp`
- `actor_exists` dispatch: `0x0078`, `0x01D8`, `0x02EC`, `0x022A`, `0x09DD`, `0x09FF` (6 total)
- `actor_connected` dispatch: `0x0079`, `0x01D9`, `0x02ED`, `0x022B`, `0x09DC`, `0x09FE` (6 total)
- `actor_moved` dispatch adds `0x02EE`

---

## US-08-7: Cat A+B Remaining (22 PIDs)

**Effort**: Small for Cat A (YAML only), medium for Cat B items.

**Cat A — YAML only (15 PIDs):**

| PIDs | Action | rAthena struct |
|---|---|---|
| `0x07DB` | `zc_ho_par_change` (existing) | `ZC_HO_PAR_CHANGE` |
| `0x081E` | `zc_el_par_change` (existing) | `ZC_EL_PAR_CHANGE` |
| `0x0B31` | `skill_add` (existing) | `ZC_ADD_SKILL` (RE/ZERO `>= 20190807`) |
| `0x0B32` | `skills_list` (existing) | `ZC_SKILLINFO_LIST` (RE/ZERO `>= 20190807`) |
| `0x0B33` | `skill_update` (existing) | `ZC_SKILLINFO_UPDATE2` (RE/ZERO `>= 20190807`) |
| `0x080F`, `0x0A09`, `0x0A96` | `add_exchange_item` (existing) | `ZC_ADD_EXCHANGE_ITEM` |
| `0x07D9`, `0x0A00`, `0x0B20` | `zc_shortcut_key_list` (existing) | `ZC_SHORTCUT_KEY_LIST` |
| `0x01B6`, `0x0B7B` | `zc_guild_info` (existing) | `ZC_GUILD_INFO` |
| `0x0859`, `0x0906`, `0x0997`, `0x0A2D`, `0x0B03` | `zc_equipwin_microscope` (existing) | `ZC_EQUIPWIN_MICROSCOPE` |

**Cat B — SYNTH_ structs needed (4 PIDs):**

| PID | Action | Size | Wire layout source |
|---|---|---|---|
| `0x02A2` | `stat_update` | 8 bytes | `clif_packetdb.hpp` size; layout from `clif.cpp` stat change path |
| `0x029D` | `skills_list` | variable | `clif_packetdb.hpp`; layout from OpenKore `skills_list` unpack |
| `0x02AD` | `pin_code_request` | 8 bytes | OpenKore `login_pin_code_request` unpack |
| `0x02CA` | `character_server_refused` | 3 bytes | Header + 1 byte reason code |
| `0x0274` | `ac_accept_login` | 8 bytes | Auth server alt format from 2006 |
| `0x08C7` | `area_spell` | 20 bytes | `clif.cpp` case 0x08c7 |

---

## US-08-8: Cat D Inventory Lists (9 PIDs + 2 new actions)

**Effort**: Complex — requires understanding the `ZC_INVENTORY_START`/`ZC_INVENTORY_END`
framing introduced in 2018 and how it interacts with `packet_itemlist_normal/equip`.

**Prerequisite investigation**: For PACKETVER `>= 20181002` (MAIN) / `>= 20180912` (RE),
rAthena wraps the inventory list in `ZC_INVENTORY_START` + items + `ZC_INVENTORY_END`.
The item list PIDs for this range (`0xB09` for normal, `0xB0A`/`0xB39` for equip) are
sent inside this frame. Determine whether our current `0xB09` handling (which we have
under `zc_inventory_start`) correctly handles the item data or whether it needs to be
split into a separate action.

**New actions**: `inventory_items_stackable` and `inventory_items_equip`.

*Stackable — packet_itemlist_normal:*

| PID | Packetver condition |
|---|---|
| `0x00A3` | `< 20071002` (else) |
| `0x01EE` | `>= 20071002 && < 20080102` |
| `0x02E8` | `>= 20080102 && < 20120925` |
| `0x0991` | `>= 20120925` (pre-RE/ZERO 2018) |

*Non-stackable — packet_itemlist_equip:*

| PID | Packetver condition |
|---|---|
| `0x00A4` | `< 20071002` (else) |
| `0x0992` | `>= 20120925 && < 20150226` |
| `0x0A0D` | `>= 20150226` (pre-2018) |
| `0x0B0A` | MAIN `>= 20181002` / RE `>= 20180912` / ZERO `>= 20180919` |
| `0x0B39` | MAIN `>= 20200916` / RE `>= 20200723` / ZERO `>= 20221024` |

**Acceptance criteria**: `inventory_items_stackable` and `inventory_items_equip`
actions created; all 9 PIDs in dispatch; events contain item array field.

---

## Complete PID Checklist

| PID | US | Cat | Action | Status |
|---|---|---|---|---|
| `0x006D` | US-08-1 | A | `char_created` (new) | [ ] |
| `0x0073` | US-08-1 | A | `zc_accept_enter` | [ ] |
| `0x02EB` | US-08-1 | A | `zc_accept_enter` | [ ] |
| `0x0B6F` | US-08-1 | A | `char_created` (new) | [ ] |
| `0x0B72` | US-08-1 | A | `received_characters_page` | [ ] |
| `0x0071` | US-08-2 | B | `received_map_server_info` | [ ] |
| `0x029A` | US-08-3 | A | `item_pickup` | [ ] |
| `0x02D4` | US-08-3 | A | `item_pickup` | [ ] |
| `0x0990` | US-08-3 | A | `item_pickup` | [ ] |
| `0x0A0C` | US-08-3 | A | `item_pickup` | [ ] |
| `0x0B41` | US-08-3 | A | `item_pickup` | [ ] |
| `0x07F6` | US-08-4 | B | `exp` (new) | [ ] |
| `0x0ACC` | US-08-4 | B | `exp` (new) | [ ] |
| `0x00AC` | US-08-5 | A | `zc_req_takeoff_equip_ack` (new) | [ ] |
| `0x08D1` | US-08-5 | A | `zc_req_takeoff_equip_ack` (new) | [ ] |
| `0x099A` | US-08-5 | A | `zc_req_takeoff_equip_ack` (new) | [ ] |
| `0x0999` | US-08-5 | A | `zc_req_wear_equip_ack` (new) | [ ] |
| `0x022A` | US-08-6 | B | `actor_exists` | [ ] |
| `0x022B` | US-08-6 | B | `actor_connected` | [ ] |
| `0x02EE` | US-08-6 | B | `actor_moved` | [ ] |
| `0x09DC` | US-08-6 | B | `actor_connected` | [ ] |
| `0x09DD` | US-08-6 | B | `actor_exists` | [ ] |
| `0x07DB` | US-08-7 | A | `zc_ho_par_change` | [ ] |
| `0x081E` | US-08-7 | A | `zc_el_par_change` | [ ] |
| `0x0B31` | US-08-7 | A | `skill_add` | [ ] |
| `0x0B32` | US-08-7 | A | `skills_list` | [ ] |
| `0x0B33` | US-08-7 | A | `skill_update` | [ ] |
| `0x080F` | US-08-7 | A | `add_exchange_item` | [ ] |
| `0x0A09` | US-08-7 | A | `add_exchange_item` | [ ] |
| `0x0A96` | US-08-7 | A | `add_exchange_item` | [ ] |
| `0x07D9` | US-08-7 | A | `zc_shortcut_key_list` | [ ] |
| `0x0A00` | US-08-7 | A | `zc_shortcut_key_list` | [ ] |
| `0x0B20` | US-08-7 | A | `zc_shortcut_key_list` | [ ] |
| `0x01B6` | US-08-7 | A | `zc_guild_info` | [ ] |
| `0x0B7B` | US-08-7 | A | `zc_guild_info` | [ ] |
| `0x0859` | US-08-7 | A | `zc_equipwin_microscope` | [ ] |
| `0x0906` | US-08-7 | A | `zc_equipwin_microscope` | [ ] |
| `0x0997` | US-08-7 | A | `zc_equipwin_microscope` | [ ] |
| `0x0A2D` | US-08-7 | A | `zc_equipwin_microscope` | [ ] |
| `0x0B03` | US-08-7 | A | `zc_equipwin_microscope` | [ ] |
| `0x02A2` | US-08-7 | B | `stat_update` | [ ] |
| `0x029D` | US-08-7 | B | `skills_list` | [ ] |
| `0x02AD` | US-08-7 | B | `pin_code_request` | [ ] |
| `0x02CA` | US-08-7 | B | `character_server_refused` | [ ] |
| `0x0274` | US-08-7 | B | `ac_accept_login` | [ ] |
| `0x08C7` | US-08-7 | B | `area_spell` | [ ] |
| `0x00A3` | US-08-8 | D | `inventory_items_stackable` (new) | [ ] |
| `0x01EE` | US-08-8 | D | `inventory_items_stackable` (new) | [ ] |
| `0x02E8` | US-08-8 | D | `inventory_items_stackable` (new) | [ ] |
| `0x0991` | US-08-8 | D | `inventory_items_stackable` (new) | [ ] |
| `0x00A4` | US-08-8 | D | `inventory_items_equip` (new) | [ ] |
| `0x0992` | US-08-8 | D | `inventory_items_equip` (new) | [ ] |
| `0x0A0D` | US-08-8 | D | `inventory_items_equip` (new) | [ ] |
| `0x0B0A` | US-08-8 | D | `inventory_items_equip` (new) | [ ] |
| `0x0B39` | US-08-8 | D | `inventory_items_equip` (new) | [ ] |

---

## New Actions Required

7 new `semantic_actions` entries must be created (vs adding implementations to existing ones):

| Action | US | Notes |
|---|---|---|
| `char_created` | US-08-1 | HC_ACCEPT_MAKECHAR; 2 PIDs |
| `exp` | US-08-4 | New; 2 SYNTH_ PIDs; EXP gain event |
| `zc_req_takeoff_equip_ack` | US-08-5 | Unequip result; 3 PIDs |
| `zc_req_wear_equip_ack` | US-08-5 | Equip result; 1 PID |
| `inventory_items_stackable` | US-08-8 | Stackable inventory list; 4 PIDs |
| `inventory_items_equip` | US-08-8 | Equip inventory list; 5 PIDs |

---

## New SYNTH_ Structs Required

14 Cat B PIDs require SYNTH_ structs in `internal/codegen/stubs/synthetic_structs.hpp`.
Struct layouts must be verified against `clif.cpp` source before writing:

| SYNTH_ name | PID | Size | Source |
|---|---|---|---|
| `SYNTH_HC_NOTIFY_ZONESVR2` | `0x0071` | 28 | `clif_send_map_login()` + recvpackets |
| `SYNTH_ZC_NOTIFY_STANDENTRY5` | `0x022A` | 58 | `clif_set_unit_idle()` + 0x022C delta |
| `SYNTH_ZC_NOTIFY_NEWENTRY5` | `0x022B` | 57 | `clif_set_unit_walking()` + 0x02ED delta |
| `SYNTH_ZC_NOTIFY_MOVEENTRY6` | `0x02EE` | 60 | `clif_set_unit_walking()` + 0x022C delta |
| `SYNTH_ZC_NOTIFY_STANDENTRY10` | `0x09DD` | var | `clif_packetdb.hpp` comment + 0x09FF delta |
| `SYNTH_ZC_NOTIFY_NEWENTRY10` | `0x09DC` | var | `clif_packetdb.hpp` comment + 0x09FE delta |
| `SYNTH_ZC_LONG_PAR_CHANGE` | `0x07F6` | 14 | `clif_displayexp()` WFIFOW sequence |
| `SYNTH_ZC_LONG_PAR_CHANGE2` | `0x0ACC` | 18 | `clif_displayexp()` WFIFOW sequence |
| `SYNTH_AC_ACCEPT_LOGIN2` | `0x0274` | 8 | OpenKore `account_server_info` unpack |
| `SYNTH_ZC_PAR_CHANGE2` | `0x02A2` | 8 | `clif_packetdb.hpp` size; clif.cpp stat path |
| `SYNTH_HC_SECOND_PASSWD_LOGIN2` | `0x02AD` | 8 | OpenKore `login_pin_code_request` unpack |
| `SYNTH_HC_REFUSE_ENTER2` | `0x02CA` | 3 | Header (2) + reason byte (1) |
| `SYNTH_ZC_SKILLINFO_LIST2` | `0x029D` | var | OpenKore `skills_list` unpack |
| `SYNTH_ZC_SKILL_ENTRY3` | `0x08C7` | 20 | `clif.cpp` case 0x08c7 + OpenKore unpack |

---

## Verification Strategy

After each US:
1. `go build ./...` must pass
2. `go test ./...` must pass
3. `go test -race ./...` must pass
4. For US-08-2 and US-08-6: golden byte test against known wire captures at the
   affected packetver ranges
5. The integration test (`go test -tags integration ./pkg/session/`) must connect
   successfully at target packetver

After all stories complete, re-run the cross-reference audit script from worklog 0060
to confirm all 55 PIDs now appear in `receive_dispatch.go`.
