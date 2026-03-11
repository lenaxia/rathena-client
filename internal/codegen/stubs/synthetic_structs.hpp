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

// 0x009B CZ_CHANGE_DIRECTION — Change character head/body direction
// parseable_packet(0x009b, 5, clif_parse_ChangeDir, 2, 4)
// [pos[0]=2=headDir (byte), pos[1]=4=dir (byte)]
// Length: 5
struct SYNTH_CZ_CHANGE_DIRECTION {
    int16 PacketType;
    uint8 headDir;  // Head direction (0–7)
    uint8 dir;      // Body direction (0–7)
} __attribute__((packed));

// 0x00AB CZ_REQ_TAKEOFF_EQUIP — Unequip an item
// parseable_packet(0x00ab, 4, clif_parse_UnequipItem, 2)
// [pos[0]=2=index (uint16, client-side inventory index)]
// Length: 4
struct SYNTH_CZ_REQ_TAKEOFF_EQUIP {
    int16  PacketType;
    uint16 index;   // Inventory index (client-side, 2 = first slot)
} __attribute__((packed));

// 0x00B8 CZ_CHOOSE_MENU — NPC menu selection
// parseable_packet(0x00b8, 7, clif_parse_NpcSelectMenu, 2, 6)
// [pos[0]=2=NpcID (uint32), pos[1]=6=select (uint8)]
// Length: 7
struct SYNTH_CZ_CHOOSE_MENU {
    int16  PacketType;
    uint32 NpcID;   // NPC block ID
    uint8  select;  // Menu index selected (1-based; 0xFF = cancel)
} __attribute__((packed));

// 0x00E4 CZ_REQ_EXCHANGE_ITEM — Initiate a trade with another player
// parseable_packet(0x00e4, 6, clif_parse_TradeRequest, 2)
// [pos[0]=2=targetAID (uint32)]
// Length: 6
struct SYNTH_CZ_REQ_EXCHANGE_ITEM {
    int16  PacketType;
    uint32 targetAID;  // Account ID of the player to trade with
} __attribute__((packed));

// 0x00E6 CZ_ACK_EXCHANGE_ITEM — Accept or reject a trade request
// parseable_packet(0x00e6, 3, clif_parse_TradeAck, 2)
// [pos[0]=2=result (uint8): 3=accept, 4=reject]
// Length: 3
struct SYNTH_CZ_ACK_EXCHANGE_ITEM {
    int16 PacketType;
    uint8 result;   // 3 = accepted, 4 = rejected
} __attribute__((packed));

// 0x00ED CZ_CANCEL_EXCHANGE_ITEM — Cancel current trade
// parseable_packet(0x00ed, 2, clif_parse_TradeCancel, 0)
// Length: 2 (header only)
struct SYNTH_CZ_CANCEL_EXCHANGE_ITEM {
    int16 PacketType;
} __attribute__((packed));

// 0x00EF CZ_EXEC_EXCHANGE_ITEM — Commit (confirm) current trade
// parseable_packet(0x00ef, 2, clif_parse_TradeCommit, 0)
// Length: 2 (header only)
struct SYNTH_CZ_EXEC_EXCHANGE_ITEM {
    int16 PacketType;
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
    UNAVAILABLE_STRUCT
    int16  PacketType;
    uint8  _padding[35]; // Unknown fields — placeholder until layout is confirmed
} __attribute__((packed));

// ============================================================================
// ZC Packets (Server → Client) — sent with raw WFIFOW macros
// ============================================================================

// 0x0086 ZC_NOTIFY_MOVE — Other entity moving (alpha/unused in modern rAthena)
// Defined but commented out in packets.hpp:664 as "Unused packet (alpha?)"
// Layout derived from the commented-out definition:
//   packetType(2) + gid(4) + moveData[6] + moveStartTime(4) = 16 bytes
// NOTE: Modern rAthena never sends 0x0086; walking entities use 0x009D (UNIT_WALKING).
// This struct exists so entity_move decode generates a working function for legacy
// packetver compatibility. At modern packetver the packet is never received.
struct SYNTH_ZC_NOTIFY_MOVE {
    int16 packetType;
    uint32 gid;           // Source entity GID
    uint8 moveData[6];    // Packed move path (from, to, dir)
    uint32 moveStartTime; // Move start tick
} __attribute__((packed));

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

