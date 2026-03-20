# WORKLOG 0060 — rAthena Cross-Reference: Gap Root Cause Analysis

**Date**: 2026-03-20
**Status**: COMPLETE (analysis only — no code changes)
**Follows**: WORKLOG 0059 (OpenKore cross-reference audit)

---

## Summary

Worklog 0059 identified gaps in our `semantic_actions` coverage by comparing against
OpenKore. This worklog answers the follow-up question: **do those gaps exist because
rAthena removed/replaced the packets, or because we simply never added them?**

Every gap PID from worklog 0059 was cross-referenced against the full rAthena source
tree (`/home/mikekao/personal/rathena/src`), searching all `.hpp` and `.cpp` files for
`DEFINE_PACKET_HEADER`, `DEFINE_PACKET_ID`, `clif_packetdb.hpp` entries, and enum
values in `packets_struct.hpp`.

**Top-line finding: almost all gaps are our omission, not rAthena removal.** rAthena has
647 packet IDs defined. Of the 72 gap actions identified in worklog 0059, 41 gap PIDs
ARE present in rAthena and need to be added to our DB. 31 are absent from rAthena for
specific, explainable reasons (old removed entries, OpenKore-only server types,
enum-based type aliases not in `DEFINE_PACKET_HEADER`).

---

## Methodology

### rAthena registry construction

Scraped all `DEFINE_PACKET_HEADER(NAME, 0xNNNN)` and `DEFINE_PACKET_ID(NAME, 0xNNNN)`
from every `.hpp` file, preserving PACKETVER `#if` context for each definition:

```
common/packets.hpp      — login + char server packets (AC_*, HC_*, CA_*, CH_*)
map/packets.hpp         — map server packets (ZC_*, CZ_*) — fixed PIDs
map/packets_struct.hpp  — map server packets — PACKETVER-conditional structs
map/clif_packetdb.hpp   — packet() + parseable_packet() registrations (no struct name)
```

Total: **647 unique PIDs** with struct names. An additional set of packet IDs appear
only as `packet(0xNNNN, len)` in `clif_packetdb.hpp` — these exist in rAthena but have
no named C struct.

### Gap PID list

The 72 gap actions from worklog 0059 produced a deduplicated set of 72 gap PIDs.
Each was looked up in the rAthena registry and classified into one of four categories.

---

## Category A: PIDs in rAthena, Missing From Our DB (Pure Omission)

**41 PIDs.** These exist in rAthena with full `DEFINE_PACKET_HEADER` structs and
correct PACKETVER conditions. They were simply never added to `semantic_actions`. The
codegen pipeline would correctly generate decode/event code for them if added.

### Authentication / Login (common/packets.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action | Notes |
|---|---|---|---|---|
| `0x006A` | `AC_REFUSE_LOGIN` | `< 20120000` (else branch) | `login_error` | Original login refused PID — never added |
| `0x006B` | `HC_ACCEPT_ENTER` | unconditional | `received_characters_info` | Original char-list PID — never added |
| `0x006D` | `HC_ACCEPT_MAKECHAR` | `< 20201007` (else branch) | *(action missing entirely)* | Char creation success — action never created |
| `0x083E` | `AC_REFUSE_LOGIN` | `>= 20120000` | `ac_refuse_login` | Post-2012 login refused — never added |
| `0x0B6F` | `HC_ACCEPT_MAKECHAR` | `>= 20201007 MAIN / >= 20211103 RE` | *(action missing)* | Modern char creation success |
| `0x0B72` | `HC_ACK_CHARINFO_PER_PAGE` | `>= 20201007 MAIN / >= 20211103 RE` | `received_characters_page` | Modern char-page ack — never added |

### Map Enter (map/packets.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action | Notes |
|---|---|---|---|---|
| `0x0073` | `ZC_ACCEPT_ENTER` | `< 20080102` | `zc_accept_enter` | Original map-loaded PID, pre-2008 |
| `0x02EB` | `ZC_ACCEPT_ENTER` | `< 20141022 OR >= 20160330` | `zc_accept_enter` | Mid-era map-loaded PID |

**Note on `0x02EB`:** The condition `< 20141022 || >= 20160330` is non-obvious. It means
rAthena uses `0x02EB` before 2014, then briefly switched to something else from
2014–2016, then switched back. The "something else" is `0x0A18` which we do have.

