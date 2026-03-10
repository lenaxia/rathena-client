// synthetic_structs.hpp — Hand-written packed struct definitions for rAthena packets
// that have no struct in rAthena source (use raw RFIFOW/WFIFOW macro access instead).
//
// Layout for each struct is derived from:
//   1. parseable_packet(id, length, handler, pos[0], pos[1], ...) — length and offsets
//   2. clif.cpp handler implementation — field types and semantics
//   3. clif_packetdb.hpp entries — confirmation of length
//
// All structs use __attribute__((packed)) to match rAthena conventions.
// These are NEVER compiled into rAthena — they exist only for gokore codegen.

#pragma once
#include <stdint.h>
typedef uint8_t  uint8;
typedef uint16_t uint16;
typedef uint32_t uint32;
typedef int16_t  int16;
typedef int32_t  int32;

// ============================================================================
// CZ Packets (Client → Server) — 2-byte header only
// ============================================================================

// 0x00EB CZ_CONCLUDE_EXCHANGE_ITEM — Confirm trade (accept)
// parseable_packet(0x00eb, 2, clif_parse_TradeOk, 0)
// Length: 2
struct SYNTH_CZ_CONCLUDE_EXCHANGE_ITEM {
    int16 PacketType;
} __attribute__((packed));

// 0x00F7 CZ_CLOSE_STORE — Close Kafra/storage interaction
// parseable_packet(0x00f7, 2, clif_parse_CloseKafra, 0)
// Length: 2
struct SYNTH_CZ_CLOSE_STORE {
    int16 PacketType;
} __attribute__((packed));

// 0x007D CZ_NOTIFY_ACTORINIT (LoadEndAck) — Client signals map load complete
// parseable_packet(0x007d, 2, clif_parse_LoadEndAck, 0)
// Length: 2
struct SYNTH_CZ_NOTIFY_ACTORINIT {
    int16 PacketType;
} __attribute__((packed));

// ============================================================================
// CZ Packets — 6-byte variants
// ============================================================================

// 0x009F CZ_ITEM_PICKUP — Pick up a floor item
// parseable_packet(0x009f, 6, clif_parse_TakeItem, 2)  [pos[0]=2 = ObjectID offset]
// Length: 6
struct SYNTH_CZ_ITEM_PICKUP {
    int16  PacketType;
    uint32 ITID;        // Object/item ID on the floor
} __attribute__((packed));

// 0x0362 CZ_ITEM_PICKUP2 — Pick up floor item (shuffle variant)
// parseable_packet(0x0362, 6, clif_parse_TakeItem, 2)
// Length: 6
struct SYNTH_CZ_ITEM_PICKUP2 {
    int16  PacketType;
    uint32 ITID;        // Object/item ID on the floor
} __attribute__((packed));

// 0x00B9 CZ_REQ_NEXT_SCRIPT — NPC "next" button clicked
// parseable_packet(0x00b9, 6, clif_parse_NpcNextClicked, 2)  [pos[0]=2 = NPCID offset]
// Length: 6
struct SYNTH_CZ_REQ_NEXT_SCRIPT {
    int16  PacketType;
    uint32 NpcID;       // NPC block ID
} __attribute__((packed));

// 0x007E CZ_REQUEST_TIME — Client tick synchronization
// parseable_packet(0x007e, 6, clif_parse_TickSend, 2)  [pos[0]=2 = Tick offset]
// Length: 6
struct SYNTH_CZ_REQUEST_TIME {
    int16  PacketType;
    uint32 clientTime;  // Client-side tick value
} __attribute__((packed));

// 0x0360 CZ_REQUEST_TIME2 — Client tick sync (shuffle variant)
// parseable_packet(0x0360, 6, clif_parse_TickSend, 2)
// Length: 6
struct SYNTH_CZ_REQUEST_TIME2 {
    int16  PacketType;
    uint32 ClientTime;  // Client-side tick value
} __attribute__((packed));

// ============================================================================
// CZ Packets — 5-byte walk packets (PosDir)
// ============================================================================

// 0x0085 CZ_REQUEST_MOVE — Walk to position (baseline format, all versions)
// parseable_packet(0x0085, 5, clif_parse_WalkToXY, 2)  [pos[0]=2 = PosDir offset]
// Length: 5.
// NOTE: grep -rn "struct PACKET_CZ_REQUEST_MOVE" across all rAthena src returns zero results.
// No real rAthena struct exists for this packet at any PACKETVER. This is the canonical
// synthetic struct for all versions of 0x0085 where the handler is clif_parse_WalkToXY.
// (In shuffle tables, 0x0085 is reassigned to other handlers — this struct only applies
// when the pos[0]=2 / 5-byte / WalkToXY assignment is active.)
struct SYNTH_CZ_REQUEST_MOVE {
    int16  PacketType;
    uint8  PosDir[3];   // Packed position: x(10 bits), y(10 bits), dir(4 bits)
} __attribute__((packed));