// ============================================================================
// EPIC-04: Missing CZ encode functions — Bucket 1 (structs from packets.hpp)
// ============================================================================
// These structs are defined in rAthena's packets.hpp (not packets_struct.hpp)
// and were missed by the codegen preprocessor pass. Mirrored here verbatim
// from GCC output at PACKETVER=20180307.
// ============================================================================

// 0x0103 CZ_REQ_EXPEL_GROUP_MEMBER — Kick member from party
// packets.hpp: struct PACKET_CZ_REQ_EXPEL_GROUP_MEMBER
// GCC verified: int16 packetType + uint32 AID + char name[24] = 31 bytes
struct SYNTH_CZ_REQ_EXPEL_GROUP_MEMBER {
    int16  PacketType;
    uint32 AID;
    char   name[24];
} __attribute__((packed));

// 0x0126 CZ_MOVE_ITEM_FROM_BODY_TO_CART — Move item body→cart
// packets_struct.hpp: struct PACKET_CZ_MOVE_ITEM_FROM_BODY_TO_CART (already exists)
// Alias provided here for consistency — codegen will use the real struct via semantics DB.

// 0x0127 CZ_MOVE_ITEM_FROM_CART_TO_BODY — Move item cart→body
// packets.hpp: struct PACKET_CZ_MOVE_ITEM_FROM_CART_TO_BODY
// GCC verified: int16 packetType + uint16 index + int32 amount = 8 bytes
struct SYNTH_CZ_MOVE_ITEM_FROM_CART_TO_BODY {
    int16  PacketType;
    uint16 index;
    int32  amount;
} __attribute__((packed));

// 0x0178 CZ_REQ_ITEMIDENTIFY — Identify an item (magnifier)
// packets.hpp: struct PACKET_CZ_REQ_ITEMIDENTIFY
// GCC verified: int16 packetType + uint16 index = 4 bytes
struct SYNTH_CZ_REQ_ITEMIDENTIFY {
    int16  PacketType;
    uint16 index;
} __attribute__((packed));

// 0x017A CZ_REQ_ITEMCOMPOSITION_LIST — Request card-composition item list
// packets.hpp: struct PACKET_CZ_REQ_ITEMCOMPOSITION_LIST
// GCC verified: int16 packetType + uint16 index = 4 bytes
struct SYNTH_CZ_REQ_ITEMCOMPOSITION_LIST {
    int16  PacketType;
    uint16 index;
} __attribute__((packed));

// 0x017C CZ_REQ_ITEMCOMPOSITION — Insert card into equipment
// packets.hpp: struct PACKET_CZ_REQ_ITEMCOMPOSITION
// GCC verified: int16 packetType + uint16 index_card + uint16 index_equip = 6 bytes
struct SYNTH_CZ_REQ_ITEMCOMPOSITION {
    int16  PacketType;
    uint16 index_card;
    uint16 index_equip;
} __attribute__((packed));

// 0x01AE CZ_REQ_MAKINGARROW — Request arrow crafting
// packets.hpp: struct PACKET_CZ_REQ_MAKINGARROW
// GCC verified: int16 packetType + uint16 itemId = 4 bytes
struct SYNTH_CZ_REQ_MAKINGARROW {
    int16  PacketType;
    uint16 itemId;
} __attribute__((packed));

// 0x01CE CZ_SELECTAUTOSPELL — Select autospell skill
// packets.hpp: struct PACKET_CZ_SELECTAUTOSPELL
// GCC verified: int16 packetType + uint32 skill_id = 6 bytes
struct SYNTH_CZ_SELECTAUTOSPELL {
    int16  PacketType;
    uint32 skill_id;
} __attribute__((packed));

