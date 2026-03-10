# EPIC-03: goKore Integration Prerequisites

**Status**: Ready for implementation  
**Created**: 2026-03-09  
**Goal**: Deliver the four missing capabilities that goKore's Epic 38 requires before
integration can begin: two codegen fixes that unblock movement and time-sync decoding,
one new encode function for walk requests, and two encode function fixes for combat.

---

## Context

goKore's Epic 38 (bot movement, combat, and skill use) requires this library to
correctly decode the two movement confirmation packets (0x0087, 0x007F) and correctly
encode the three C→S action packets (walk, attack, skill use). All four stories are
currently blocked:

- `CharacterMoves_0x0087` and `Sync_0x007F` are SKIP stubs because their structs live in
  `src/map/packets.hpp`, which `buildVersionTable` does not read.
- `EncodeMoveTo` does not exist — no encode file was generated for the `move_to` action.
- `EncodeActorAction` always returns `nil` — the encode switch body is empty.
- `EncodeSkillUse` has a duplicate condition making the second case arm unreachable.

None of these conflicts with EPIC-02. US-13 (codegen output quality) touches templates
but not the VersionTable injection mechanism. The field names used in the encode
implementations are all already PascalCase. All four stories are independent.

---

## Story Map

```
US-15  VersionTable injection (0x0087 + 0x007F)  ──────────────────────┐
US-16  EncodeMoveTo (0x035F)                       ──────────────────────┤
US-17  EncodeActorAction (0x085a / packetver-dependent)  ────────────────┤
US-18  EncodeSkillUse fix (0x01DE unreachable arm)  ─────────────────────┘
                                                                          │
                                                                          ▼
                                               goKore Epic 38 integration unblocked
```

All four stories are independent and can proceed in parallel.