// 0x035F CZ_REQUEST_MOVE2 — Walk to position (shuffle variant)
// parseable_packet(0x035f, 5, clif_parse_WalkToXY, 2)
// Length: 5
struct SYNTH_CZ_REQUEST_MOVE2 {
    int16  PacketType;
    uint8  dest[3];     // Packed position: x(10 bits), y(10 bits), dir(4 bits)
} __attribute__((packed));

// ============================================================================
// CZ Packets — 8-byte variants
// ============================================================================

// 0x009F → see 6-byte section above

// 0x00A7 CZ_USE_ITEM — Use an inventory item
// parseable_packet(0x00a7, 8, clif_parse_UseItem, 2, 4)  [pos[0]=2=index, pos[1]=4=AID]
// Length: 8
struct SYNTH_CZ_USE_ITEM {
    int16  PacketType;
    uint16 index;       // Inventory index (client-side, 2 = first slot)
    uint32 AID;         // Account ID (for target verification)
} __attribute__((packed));

// 0x00F3 CZ_MOVE_ITEM_FROM_BODY_TO_STORE — Move item to Kafra storage
// parseable_packet(0x00f3, 8, clif_parse_MoveToKafra, 2, 4)  [pos[0]=2=index, pos[1]=4=amount]
// Length: 8
struct SYNTH_CZ_MOVE_ITEM_FROM_BODY_TO_STORE {
    int16  PacketType;
    uint16 index;       // Inventory index
    uint32 amount;      // Amount to store
} __attribute__((packed));

// 0x00F5 CZ_MOVE_ITEM_FROM_STORE_TO_BODY — Retrieve item from Kafra storage
// parseable_packet(0x00f5, 8, clif_parse_MoveFromKafra, 2, 4)  [pos[0]=2=index, pos[1]=4=amount]
// Length: 8
struct SYNTH_CZ_MOVE_ITEM_FROM_STORE_TO_BODY {
    int16  PacketType;
    uint16 index;       // Storage index
    uint32 amount;      // Amount to retrieve
} __attribute__((packed));

// ============================================================================
// CZ Packets — fixed-length, other sizes
// ============================================================================

// 0x0363 CZ_ITEM_THROW2 — Drop an item (shuffle variant)
// parseable_packet(0x0363, 6, clif_parse_DropItem, 2, 4)  [pos[0]=2=Index, pos[1]=4=Amount]
// Length: 6
struct SYNTH_CZ_ITEM_THROW2 {
    int16  PacketType;
    uint16 Index;       // Inventory index
    uint16 Amount;      // Amount to drop
} __attribute__((packed));

// 0x085a CZ_REQUEST_ACT (shuffle variant at PACKETVER >= 20200401)
// parseable_packet(0x085a, 7, clif_parse_ActionRequest, 2, 6)
// [pos[0]=2=TargetGID, pos[1]=6=Action]
// Length: 7, fixed. Same layout as 0x0089 but shuffled packet ID.
// GCC-verified at PACKETVER=20200401: packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0)
struct SYNTH_CZ_REQUEST_ACT {
    int16  PacketType;
    uint32 TargetGID;   // Target actor GID (pos[0]=2)
    uint8  Action;      // Action type: 7=normal attack, 0=sit, 2=stand (pos[1]=6)
} __attribute__((packed));

// 0x0862 CZ_USE_SKILL_TOID (shuffle variant at PACKETVER >= 20200401)
// parseable_packet(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6)
// [pos[0]=2=skillLevel, pos[1]=4=skillID, pos[2]=6=targetID]
// Length: 10, fixed.
// GCC-verified at PACKETVER=20200401: packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0)
struct SYNTH_CZ_USE_SKILL_TOID {
    int16  PacketType;
    uint16 SkillLv;     // Skill level (pos[0]=2)
    uint16 SkillID;     // Skill ID (pos[1]=4)
    uint32 TargetID;    // Target actor GID (pos[2]=6)
} __attribute__((packed));


// parseable_packet(0x0116, 10, clif_parse_UseSkillToPos, 2, 4, 6, 8)
// [pos[0]=2=skillLevel, pos[1]=4=skillID, pos[2]=6=xPos, pos[3]=8=yPos]
// Length: 10
struct SYNTH_CZ_USE_SKILL_TOGROUND {
    int16  PacketType;
    uint16 skillLevel;  // Skill level
    uint16 skillID;     // Skill ID
    uint16 xPos;        // Target X coordinate
    uint16 yPos;        // Target Y coordinate
} __attribute__((packed));

