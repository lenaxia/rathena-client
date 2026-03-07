# rAthena Wire Protocol: Non-Trivial Byte Packing — Complete Reference

**Purpose**: Document every place in rAthena where a packet field is NOT a plain integer and
requires bit-level packing/unpacking, plus every case of struct/packet ID reuse across PACKETVER
ranges. Written as a reference for replacing the goKore network stack.

**rAthena source files consulted**:
- `src/map/clif.cpp` — WBUFPOS/RBUFPOS function definitions and all call sites; packet encode/decode logic
- `src/map/packets.hpp` — map-server ↔ client packet struct definitions
- `src/map/packets_struct.hpp` — map-server ↔ client packet struct definitions (older/larger structs)
- `src/common/packets.hpp` — login/char-server ↔ client packet struct definitions
- `src/map/clif_packetdb.hpp` — base packet length/handler registration table
- `src/map/clif_shuffle.hpp` — per-PACKETVER packet ID shuffling table (CZ_ alternate IDs)
- `src/map/clif_obfuscation.hpp` — PACKET_OBFUSCATION key table (ID-only XOR, not payload)
- All other `src/map/*.cpp`, `src/char/*.cpp`, `src/login/*.cpp` searched for WBUFPOS/RBUFPOS usage

**Key finding — scope of non-trivial byte packing**:
WBUFPOS/RBUFPOS are used **only** in `src/map/clif.cpp`. No other `.cpp` file in the entire
rAthena codebase calls these functions. All other inter-server traffic (char↔map, login↔char)
uses plain `WFIFOW`/`WFIFOL` without bit-packing.

---

## 1. The Two Core Packed Formats

All non-trivial position encoding in the entire Ragnarok Online wire protocol reduces to exactly
two functions, both defined as `static inline` in `clif.cpp` at lines 173–249.

---

### Format A: WBUFPOS — 3-byte packed position + direction

**rAthena source** (`clif.cpp:173–178`):

```c
static inline void WBUFPOS(uint8* p, uint16 pos, int16 x, int16 y, unsigned char dir) {
    p += pos;
    p[0] = (uint8)(x >> 2);
    p[1] = (uint8)((x << 6) | ((y >> 4) & 0x3f));
    p[2] = (uint8)((y << 4) | (dir & 0xf));
}
```

**Wire layout** (24 bits total):

```
Byte 0: [x9 x8 x7 x6 x5 x4 x3 x2]
Byte 1: [x1 x0 y9 y8 y7 y6 y5 y4]
Byte 2: [y3 y2 y1 y0 d3 d2 d1 d0]
```

- x: 10-bit coordinate, bits [23:14]
- y: 10-bit coordinate, bits [13:4]
- dir: 4-bit direction, bits [3:0]

**rAthena RBUFPOS read-back** (`clif.cpp:197–211`):

```c
static inline void RBUFPOS(const uint8* p, uint16 pos, int16* x, int16* y, unsigned char* dir) {
    p += pos;
    if (x)   x[0]   = ((p[0] & 0xff) << 2) | (p[1] >> 6);
    if (y)   y[0]   = ((p[1] & 0x3f) << 4) | (p[2] >> 4);
    if (dir) dir[0] = (p[2] & 0x0f);
}
```

**Direction values** (4-bit, `dir & 0xf`):

| Value | Direction |
|-------|-----------|
| 0 | North |
| 1 | NW |
| 2 | West |
| 3 | SW |
| 4 | South |
| 5 | SE |
| 6 | East |
| 7 | NE |
| 8 | North (repeat) |

**Used by**: `WFIFOPOS` (inline wrapper, `clif.cpp:193`).

---

### Format B: WBUFPOS2 — 6-byte packed movement (from→to + sub-cell offsets)

**rAthena source** (`clif.cpp:182–190`):

```c
// client-side: x0 += sx0*0.0625-0.5 and y0 += sy0*0.0625-0.5
static inline void WBUFPOS2(uint8* p, uint16 pos,
    int16 x0, int16 y0, int16 x1, int16 y1, uint8 sx0, uint8 sy0) {
    p += pos;
    p[0] = (uint8)(x0 >> 2);
    p[1] = (uint8)((x0 << 6) | ((y0 >> 4) & 0x3f));
    p[2] = (uint8)((y0 << 4) | ((x1 >> 6) & 0x0f));
    p[3] = (uint8)((x1 << 2) | ((y1 >> 8) & 0x03));
    p[4] = (uint8)y1;
    p[5] = (uint8)((sx0 << 4) | (sy0 & 0x0f));
}
```

**Wire layout** (48 bits total):