### Skill Packets (map/packets_struct.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action |
|---|---|---|---|
| `0x013E` | `ZC_USESKILL_ACK` | MAIN `>= 20090406` / SAK `>= 20080618` / RE `>= 20080827` | `skill_cast` / `zc_useskill_ack` |
| `0x0B1A` | `ZC_USESKILL_ACK` | MAIN `>= 20181212` / RE `>= 20181212` / ZERO `>= 20190130` | `skill_cast` / `zc_useskill_ack` |
| `0x0235` | `ZC_HOSKILLINFO_LIST` | unconditional | `zc_hoskillinfo_list` / `skills_list` |
| `0x0239` | `ZC_HOSKILLINFO_UPDATE` | unconditional | `zc_hoskillinfo_update` / `skill_update` |
| `0x07E1` | `ZC_SKILLINFO_UPDATE2` | else branch (pre-RE/ZERO 2019) | `zc_skillinfo_update2` / `skill_update` |
| `0x0B31` | `ZC_ADD_SKILL` | RE `>= 20190807` / ZERO `>= 20190918` | `skill_add` |
| `0x0B32` | `ZC_SKILLINFO_LIST` | RE `>= 20190807` / ZERO `>= 20190918` | `skills_list` |
| `0x0B33` | `ZC_SKILLINFO_UPDATE2` | RE `>= 20190807` / ZERO `>= 20190918` | `skill_update` |

### Stat Packets (map/packets.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action | Notes |
|---|---|---|---|---|
| `0x01AB` | `ZC_PAR_CHANGE_USER` | unconditional | `zc_par_change_user` / `stat_update` | Stat change for OTHER players on screen |
| `0x07DB` | `ZC_HO_PAR_CHANGE` | else branch | `stat_update` | Homunculus stat change |
| `0x081E` | `ZC_EL_PAR_CHANGE` | unconditional | `stat_update` | Elemental stat change |

### Item Pickup (map/packets_struct.hpp)

All five are the same struct `ZC_ITEM_PICKUP_ACK`, with new fields added at each version
bump. The versions are `#elif` branches — exactly one is active per PACKETVER.

| PID | PACKETVER Condition | OpenKore Length |
|---|---|---|
| `0x029A` | `>= 20061218` | 27 |
| `0x02D4` | `>= 20071002` | 29 |
| `0x0990` | `>= 20120925` | 31 |
| `0x0A0C` | `>= 20150226` | 61 |
| `0x0B41` | MAIN `>= 20200916` / RE `>= 20200723` / ZERO `>= 20221024` | 70 |

We currently have only `0x00A0` (pre-20061218) and `0x0A37` (skipped range — needs
investigation). The five above are all in rAthena with proper structs. All omitted.

### Equipment Handling (map/packets.hpp + packets_struct.hpp)

| PID | rAthena Struct | PACKETVER Condition | Action | Notes |
|---|---|---|---|---|
| `0x00AC` | `ZC_REQ_TAKEOFF_EQUIP_ACK` | else branch (pre-20110824) | *(missing)* | Unequip result — classic |
| `0x08D1` | `ZC_REQ_TAKEOFF_EQUIP_ACK` | `>= 20110824` | *(missing)* | Unequip result — 2011+ |
| `0x099A` | `ZC_REQ_TAKEOFF_EQUIP_ACK` | `>= 20130000` | *(missing)* | Unequip result — 2013+ |
| `0x0999` | `ZC_REQ_WEAR_EQUIP_ACK` | MAIN `>= 20121205` / RE `>= 20121107` / ZERO | *(missing)* | Equip result — we have CZ send side, not ZC ack |

The `unequip_item` and equip-result **actions do not exist at all** in our
`semantic_actions`. The send-side `equip_item` / `unequip_item` actions exist (CZ), but
the corresponding server acknowledgement packets (ZC) were never added.

### Trade / Exchange (map/packets_struct.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action |
|---|---|---|---|
| `0x080F` | `ZC_ADD_EXCHANGE_ITEM` | `>= 20100223` | `add_exchange_item` |
| `0x0A09` | `ZC_ADD_EXCHANGE_ITEM` | `>= 20150226` | `add_exchange_item` |
| `0x0A96` | `ZC_ADD_EXCHANGE_ITEM` | MAIN `>= 20161102` / RE `>= 20161026` / ZERO | `add_exchange_item` |