// 0x0436 CZ_ENTER2 — Map server connection request (new format)
// parseable_packet(0x0436, 19, clif_parse_WantToConnection, 2, 6, 10, 14, 18)
// [pos[0]=2=AID, pos[1]=6=GID, pos[2]=10=AuthCode, pos[3]=14=clientTime, pos[4]=18=Sex]
// Length: 19
struct SYNTH_CZ_ENTER {
    int16  PacketType;
    uint32 AID;         // Account ID
    uint32 GID;         // Character ID
    int32  AuthCode;    // Session/auth code
    uint32 clientTime;  // Client tick
    uint8  Sex;         // Character sex
} __attribute__((packed));

// ============================================================================
// CZ Packets — variable-length
// ============================================================================

// 0x008C CZ_REQUEST_CHAT — Public chat message
// parseable_packet(0x008c, -1, clif_parse_GlobalMessage, 2, 4)
// [pos[0]=2=Length, pos[1]=4=MessageStart]
// Layout: [PacketType][Length][message (null-terminated, variable)]
// Note: message format is "CharName : text\0"
struct SYNTH_CZ_REQUEST_CHAT {
    int16  PacketType;
    uint16 Length;      // Total packet length including header
    char   message[1];  // Variable-length null-terminated message (flexible array)
} __attribute__((packed));

// ============================================================================
// CH Packets (Client → Char Server)
// These are NOT in rAthena map server code. Layouts from OpenKore + packet docs.
// ============================================================================

// 0x0065 CH_ENTER — Char server authentication (old format)
// packet(0x0065, 17) — 17 bytes total
// Layout confirmed from OpenKore game_login handler
struct SYNTH_CH_ENTER_0x0065 {
    int16  PacketType;
    uint32 AID;         // Account ID
    uint32 AuthCode;    // Login session key 1
    uint32 login_id2;   // Login session key 2
    uint16 clienttype;  // Client type / user level
    uint8  sex;         // Account sex
} __attribute__((packed));

// 0x0275 CH_ENTER — Char server authentication (newer format, 37 bytes)
// packet(0x0275, 37) — but no struct in rAthena source at all.
// Layout unknown beyond PacketType. Fields beyond PacketType are placeholder padding.
//
// WARNING: DO NOT USE IN ENCODE PIPELINE.
// The 35 bytes of _padding are zeroed placeholders for unknown fields.
// This struct exists only for length accounting. Any encode using this struct will
// silently write 35 zero bytes for the unknown fields. Mark fields as unimplemented
// and exclude this struct from send codegen until the layout is confirmed.
struct SYNTH_CH_ENTER {
    int16  PacketType;
    uint8  _padding[35]; // Unknown fields — placeholder until layout is confirmed
} __attribute__((packed));

// ============================================================================
// ZC Packets (Server → Client) — sent with raw WFIFOW macros
// ============================================================================

// 0x0283 ZC_ACCEPT_ENTER2 (ZC_AID) — Map entry confirmation + account ID
// packet(0x0283, 6) — 6 bytes
// Confirmed: WFIFOW(fd,0)=0x0283, WFIFOL(fd,2)=account_id
struct SYNTH_ZC_AID {
    int16  PacketType;
    uint32 AID;         // Account ID (map server confirms client identity)
} __attribute__((packed));

// 0x00CB ZC_PC_SELL_RESULT — Result of selling items to NPC
// packet(0x00cb, 3) — 3 bytes
// Confirmed: clif.cpp:12332-12337 WFIFOW(fd,0)=0xcb; WFIFOB(fd,2)=result
struct SYNTH_ZC_PC_SELL_RESULT {
    int16 PacketType;
    uint8 result;       // 0=success, 1=failure
} __attribute__((packed));

// 0x01A6 ZC_PETEGG_LIST — List of pet egg items in inventory
// packet(0x01a6, -1) — variable length
// Confirmed: clif.cpp:8238-8265 WFIFOW(fd,0)=0x1a6; WFIFOW(fd,2)=len; indices at 4+n*2
struct SYNTH_ZC_PETEGG_LIST {
    int16  PacketType;
    uint16 PacketLength; // Total packet length
    uint16 eggs[];       // Variable-length array of inventory indices (flexible array)
} __attribute__((packed));

// ============================================================================
// ZC Packets (Server → Client) — structless (raw WFIFOW only), Gap C
// Each layout is derived from:
//   1. clif_packetdb.hpp packet(id, length) registration
//   2. clif.cpp WFIFOW/WFIFOL call sites for the relevant function
// ============================================================================