**Priority order** (per goKore's assessment):  
US-15 is load-bearing — movement and time-sync are the foundation of bot operation.
US-16 directly depends on US-15 being complete for end-to-end walk testing.
US-17 and US-18 unblock combat and skills; the bot can move and loot without them.

---

## Correction to goKore's Original Request

The original request listed `CZ_REQUEST_ACT2 (0x0437)` as the attack packet for
`EncodeActorAction`. This is **incorrect**. GCC verification at PACKETVER=20200401
shows:

```
packetdb_addpacket(0x0437, 5, clif_parse_WalkToXY, 2, 0);   // walk — move_to
packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0);  // attack
```

`0x0437` is reassigned to `clif_parse_WalkToXY` in the shuffle table for PACKETVER
>= 20120307 and remains so through at least 20200401. The SemanticDB entry for
`move_to → 0x0437` is correct. The correct ActionRequest packet at 20200401 is
`0x085a` (and a second entry `0x088e` also maps `ActionRequest` — see US-17 for
the full packetver analysis). US-17 uses the GCC-verified packet IDs, not the
original request's `0x0437`.

---

## US-15 — VersionTable Injection for 0x0087 and 0x007F

### User Story

**As a** goKore operator running the bot,  
**I want** `CharacterMoves_0x0087` and `Sync_0x007F` to be generated decode functions
instead of SKIP stubs,  
**so that** the bot can track character movement on the map and complete the
walk-confirmation handshake (SEND 0x035F → RECV 0x007F) that every movement command
depends on.

### Problem

`pkg/decode/character_moves.go` and `pkg/decode/sync.go` currently contain only SKIP
comments:

```
// SKIP CharacterMoves_0x0087: struct PACKET_ZC_NOTIFY_PLAYERMOVE not found in VersionTable
// SKIP Sync_0x007F: struct PACKET_ZC_NOTIFY_TIME not found in VersionTable
```

Root cause: `buildVersionTable` reads `src/map/packets_struct.hpp` exclusively. Both
target structs live in `src/map/packets.hpp`. The codegen already has a working
injection mechanism for `src/common/packets.hpp` (`injectCommonPacketStructs` in
`internal/codegen/main.go:190`). The fix is to add an analogous injection function
for `src/map/packets.hpp`.

The target structs, GCC-verified at PACKETVER=20181121:

```c
// src/map/packets.hpp
struct PACKET_ZC_NOTIFY_PLAYERMOVE {
    int16  packetType;      // offset 0, size 2
    uint32 moveStartTime;   // offset 2, size 4
    uint8  moveData[6];     // offset 6, size 6 — use packing.DecodeMoveData
};  // total: 12 bytes, fixed

struct PACKET_ZC_NOTIFY_TIME {
    int16  PacketType;      // offset 0, size 2
    uint32 time;            // offset 2, size 4
};  // total: 6 bytes, fixed
```

Both lengths are already correct in `pkg/session/lengths_map.go` (`t[0x007F] = 6`,
`t[0x0087] = 12`) and verified in `pkg/fsm/live_integration_test.go`. The session
framing is not a blocker — only the decode functions are missing.

### Implementation

**Step 1 — Add `injectMapPacketStructs` to `internal/codegen/main.go`.**

Model it exactly on `injectCommonPacketStructs` (lines 186–274). Key differences:

- Source path: `src/map/packets.hpp` (not `src/common/packets.hpp`)
- Struct list:

```go
var mapStructsToInject = []string{
    "PACKET_ZC_NOTIFY_PLAYERMOVE",  // 0x0087 — character_moves
    "PACKET_ZC_NOTIFY_TIME",        // 0x007F — sync
}
```

Call it immediately after `injectCommonPacketStructs` in `run()`:

```go
if err := injectMapPacketStructs(cfg, vt); err != nil {
    return fmt.Errorf("inject map packet structs: %w", err)
}
log.Printf("  VersionTable now has %d structs (after map injection)", len(vt))
```

**Step 2 — Verify SemanticDB field mappings.**

Query the SemanticDB via MCP for the `character_moves` and `sync` actions:

```
semantics_get_action("character_moves")
semantics_get_action("sync")
```

Confirm the `field_mapping` expressions reference the correct rAthena field names
(`moveStartTime`, `moveData`, `time`, `PacketType`). If any field mapping uses an
incorrect name (wrong case or OpenKore alias), update it via
`semantics_update_field_mapping` before regenerating.

For `moveData [6]byte` in `character_moves`: the SemanticDB mapping must emit
`packing.DecodeMoveData(packet.moveData)` or the raw `[6]byte` — do not emit a
direction comment (see EPIC-02 Bug 13-A for why).

**Step 3 — Regenerate and verify.**

Run `go run ./internal/codegen/main.go`. Confirm both SKIP comments are gone and
the generated functions are present:

```go
// pkg/decode/character_moves.go
func CharacterMoves_0x0087(data []byte, pv uint32) events.CharacterMoves {
    // ... reads moveStartTime, moveData
}

// pkg/decode/sync.go
func Sync_0x007F(data []byte, pv uint32) events.Sync {
    // ... reads time field
}
```

**Step 4 — Add golden tests.**

Add to `pkg/decode/character_moves_test.go` and `pkg/decode/sync_test.go`:

- Construct a 12-byte `CharacterMoves` packet with known `moveStartTime` and
  `moveData`; assert both decode correctly.
- Construct a 6-byte `Sync` packet with a known `time` value; assert it decodes
  correctly.

Follow the golden test pattern established in `pkg/decode/` — construct bytes at
GCC-verified offsets, not from the generated code.

### Acceptance Criteria

- [ ] `injectMapPacketStructs` function added to `internal/codegen/main.go`
- [ ] `mapStructsToInject` slice contains `PACKET_ZC_NOTIFY_PLAYERMOVE` and
  `PACKET_ZC_NOTIFY_TIME`
- [ ] `pkg/decode/character_moves.go` no longer contains `// SKIP`; function
  `CharacterMoves_0x0087` is present and reads `moveStartTime` and `moveData`
- [ ] `pkg/decode/sync.go` no longer contains `// SKIP`; function `Sync_0x007F`
  is present and reads `time`
- [ ] `MoveData` comment in generated `events.CharacterMoves` does NOT say "direction"
  (EPIC-02 Bug 13-A; verify the template fix from US-13 is in place, or apply it here)
- [ ] Golden test for `CharacterMoves_0x0087`: 12-byte input at GCC-verified offsets,
  `moveStartTime` and `moveData` assert correctly
- [ ] Golden test for `Sync_0x007F`: 6-byte input, `time` field asserts correctly
- [ ] `go test ./pkg/decode/` passes
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us15_versiontable_inject.md` written

---

## US-16 — Implement EncodeMoveTo (0x035F)

### User Story

**As a** goKore bot,  
**I want** `pkg/encode.EncodeMoveTo(req, packetver)` to return a correctly formed
5-byte walk request packet,  
**so that** the bot can send walk commands to the map server and await the `0x007F`
confirmation decoded by US-15.

### Problem

`pkg/encode/move_to.go` does not exist. No encode function was generated for the
`move_to` action. The root cause is a codegen limitation: the encode generator cannot
produce field assignments for `[3]byte` fields derived from `dest[3]` in
`SYNTH_CZ_REQUEST_MOVE2`. The struct and SemanticDB entry are both correct; only the
output file is missing.

The semantic entry in `mappings.yaml` (verified):

```yaml
move_to:
  implementations:
    - packet_id: "0x035F"
      struct_name: SYNTH_CZ_REQUEST_MOVE2
      field_mapping:
        Coords: '[3]byte(packet.dest[:])'
```

The synthetic struct in `internal/codegen/stubs/synthetic_structs.hpp` (verified):

```c
struct SYNTH_CZ_REQUEST_MOVE2 {
    int16  PacketType;
    uint8  dest[3];  // Packed position: x(10 bits), y(10 bits), dir(4 bits)
} __attribute__((packed));
// total: 5 bytes
```

The corresponding `send.MoveTo` struct does not exist — it must be created manually
because the current SemanticDB canonical params define `Coords [3]byte`, not
unpacked `X`/`Y` coordinates.

### Implementation

**Step 1 — Create `pkg/send/move_to.go` manually.**

The canonical params (`Coords [3]byte`) are not ergonomic for a bot caller that works
in X/Y coordinates. Create a bot-facing struct:

```go
// pkg/send/move_to.go
// Code generated manually — see docs/BACKLOG/EPIC-03.

package send

// MoveTo is the C→S request struct for the move_to action.
// X and Y are the destination tile coordinates.
// packing.EncodePosDir is called inside EncodeMoveTo to pack them.
type MoveTo struct {
    X uint16
    Y uint16
}
```

This is a deliberate deviation from the SemanticDB canonical params. Document in the
file header that `Coords [3]byte` is the canonical wire representation and `X`/`Y`
are unpacked for caller convenience.

**Step 2 — Create `pkg/encode/move_to.go` manually.**

```go
// pkg/encode/move_to.go
// Code generated manually — see docs/BACKLOG/EPIC-03.

package encode

import (
    "github.com/lenaxia/rathena-client/pkg/packing"
    "github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeMoveTo encodes a walk request for the map server.
// 0x035F CZ_REQUEST_MOVE2: 5 bytes, fixed, all PACKETVER >= 20120307.
// Returns a fixed [5]byte so the caller can stack-allocate without a heap copy.
func EncodeMoveTo(req send.MoveTo, packetver uint32) [5]byte {
    var p [5]byte
    p[0] = 0x5F
    p[1] = 0x03
    coords := packing.EncodePosDir(req.X, req.Y, 0)
    p[2] = coords[0]
    p[3] = coords[1]
    p[4] = coords[2]
    _ = packetver
    return p
}
```

**Return type design note**: `[5]byte` (not `[]byte`) is intentional. This is a
fixed-size, fixed-packet function with a known compile-time size. Returning a fixed
array avoids a heap allocation on the send path. Callers that need a `[]byte` slice
can write `p := enc.EncodeMoveTo(req, pv); conn.Write(p[:])`. If the rest of the
encode API standardizes on `[]byte` (an EPIC-02 US-13 concern), revisit this choice
then — for now, fixed array is strictly better for a hot-path send.

**Step 3 — Verify `packing.EncodePosDir` exists and has the right signature.**

```go
// pkg/packing — expected signature:
func EncodePosDir(x, y uint16, dir uint8) [3]byte
```

Confirm in `pkg/packing/packing.go`. If the function is named differently or has a
different signature, adjust the encode implementation accordingly before proceeding.

**Step 4 — Add a unit test.**

```go
// pkg/encode/move_to_test.go
func TestEncodeMoveTo_KnownCoords(t *testing.T) {
    req := send.MoveTo{X: 100, Y: 200}
    p := EncodeMoveTo(req, 20200401)
    if p[0] != 0x5F || p[1] != 0x03 {
        t.Fatalf("packet ID: got %02X %02X, want 5F 03", p[0], p[1])
    }
    // Round-trip: decode the 3 coord bytes and verify X, Y recovered
    x, y, _ := packing.DecodePosDir(p[2], p[3], p[4])
    if x != 100 || y != 200 {
        t.Fatalf("coords round-trip: got (%d, %d), want (100, 200)", x, y)
    }
}
```

Adjust the round-trip call to match `packing.DecodePosDir`'s actual signature.

### Acceptance Criteria

- [ ] `pkg/send/move_to.go` exists with `MoveTo{X, Y uint16}` struct and header
  comment explaining the deviation from canonical `Coords [3]byte` params
- [ ] `pkg/encode/move_to.go` exists with `EncodeMoveTo(req send.MoveTo, packetver uint32) [5]byte`
- [ ] Packet ID bytes `0x5F 0x03` are correct (little-endian 0x035F)
- [ ] `packing.EncodePosDir` is called — no manual bit packing in the encode function
- [ ] Unit test round-trips X=100, Y=200 through encode → decode and recovers original coords
- [ ] `go test ./pkg/encode/` passes
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us16_encode_move_to.md` written

---

## US-17 — Implement EncodeActorAction (Attack Request)

### User Story

**As a** goKore bot in combat,  
**I want** `pkg/encode.EncodeActorAction(req, packetver)` to return a correctly formed
7-byte attack request packet,  
**so that** the bot can initiate attacks against monsters and players.

### Problem

`pkg/encode/actor_action.go` always returns `nil`:

```go
func EncodeActorAction(req send.ActorAction, packetver uint32) []byte {
    switch {
    }
    return nil
}
```

The original goKore request identified the packet as `CZ_REQUEST_ACT2 (0x0437)`.
**This is wrong.** GCC preprocessing at PACKETVER=20200401 shows:

```
packetdb_addpacket(0x0437, 5, clif_parse_WalkToXY, 2, 0);      // walk — NOT attack
packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0); // attack
packetdb_addpacket(0x088e, 7, clif_parse_ActionRequest, 2, 6, 0); // attack (second entry)
```

`0x0437` is `clif_parse_WalkToXY` at packetver 20200401. The SemanticDB entry
`move_to → 0x0437` is correct. The attack packet at packetver 20200401 is `0x085a`
(the primary shuffle assignment) and `0x088e` (a second assignment — verify against
rAthena's shuffle table to determine which is the canonical send target; `0x085a` is
the first encountered and should be used unless the table indicates otherwise).

The wire format for `clif_parse_ActionRequest` is position-indexed (`parseable_packet`
offsets pos[0]=2 and pos[1]=6 for `0x085a`):

```
byte[0-1]  packet ID (0x5A 0x08 = 0x085a LE)
byte[2-5]  TargetGID   uint32 LE   (pos[0]=2)
byte[6]    Action      uint8        (pos[1]=6)
```

Total: 7 bytes, fixed.

There is no named C struct for this layout in `packets_struct.hpp` — rAthena reads
it via `RFIFOB`/`RFIFOL` with position offsets. There is a `PACKET_CZ_REQUEST_ACT`
entry in the SemanticDB for decode (`0x0089`) but no encode entry for `0x085a` yet.

### Struct orientation note

`send.ActorAction` was generated from the S→C `actor_action` semantic action (server
notifies client of an action). It carries server-notification fields (`SourceID`,
`AttackerMoveTime`, `Damage`, etc.) that are irrelevant for a C→S attack request.
The C→S request only needs `TargetID` and `Type` (action type, e.g. 7 = normal attack).
Both fields exist in `send.ActorAction`; using them is functional for an unblock.

This struct orientation mismatch is tracked as technical debt. A future epic should
introduce a dedicated `send.AttackRequest{TargetID uint32; Type uint8}` struct and
action. For now, `send.ActorAction.TargetID` and `send.ActorAction.Type` are used.

### Implementation

**Step 1 — GCC-verify the packet ID for the target packetver.**

Before writing any code, run:

```bash
gcc -E -DPACKETVER=20200401 \
    -I/path/to/rathena/src \
    /path/to/rathena/src/map/clif_packetdb.hpp \
    | grep clif_parse_ActionRequest
```

Expected output (verified during EPIC-03 planning):

```
packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0);
packetdb_addpacket(0x088e, 7, clif_parse_ActionRequest, 2, 6, 0);
```

Use `0x085a` as the primary packet ID. Both entries have identical field positions
(pos[0]=2, pos[1]=6) and identical size (7 bytes), so the implementation is the same
for both — only the packet ID bytes differ. Implement for `0x085a` first; extend to
`0x088e` if goKore's integration test shows the server uses the second variant.

**Step 2 — Implement `EncodeActorAction`.**

```go
// pkg/encode/actor_action.go  (replace the generated stub entirely)
package encode

import (
    "encoding/binary"

    "github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeActorAction encodes an attack or action request for the map server.
// 0x085a clif_parse_ActionRequest: 7 bytes, fixed, PACKETVER >= 20200401.
// Wire layout: [0-1] packetID, [2-5] TargetID uint32 LE, [6] Action uint8.
//
// Note: send.ActorAction is a server-notification struct reused here for the
// C→S case. Only TargetID and Type are read; all other fields are ignored.
// A dedicated send.AttackRequest struct is tracked as future technical debt.
func EncodeActorAction(req send.ActorAction, packetver uint32) []byte {
    p := make([]byte, 7)
    p[0] = 0x5A
    p[1] = 0x08
    binary.LittleEndian.PutUint32(p[2:], req.TargetID)
    p[6] = req.Type
    _ = packetver
    return p
}
```

**Step 3 — Update the file header.**

The file currently begins `// Code generated by internal/codegen. DO NOT EDIT.`
This is no longer true once the switch body is hand-filled. Replace with:

```go
// Manually implemented — codegen stub filled in per EPIC-03 US-17.
// See docs/BACKLOG/EPIC-03_gokore_integration_prereqs.md
```

**Step 4 — Add a unit test.**

```go
func TestEncodeActorAction_Attack(t *testing.T) {
    req := send.ActorAction{TargetID: 0xDEADBEEF, Type: 7}
    p := EncodeActorAction(req, 20200401)
    if len(p) != 7 {
        t.Fatalf("len: got %d, want 7", len(p))
    }
    if p[0] != 0x5A || p[1] != 0x08 {
        t.Fatalf("packet ID: got %02X %02X, want 5A 08", p[0], p[1])
    }
    targetID := binary.LittleEndian.Uint32(p[2:])
    if targetID != 0xDEADBEEF {
        t.Fatalf("TargetID: got %08X, want DEADBEEF", targetID)
    }
    if p[6] != 7 {
        t.Fatalf("Type: got %d, want 7", p[6])
    }
}
```

### Acceptance Criteria

- [ ] GCC verification run and result documented in worklog (packet ID `0x085a`
  confirmed as `clif_parse_ActionRequest` at PACKETVER=20200401; `0x0437` confirmed
  as `clif_parse_WalkToXY`)
- [ ] `pkg/encode/actor_action.go` returns a 7-byte slice with correct packet ID,
  `TargetID` (LE uint32), and `Type` (uint8) at the verified positions
- [ ] File header updated to remove `DO NOT EDIT` and reference EPIC-03
- [ ] `encoding/binary` import added
- [ ] Unit test `TestEncodeActorAction_Attack` passes
- [ ] `req.Type = 7` (normal attack) produces `p[6] == 7`
- [ ] `go test ./pkg/encode/` passes
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us17_encode_actor_action.md` written

---

## US-18 — Fix EncodeSkillUse Unreachable Branch

### User Story

**As a** goKore bot using skills,  
**I want** `pkg/encode.EncodeSkillUse(req, packetver)` to return a correctly formed
skill use packet instead of an effectively-nil result,  
**so that** the bot can cast skills (damage, buff, AoE) without silently sending
the wrong packet data.

### Problem

`pkg/encode/skill_use.go` has a duplicate condition making the second case arm
unreachable:

```go
func EncodeSkillUse(req send.SkillUse, packetver uint32) []byte {
    switch {
    case packetver >= 20030000: // 0x0114  ← first arm: always matches
        p := make([]byte, 33)
        p[0] = 0x14; p[1] = 0x01
        return p
    case packetver >= 20030000: // 0x01DE  ← unreachable: same condition
        p := make([]byte, 33)
        p[0] = 0xde; p[1] = 0x01
        return p
    }
    return nil
}
```

Because Go's `switch` evaluates cases top to bottom and stops at the first match,
the second arm (`0x01DE`) never executes. Both arms also return 33-byte buffers with
only the packet ID set — all payload fields are zero.

Additionally, the two structs referenced (`0x0114 CZ_USE_SKILL` and `0x01DE
CZ_USE_SKILL2`) differ: `0x0114` is 33 bytes, `0x01DE` is 10 bytes. Returning a
33-byte payload for `0x01DE` is a framing error.

GCC verification at PACKETVER=20200401:

```
packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
packetdb_addpacket(0x089b, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
```

The skill-to-ID packet at 20200401 is `0x0862` (and `0x089b` — same pattern as
`ActionRequest`). Both are 10 bytes. Wire layout for `clif_parse_UseSkillToId`
with pos[0]=2, pos[1]=4, pos[2]=6:

```
byte[0-1]  packet ID (LE)
byte[2-3]  SkillLv     uint16 LE   (pos[0]=2)
byte[4-5]  SkillID     uint16 LE   (pos[1]=4)
byte[6-9]  TargetID    uint32 LE   (pos[2]=6)
```

Total: 10 bytes, fixed.

The original goKore request proposed fixing the bug by collapsing to a single
`0x01DE` case. That is incorrect for PACKETVER=20200401 — `0x01DE` is not the
packet ID in modern shuffle tables. The correct packet ID must be GCC-verified
before implementing.

### Struct orientation note

`send.SkillUse` was generated from the S→C `skill_use` semantic action. It carries
decode-side fields (`SourceID`, `Src_speed`, `Dst_speed`, `Cause`, `Option`,
`Damage`, `Flag`, `Tick`) that are server-notification fields irrelevant for C→S.
The three fields needed for the C→S request (`Lv`, `SkillID`, `TargetID`) do exist
in the struct and are sufficient for this implementation.

This struct orientation mismatch is the same pattern as US-17 and is tracked as
future technical debt (a dedicated `send.SkillRequest` struct in a later epic).

### Implementation

**Step 1 — GCC-verify the packet ID.**

```bash
gcc -E -DPACKETVER=20200401 \
    -I/path/to/rathena/src \
    /path/to/rathena/src/map/clif_packetdb.hpp \
    | grep clif_parse_UseSkillToId
```

Expected (verified during EPIC-03 planning):

```
packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
packetdb_addpacket(0x089b, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
```

Also verify the field offsets using `rAthena/src/map/packets_struct.hpp` — look up
the struct used by `clif_parse_UseSkillToId` and confirm pos[0]=2 (SkillLv),
pos[1]=4 (SkillID), pos[2]=6 (TargetID), total 10 bytes.

**Step 2 — Rewrite `EncodeSkillUse`.**

```go
// pkg/encode/skill_use.go  (replace the generated stub entirely)
package encode

import (
    "encoding/binary"

    "github.com/lenaxia/rathena-client/pkg/send"
)

// EncodeSkillUse encodes a skill-to-actor use request for the map server.
// 0x0862 clif_parse_UseSkillToId: 10 bytes, fixed, PACKETVER >= 20200401.
// Wire layout: [0-1] packetID, [2-3] Lv uint16 LE, [4-5] SkillID uint16 LE,
// [6-9] TargetID uint32 LE.
//
// Note: send.SkillUse is a server-notification struct reused here for the C→S
// case. Only Lv, SkillID, and TargetID are read; all other fields are ignored.
// A dedicated send.SkillRequest struct is tracked as future technical debt.
func EncodeSkillUse(req send.SkillUse, packetver uint32) []byte {
    p := make([]byte, 10)
    p[0] = 0x62
    p[1] = 0x08
    binary.LittleEndian.PutUint16(p[2:], req.Lv)
    binary.LittleEndian.PutUint16(p[4:], req.SkillID)
    binary.LittleEndian.PutUint32(p[6:], req.TargetID)
    _ = packetver
    return p
}
```

**Note on the original request's `0x01DE` proposal**: the original request suggested
using `0x01DE (CZ_USE_SKILL2)` as the single implementation. `0x01DE` is a
non-shuffle fallback entry in earlier PACKETVER tables. At PACKETVER=20200401, the
server expects `0x0862`. If the library must support multiple PACKETVER ranges,
add conditional branches; for the single-target packetver (20200401) a single-case
implementation is correct and simpler.

**Step 3 — Update the file header.**

Same as US-17: remove `DO NOT EDIT` and replace with a manual implementation note.

**Step 4 — Add a unit test.**

```go
func TestEncodeSkillUse_ToActor(t *testing.T) {
    req := send.SkillUse{Lv: 5, SkillID: 114, TargetID: 0xCAFEBABE}
    p := EncodeSkillUse(req, 20200401)
    if len(p) != 10 {
        t.Fatalf("len: got %d, want 10", len(p))
    }
    if p[0] != 0x62 || p[1] != 0x08 {
        t.Fatalf("packet ID: got %02X %02X, want 62 08", p[0], p[1])
    }
    lv := binary.LittleEndian.Uint16(p[2:])
    if lv != 5 {
        t.Fatalf("Lv: got %d, want 5", lv)
    }
    skillID := binary.LittleEndian.Uint16(p[4:])
    if skillID != 114 {
        t.Fatalf("SkillID: got %d, want 114", skillID)
    }
    targetID := binary.LittleEndian.Uint32(p[6:])
    if targetID != 0xCAFEBABE {
        t.Fatalf("TargetID: got %08X, want CAFEBABE", targetID)
    }
}
```

### Acceptance Criteria

- [ ] GCC verification run and result documented in worklog (`0x0862` confirmed as
  `clif_parse_UseSkillToId`, 10 bytes, at PACKETVER=20200401)
- [ ] The duplicate-condition `switch` is removed; there are no unreachable case arms
- [ ] `EncodeSkillUse` returns a 10-byte slice with correct packet ID, `Lv`, `SkillID`,
  and `TargetID` at the verified positions
- [ ] File header updated to remove `DO NOT EDIT` and reference EPIC-03
- [ ] `encoding/binary` import added (was missing from the generated file)
- [ ] Unit test `TestEncodeSkillUse_ToActor` passes
- [ ] No 33-byte buffers remain in the file (the old wrong-size allocation is gone)
- [ ] `go test ./pkg/encode/` passes
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us18_encode_skill_use.md` written

---

## Exit Criteria for EPIC-03

EPIC-03 is complete when all of the following are true:

1. `go build ./...` — clean
2. `go test ./...` — all pass, including the new tests from US-15 through US-18
3. `go test -race ./pkg/...` — no data races
4. `pkg/decode/character_moves.go` — no `// SKIP` comment; `CharacterMoves_0x0087`
   function present and reading `moveStartTime` and `moveData`
5. `pkg/decode/sync.go` — no `// SKIP` comment; `Sync_0x007F` function present
   and reading `time`
6. `pkg/encode/move_to.go` — exists; `EncodeMoveTo` returns `[5]byte` with packet ID
   `0x5F 0x03` and packing-encoded coords
7. `pkg/encode/actor_action.go` — returns 7-byte slice for `0x085a` with correct
   TargetID and Type positions (GCC-verified); does not return `nil`
8. `pkg/encode/skill_use.go` — returns 10-byte slice for `0x0862` with correct Lv,
   SkillID, TargetID positions (GCC-verified); no duplicate condition arms; no
   33-byte buffers
9. All four stories have worklogs written in `docs/WORKLOG/`

---

## What This Epic Does NOT Cover

These are explicitly deferred:

- Dedicated C→S structs for attack and skill use (`send.AttackRequest`,
  `send.SkillRequest`) — the struct orientation debt in US-17 and US-18 is tracked
  but deferred to a post-Epic-38 cleanup epic
- Full packetver range support for `EncodeActorAction` and `EncodeSkillUse` across
  all PACKETVER breakpoints — the single-case implementations cover packetver 20200401
  which is the target server; multi-version support is deferred
- `EncodeMoveTo` for the older `0x0085` format (non-shuffle baseline) — goKore's
  target server uses the shuffle table where `0x035F` is the walk packet
- Any new event or send struct types beyond what is explicitly listed above
- Inventory, storage, NPC dialog, and other game packets — goKore handles those
  incrementally after Epic 38
