# Phantom Struct Names in SemanticDB

Investigation of 48 SemanticDB `rathena_struct` names that do not match any struct
definition in rAthena source (as confirmed by `grep -rn "struct <name>" src/**/*.hpp`).

Every claim in this file is backed by GCC preprocessing output or explicit grep results.
GCC command used: `g++ -E -P -DPACKETVER=20181002 -I./src ... src/map/packets_struct.hpp src/map/packets.hpp`

---

## Category A: Wrong Name — Real rAthena Struct Exists

SemanticDB uses the wrong struct name. These need to be corrected to point to the real
rAthena struct.

| Phantom Name | Packet ID(s) | Real Struct Name | File | Notes |
|---|---|---|---|---|
| `PACKET_ZC_NOTIFY_STANDENTRY` | 0x0078 | `packet_idle_unit` | `packets_struct.hpp:832` | idle_unitType=0x78 for PACKETVER<4 |
| `PACKET_ZC_NOTIFY_STANDENTRY2` | 0x01D8 | `packet_idle_unit` | `packets_struct.hpp:832` | idle_unitType=0x1d8 for PACKETVER<7 |
| `PACKET_ZC_NOTIFY_STANDENTRY11` | 0x09FF | `packet_idle_unit` | `packets_struct.hpp:832` | idle_unitType=0x9ff for PACKETVER>=20150513 |
| `PACKET_ZC_NOTIFY_NEWENTRY` | 0x0079 | `packet_spawn_unit` | `packets_struct.hpp:687` | spawn_unitType=0x79 for PACKETVER<4 |
| `PACKET_ZC_NOTIFY_NEWENTRY2` | 0x01D9 | `packet_spawn_unit` | `packets_struct.hpp:687` | spawn_unitType=0x1d9 for PACKETVER<7 |
| `PACKET_ZC_NOTIFY_NEWENTRY3` | 0x02ED | `packet_spawn_unit` | `packets_struct.hpp:687` | spawn_unitType=0x2ed for PACKETVER<20091103 |
| `PACKET_ZC_NOTIFY_NEWENTRY11` | 0x09FE | `packet_spawn_unit` | `packets_struct.hpp:687` | spawn_unitType=0x9fe for PACKETVER>=20150513 |
| `PACKET_ZC_NOTIFY_MOVEENTRY` | 0x007B, 0x09FD | `packet_unit_walking` | `packets_struct.hpp:758` | unit_walkingType=0x7b (PACKETVER<4), 0x9fd (>=20150513) |
| `PACKET_ZC_NOTIFY_MOVEENTRY2` | 0x01DA | `packet_unit_walking` | `packets_struct.hpp:758` | unit_walkingType=0x1da for PACKETVER<7 |
| `PACKET_ZC_NOTIFY_MOVEENTRY3` | 0x022C | `packet_unit_walking` | `packets_struct.hpp:758` | unit_walkingType=0x22c for PACKETVER<20080102 |
| `PACKET_ZC_NOTIFY_MOVEENTRY4` | 0x02EC | `packet_unit_walking` | `packets_struct.hpp:758` | unit_walkingType=0x2ec for PACKETVER<20091103 |
| `PACKET_ZC_NOTIFY_MOVEENTRY10` | 0x09DB | `packet_unit_walking` | `packets_struct.hpp:758` | unit_walkingType=0x9db for PACKETVER<20150513 (>=20131223) |
| `PACKET_DROPFLOORITEM` | 0x009E | `packet_dropflooritem` | `packets_struct.hpp:597` | dropflooritemType=0x9e for PACKETVER<=20130000 |
| `PACKET_ZC_ITEM_FALL_ENTRY3` | 0x0ADD | `packet_dropflooritem` | `packets_struct.hpp:597` | dropflooritemType=0xadd for PACKETVER_ZERO or >=20180418 |
| `PACKET_ZC_NORMAL_ITEMLIST` | 0x00A3 | `packet_itemlist_normal` | `packets_struct.hpp:1187` | inventorylistnormalType=0xa3 for PACKETVER<20071002 |
| `PACKET_ZC_EQUIPMENT_ITEMLIST` | 0x00A4 | `packet_itemlist_equip` | `packets_struct.hpp:1196` | inventorylistequipType=0xa4 for PACKETVER<20071002 |
| `PACKET_ZC_INVENTORY_ITEMLIST_NORMAL` | 0x0B09 | `packet_itemlist_normal` | `packets_struct.hpp:1187` | inventorylistnormalType=0xb09 for PACKETVER_RE_NUM>=20180912 |
| `PACKET_ZC_INVENTORY_ITEMLIST_EQUIP` | 0x0B0A | `packet_itemlist_equip` | `packets_struct.hpp:1196` | inventorylistequipType=0xb0a for PACKETVER_MAIN_NUM>=20181002 |
| `PACKET_ZC_MSG_STATE_CHANGE` | 0x0196 | `packet_sc_notick` | `packets_struct.hpp:530` | status_change_endType=0x196 (constant) |
| `PACKET_ZC_NOTIFY_ACT2` | 0x02E1 | `packet_damage` | `packets_struct.hpp:1469` | damageType=0x2e1 for PACKETVER>=20071113 and <20131223 |
| `PACKET_ZC_NOTIFY_ACT_DAMAGE` | 0x08C8 | `packet_damage` | `packets_struct.hpp:1469` | damageType=0x8c8 for PACKETVER>=20131223 |
| `PACKET_ZC_SKILL_ENTRY` | 0x011F | `packet_skill_entry` | `packets_struct.hpp:1434` | skill_entryType=0x11f for PACKETVER<20110718 |
| `PACKET_ZC_STATUS_CHANGE2` | 0x043F | `packet_status_change2` | `packets_struct.hpp:927` | status_change2Type=0x43f (constant) |
| `PACKET_ZC_HP_INFO` | 0x0977 | `packet_monster_hp` | `packets_struct.hpp:524` | Confirmed in clif.cpp:19947 comment |
| `PACKET_ZC_USE_ITEM_ACK2` | 0x01C8 | `PACKET_ZC_USE_ITEM_ACK` | `packets_struct.hpp:2577` | useItemAckType=0x1c8 for PACKETVER<=3 |