We have `0x00E9` (classic) but not these three later variants.

### UI / Misc (map/packets_struct.hpp)

| PID | rAthena Struct | PACKETVER Condition | Our Action |
|---|---|---|---|
| `0x07D9` | `ZC_SHORTCUT_KEY_LIST` | `>= 20090603` | `zc_shortcut_key_list` |
| `0x0A00` | `ZC_SHORTCUT_KEY_LIST` | MAIN `>= 20141022` / RE `>= 20141015` / ZERO | `zc_shortcut_key_list` |
| `0x0B20` | `ZC_SHORTCUT_KEY_LIST` | MAIN `>= 20190522` / RE `>= 20190508` / ZERO `>= 20190605` | `zc_shortcut_key_list` |
| `0x01B6` | `ZC_GUILD_INFO` | else branch | `zc_guild_info` |
| `0x0B7B` | `ZC_GUILD_INFO` | `>= 20200902` | `zc_guild_info` |
| `0x0859` | `ZC_EQUIPWIN_MICROSCOPE` | `>= 20101123` | `zc_equipwin_microscope` |
| `0x0906` | `ZC_EQUIPWIN_MICROSCOPE` | MAIN `>= 20111207` / RE `>= 20111122` | `zc_equipwin_microscope` |
| `0x0997` | `ZC_EQUIPWIN_MICROSCOPE` | MAIN `>= 20121205` / RE `>= 20121107` | `zc_equipwin_microscope` |
| `0x0A2D` | `ZC_EQUIPWIN_MICROSCOPE` | `>= 20140820` | `zc_equipwin_microscope` |
| `0x0B03` | `ZC_EQUIPWIN_MICROSCOPE` | MAIN `>= 20180801` / RE `>= 20180801` / ZERO `>= 20180808` | `zc_equipwin_microscope` |

---

## Category B: PIDs in clif_packetdb.hpp Only (No Struct Name — Handled Inline)

**11 PIDs.** These appear as `packet(0xNNNN, len)` or `parseable_packet(...)` in
`clif_packetdb.hpp` but have **no `DEFINE_PACKET_HEADER` struct**. rAthena sends or
receives them using raw `WFIFOW`/`RFIFOW` calls in `clif.cpp` rather than via typed
C structs. They exist in rAthena but cannot be processed by our codegen pipeline
(which requires a struct definition).

| PID | Found in | Length | OpenKore handler | Notes |
|---|---|---|---|---|
| `0x0071` | `clif_packetdb.hpp` | 28 | `received_character_ID_and_Map` | Zone server redirect (pre-2017). rAthena sends this via `WFIFOW` without a struct. Was deliberately removed from our DB in worklog 0042 due to wrong struct. Needs a SYNTH_ struct. |
| `0x0072` | `clif_packetdb.hpp` | 22 | `received_characters` | Character list size/page count. A `parseable_packet` (C→S), not S→C. OpenKore misidentifies direction here — it is actually a C→S packet that tells the server how many chars to send. |
| `0x022A` | `clif_packetdb.hpp` | 58 | `actor_exists` | Actor idle (2009–2013). No struct name in source — handled as `packet(0x022a, 58)` only. Struct data is known from clif.cpp usage. Needs SYNTH_ struct. |
| `0x022B` | `clif_packetdb.hpp` | 57 | `actor_connected` | Actor walk-in (2009–2013). Same situation. |
| `0x02EE` | `clif_packetdb.hpp` | 60 | `actor_moved` | Actor moving (2009–2013). Same situation. |
| `0x0274` | `clif_packetdb.hpp` | 8 | `account_server_info` | Alt login-success format. 8 bytes — a login-server response format from 2009. |
| `0x029D` | `clif_packetdb.hpp` | -1 | `skills_list` | Skill info list variant. No struct; variable length. |
| `0x02A2` | `clif_packetdb.hpp` | 8 | `stat_info` | Stat update variant (8 bytes). No struct. Used in some older packetver ranges. |
| `0x02AD` | `clif_packetdb.hpp` | 8 | `login_pin_code_request` | PIN code request. No struct. |
| `0x02CA` | `clif_packetdb.hpp` | 3 | `login_error_game_login_server` | Char server refused. No struct. |
| `0x07F6` | `clif_packetdb.hpp` | 14 | `exp` | EXP gain (pre-2017). Handled in `clif_displayexp()` via `WFIFOW` without a struct. |
| `0x08C7` | `clif_packetdb.hpp` | 20 | `area_spell` | Area spell variant. Documented in `clif.cpp` comments as `case 0x08c7`. No named struct. |
| `0x0ACC` | `clif_packetdb.hpp` | 18 | `exp` | EXP gain (2017+). Also handled in `clif_displayexp()` via `WFIFOW`. |
| `0x09DC` | `clif_packetdb.hpp` | -1 | `actor_connected` | `// ZC_NOTIFY_NEWENTRY10` — variable length, no struct in headers. |
| `0x09DD` | `clif_packetdb.hpp` | -1 | `actor_exists` | `// ZC_NOTIFY_STANDENTRY10` — variable length, no struct in headers. |