```
Byte 0: [x0_9 x0_8 x0_7 x0_6 x0_5 x0_4 x0_3 x0_2]
Byte 1: [x0_1 x0_0 y0_9 y0_8 y0_7 y0_6 y0_5 y0_4]
Byte 2: [y0_3 y0_2 y0_1 y0_0 x1_9 x1_8 x1_7 x1_6]
Byte 3: [x1_5 x1_4 x1_3 x1_2 x1_1 x1_0 y1_9 y1_8]
Byte 4: [y1_7 y1_6 y1_5 y1_4 y1_3 y1_2 y1_1 y1_0]
Byte 5: [sx0_3 sx0_2 sx0_1 sx0_0 sy0_3 sy0_2 sy0_1 sy0_0]
```

- x0, y0: 10-bit FROM coordinates
- x1, y1: 10-bit TO coordinates
- sx0, sy0: 4-bit sub-cell offsets — **NOT direction**
  - Used by the client for sub-cell movement interpolation:
    `x0 += sx0 * 0.0625 - 0.5`, `y0 += sy0 * 0.0625 - 0.5`
  - rAthena passes `ud.sx` and `ud.sy` — the unit's current sub-cell position
  - In `clif_walkok` (player's own movement), rAthena hardcodes `sx0=8, sy0=8` (`clif.cpp:2057`)

**CRITICAL**: There is **no direction** in the 6-byte format. Byte 5 is sub-cell offsets only.
Any goKore code reading `data[5]` as direction is wrong.

**rAthena RBUFPOS2 read-back** (`clif.cpp:214–240`):

```c
static inline void RBUFPOS2(const uint8* p, uint16 pos,
    int16* x0, int16* y0, int16* x1, int16* y1,
    unsigned char* sx0, unsigned char* sy0) {
    p += pos;
    if (x0)  x0[0]  = ((p[0] & 0xff) << 2) | (p[1] >> 6);
    if (y0)  y0[0]  = ((p[1] & 0x3f) << 4) | (p[2] >> 4);
    if (x1)  x1[0]  = ((p[2] & 0x0f) << 6) | (p[3] >> 2);
    if (y1)  y1[0]  = ((p[3] & 0x03) << 8) | (p[4] >> 0);
    if (sx0) sx0[0] = (p[5] & 0xf0) >> 4;   // HIGH nibble = sx0
    if (sy0) sy0[0] = (p[5] & 0x0f) >> 0;   // LOW  nibble = sy0
}
```

Note: `RFIFOPOS2` is defined as a wrapper at `clif.cpp:248` but is **never called** from any
rAthena source file — all RBUFPOS2 decoding is done via direct `RBUFPOS` / `RBUFPOS2` calls.

---

## 2. Every rAthena Call Site for WBUFPOS / WBUFPOS2

These are the exact lines in `clif.cpp` where the packed formats are written into outgoing packets.
This is the complete set — confirmed by exhaustive grep of the entire rAthena source tree.

### WBUFPOS call sites (server → client, 3-byte format)

| clif.cpp line | Function / context | Packet struct field | Semantic |
|---|---|---|---|
| 760 | `clif_authok` | `packet.posDir` | ZC_ACCEPT_ENTER (0x73/0x2eb/0xa18) — map entry, player's own position |
| 1085 | `clif_set_unit_idle` | `p.PosDir[0]` | ZC_NOTIFY_STANDENTRY / packet_idle_unit — idle actor visible to others |
| 1157 | `clif_set_unit_idle` (spawn path) | `p.PosDir[0]` | ZC_NOTIFY_NEWENTRY / packet_spawn_unit — new actor spawned near player |
| 1252 | `clif_set_unit_idle2` | `p.PosDir[0]` | packet_idle_unit2 — older idle actor variant |
| 1305 | `clif_set_unit_idle2` (spawn path) | `p.PosDir[0]` | packet_spawn_unit2 — older spawn variant |
| 2586 | `clif_set_unit_idle` (NPC self) | `p.PosDir[0]` | packet_idle_unit — NPC appears to self |

### WBUFPOS2 call sites (server → client, 6-byte format)

| clif.cpp line | Function / context | Packet struct field | Semantic |
|---|---|---|---|
| 1413 | `clif_set_unit_walking` | `p.MoveData[0]` | packet_unit_walking — actor is currently moving, broadcast to others |
| 2057 | `clif_walkok` | `packet.moveData` | PACKET_ZC_NOTIFY_PLAYERMOVE (0x87) — confirms to player their own walk |

### RBUFPOS call sites (client → server, rAthena parsing the 3-byte dest/pos field)

| clif.cpp line | Function / context | Semantic |
|---|---|---|
| 11396 | `clif_parse_WalkToXY` (CZ_REQUEST_MOVE 0x85, CZ_REQUEST_MOVE2 0x35f) | Client requests movement to a tile. Direction pointer passed as `nullptr` — only x/y are read, direction nibble is discarded |
| 15722 | `clif_parse_HomMoveTo` (CZ_REQUEST_MOVENPC 0x0232) | Homunculus/mercenary move to target tile. Direction pointer passed as `nullptr` — only x/y are read |

---

## 3. Packet Struct Definitions for the Packed Fields

### `struct PACKET_ZC_ACCEPT_ENTER` — map entry confirmation (`packets.hpp:546–574`)

Three PACKETVER variants, all with `uint8 posDir[3]`:

```c
// PACKETVER < 20080102 → opcode 0x73
struct PACKET_ZC_ACCEPT_ENTER {
    int16  packetType;
    uint32 startTime;
    uint8  posDir[3];   // ← WBUFPOS
    uint8  xSize;       // always 5, ignored by client
    uint8  ySize;       // always 5, ignored by client
};

// PACKETVER >= 20080102 (and >= 20160330) → opcode 0x2eb
struct PACKET_ZC_ACCEPT_ENTER {
    int16  packetType;
    uint32 startTime;
    uint8  posDir[3];   // ← WBUFPOS
    uint8  xSize;
    uint8  ySize;
    uint16 font;
};

// PACKETVER >= 20141022 && < 20160330 → opcode 0xa18
struct PACKET_ZC_ACCEPT_ENTER {
    int16  packetType;
    uint32 startTime;
    uint8  posDir[3];   // ← WBUFPOS
    uint8  xSize;
    uint8  ySize;
    uint16 font;
    uint8  sex;
};
```

---

### `struct packet_authok` — older ZC_ACCEPT_ENTER variant (`packets_struct.hpp:509–522`)

```c
struct packet_authok {
    int16  PacketType;
    uint32 startTime;
    uint8  PosDir[3];   // ← WBUFPOS
    uint8  xSize;
    uint8  ySize;
#if PACKETVER >= 20080102
    int16  font;
#endif
#if PACKETVER >= 20141022 && PACKETVER < 20160330
    uint8  sex;
#endif
} __attribute__((packed));
```

---

### `struct packet_idle_unit` — standing/idle actor visible to others (`packets_struct.hpp:832`)

```c
struct packet_idle_unit {
    int16  PacketType;
#if PACKETVER >= 20091103
    int16  PacketLength;
    uint8  objecttype;
#endif
    ...                 // GID, speed, bodyState, healthState, effectState, appearance
    uint8  isPKModeON;
    uint8  sex;
    uint8  PosDir[3];   // ← WBUFPOS — position + facing direction
    uint8  xSize;
    uint8  ySize;
    uint8  state;
    int16  clevel;
    ...                 // font, maxHP/HP, isBoss, body, name (PACKETVER-gated)
} __attribute__((packed));
```

---

### `struct packet_spawn_unit` — new actor spawning near player (`packets_struct.hpp:687`)

```c
struct packet_spawn_unit {
    int16  PacketType;
    ...                 // GID, speed, appearance fields
    uint8  isPKModeON;
    uint8  sex;
    uint8  PosDir[3];   // ← WBUFPOS
    uint8  xSize;
    uint8  ySize;
    int16  clevel;
    ...                 // PACKETVER-gated fields
} __attribute__((packed));
```

---

### `struct packet_idle_unit2` / `packet_spawn_unit2` — older variants (`packets_struct.hpp:619, 656`)

Both structs are guarded by `#if PACKETVER < 20091103`. When `PACKETVER >= 20091103` the struct
body is replaced with `UNAVAILABLE_STRUCT` (rAthena macro that makes the struct compile to an
unusable zero-size type), so these variants are unreachable at modern packet versions. Both
contain `uint8 PosDir[3]` filled with WBUFPOS when active.

---

### `struct packet_unit_walking` — moving actor visible to others (`packets_struct.hpp:758`)

```c
struct packet_unit_walking {
    int16  PacketType;
#if PACKETVER >= 20091103
    int16  PacketLength;
#endif
    ...                 // GID, speed, appearance fields
    uint32 moveStartTime;
    ...                 // more appearance fields
    uint8  isPKModeON;
    uint8  sex;
    uint8  MoveData[6]; // ← WBUFPOS2 — from/to + sx/sy sub-cell offsets
    uint8  xSize;
    uint8  ySize;
    int16  clevel;
    ...                 // PACKETVER-gated fields
} __attribute__((packed));
```

---

### `struct PACKET_ZC_NOTIFY_PLAYERMOVE` — player's own movement confirmation (`packets.hpp:673`)

```c
struct PACKET_ZC_NOTIFY_PLAYERMOVE {
    int16  packetType;     // 0x87
    uint32 moveStartTime;
    uint8  moveData[6];    // ← WBUFPOS2 (sx0=8, sy0=8 hardcoded by rAthena)
} __attribute__((packed));
```

Note: rAthena `clif_walkok` always passes `sx0=8, sy0=8` for this packet, so byte 5 is always
`0x88` for the player's own movement confirmation.

---

### `struct PACKET_ZC_NOTIFY_MOVE` — **commented out / unused** (`packets.hpp:664`)

```c
// Unused packet (alpha?)
/*
struct PACKET_ZC_NOTIFY_MOVE {
    int16  packetType;    // would be 0x86
    uint32 gid;
    uint8  moveData[6];   // ← WBUFPOS2
    uint32 moveStartTime;
};
*/
```

**0x86 is unused in current rAthena.** Movement of other units uses `packet_unit_walking`
(via `clif_set_unit_walking`). The 0x86 struct in goKore's generated code is from an older
OpenKore reference and does not correspond to a struct actively sent by modern rAthena.

---

### `struct PACKET_CZ_REQUEST_MOVENPC` — C→S homunculus/mercenary move (`packets.hpp:1449–1453`)

```c
struct PACKET_CZ_REQUEST_MOVENPC {
    int16  packetType;    // 0x232
    uint32 GID;
    uint8  PosDir[3];     // ← WBUFPOS (direction nibble discarded by rAthena on receive)
} __attribute__((packed));
DEFINE_PACKET_HEADER(CZ_REQUEST_MOVENPC, 0x232)
```

Parsed at `clif.cpp:15722` with `RBUFPOS(p->PosDir, 0, &x, &y, nullptr)` — direction discarded.

---

### CZ_REQUEST_MOVE — client sends 3-byte destination (`clif.cpp:11372`)

```
0085 <dest>.3B   (CZ_REQUEST_MOVE)
035f <dest>.3B   (CZ_REQUEST_MOVE2)
```

No named C struct in packets_struct.hpp for the CZ side — rAthena reads directly via:

```c
RFIFOPOS(fd, packet_db[RFIFOW(fd,0)].pos[0], &x, &y, nullptr);
// nullptr = direction is ignored server-side; only x/y are used
```

The 3-byte dest is encoded by the client using the same WBUFPOS bit layout. The direction nibble
(low 4 bits of byte 2) is present in the wire data but rAthena discards it.

---

## 4. Bit Layout Reference Diagrams

### 3-byte WBUFPOS decode (canonical)

```
Input bytes: p[0] p[1] p[2]

x  = (p[0] << 2) | (p[1] >> 6)          → 10-bit value, range 0–1023
y  = ((p[1] & 0x3F) << 4) | (p[2] >> 4) → 10-bit value, range 0–1023
dir = p[2] & 0x0F                         → 4-bit value, range 0–8 (0–7 used)
```

Bit positions within the 24-bit word:
```
bit: 23 22 21 20 19 18 17 16 | 15 14 13 12 11 10 9  8  | 7  6  5  4  3  2  1  0
     x9 x8 x7 x6 x5 x4 x3 x2   x1 x0 y9 y8 y7 y6 y5 y4   y3 y2 y1 y0 d3 d2 d1 d0
```

### 6-byte WBUFPOS2 decode (canonical)

```
Input bytes: p[0] p[1] p[2] p[3] p[4] p[5]

fromX = (p[0] << 2) | (p[1] >> 6)                    → 10-bit
fromY = ((p[1] & 0x3F) << 4) | (p[2] >> 4)           → 10-bit
toX   = ((p[2] & 0x0F) << 6) | (p[3] >> 2)           → 10-bit
toY   = ((p[3] & 0x03) << 8) | p[4]                  → 10-bit
sx0   = (p[5] & 0xF0) >> 4                            → 4-bit sub-cell offset X
sy0   = (p[5] & 0x0F)                                 → 4-bit sub-cell offset Y

// NO DIRECTION. p[5] is sub-cell offsets, not direction.
```

### WBUFPOS encode (canonical, for client→server move requests)

```
p[0] = x >> 2
p[1] = (x << 6) | ((y >> 4) & 0x3F)
p[2] = (y << 4) | (dir & 0x0F)
```

---

## 5. Which Packets Use Which Format — Complete Mapping

| Packet ID(s) | rAthena name | C struct | Packed field | Format | Direction |
|---|---|---|---|---|---|
| 0x0073 | ZC_ACCEPT_ENTER | PACKET_ZC_ACCEPT_ENTER | posDir[3] | WBUFPOS | S→C |
| 0x02EB | ZC_ACCEPT_ENTER2 | PACKET_ZC_ACCEPT_ENTER | posDir[3] | WBUFPOS | S→C |
| 0x0A18 | ZC_ACCEPT_ENTER3 | PACKET_ZC_ACCEPT_ENTER | posDir[3] | WBUFPOS | S→C |
| 0x0078 | ZC_NOTIFY_STANDENTRY | packet_idle_unit | PosDir[3] | WBUFPOS | S→C |
| 0x01D8 | ZC_NOTIFY_STANDENTRY2 | packet_idle_unit | PosDir[3] | WBUFPOS | S→C |
| 0x09FF | ZC_NOTIFY_STANDENTRY11 | packet_idle_unit | PosDir[3] | WBUFPOS | S→C |
| 0x0079 | ZC_NOTIFY_NEWENTRY | packet_spawn_unit | PosDir[3] | WBUFPOS | S→C |
| 0x01D9 | ZC_NOTIFY_NEWENTRY2 | packet_spawn_unit | PosDir[3] | WBUFPOS | S→C |
| 0x02ED | ZC_NOTIFY_NEWENTRY3 | packet_spawn_unit | PosDir[3] | WBUFPOS | S→C |
| 0x09FE | ZC_NOTIFY_NEWENTRY11 | packet_spawn_unit | PosDir[3] | WBUFPOS | S→C |
| 0x02EC | ZC_NOTIFY_MOVEENTRY4 | packet_idle_unit2 | PosDir[3] | WBUFPOS | S→C |
| 0x007B | ZC_NOTIFY_MOVEENTRY | packet_unit_walking | MoveData[6] | WBUFPOS2 | S→C |
| 0x01DA | ZC_NOTIFY_MOVEENTRY2 | packet_unit_walking | MoveData[6] | WBUFPOS2 | S→C |
| 0x022C | ZC_NOTIFY_MOVEENTRY3 | packet_unit_walking | MoveData[6] | WBUFPOS2 | S→C |
| 0x09DB | ZC_NOTIFY_MOVEENTRY10 | packet_unit_walking | MoveData[6] | WBUFPOS2 | S→C |
| 0x09FD | ZC_NOTIFY_MOVEENTRY11 | packet_unit_walking | MoveData[6] | WBUFPOS2 | S→C |
| 0x0087 | ZC_NOTIFY_PLAYERMOVE | PACKET_ZC_NOTIFY_PLAYERMOVE | moveData[6] | WBUFPOS2 | S→C |
| 0x0085 | CZ_REQUEST_MOVE | (raw buffer) | dest[3] | WBUFPOS | C→S |
| 0x035F | CZ_REQUEST_MOVE2 | (raw buffer) | dest[3] | WBUFPOS | C→S |
| 0x0232 | CZ_REQUEST_MOVENPC | PACKET_CZ_REQUEST_MOVENPC | PosDir[3] | WBUFPOS | C→S |
| 0x0086 | ZC_NOTIFY_MOVE | (UNUSED in modern rAthena) | moveData[6] | WBUFPOS2 | — |

---

## 6. Packet ID Reuse / Same-Name Structs Across PACKETVER

### Pattern A: Same struct name, different packet ID, different struct body (PACKETVER-gated `#if/#elif/#else`)

This is the dominant pattern in both `packets.hpp` and `packets_struct.hpp`. The C preprocessor
picks exactly one definition at compile time. The struct name is an alias — what matters for the
wire is the packet ID selected by `DEFINE_PACKET_HEADER`.

**Representative examples from `packets.hpp`** (S→C, map server → game client):

| Struct name | PACKETVER condition | Packet ID | Key difference |
|---|---|---|---|
| `PACKET_ZC_ACCEPT_ENTER` | `< 20080102` | 0x0073 | No `font` field |
| `PACKET_ZC_ACCEPT_ENTER` | `>= 20080102` (and `>= 20160330`) | 0x02EB | Adds `uint16 font` |
| `PACKET_ZC_ACCEPT_ENTER` | `>= 20141022 && < 20160330` | 0x0A18 | Adds `uint16 font` + `uint8 sex` |
| `PACKET_ZC_NPCACK_SERVERMOVE` | `< 20170315` | 0x0092 | No `domain` field |
| `PACKET_ZC_NPCACK_SERVERMOVE` | `>= 20170315` | 0x0AC7 | Adds `char domain[128]` |
| `PACKET_ZC_NOTIFY_ACT` | `< 20071113` | 0x008A | `damage`/`damage2` are `int16` |
| `PACKET_ZC_NOTIFY_ACT` | `>= 20071113` | 0x02E1 | `damage`/`damage2` promoted to `int32` |
| `PACKET_ZC_NOTIFY_ACT` | `>= 20131223` | 0x08C8 | Adds `int8 isSPDamage` field |
| `PACKET_ZC_RECOVERY` | `< 20141022` | 0x013D | `amount` is `int16` |
| `PACKET_ZC_RECOVERY` | `>= 20141022` | 0x0A27 | `amount` promoted to `int32` |
| `PACKET_ZC_ACK_WHISPER` | `< 20131223` | 0x0098 | No `CID` field |
| `PACKET_ZC_ACK_WHISPER` | `>= 20131223` | 0x09DF | Adds `uint32 CID` |
| `PACKET_ZC_REQ_EXCHANGE_ITEM` | `<= 6` | 0x009A | No `targetId`/`targetLv` |
| `PACKET_ZC_REQ_EXCHANGE_ITEM` | `> 6` | 0x01F4 | Adds `uint32 targetId`, `uint16 targetLv` |
| `PACKET_ZC_ACK_EXCHANGE_ITEM` | `<= 6` | 0x00E7 | No `targetId`/`targetLv` |
| `PACKET_ZC_ACK_EXCHANGE_ITEM` | `> 6` | 0x01F5 | Adds `uint32 targetId`, `uint16 targetLv` |
| `PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK` | `< 20110824` | 0x00AC | `wearLocation` is `uint16`, `flag` is `bool` |
| `PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK` | `>= 20110824` | 0x08D1 | `wearLocation` is `uint16`, `flag` is `uint8` |
| `PACKET_ZC_REQ_TAKEOFF_EQUIP_ACK` | `>= 20130000` | 0x099A | `wearLocation` promoted to `uint32` |
| `PACKET_ZC_DELETEITEM_FROM_MCSTORE` | `< 20141016` | 0x0137 | Only index + amount |
| `PACKET_ZC_DELETEITEM_FROM_MCSTORE` | `>= 20141016` | 0x09E5 | Adds `buyerCID`, `date`, `zeny` |
| `PACKET_ZC_ACK_REQNAME_BYGID` | `< 20180221` | 0x0194 | No `flag` field |
| `PACKET_ZC_ACK_REQNAME_BYGID` | `>= 20180221` | 0x0AF7 | Adds `uint16 flag` |
| `PACKET_ZC_HO_PAR_CHANGE` | `< 20210000` (main) | 0x07DB | `value` is `int32` |
| `PACKET_ZC_HO_PAR_CHANGE` | `>= 20210000` (main) | 0x0BA5 | `value` promoted to `uint64` |
| `PACKET_ZC_EFST_SET_ENTER` | `< 20120618` | 0x08FF | No `duration2` field |
| `PACKET_ZC_EFST_SET_ENTER` | `>= 20120618` | 0x0984 | Adds `uint32 duration2` |
| `PACKET_ZC_UPDATE_CHARSTAT` | `!defined(PACKETVER)` | 0x016D | No `gender`/`hairStyle`/`hairColor` |
| `PACKET_ZC_UPDATE_CHARSTAT` | `defined(PACKETVER)` | 0x01F2 | Adds `gender`, `hairStyle`, `hairColor` |

**C→S examples from `packets.hpp`**:

| Struct name | PACKETVER condition | Packet ID | Key difference |
|---|---|---|---|
| `PACKET_CZ_REQ_WEAR_EQUIP` | `< 20120925` | 0x00A9 | `position` is `uint16` |
| `PACKET_CZ_REQ_WEAR_EQUIP` | `>= 20120925` | 0x0998 | `position` promoted to `uint32` |

**`packets_struct.hpp` example — PACKET_ZC_EQUIPWIN_MICROSCOPE** (6 different IDs):

| PACKETVER condition | Packet ID | Key difference |
|---|---|---|
| `>= 20071211` (older branch) | 0x02D7 | No `robe` field |
| `>= 20101123` | 0x0859 | Adds `int16 robe` |
| `>= 20111207` (main) | 0x0906 | Same as 0x0859 body (note: EQUIPITEM_INFO content may vary) |
| `>= 20121205` (main) | 0x0997 | Same fields |
| `>= 20140820` | 0x0A2D | Same fields |
| `>= 20180801` | 0x0B03 | Adds `int16 body2` |
| `>= 20200916` (main) | 0x0B37 | Same as 0x0B03 body |

---

### Pattern B: Same struct name, **same packet ID**, different struct body (same-ID PACKETVER variation)

This is rarer and more dangerous: the packet ID is stable but the wire layout changes. The client
and server must both be on the same PACKETVER build for the decode to be correct.

| Struct name | Packet ID | PACKETVER condition | Layout change |
|---|---|---|---|
| `PACKET_ZC_FRIENDS_STATE` | **0x0206** | `< 20180221` | `{ AID, CID, offline }` — 9 bytes payload |
| `PACKET_ZC_FRIENDS_STATE` | **0x0206** | `>= 20180221` | `{ AID, CID, offline, name[24] }` — 33 bytes payload |

Note: Same packet ID 0x0206 is used for both versions. The `name` field is only present in the
newer version. A decoder must know the PACKETVER to know whether to expect 9 or 33 bytes after
the 2-byte packet type header.

---

### Pattern C: Same-name PACKETVER-gated structs in `common/packets.hpp` (login/char server)

These affect the login and char server protocols (not map server). All are different packet IDs:

| Struct name | PACKETVER | Packet ID | Context |
|---|---|---|---|
| `PACKET_AC_ACCEPT_LOGIN` | `< 20170315` | 0x0069 | Login accept; char_server sub-struct lacks `domain` |
| `PACKET_AC_ACCEPT_LOGIN` | `>= 20170315` | 0x0AC4 | Adds `char domain[128]` + web auth token |
| `PACKET_AC_REFUSE_LOGIN` | `< 20120000` | 0x006A | `error` is `uint8` |
| `PACKET_AC_REFUSE_LOGIN` | `>= 20120000` | 0x083E | `error` promoted to `uint32` |
| `PACKET_CH_MAKE_CHAR` | `< 20120307` | 0x0067 | Includes base stat bytes (str/agi/vit/int/dex/luk) |
| `PACKET_CH_MAKE_CHAR` | `>= 20120307` | 0x0970 | Stat bytes removed |
| `PACKET_CH_MAKE_CHAR` | `>= 20151001` | 0x0A39 | Adds `uint32 job`, `uint8 sex` |
| `PACKET_CH_DELETE_CHAR` | `< 20040419` | 0x0068 | `key` is 40 bytes |
| `PACKET_CH_DELETE_CHAR` | `>= 20040419` | 0x01FB | `key` expanded to 50 bytes |
| `PACKET_HC_NOTIFY_ZONESVR` | `< 20170315` | 0x0081 | No `domain` field |
| `PACKET_HC_NOTIFY_ZONESVR` | `>= 20170315` | 0x0AC5 | Adds `char domain[128]` |
| `PACKET_HC_ACCEPT_MAKECHAR` | `< 20201007` (main) | 0x006D | Same body |
| `PACKET_HC_ACCEPT_MAKECHAR` | `>= 20201007` (main) | 0x0B6F | Same body, new ID |

**Note on 0x0081 ambiguity**: At `PACKETVER < 20170315`, `HEADER_HC_NOTIFY_ZONESVR = 0x0081`
and `HEADER_SC_NOTIFY_BAN = 0x0081`. These are sent by different servers (char server vs login
server) so they never collide on the same TCP connection. Not a wire-level conflict.

---

### Pattern D: Packet ID reuse for genuinely different semantic purposes (historical)

This is extremely rare and only occurs at very ancient PACKETVER values:

| Packet ID | Name 1 | Name 2 | Conflict condition |
|---|---|---|---|
| 0x009A | `ZC_BROADCAST` | `ZC_REQ_EXCHANGE_ITEM` (old) | At `PACKETVER <= 6`, both defined with this ID. `DEFINE_PACKET_HEADER` is a `const int16` so the second definition silently overrides the first at that version. In practice, modern rAthena builds at `PACKETVER > 6` where `ZC_REQ_EXCHANGE_ITEM` gets 0x01F4 — no collision. |

---

### Pattern E: Two distinct struct names, same packet ID, different struct body (same-direction)

Found only in `packets_struct.hpp`:

| Struct 1 | Struct 2 | Shared ID | Condition |
|---|---|---|---|
| `PACKET_ZC_SAY_DIALOG` | `PACKET_ZC_SAY_DIALOG2` | 0x00B4 | `PACKETVER_MAIN_NUM < 20220504`: both defined as 0x00B4. At `>= 20220504`, `DIALOG2` gets 0x0972. **In practice**: clif.cpp only sends `PACKET_ZC_SAY_DIALOG` (via `HEADER_ZC_SAY_DIALOG`); `DIALOG2` is unused. |
| `PACKET_ZC_WAIT_DIALOG` | `PACKET_ZC_WAIT_DIALOG2` | 0x00B5 | Same pattern. At `< 20220504` both are 0x00B5. clif.cpp only sends `PACKET_ZC_WAIT_DIALOG`. |

---

## 7. Packet Obfuscation (PACKET_OBFUSCATION)

rAthena optionally XORs the 2-byte **packet ID** (not the payload) using a rolling LCG key:

```c
// On receive (clif_parse):
cmd = (cmd ^ ((sd->cryptKey >> 16) & 0x7FFF));
// Key update after each packet:
sd->cryptKey = ((sd->cryptKey * clif_cryptKey[1]) + clif_cryptKey[2]) & 0xFFFFFFFF;
```

- Only the 15-bit packet ID is XORed (`& 0x7FFF` masks the top bit).
- Payload bytes are transmitted as-is; WBUFPOS/WBUFPOS2 encoding is unaffected.
- Per-version keys are listed in `clif_obfuscation.hpp` starting at PACKETVER 20110817.
- Per-version packet ID shuffling (different IDs for same logical packets) is in `clif_shuffle.hpp`.

---

## 8. Sub-Cell Offset Semantics (sx0, sy0 in WBUFPOS2)

rAthena comment in `clif.cpp:181`:
> client-side: `x0 += sx0 * 0.0625 - 0.5` and `y0 += sy0 * 0.0625 - 0.5`

- sx0 and sy0 are 4-bit values (0–15)
- The client uses them to smoothly interpolate the start position of movement
- rAthena typically passes `ud.sx` and `ud.sy` — the unit data's sub-cell position
- For `clif_walkok` (player's own movement), rAthena hardcodes `sx0=8, sy0=8`, so byte 5
  is always `0x88`, which decodes as sx0=8, sy0=8 → both offsets are `8*0.0625-0.5 = 0.0`
  (neutral, no sub-cell shift)
- For `clif_set_unit_walking` (other entities), actual `ud.sx` / `ud.sy` values are used

The new network stack can safely ignore sx0/sy0 for bot purposes (they are cosmetic interpolation
hints), but must not mistake byte 5 for a direction byte.

---

## 9. Known Inconsistencies in goKore Decode Functions

When implementing the new network stack, avoid these bugs present in the current goKore code:

### Bug 1: Three different direction formulas for 6-byte MoveData

Only `movement/handler.go:decodeMoveData` is correct per rAthena:

| File | Function | Direction formula | Correct? |
|---|---|---|---|
| `handlers/movement/handler.go:482` | `decodeMoveData` | always returns 0 | **YES** — 6-byte has no direction |
| `handlers/movement/registration.go:490` | `decodeMoveDataInternal` | `data[5] & 0x0F` (low nibble) | NO — that is sy0 |
| `handlers/movement/handler.go:550` | `decodeMovementString` | `data[5] & 0x0F` (low nibble) | NO — that is sy0 |
| `handlers/actors/handler.go:88` | `decodeMoveData` | `(data[5] & 0xF0) >> 4` (high nibble) | NO — that is sx0 |

The correct read-back per RBUFPOS2 is:
- `sx0 = (p[5] & 0xF0) >> 4`
- `sy0 = (p[5] & 0x0F)`
- direction = **not present**

### Bug 2: `decodeCoords` (movement/handler.go:523) has a uint32 overflow

```go
// DEPRECATED function — DO NOT port
func decodeCoords(coords uint32) (...) {
    data[4] = byte(coords >> 32)  // always 0 — uint32 only has 32 bits
    data[5] = byte(coords >> 40)  // always 0 — uint32 only has 32 bits
}
```

### Bug 3: String-to-[]byte conversion for binary position fields

In older packet variants (pre-20091103), rAthena sets `PosDir` and `MoveData` fields as Go
`string` types in the generated structs. Converting these via `[]byte(someString)` is lossy for
non-UTF8 byte values (bytes > 0x7F get mangled). The new network stack should use `[]byte` or
`[N]byte` for all packed binary fields.

### Bug 4: `packCoordinates` (send/builders/map_builder.go:87) uses OpenKore sentinel

```go
bits |= uint32(0x44) << 24  // hardcodes direction=4 in the output
```

This differs from rAthena's WBUFPOS which takes direction as an explicit parameter. The 0x44 top
byte bleeds a `dir=4` (south) into byte 2's low nibble. The server ignores the direction nibble on
CZ_REQUEST_MOVE (passes `nullptr` to RBUFPOS), so this does not break functionality, but it is
non-standard. A correct encoder should set `p[2] = (y << 4) | (dir & 0x0F)` with an explicit
direction argument.

---

## 10. The Complete goKore Decode Reference (for new network stack)

Correct Go implementations to use verbatim in the replacement stack:

```go
// DecodePosDir decodes a 3-byte WBUFPOS field.
// data must be at least 3 bytes.
func DecodePosDir(data []byte) (x, y uint16, dir uint8) {
    x   = uint16(data[0])<<2 | uint16(data[1]>>6)
    y   = (uint16(data[1]&0x3F) << 4) | uint16(data[2]>>4)
    dir = data[2] & 0x0F
    return
}

// EncodePosDir encodes x, y, dir into a 3-byte WBUFPOS field.
func EncodePosDir(x, y uint16, dir uint8) [3]byte {
    var p [3]byte
    p[0] = byte(x >> 2)
    p[1] = byte(x<<6) | byte((y>>4)&0x3F)
    p[2] = byte(y<<4) | (dir & 0x0F)
    return p
}

// DecodeMoveData decodes a 6-byte WBUFPOS2 field.
// data must be at least 6 bytes.
// Note: there is NO direction in the 6-byte format.
// byte[5] encodes sub-cell offsets sx0 (high nibble) and sy0 (low nibble).
func DecodeMoveData(data []byte) (fromX, fromY, toX, toY uint16, sx0, sy0 uint8) {
    fromX = uint16(data[0])<<2 | uint16(data[1]>>6)
    fromY = (uint16(data[1]&0x3F) << 4) | uint16(data[2]>>4)
    toX   = (uint16(data[2]&0x0F) << 6) | uint16(data[3]>>2)
    toY   = (uint16(data[3]&0x03) << 8) | uint16(data[4])
    sx0   = (data[5] & 0xF0) >> 4
    sy0   = data[5] & 0x0F
    return
}

// EncodeMoveData encodes from/to coordinates and sub-cell offsets into
// a 6-byte WBUFPOS2 field.
func EncodeMoveData(fromX, fromY, toX, toY uint16, sx0, sy0 uint8) [6]byte {
    var p [6]byte
    p[0] = byte(fromX >> 2)
    p[1] = byte(fromX<<6) | byte((fromY>>4)&0x3F)
    p[2] = byte(fromY<<4) | byte((toX>>6)&0x0F)
    p[3] = byte(toX<<2) | byte((toY>>8)&0x03)
    p[4] = byte(toY)
    p[5] = (sx0 << 4) | (sy0 & 0x0F)
    return p
}
```