**Total Category A: 25 packets** (some share real struct names due to versioning)

---

## Category B: Genuinely Structless — No rAthena Struct

These packets have no struct definition anywhere in rAthena source. They use raw RFIFOW/WFIFOW
macro access via `packet_db[cmd].pos[]` offsets. Synthetic structs must be written for these.

### CZ Packets (Client → Server)

| Phantom Name | Packet ID(s) | Length | Handler | Layout | Source Evidence |
|---|---|---|---|---|---|
| `PACKET_CZ_CLOSE_STORE` | 0x00F7 | 2 | `clif_parse_CloseKafra` | `[int16 PacketType]` | `clif_packetdb.hpp:97 parseable_packet(0x00f7,2,...)` |
| `PACKET_CZ_CONCLUDE_EXCHANGE_ITEM` | 0x00EB | 2 | `clif_parse_TradeOk` | `[int16 PacketType]` | `clif_packetdb.hpp:92 parseable_packet(0x00eb,2,...)` |
| `PACKET_CZ_ITEM_PICKUP` | 0x009F | 6 | `clif_parse_TakeItem` | `[int16 PacketType][uint32 ObjectID]` | `clif_packetdb.hpp:50 parseable_packet(0x009f,6,...,pos[0]=2)` |
| `PACKET_CZ_ITEM_PICKUP2` | 0x0362 | 6 | `clif_parse_TakeItem` | `[int16 PacketType][uint32 ObjectID]` | `clif_packetdb.hpp:1384 parseable_packet(0x0362,6,...,pos[0]=2)` |
| `PACKET_CZ_ITEM_THROW2` | 0x0363 | 6 | `clif_parse_DropItem` | `[int16 PacketType][uint16 Index][uint16 Amount]` | `clif_packetdb.hpp:1385 parseable_packet(0x0363,6,...,pos[0]=2,pos[1]=4)` |
| `PACKET_CZ_MOVE_ITEM_FROM_BODY_TO_STORE` | 0x00F3 | 8 | `clif_parse_MoveToKafra` | `[int16 PacketType][uint16 Index][uint32 Amount]` | `clif_packetdb.hpp:95 parseable_packet(0x00f3,8,...,pos[0]=2,pos[1]=4)` |
| `PACKET_CZ_MOVE_ITEM_FROM_STORE_TO_BODY` | 0x00F5 | 8 | `clif_parse_MoveFromKafra` | `[int16 PacketType][uint16 Index][uint32 Amount]` | `clif_packetdb.hpp:96 parseable_packet(0x00f5,8,...,pos[0]=2,pos[1]=4)` |
| `PACKET_CZ_NOTIFY_ACTORINIT` | 0x007D | 2 | `clif_parse_LoadEndAck` | `[int16 PacketType]` | `clif_packetdb.hpp:32 parseable_packet(0x007d,2,...)` |
| `PACKET_CZ_REQUEST_CHAT` | 0x008C | variable | `clif_parse_GlobalMessage` | `[int16 PacketType][uint16 Length][char* Msg]` | `clif_packetdb.hpp:40 parseable_packet(0x008c,-1,...,pos[0]=2,pos[1]=4)` |
| `PACKET_CZ_REQUEST_MOVE2` | 0x0437, 0x035F | 5 | `clif_parse_WalkToXY` | `[int16 PacketType][uint8[3] PosDir]` | `clif_packetdb.hpp:1438,1381 parseable_packet(...,5,...,pos[0]=2)` |
| `PACKET_CZ_REQUEST_TIME` | 0x007E | 6 | `clif_parse_TickSend` | `[int16 PacketType][uint32 Tick]` | `clif_packetdb.hpp:33 parseable_packet(0x007e,6,...,pos[0]=2)` |
| `PACKET_CZ_REQUEST_TIME2` | 0x0360 | 6 | `clif_parse_TickSend` | `[int16 PacketType][uint32 Tick]` | `clif_packetdb.hpp:1382 parseable_packet(0x0360,6,...,pos[0]=2)` |
| `PACKET_CZ_REQ_NEXT_SCRIPT` | 0x00B9 | 6 | `clif_parse_NpcNextClicked` | `[int16 PacketType][uint32 NPCID]` | `clif_packetdb.hpp:63 parseable_packet(0x00b9,6,...,pos[0]=2)` |
| `PACKET_CZ_USE_ITEM` | 0x00A7 | 8 | `clif_parse_UseItem` | `[int16 PacketType][uint16 Index][uint32 AccountID]` | `clif_packetdb.hpp:56 parseable_packet(0x00a7,8,...,pos[0]=2,pos[1]=4)` |
| `PACKET_CZ_USE_SKILL_TOGROUND` | 0x0116 | 10 | `clif_parse_UseSkillToPos` | `[int16 PacketType][uint16 SkillLevel][uint16 SkillID][uint16 X][uint16 Y]` | `clif_packetdb.hpp:114 parseable_packet(0x0116,10,...,pos[0]=2,pos[1]=4,pos[2]=6,pos[3]=8)` |
| `PACKET_CZ_ENTER` | 0x0436 | 19 | `clif_parse_WantToConnection` | `[int16 PacketType][uint32 AccountID][uint32 CharID][uint32 AuthCode][uint32 Tick][uint8 Sex]` | `clif_packetdb.hpp:1148 parseable_packet(0x0436,19,...,pos[0]=2,pos[1]=6,pos[2]=10,pos[3]=14,pos[4]=18)` |