**Why rAthena uses inline WFIFOW instead of structs for some packets:**

The `clif_displayexp()` function (found at `clif.cpp:18969`) is representative:

```cpp
void clif_displayexp(map_session_data *sd, t_exp exp, char type, bool quest, bool lost) {
#if PACKETVER >= 20170830
    int32 cmd = 0xacc;
#else
    int32 cmd = 0x7f6;
#endif
    WFIFOHEAD(fd, packet_len(cmd));
    WFIFOW(fd, 0) = cmd;
    WFIFOL(fd, 2) = sd->id;
#if PACKETVER >= 20170830
    WFIFOQ(fd, 6) = client_exp(exp) * (lost ? -1 : 1);  // int64
#else
    WFIFOL(fd, 6) = client_exp(exp) * (lost ? -1 : 1);  // int32
#endif
    WFIFOW(fd, 10+offset) = type;
    ...
}
```

rAthena writes these fields directly using `WFIFOW`/`WFIFOL`/`WFIFOQ` macros rather
than populating a typed struct. These are older code paths that predate the struct-based
approach. For our codegen to handle them, we need to add SYNTH_ structs in
`internal/codegen/stubs/synthetic_structs.hpp`.

### The EXP Packet Structure (from clif.cpp)

Derived from `clif_displayexp`:

```
Pre-20170830 (0x07F6, length 14):
  offset 0: uint16 packetType  = 0x07F6
  offset 2: uint32 id          (actor ID)
  offset 6: uint32 exp         (experience value, int32)
  offset 10: uint16 type       (SP_BASEEXP=1 or SP_JOBEXP=2)
  offset 12: uint16 quest      (1 if quest exp, 0 otherwise)

Post-20170830 (0x0ACC, length 18):
  offset 0: uint16 packetType  = 0x0ACC
  offset 2: uint32 id
  offset 6: uint64 exp         (int64)
  offset 10: uint32 (padding/extension)
  offset 14: uint16 type
  offset 16: uint16 quest
```

### The Actor Idle/Walk/Move Packets (0x022A/022B/02EE)

These are the 2009–2013 era actor notification packets. They appear only as length
declarations in `clif_packetdb.hpp`:

```
packet(0x022a, 58);   // ZC_NOTIFY_STANDENTRY5 (actor idle)
packet(0x022b, 57);   // ZC_NOTIFY_NEWENTRY5   (actor walk-in)
packet(0x022c, 64);   // ZC_NOTIFY_MOVEENTRY5  (actor moving) — we have this one
packet(0x02ee, 60);   // ZC_NOTIFY_MOVEENTRY6  (actor moving, later)
```

The struct layout for these can be inferred from OpenKore's unpack strings and
from the newer typed struct variants (`0x022C` which we do have has the same field
layout at similar offsets).

---

## Category C: PIDs Absent from rAthena Entirely (OpenKore-Only Server Types)

**9 PIDs.** These appear in OpenKore's `kRO/Sakexe_0.pm` but have no trace in any
rAthena source file. They correspond to login/auth server protocol variants that rAthena
has consolidated or dropped in favour of newer formats.

