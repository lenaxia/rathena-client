# TECH-DEBT-02: Intentionally Skipped CZ Encode Packets

## Status: Deferred — low priority

## Context

Following the v0.2.6 audit, 24 CZ (client-to-server) packets in rAthena's
`clif_packetdb.hpp` were intentionally excluded from the encode pipeline.
This document records the rationale so the decision is not lost.

## Categories

### GM-only commands (11 packets) — will not implement

These require GM/administrator privilege level on the server. A normal
client automation pipeline has no legitimate use for them.

| Packet ID | Handler | Description |
|-----------|---------|-------------|
| `0x00CC` | `clif_parse_GMKick` | Kick player by account ID |
| `0x00CE` | `clif_parse_GMKickAll` | Kick all online players |
| `0x013F` | `clif_parse_GM_Item_Monster` | Spawn item/monster by name (26-byte) |
| `0x0149` | `clif_parse_GMReqNoChat` | Silence/mute a player |
| `0x01BA` | `clif_parse_GMShift` | Warp to player by name |
| `0x01BB` | `clif_parse_GMShift` | Warp to player by name (shuffle slot) |
| `0x01BC` | `clif_parse_GMRecall` | Recall player to self by name |
| `0x01BD` | `clif_parse_GMRecall` | Recall player to self (shuffle slot) |
| `0x01DF` | `clif_parse_GMReqAccountName` | Request account name by AID |
| `0x0842` | `clif_parse_GMRecall2` | Recall player by AID (modern) |
| `0x0843` | `clif_parse_GMRemove2` | Kick player by AID (modern) |
| `0x09CE` | `clif_parse_GM_Item_Monster` | Spawn item/monster by name (102-byte) |

### Obsolete chatroom subsystem (3 packets) — will not implement

The in-world chatroom feature (CZ_CREATE_CHATROOM etc.) is unused by
modern clients and has no value for the goKore use-case.

| Packet ID | Handler | Description |
|-----------|---------|-------------|
| `0x00E0` | `clif_parse_ChangeChatOwner` | Transfer chatroom ownership |
| `0x00E2` | `clif_parse_KickFromChat` | Kick player from chatroom |
| `0x00E3` | `clif_parse_ChatLeave` | Leave chatroom |

### Null handlers (4 packets) — will not implement

Registered in the packetdb with `nullptr` — the server discards them
unconditionally. There is no observable effect from sending these.

| Packet ID | Length | Notes |
|-----------|--------|-------|
| `0x08EF` | 6 | Ragexe placeholder, no server-side processing |
| `0x08F1` | 6 | Ragexe placeholder, no server-side processing |
| `0x08F5` | -1 | Variable-length placeholder, no server-side processing |
| `0x08FB` | 6 | Ragexe placeholder, no server-side processing |

### dull no-op handler (1 packet) — will not implement

`clif_parse_dull` is an explicitly labeled no-op function — the server
reads and drops the packet payload.

| Packet ID | Handler | Description |
|-----------|---------|-------------|
| `0x08DD` | `clif_parse_dull` | BG queue lobby packet — server ignores payload |

### Deferred — possible future value (4 packets)

These have real server-side handling but were deprioritised. Implement
if a goKore use-case requires them.

| Packet ID | Handler | Canonical Name | Notes |
|-----------|---------|----------------|-------|
| `0x014D` | `clif_parse_GuildCheckMaster` | `CZ_REQ_IS_GUILDBOSS` | Request guild-master flag — header-only, 2 bytes |
| `0x014F` | `clif_parse_GuildRequestInfo` | `CZ_REQ_GUILD_MENUINTERFACE` | Request guild info by type — 6 bytes |
| `0x085D` | `clif_parse_PartyBookingRegisterReq` | `CZ_PARTY_BOOKING_REQ_REGISTER` | Party booking register — 18 bytes (`level W + mapid W + jobs[10] B`) |
| `0x09E9` | `clif_parse_dull` | `CZ_CLOSE_MAILBOX` | Close mailbox notification — header-only, 2 bytes; `dull` handler means server ignores it, but client sends it |

## Resolution criteria

Promote a packet from this list to a full implementation when a goKore
feature explicitly requires it. Follow the standard process in
`docs/ADDING_PACKETS.md`.