// 0x01FD CZ_REQ_ITEMREPAIR1 — Request weapon repair
// packets.hpp: struct PACKET_CZ_REQ_ITEMREPAIR1 { int16 packetType; REPAIRITEM_INFO1 item; }
// REPAIRITEM_INFO1: int16 index + uint16 itemId + uint8 refine + EQUIPSLOTINFO(uint16[4]=8) = 12 bytes
// Total: 2 + 12 = 14 bytes
struct SYNTH_CZ_REQ_ITEMREPAIR1 {
    int16  PacketType;
    int16  index;
    uint16 itemId;
    uint8  refine;
    uint16 card[4];
} __attribute__((packed));

// 0x0232 CZ_REQUEST_MOVENPC — Move homunculus to position
// packets.hpp: struct PACKET_CZ_REQUEST_MOVENPC
// GCC verified: int16 packetType + uint32 GID + uint8 PosDir[3] = 9 bytes
// Note: PosDir[3] requires packing.EncodePosDir — this action stays hand-written
// (same reason as move_to). Struct provided for reference only.
struct SYNTH_CZ_REQUEST_MOVENPC {
    int16  PacketType;
    uint32 GID;
    uint8  PosDir[3];
} __attribute__((packed));

// ============================================================================
// EPIC-04: Missing CZ encode functions — Bucket 2A (fixed-size, no struct)
// ============================================================================
// These packets are registered via raw parseable_packet(0xNNNN, fixedLen, ...)
// with no struct. Layouts derived from clif.cpp RFIFOW/RFIFOB/RFIFOL calls
// and parseable_packet pos[] offsets.
// ============================================================================

// 0x00B2 CZ_RESTART — Respawn or return to char select
// parseable_packet(0x00b2, 3, clif_parse_Restart, 2)
// clif.cpp: RFIFOB(fd, pos[0]=2) = type (0=respawn, 1=char select)
// Layout: int16 PacketType + uint8 type = 3 bytes
struct SYNTH_CZ_RESTART {
    int16 PacketType;
    uint8 type;
} __attribute__((packed));

// 0x00BB CZ_STATUS_CHANGE — Request to increase a base status
// parseable_packet(0x00bb, 5, clif_parse_StatusUp, 2, 4)
// clif.cpp: RFIFOW(pos[0]=2)=statusType, RFIFOB(pos[1]=4)=amount
// Layout: int16 PacketType + uint16 statusType + uint8 amount = 5 bytes
struct SYNTH_CZ_STATUS_CHANGE {
    int16  PacketType;
    uint16 statusType;
    uint8  amount;
} __attribute__((packed));

// 0x0102 CZ_CHANGE_GROUPEXPOPTION — Toggle party share/split options
// parseable_packet(0x0102, 6, clif_parse_PartyChangeOption, 2)
// clif.cpp: RFIFOW(pos[0]=2)=option (bit field: exp share | item share)
// Layout: int16 PacketType + uint16 expOption + uint16 itemOption = 6 bytes
// Note: only 1 uint32 field at offset 2 per actual RFIFOL usage — verify
struct SYNTH_CZ_CHANGE_GROUPEXPOPTION {
    int16  PacketType;
    uint32 option;
} __attribute__((packed));

// 0x0112 CZ_UPGRADE_SKILLLEVEL — Request to increase a skill level
// parseable_packet(0x0112, 4, clif_parse_SkillUp, 2)
// clif.cpp: RFIFOW(pos[0]=2) = skill_id
// Layout: int16 PacketType + uint16 skill_id = 4 bytes
struct SYNTH_CZ_UPGRADE_SKILLLEVEL {
    int16  PacketType;
    uint16 skill_id;
} __attribute__((packed));