| PID | OpenKore handler | OpenKore length | Explanation |
|---|---|---|---|
| `0x0276` | `account_server_info` | 0 (some), variable | Appears to be a shuffle alias or an auth variant from a discontinued server type. Not found anywhere in rAthena. |
| `0x0AC9` | `account_server_info` | variable | Token-based auth format. OpenKore supports this from 2017, but rAthena's current token auth uses `0x0AC4` (which we have). `0x0AC9` may be from a different server implementation. |
| `0x0B60` | `account_server_info` | variable | Latest OpenKore login variant (2020+). Not in rAthena's `DEFINE_PACKET_HEADER`. May correspond to a server build OpenKore supports but rAthena does not expose via its struct system. |
| `0x0ACD` | `login_error` | 23 | Login error variant (2017+). Not in rAthena. Likely from a different server emulator or a Warp Portal-specific format that OpenKore supports. |
| `0x0AE0` | `login_error` | 30 | Login error variant (2017+). Same situation. |
| `0x0B02` | `login_error` | 26 | Login error variant (2020+). Not in rAthena. |
| `0x0AE9` | `login_pin_code_request` | 13 | PIN code variant (2017+). OpenKore uses this; not in rAthena's headers. |
| `0x08D0` | `equip_item` | 9 | Equip result variant. Not in rAthena's `DEFINE_PACKET_HEADER`. rAthena uses `0x08D0` is actually `ZC_PCBANG_EFFECT` (a different packet). There may be a naming collision in OpenKore's mapping. |

**On `0x08D0`:** rAthena does NOT define `0x08D0` as an equip-result packet. OpenKore
assigns `equip_item` to `0x08D0`. Cross-checking: rAthena's equip result is
`ZC_REQ_WEAR_EQUIP_ACK` at `0x0999` (post-2012). The `0x08D0` → `equip_item` mapping
in OpenKore likely refers to an older Aegis/eAthena server format that rAthena does
not implement.

---

## Category D: Inventory List Packets (Enum-Based, Not DEFINE_PACKET_HEADER)

**5 PIDs.** The inventory list packets are the most interesting case. They do NOT use
`DEFINE_PACKET_HEADER` — instead, rAthena defines their IDs via a C++ enum in
`packets_struct.hpp` and sends them using the untyped `packet_itemlist_normal` /
`packet_itemlist_equip` structs with the PID set at runtime.

From `src/map/packets_struct.hpp`:

```cpp
// inventorylistnormalType — stackable items
#if PACKETVER_RE_NUM >= 20180912 || ...
    inventorylistnormalType = 0xb09,   // we have this
#elif PACKETVER >= 20120925
    inventorylistnormalType = 0x991,   // missing
#elif PACKETVER >= 20080102
    inventorylistnormalType = 0x2e8,   // missing
#elif PACKETVER >= 20071002
    inventorylistnormalType = 0x1ee,   // missing
#else
    inventorylistnormalType = 0xa3,    // missing

// inventorylistequipType — equippable items (non-stackable)
#if PACKETVER_MAIN_NUM >= 20200916 || ...
    inventorylistequipType = 0xb39,    // missing (item_list_nonstackable?)
#elif PACKETVER_MAIN_NUM >= 20181002 || ...
    inventorylistequipType = 0xb0a,    // missing
#elif PACKETVER >= 20150226
    inventorylistequipType = 0xa0d,    // missing
#elif PACKETVER >= 20120925
    inventorylistequipType = 0x992,    // missing
#elif PACKETVER >= 20080102
    inventorylistequipType = 0x2d0,    // missing
#elif PACKETVER >= 20071002
    inventorylistequipType = 0x295,    // missing
#else
    inventorylistequipType = 0xa4,     // missing
```

These are **not** absent from rAthena — they are the inventory list packets and rAthena
absolutely sends them. They are absent from our DB because:

1. They use the `packet_itemlist_normal` / `packet_itemlist_equip` struct types
   (old-style `struct` not `PACKET_` prefixed), which our codegen may not pick up
2. The PID is set dynamically at runtime via the enum, not via `DEFINE_PACKET_HEADER`
3. Our codegen scans for `DEFINE_PACKET_HEADER` — so it found `0xB09` (which we have
   as `ZC_INVENTORY_START`) but missed the older `inventorylistnormalType` variants

The only reason we have `0xB09` is that `ZC_INVENTORY_START` happens to share that PID
and uses `DEFINE_PACKET_HEADER`. The older inventory list PIDs have no such definition.

**Inventory list PIDs that exist in rAthena (enum-based) but are missing from our DB:**