// 0x008E ZC_NOTIFY_PLAYERCHAT — Server echoes player chat back to client
// clif_packetdb.hpp:42  packet(0x008e,-1)
// clif.cpp:6663  clif_displaymessage — layout: 008e <packetLen>.W <message>.?B
// Length: variable; PacketLength at bytes [2:4] gives total frame size.
struct SYNTH_ZC_NOTIFY_PLAYERCHAT {
    int16  PacketType;
    uint16 PacketLength; // Total packet length including header
    char   message[];    // Null-terminated message string (flexible array)
} __attribute__((packed));

// 0x02D9 ZC_CONFIG — Server sends config type + enabled flag
// clif_packetdb.hpp:920  packet(0x02d9,10)
// clif.cpp:10284  clif_configuration — layout: 02d9 <type>.L <value>.L
// Length: 2 + 4 + 4 = 10 bytes
struct SYNTH_ZC_CONFIG {
    int16  PacketType;
    uint32 type;    // enum e_config_type
    uint32 value;   // 0=disabled, 1=enabled
} __attribute__((packed));

// 0x0A24 ZC_ACH_UPDATE — Single achievement update
// clif_packetdb.hpp:1767  packet(0x0A24,66)  [inside #if PACKETVER >= 20150513]
// clif.cpp:21864  WFIFOW(fd,0)=0xa24; layout from WFIFOL/WFIFOW calls:
//   +2  uint32 total_score
//   +6  uint16 level
//   +8  uint32 achievement_exp
//   +12 uint32 achievement_exp_tnl
//   +16 uint32 achievement_id
//   +20 uint8  is_complete
//   +21 uint32 count[10]  (MAX_ACHIEVEMENT_OBJECTIVES=10, each 4 bytes = 40 bytes)
//   +61 uint32 completed_epoch
//   +65 uint8  rewarded
// Total: 2+4+2+4+4+4+1+40+4+1 = 66 bytes
struct SYNTH_ZC_ACH_UPDATE {
    int16  PacketType;
    uint32 total_score;
    uint16 level;
    uint32 achievement_exp;
    uint32 achievement_exp_tnl;
    uint32 achievement_id;
    uint8  is_complete;
    uint32 count[10];
    uint32 completed_epoch;
    uint8  rewarded;
} __attribute__((packed));

// 0x0ADE ZC_OVERWEIGHT_PERCENT — Weight limit threshold percentage
// clif_packetdb.hpp:1891  packet(0x0ADE,6)  [inside #if PACKETVER >= 20171025]
// clif.cpp:22036  WFIFOW(fd,0)=0xADE; WFIFOL(fd,2)=battle_config.natural_heal_weight_rate
// Layout: 0ADE <percentage>.L
// Length: 2 + 4 = 6 bytes
struct SYNTH_ZC_OVERWEIGHT_PERCENT {
    int16  PacketType;
    uint32 percent; // Natural heal weight rate percentage
} __attribute__((packed));

// 0x0A9B ZC_EQUIPSWITCH_LIST — Full list of equip-switch window items
// clif_packetdb.hpp:1860  packet(0x0A9B,-1)  [inside #if PACKETVER >= 20170208]
// clif.cpp:22231  WFIFOW(fd,0)=0xa9b; WFIFOW(fd,2)=offset (total length)
// Layout: 0a9b <length>.W { <index>.W <position>.L }*
// Length: variable; PacketLength at bytes [2:4] gives total frame size.
struct SYNTH_ZC_EQUIPSWITCH_LIST {
    int16  PacketType;
    uint16 PacketLength; // Total packet length including header
    uint8  items[];      // Variable-length array of {uint16 index; uint32 position} records
} __attribute__((packed));

// 0x0A23 ZC_ALL_ACH_LIST — Full achievement list sent on login
// clif_packetdb.hpp:1766  packet(0x0A23,-1)  [inside #if PACKETVER >= 20150513]
// clif.cpp:21829  WFIFOW(fd,0)=0xa23; WFIFOW(fd,2)=len
// Layout: 0a23 <length>.W <count>.L <total_score>.L <level>.W
//         <ach_exp>.L <ach_exp_tnl>.L { achievement_record }*count
// Length: variable; PacketLength at bytes [2:4] gives total frame size.
struct SYNTH_ZC_ALL_ACH_LIST {
    int16  PacketType;
    uint16 PacketLength; // Total packet length including header
    uint8  data[];       // Variable-length payload (flexible array)
} __attribute__((packed));