// 0x011D CZ_REMEMBER_WARPPOINT — Save current position as memo point
// parseable_packet(0x011d, 2, clif_parse_RequestMemo, 0)
// No payload fields — header only.
// Layout: int16 PacketType = 2 bytes
struct SYNTH_CZ_REMEMBER_WARPPOINT {
    int16 PacketType;
} __attribute__((packed));

// 0x012A CZ_REMOVE_AID — Remove cart/falcon/peco
// parseable_packet(0x012a, 2, clif_parse_RemoveOption, 0)
// No payload fields — header only.
// Layout: int16 PacketType = 2 bytes
struct SYNTH_CZ_REMOVE_AID {
    int16 PacketType;
} __attribute__((packed));

// 0x012E CZ_CLOSE_AUCTION — Close own vending shop
// parseable_packet(0x012e, 2, clif_parse_CloseVending, 0)
// No payload fields — header only.
// Layout: int16 PacketType = 2 bytes
struct SYNTH_CZ_CLOSE_AUCTION {
    int16 PacketType;
} __attribute__((packed));

// 0x0130 CZ_PC_PURCHASE_ITEMLIST_FROMMC2 — Request vending shop item list
// parseable_packet(0x0130, 6, clif_parse_VendingListReq, 2)
// clif.cpp: RFIFOL(pos[0]=2) = AID of vending player
// Layout: int16 PacketType + uint32 AID = 6 bytes
struct SYNTH_CZ_REQ_CLICK_TO_BUYING_STORE {
    int16  PacketType;
    uint32 AID;
} __attribute__((packed));

// 0x018A CZ_REQ_DISCONNECT — Request to quit/disconnect
// parseable_packet(0x018a, 4, clif_parse_QuitGame, 2)
// clif.cpp: RFIFOW(pos[0]=2) = type (0=quit, 1=char select in some versions)
// Layout: int16 PacketType + uint16 type = 4 bytes
struct SYNTH_CZ_REQ_DISCONNECT {
    int16  PacketType;
    uint16 type;
} __attribute__((packed));

// 0x019F CZ_CATCH_MONSTER — Throw a Shining Stone to catch a monster
// parseable_packet(0x019f, 6, clif_parse_CatchPet, 2)
// clif.cpp: RFIFOL(pos[0]=2) = target monster GID
// Layout: int16 PacketType + uint32 targetId = 6 bytes
struct SYNTH_CZ_CATCH_MONSTER {
    int16  PacketType;
    uint32 targetId;
} __attribute__((packed));

// 0x01A1 CZ_PET_ACT — Pet menu action
// parseable_packet(0x01a1, 3, clif_parse_PetMenu, 2)
// clif.cpp: RFIFOB(pos[0]=2) = action (0=feed, 1=performance, 2=return egg, 3=unequip)
// Layout: int16 PacketType + uint8 action = 3 bytes
struct SYNTH_CZ_PET_ACT {
    int16 PacketType;
    uint8 action;
} __attribute__((packed));

// 0x01A7 CZ_SELECT_PETEGG — Hatch a pet egg
// parseable_packet(0x01a7, 4, clif_parse_SelectEgg, 2)
// clif.cpp: RFIFOW(pos[0]=2) = inventory index of the egg
// Layout: int16 PacketType + uint16 index = 4 bytes
struct SYNTH_CZ_SELECT_PETEGG {
    int16  PacketType;
    uint16 index;
} __attribute__((packed));

// 0x01A9 CZ_SEND_MBMC_CASH — Send emotion to a target actor
// parseable_packet(0x01a9, 6, clif_parse_SendEmotion, 2)
// clif.cpp: RFIFOL(pos[0]=2) = target GID
// Layout: int16 PacketType + uint32 targetId = 6 bytes
struct SYNTH_CZ_SEND_MBMC_CASH {
    int16  PacketType;
    uint32 targetId;
} __attribute__((packed));

// 0x01AF CZ_CHANGE_CART — Change cart appearance/level
// parseable_packet(0x01af, 4, clif_parse_ChangeCart, 2)
// clif.cpp: RFIFOW(pos[0]=2) = cart number (1–5)
// Layout: int16 PacketType + uint16 num = 4 bytes
struct SYNTH_CZ_CHANGE_CART {
    int16  PacketType;
    uint16 num;
} __attribute__((packed));