| PID | Enum value | PACKETVER Condition | Description |
|---|---|---|---|
| `0x00A3` | `inventorylistnormalType` | `< 20071002` (else) | Stackable items — classic |
| `0x01EE` | `inventorylistnormalType` | `>= 20071002 && < 20080102` | Stackable items — 2007 |
| `0x02E8` | `inventorylistnormalType` | `>= 20080102 && < 20120925` | Stackable items — 2008–2012 |
| `0x0991` | `inventorylistnormalType` | `>= 20120925` (pre-2018) | Stackable items — 2012–2018 |
| `0xA4`  | `inventorylistequipType` | `< 20071002` (else) | Non-stackable items — classic |
| `0x0295` | `inventorylistequipType` | `>= 20071002 && < 20080102` | Non-stackable — 2007 |
| `0x02D0` | `inventorylistequipType` | `>= 20080102 && < 20120925` | Non-stackable — 2008 |
| `0x0992` | `inventorylistequipType` | `>= 20120925 && < 20150226` | Non-stackable — 2012 |
| `0x0A0D` | `inventorylistequipType` | `>= 20150226` (pre-2018) | Non-stackable — 2015 |
| `0x0B0A` | `inventorylistequipType` | MAIN `>= 20181002` / RE `>= 20180912` | Non-stackable — 2018 (item_list_nonstackable) |
| `0x0B39` | `inventorylistequipType` | MAIN `>= 20200916` / RE `>= 20200723` | Non-stackable — 2020 |

Note: `0xB09` (stackable, 2018+) is already in our DB as part of
`ZC_INVENTORY_START`/`zc_inventory_start`. However we may be decoding the wrong struct
for it — `ZC_INVENTORY_START` is the session-boundary marker, not the item list itself.

---

## Consolidated Root-Cause Table

| PID | In rAthena? | Root Cause of Gap |
|---|---|---|
| `0x006A` | YES (`DEFINE_PACKET_HEADER`) | Never added to our DB |
| `0x006B` | YES | Never added |
| `0x006D` | YES | Action `char_created` never created |
| `0x006B` | YES | Never added |
| `0x083E` | YES | Never added |
| `0x0B6F` | YES | Action never created |
| `0x0B72` | YES | Never added |
| `0x0073` | YES | Never added (old packetver) |
| `0x02EB` | YES | Never added |
| `0x013E` | YES | Never added (older `ZC_USESKILL_ACK`) |
| `0x0B1A` | YES | Never added (newer `ZC_USESKILL_ACK`) |
| `0x0235` | YES | Never added (homunc skills list) |
| `0x0239` | YES | Never added (homunc skill update) |
| `0x07E1` | YES | Never added |
| `0x0B31`/`0x0B32`/`0x0B33` | YES | RE/ZERO variants never added |
| `0x01AB` | YES | ZC_PAR_CHANGE_USER never added |
| `0x07DB` | YES | Homunc stat change never added |
| `0x081E` | YES | Elemental stat change never added |
| `0x029A`/`0x02D4`/`0x0990`/`0x0A0C`/`0x0B41` | YES | All `ZC_ITEM_PICKUP_ACK` variants except first and last never added |
| `0x00AC`/`0x08D1`/`0x099A` | YES | `ZC_REQ_TAKEOFF_EQUIP_ACK` — action never created |
| `0x0999` | YES | `ZC_REQ_WEAR_EQUIP_ACK` — equip ack action never created |
| `0x080F`/`0x0A09`/`0x0A96` | YES | Later `ZC_ADD_EXCHANGE_ITEM` variants never added |
| `0x07D9`/`0x0A00`/`0x0B20` | YES | Older/newer `ZC_SHORTCUT_KEY_LIST` variants never added |
| `0x01B6`/`0x0B7B` | YES | Earlier/later `ZC_GUILD_INFO` never added |
| `0x0859`/`0x0906`/`0x0997`/`0x0A2D`/`0x0B03` | YES | All mid-era `ZC_EQUIPWIN_MICROSCOPE` never added |
| `0x0071` | clif_packetdb only | No struct; needs SYNTH_ |
| `0x0072` | clif_packetdb only | Actually C→S (`parseable_packet`) — wrong direction in OpenKore |
| `0x022A`/`0x022B`/`0x02EE` | clif_packetdb only | Older actor packets — no struct; need SYNTH_ |
| `0x09DC`/`0x09DD` | clif_packetdb only | `ZC_NOTIFY_NEWENTRY10`/`STANDENTRY10` — no struct |
| `0x07F6`/`0x0ACC` | clif_packetdb only | EXP packets — inline `WFIFOW` in `clif_displayexp()` |
| `0x02A2`/`0x02AD`/`0x02CA`/`0x029D`/`0x08C7` | clif_packetdb only | Various inline packets — no struct |
| `0x00A3`/`0x01EE`/`0x02E8`/`0x0991` | enum-based | `inventorylistnormalType` enum — no `DEFINE_PACKET_HEADER` |
| `0x0274` | clif_packetdb only | Auth server alt-format — no struct |
| `0x0276`/`0x0AC9`/`0x0B60` | NOT in rAthena | OpenKore-only server type support |
| `0x0ACD`/`0x0AE0`/`0x0B02` | NOT in rAthena | Login error formats not in rAthena |
| `0x0AE9` | NOT in rAthena | PIN code variant not in rAthena |
| `0x08D0` | NOT in rAthena | OpenKore misidentifies — rAthena `0x08D0` is a different packet |

