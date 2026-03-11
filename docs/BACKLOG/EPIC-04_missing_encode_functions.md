# EPIC-04 — Missing C→S Encode Functions

**Status**: In Progress  
**Discovered**: 2026-03-11  
**Target**: v0.2.5

## Summary

A systematic audit of `clif_packetdb.hpp` against `pkg/encode/` revealed ~39 missing C→S
encode functions. The previous encode coverage pass only found packets that already had
`PACKET_CZ_*` struct definitions in `packets_struct.hpp`. Two other categories were missed:

1. Structs defined in `packets.hpp` (not `packets_struct.hpp`) — codegen preprocessor pass
   was only reading `packets_struct.hpp` for CZ packets
2. Packets registered as raw `parseable_packet(0xNNNN, fixedLen, ...)` with no struct at all

## Bucket 1 — Real structs in packets.hpp (codegen-able, 15 packets)

These have `PACKET_CZ_*` structs in `packets.hpp`. Add to semantics DB, regenerate.

| clif handler | Struct | Packet ID | Notes |
|---|---|---|---|
| `RemovePartyMember` | `PACKET_CZ_REQ_EXPEL_GROUP_MEMBER` | 0x0103 | int16+uint32+char[24] = 31 bytes |
| `GetItemFromCart` | `PACKET_CZ_MOVE_ITEM_FROM_CART_TO_BODY` | 0x0127 | int16+uint16+int32 = 8 bytes |
| `ItemIdentify` | `PACKET_CZ_REQ_ITEMIDENTIFY` | 0x0178 | int16+uint16 = 4 bytes |
| `UseCard` | `PACKET_CZ_REQ_ITEMCOMPOSITION_LIST` | 0x017A | int16+uint16 = 4 bytes |
| `InsertCard` | `PACKET_CZ_REQ_ITEMCOMPOSITION` | 0x017C | int16+uint16+uint16 = 6 bytes |
| `SelectArrow` | `PACKET_CZ_REQ_MAKINGARROW` | 0x01AE | int16+uint16 = 4 bytes |
| `AutoSpell` | `PACKET_CZ_SELECTAUTOSPELL` | 0x01CE | int16+uint32 = 6 bytes |
| `HomMoveTo` | `PACKET_CZ_REQUEST_MOVENPC` | 0x0232 | int16+uint32+uint8[3] = 9 bytes (PosDir — manual) |
| `RepairItem` | `PACKET_CZ_REQ_ITEMREPAIR1` | 0x01FD | int16+REPAIRITEM_INFO1 — check struct size |
| `ProduceMix` | `PACKET_CZ_REQMAKINGITEM` | varies | int16+uint16+uint16[3] = 10 bytes |
| `Cooking` | `PACKET_CZ_REQ_MAKINGITEM` | varies | int16+int16+uint16 = 6 bytes |
| `SkillSelectMenu` | `PACKET_CZ_SKILL_SELECT_RESPONSE` | varies | int16+int32+int16 = 8 bytes |
| `PurchaseReq` | `PACKET_CZ_PC_PURCHASE_ITEMLIST_FROMMC` | 0x0134 | variable (list[]) — manual |
| `PurchaseReq2` | `PACKET_CZ_PC_PURCHASE_ITEMLIST_FROMMC2` | varies | variable (list[]) — manual |
| `PutItemToCart` | `PACKET_CZ_MOVE_ITEM_FROM_BODY_TO_CART` | 0x0126 | int16+int16+int32 = 8 bytes (already in semantics DB) |

## Bucket 2 — No struct, needs SYNTH or is permanently manual (24 packets)

### Sub-bucket 2A — Fixed-size, add SYNTH structs (20 packets)

| clif handler | Packet ID | Size | Wire layout |
|---|---|---|---|
| `Restart` | 0x00B2 | 3 | int16 pktType + uint8 type |
| `StatusUp` | 0x00BB | 5 | int16 pktType + int16 statusType + int16 amount (wait — 5 bytes: int16+uint16+uint8?) |
| `SkillUp` | 0x0112 | 4 | int16 pktType + uint16 skillId |
| `RequestMemo` | 0x011D | 2 | int16 pktType only |
| `PartyChangeOption` | 0x0102 | 6 | int16 pktType + uint16 expOption + uint16 itemOption |
| `RemoveOption` | 0x012A | 2 | int16 pktType only |
| `CloseVending` | 0x012E | 2 | int16 pktType only |
| `VendingListReq` | 0x0130 | 6 | int16 pktType + uint32 AID |
| `QuitGame` | 0x018A | 4 | int16 pktType + uint16 type |
| `CatchPet` | 0x019F | 6 | int16 pktType + uint32 targetId |
| `PetMenu` | 0x01A1 | 3 | int16 pktType + uint8 action |
| `SelectEgg` | 0x01A7 | 4 | int16 pktType + uint16 index |
| `SendEmotion` | 0x01A9 | 6 | int16 pktType + uint32 targetId |
| `ChangeCart` | 0x01AF | 4 | int16 pktType + uint16 num |
| `FriendsListAdd` | 0x0202 | 26 | int16 pktType + char name[24] |
| `FriendsListRemove` | 0x0203 | 10 | int16 pktType + uint32 AID + uint32 CID |
| `FriendsListReply` | 0x0208 | 11/14 | int16 pktType + uint32 AID + uint32 CID + uint8 result (11 bytes pre-shuffle, 14 bytes post — verify) |
| `WeaponRefine` | 0x0222 | 6 | int16 pktType + uint32 index |
| `HomMenu` | 0x022D | 5 | int16 pktType + uint32 homId + uint8 type (wait — 5 bytes: int16+uint16+uint8?) |
| `HomAttack` | 0x0233 | 11 | int16 pktType + uint32 targetId + uint32 homId + uint8 action |

### Sub-bucket 2B — Variable-length, permanently manual (4 packets)

These use `clif_process_message` on the server side. Wire format verified from clif.cpp source.

| clif handler | Packet ID | Wire format |
|---|---|---|
| `PartyMessage` | 0x0108 | `uint16 pktType \| uint16 pktLen \| "Name : Message\0"` |
| `GuildMessage` | 0x017E | `uint16 pktType \| uint16 pktLen \| "Name : Message\0"` |
| `BattleChat` | 0x02DB | `uint16 pktType \| uint16 pktLen \| "Name : Message\0"` |
| `WisMessage` | 0x0096 | `uint16 pktType \| uint16 pktLen \| char target[24] \| "Message\0"` |

Note: Party/Guild/BattleChat format is identical to `public_chat` (0x008C/0x00F3).
Whisper differs: fixed 24-byte target name precedes the message body.

## Root cause

The codegen preprocessor pass for C→S packets only reads `packets_struct.hpp`. Bucket 1
structs live in `packets.hpp` (old-era packets before the struct-per-file refactor). This
is the same root cause as the SYNTH_CZ injection work in v0.2.3, but for a different file.

Long-term fix: extend the codegen preprocessor to also read `packets.hpp` for CZ structs.
Near-term fix: add missing structs directly to `synthetic_structs.hpp` (same pattern used
for the 7 SYNTH structs added in v0.2.3).

## Work items

- [x] EPIC document created
- [ ] GCC verify all Bucket 1 struct layouts
- [ ] Add Bucket 1 structs to synthetic_structs.hpp (those missing from packets_struct.hpp)
- [ ] Add Bucket 1 + 2A actions to semantics DB
- [ ] Regenerate pkg/encode/ and pkg/send/
- [ ] Implement Bucket 2B manual encode functions (4 variable-length chat packets)
- [ ] Run full test suite
- [ ] Tag v0.2.5