### ZC Packets (Server → Client, sent with raw WFIFOW macros)

| Phantom Name | Packet ID | Length | Layout | Source Evidence |
|---|---|---|---|---|
| `PACKET_ZC_AID` | 0x0283 | 6 | `[int16 PacketType][uint32 AccountID]` | `clif_packetdb.hpp:800 packet(0x0283,6)`. Docs: ZC_ACCEPT_ENTER2 (map entry notification). No struct in rAthena source. |
| `PACKET_ZC_PC_SELL_RESULT` | 0x00CB | 3 | `[int16 PacketType][uint8 Result]` | `clif.cpp:12325-12337 WFIFOW(fd,0)=0xcb; WFIFOB(fd,2)=result`. No struct defined. |
| `PACKET_ZC_PETEGG_LIST` | 0x01A6 | variable | `[int16 PacketType][uint16 Length][uint16[] Indices]` | `clif.cpp:8238-8265 raw WFIFOW macros, variable length`. No struct. |
| `PACKET_ZC_QUEST_NOTIFY_EFFECT` | 0x02B3 | 107 | Unknown — only registered in clif_packetdb | `clif_packetdb.hpp:896 packet(0x02b3,107)`. No function sends this in rAthena. Possibly legacy/deprecated. |

### CH Packets (Client → Char Server — outside map server)