---

## Implications for Implementation

### What can be added immediately via codegen (Category A — 41 PIDs)

All Category A packets have proper `DEFINE_PACKET_HEADER` structs. Adding them to
`semantic_actions` in `mappings.yaml` and re-running codegen will produce correct
decode functions, event structs, and dispatch entries with zero manual work.

Priority order:

1. **Map/login phase completeness** — `0x006A`, `0x006B`, `0x0073`, `0x02EB`, `0x083E`
2. **Skill system** — `0x013E`, `0x0B1A`, `0x0235`, `0x0239`, `0x07E1`, `0x0B31–33`
3. **Item pickup** — `0x029A`, `0x02D4`, `0x0990`, `0x0A0C`, `0x0B41`
4. **Equipment acks** — `0x00AC`, `0x08D1`, `0x099A`, `0x0999` (new actions needed)
5. **Stat updates** — `0x01AB`, `0x07DB`, `0x081E`
6. **Trade** — `0x080F`, `0x0A09`, `0x0A96`
7. **UI completeness** — shortcut key list, guild info, equip window variants

### What requires SYNTH_ structs (Category B — 15 PIDs)

These require new entries in
`internal/codegen/stubs/synthetic_structs.hpp` defining the struct layout from
clif.cpp source analysis or OpenKore unpack strings, then adding to `mappings.yaml`.

Most important:
- EXP packets (`0x07F6`, `0x0ACC`) — struct layout is fully known from `clif_displayexp()`
- Actor packets (`0x022A`, `0x022B`, `0x02EE`) — layout inferable from 0x022C (which we have)

### What requires enum-based handling (Category D — inventory lists)

The `packet_itemlist_normal` / `packet_itemlist_equip` structs exist and are used by
codegen already (the `0xB09` path). Adding the older PIDs requires understanding that
`inventorylistnormalType` is a runtime-set PID that maps to different values per
PACKETVER — the same struct `packet_itemlist_normal` is used throughout. The codegen
needs to register multiple PIDs for the same struct with appropriate PACKETVER ranges.

### What to skip (Category C — 9 PIDs)

The 9 PIDs absent from rAthena entirely are for OpenKore's support of non-rAthena
server implementations. Since we target rAthena specifically, these should remain
absent. The one exception worth investigating is `0x08D0` — if OpenKore's `equip_item`
handler at `0x08D0` is widely used, we need to confirm whether any rAthena-derived
server actually sends it.

---

## Files Consulted

```
/home/mikekao/personal/rathena/src/common/packets.hpp          — AC/HC/CA/CH structs
/home/mikekao/personal/rathena/src/map/packets.hpp             — ZC/CZ fixed structs
/home/mikekao/personal/rathena/src/map/packets_struct.hpp      — ZC/CZ conditional structs
/home/mikekao/personal/rathena/src/map/clif_packetdb.hpp       — packet/parseable_packet IDs
/home/mikekao/personal/rathena/src/map/clif.cpp                — inline WFIFOW senders
/home/mikekao/personal/rathena/src/config/packets.hpp          — PACKETVER defaults
```

No code was changed in this session.