// 0x0202 CZ_ADD_FRIENDS — Add a player to friends list
// parseable_packet(0x0202, 26, clif_parse_FriendsListAdd, 2)
// clif.cpp: RFIFOCP(pos[0]=2) = player name (NAME_LENGTH=24 bytes)
// Layout: int16 PacketType + char name[24] = 26 bytes
struct SYNTH_CZ_ADD_FRIENDS {
    int16 PacketType;
    char  name[24];
} __attribute__((packed));

// 0x0203 CZ_DELETE_FRIENDS — Remove a player from friends list
// parseable_packet(0x0203, 10, clif_parse_FriendsListRemove, 2, 6)
// clif.cpp: RFIFOL(pos[0]=2)=AID, RFIFOL(pos[1]=6)=CID
// Layout: int16 PacketType + uint32 AID + uint32 CID = 10 bytes
struct SYNTH_CZ_DELETE_FRIENDS {
    int16  PacketType;
    uint32 AID;
    uint32 CID;
} __attribute__((packed));

// 0x0208 CZ_ACK_REQ_ADD_FRIENDS — Reply to a friend request
// parseable_packet(0x0208, 11, clif_parse_FriendsListReply, 2, 6, 10)  [PACKETVER < 6]
// parseable_packet(0x0208, 14, clif_parse_FriendsListReply, 2, 6, 10)  [PACKETVER >= 6]
// clif.cpp: RFIFOL(pos[0]=2)=AID, RFIFOL(pos[1]=6)=CID, RFIFOB/L(pos[2]=10)=reply
// PACKETVER >= 6: reply is uint32 (4 bytes) → total 14 bytes
// PACKETVER < 6:  reply is uint8  (1 byte)  → total 11 bytes
// Using PACKETVER >= 6 layout (20040000+, all supported versions)
// Layout: int16 PacketType + uint32 AID + uint32 CID + uint32 reply = 14 bytes
struct SYNTH_CZ_ACK_REQ_ADD_FRIENDS {
    int16  PacketType;
    uint32 AID;
    uint32 CID;
    uint32 reply;  // 0=reject, 1=accept
} __attribute__((packed));

// 0x0222 CZ_REQ_WEAPONREFINE — Request weapon refine at NPC
// parseable_packet(0x0222, 6, clif_parse_WeaponRefine, 2)
// clif.cpp: RFIFOL(pos[0]=2) = inventory index
// Layout: int16 PacketType + uint32 index = 6 bytes
struct SYNTH_CZ_REQ_WEAPONREFINE {
    int16  PacketType;
    uint32 index;
} __attribute__((packed));

// 0x022D CZ_COMMAND_MER — Homunculus menu command
// parseable_packet(0x022d, 5, clif_parse_HomMenu, 2, 4)
// clif.cpp: RFIFOW(pos[0]=2)=homId, RFIFOB(pos[1]=4)=action
// Layout: int16 PacketType + uint16 homId + uint8 action = 5 bytes
struct SYNTH_CZ_COMMAND_MER {
    int16  PacketType;
    uint16 homId;
    uint8  action;
} __attribute__((packed));

// 0x0233 CZ_REQUEST_ACTNPC — Homunculus attack target
// parseable_packet(0x0233, 11, clif_parse_HomAttack, 2, 6, 10)
// clif.cpp: RFIFOL(pos[0]=2)=targetId, RFIFOL(pos[1]=6)=homId, RFIFOB(pos[2]=10)=action
// Layout: int16 PacketType + uint32 targetId + uint32 homId + uint8 action = 11 bytes
struct SYNTH_CZ_REQUEST_ACTNPC {
    int16  PacketType;
    uint32 targetId;
    uint32 homId;
    uint8  action;
} __attribute__((packed));