| Phantom Name | Packet ID | Length | Notes |
|---|---|---|---|
| `PACKET_CH_ENTER_0x0065` | 0x0065 | 17 | Client→char server auth. Not in map server code at all. `clif_packetdb.hpp:11 packet(0x0065,17)`. |
| `PACKET_CH_ENTER` | 0x0275 | 37 | Client→char server auth (variant). Not in rAthena map server source at all. |

**Total Category B: 23 packets**

---

## Summary

| Category | Count |
|---|---|
| A: Wrong name → real rAthena struct | 25 |
| B: Genuinely structless (need synthetic struct or raw) | 23 |
| **Total phantoms** | **48** |

---

## Action Plan

### Category A fixes
Update `semantics/mappings.yaml` to use real rAthena struct names. No synthetic structs needed.

Key mappings to fix:
- STANDENTRY/STANDENTRY2/STANDENTRY11 → `packet_idle_unit`
- NEWENTRY/NEWENTRY2/NEWENTRY3/NEWENTRY11 → `packet_spawn_unit`
- MOVEENTRY/MOVEENTRY2/MOVEENTRY3/MOVEENTRY4/MOVEENTRY10 → `packet_unit_walking`
- DROPFLOORITEM / ITEM_FALL_ENTRY3 → `packet_dropflooritem`
- NORMAL_ITEMLIST / INVENTORY_ITEMLIST_NORMAL → `packet_itemlist_normal`
- EQUIPMENT_ITEMLIST / INVENTORY_ITEMLIST_EQUIP → `packet_itemlist_equip`
- MSG_STATE_CHANGE → `packet_sc_notick`
- NOTIFY_ACT2 / NOTIFY_ACT_DAMAGE → `packet_damage`
- SKILL_ENTRY → `packet_skill_entry`
- STATUS_CHANGE2 → `packet_status_change2`
- HP_INFO → `packet_monster_hp`
- USE_ITEM_ACK2 → `PACKET_ZC_USE_ITEM_ACK`

### Category B synthetic structs
Write `internal/codegen/stubs/synthetic_structs.hpp` with hand-written packed structs for:
- All CZ structless packets (CLOSE_STORE, REQUEST_MOVE2, REQUEST_TIME, etc.)
- ZC_AID, ZC_PC_SELL_RESULT, ZC_PETEGG_LIST
- CH_ENTER packets (if needed for OpenKore send side)

Mark these in SemanticDB with `synthetic: true` flag (pending schema update).

### Notes on ZC_PAR_4JOB_CHANGE (0x0B25)
Only `DEFINE_PACKET_ID(ZC_PAR_4JOB_CHANGE, 0x0b25)` exists in `packets_struct.hpp:347`.
No struct, no function uses it. It is an ID-only definition. Not in `clif_packetdb.hpp` at all
(not registered). **Status: unused/future packet. No action needed.**

### Notes on ZC_QUEST_NOTIFY_EFFECT (0x02B3)
Only in `clif_packetdb.hpp:896 packet(0x02b3,107)`. No sending function uses it.
`clif_quest_show_event` actually sends `0x0446` (ZC_QUEST_NOTIFY_EFFECT2), not 0x02B3.
**Status: legacy/deprecated. No struct, no active usage.**

### Notes on PACKET_CZ_NOTIFY_ACTORINIT (0x007D)
The SemanticDB phantom name `PACKET_CZ_NOTIFY_ACTORINIT` is direction-confused: the real
rAthena struct `PACKET_ZC_NOTIFY_ACTORINIT` (defined at `packets_struct.hpp:4029`) is for
packet ID `0x0b1b` (server→client). The packet `0x007D` is client→server (CZ), 2 bytes,
handled by `clif_parse_LoadEndAck`. It is structless. Synthetic struct needed.
