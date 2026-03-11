# Adding Packets and PACKETVERs — LLM Implementation Guide

**Audience**: LLMs and developers adding new packet coverage or fixing individual decode/encode gaps.

**Ground rule**: This guide describes three distinct kinds of work. Read all three sections before starting any task; they share infrastructure but have different scope and risk.

| Kind of work | When to do it | Risk |
|---|---|---|
| A. Fix an incomplete decode function | A field in an existing event struct is always zero | Low — surgical change to one function |
| B. Add a new encode function | A new C→S action needs an encode function | Low — surgical change to one function |
| C. Add a new packet entirely | A packet ID has no handler and no event struct yet | High — touches codegen, semantics DB, session lengths, and new files |

---

## Table of Contents

1. [Mandatory prerequisites for every task](#1-mandatory-prerequisites-for-every-task)
2. [Understanding the generated code patterns](#2-understanding-the-generated-code-patterns)
3. [Kind A — Fixing an incomplete decode function](#3-kind-a--fixing-an-incomplete-decode-function)
4. [Kind B — Implementing a stub encode function](#4-kind-b--implementing-a-stub-encode-function)
5. [Kind C — Adding a new packet entirely](#5-kind-c--adding-a-new-packet-entirely)
6. [Adding a new PACKETVER range](#6-adding-a-new-packetver-range)
7. [Known incomplete areas — inventory of gaps](#7-known-incomplete-areas--inventory-of-gaps)
8. [Testing requirements](#8-testing-requirements)
9. [Common mistakes](#9-common-mistakes)
10. [Quick reference: helper functions](#10-quick-reference-helper-functions)

---

## 1. Mandatory prerequisites for every task

Before writing a single line of code for any packet work:

### Step 1 — GCC preprocess the relevant struct

```bash
# For map server packets (packets_struct.hpp — no stubs needed)
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src \
    -I ~/personal/rathena/src/map \
    -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A 40 "struct packet_idle_unit "

# For packets.hpp structs (needs stub)
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/packets_hpp_stub.h \
    ~/personal/rathena/src/map/packets.hpp 2>/dev/null \
    | grep -A 40 "struct PACKET_NAME_HERE "

# For login/char server packets (common/packets.hpp — needs stub)
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/common_hpp_stub.h \
    ~/personal/rathena/src/common/packets.hpp 2>/dev/null \
    | grep -A 40 "struct PACKET_NAME_HERE "
```

**You must do this for every PACKETVER breakpoint that matters.** The struct layout changes between versions; the GCC output is the only trustworthy field list.

The preprocess_check.sh script validates all headers compile cleanly:
```bash
./validation/preprocess_check.sh 20180307
```

### Step 2 — Query the SemanticDB via MCP

```
semantics_get("0x00B0")           ← existing packet
semantics_list_fields("0x00B0")   ← field list for the packet
```

The DB is a starting point, not ground truth. It has 306+ known validation errors. Always verify the DB field list matches the GCC output. If they differ, fix the DB via MCP first (never edit `semantics/mappings.yaml` directly).

### Step 3 — Run validation scripts

```bash
./validation/preprocess_check.sh YYYYMMDD   # headers preprocess clean
./validation/db_validate.sh                  # DB quality checks
```

### Step 4 — Document in work log

Before starting implementation, create a work log entry:
```bash
cd docs/WORKLOG
NEXT=$(printf "%04d" $(($(ls -1 [0-9][0-9][0-9][0-9]_*.md 2>/dev/null | sed 's/_.*//' | sort -n | tail -1) + 1)))
# Create ${NEXT}_YYYY-MM-DD_description.md
```

Paste the GCC preprocess output of the relevant struct into the work log.

---

## 2. Understanding the generated code patterns

### Decode function anatomy

Every generated decode function follows this invariant pattern:

```go
// pkg/decode/actor_exists.go (generated)
func ActorExists_0x09FF(data []byte, packetver uint32) events.ActorExists {
    var e events.ActorExists
    if packetver >= 20181121 {
        e.ID = leU32(data, 9)           // rAthena: GID (offset 9, size 4)
        e.GuildID = leU32(data, 49)     // rAthena: GUID (offset 49, size 4)
        e.PosDir = [3]byte(data[63:66]) // rAthena: PosDir (offset 63, size 3)
        e.Name = nullTermString(data[84:108]) // rAthena: name (offset 84, size 24)
        // e.Shield = zero (field absent/defaulted in this version)
    } else if packetver >= 20150513 {
        // ... different offsets for older struct layout
    }
    return e
}
```

**Key rules:**
- `e` is a stack-allocated value type — never take its address, never return a pointer
- `leU32(data, off)`, `leI16(data, off)`, etc. are the only allowed read helpers (see §10)
- Comments `// rAthena: FIELDNAME (offset N, size M)` cite the C field name and GCC-verified offset
- Comments `// e.Field = zero (field absent/defaulted in this version)` document intentional zero-filling

### Encode function anatomy

Two patterns: fixed-size return (no alloc) and variable-size return (alloc):

```go
// Fixed-size: pkg/encode/npc_contact.go (generated, complete)
func EncodeNpcContact(req send.NpcContact, packetver uint32) [7]byte {
    var p [7]byte
    p[0] = 0x90; p[1] = 0x00       // packet ID 0x0090 little-endian
    leU32Put(p[2:], req.NPCID)     // rAthena: AID
    p[6] = req.Type                // rAthena: type
    _ = packetver
    return p
}

// Variable-size: pkg/encode/send_chat.go (STUB — always returns nil)
func EncodeSendChat(req send.SendChat, packetver uint32) []byte {
    switch {
    }
    return nil
}
```

**Fixed-size functions return `[N]byte`** — no heap allocation. The caller does `pkt[:]` to get a slice for writing.  
**Variable-size functions return `[]byte`** — one allocation per call; acceptable for infrequent sends.

### Session length table anatomy

```go
// pkg/session/lengths_map.go (generated)
func populateMapLengths(pv uint32, t *[65536]int16) {
    t[0x0064] = 55    // CA_LOGIN, fixed 55 bytes
    t[0x006B] = -1    // HC_ACCEPT_ENTER, variable (length in bytes [2:4])
    // ...
    if pv >= 20181002 {
        t[0x09FF] = -1   // PACKET_ZC_NOTIFY_STANDENTRY9, variable
    }
}
```

`-1` = variable-length packet (frame length read from bytes `[2:4]`).  
`0` = unknown (causes `ErrUnknownPacket` when received; the TCP stream desynchronizes).  
Positive = fixed byte count.

If a packet ID is missing from the lengths table and the server sends it, `Feed()` will return `ErrUnknownPacket` and the connection must be closed.

---

## 3. Kind A — Fixing an incomplete decode function

### When this applies

A decode function exists and compiles, but one or more of its fields are always zero because:

1. **Field absent in SemanticDB**: the field exists in the rAthena struct but was not mapped in `semantics/mappings.yaml`
2. **Wrong offset in SemanticDB**: the DB field offset or type was wrong and codegen used the wrong read

As of v0.2.4 there are no known gaps of this type for main kRO packets. This workflow applies when adding a new PACKETVER breakpoint that shifts field offsets, or when a rAthena update introduces new fields.

### Identifying the gap

```bash
# Find any remaining "implement manually" placeholders (should be 0 for main kRO)
grep -rn "implement manually" pkg/decode/ | grep -v "_test.go"

# Verify a specific field is populated
# e.g. check that ActorExists_0x09FF populates e.Name correctly
```

### Workflow

**Step 1 — GCC verify the struct layout**

```bash
# Example: fixing Name field in ActorMoved_0x09DB
g++ -E -P -DPACKETVER=20200401 -DPACKETVER_MAIN_NUM=20200401 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -A 60 "struct packet_unit_walking "
```

This gives you the exact C struct layout. Identify the field, its offset, and size.

**Step 2 — Calculate the Go offset**

The Go decode function uses byte offsets that include the 2-byte packet ID header (field 0). Count:
- Byte 0–1: packet ID (`int16` / `uint16`)
- Byte 2–N: rest of struct

For a C struct field at C offset `X` bytes into the struct body, the Go decode offset is `X + 2` (for the prepended packet ID header) — UNLESS the struct itself starts with `PacketType int16` (which most rAthena structs do, at C offset 0). In that case the struct field offset IS the wire offset.

**The safest approach**: look at adjacent known-correct fields in the same decode function and verify the offsets match the GCC output for the same packetver. Then place your new field at the corresponding offset.

**Step 3 — Identify the correct decode helper**

| rAthena type | Go event field type | Decode helper |
|---|---|---|
| `uint8` / `char` | `uint8` | `data[off]` |
| `int8` | `int8` | `int8(data[off])` |
| `uint16` | `uint16` | `leU16(data, off)` |
| `int16` / `short` | `int16` | `leI16(data, off)` |
| `uint32` | `uint32` | `leU32(data, off)` |
| `int32` | `int32` | `leI32(data, off)` |
| `uint64` | `uint64` | `leU64(data, off)` |
| `int64` | `int64` | `leI64(data, off)` |
| `char name[N]` | `string` | `nullTermString(data[off : off+N])` |
| `uint8 PosDir[3]` | `[3]byte` | `[3]byte(data[off : off+3])` |
| `uint8 MoveData[6]` | `[6]byte` | `[6]byte(data[off : off+6])` |

**Step 4 — Write the fix**

Replace the `// e.Name: complex expression — implement manually` comment with the actual read:

```go
// Before (gap):
// e.Name: complex expression — implement manually
//   (strings.TrimRight(string(packet.name), "\x00"))

// After (correct):
e.Name = nullTermString(data[90:114])  // rAthena: name (offset 90, size 24)
```

The comment format `// rAthena: FIELDNAME (offset N, size M)` is mandatory — it ties the code to the C source.

**Step 5 — Write a golden test**

```go
// In pkg/decode/actor_moved_test.go (create if it doesn't exist)
func TestActorMoved_0x09DB_Name_Decoded(t *testing.T) {
    // Build a synthetic packet matching GCC struct layout at PACKETVER 20200401
    // GCC output: packet_unit_walking at 20200401 — name[24] at offset 90
    data := make([]byte, 120)  // must be >= struct size
    putU16LE(data, 0, 0x09DB)  // packet ID
    putU32LE(data, 9, 1001)    // GID
    copy(data[90:], "Porings\x00") // name[24]

    e := ActorMoved_0x09DB(data, 20200401)
    if e.ID != 1001 {
        t.Errorf("ID = %d, want 1001", e.ID)
    }
    if e.Name != "Porings" {
        t.Errorf("Name = %q, want \"Porings\"", e.Name)
    }
}
```

**Important**: golden test bytes MUST be constructed from the GCC struct layout output, not from intuition. Paste the relevant GCC output block into the test file as a comment:

```go
// GCC struct layout:
// g++ -E -P -DPACKETVER=20200401 ... | grep -A 60 "struct packet_unit_walking "
// packet_unit_walking at 20200401:
//   int16 PacketType       off=0   size=2
//   int16 PacketLength     off=2   size=2
//   ...
//   char name[24]          off=90  size=24
```

**Step 6 — Run all tests and benchmarks**

```bash
go build ./...
go test ./pkg/decode/...
go test -bench=. -benchmem ./pkg/decode/...  # must still be 0 allocs/op
```

**Step 7 — Remove the old gap test (if the gap test asserted broken behavior)**

If `pkg/decode/gaps_test.go` has a test asserting the old broken zero-value behavior for this field, update or remove it. Gap tests document known broken behavior; once the behavior is fixed they become misleading.

---

## 4. Kind B — Implementing a stub encode function

As of v0.2.4 there are **no remaining stub encode functions** for gameplay packets. All 126 generated encode functions produce correct payloads.

The only encode functions not generated are FSM-owned auth packets — `game_login`, `map_loaded`, `time_sync_response`, `enter_world` — which are built directly in `pkg/fsm/packets.go` because they are 2-byte header-only packets with no rAthena struct definition. These should not be reimplemented as generated encode functions.

If you are adding a new C→S packet, follow the workflow below.

### Workflow for a new encode function

**Step 1 — Find the rAthena struct or clif_packetdb entry**

```bash
# Check packets_struct.hpp first
grep -n "PACKET_CZ_MY_ACTION" ~/personal/rathena/src/map/packets_struct.hpp

# If not there, check clif_packetdb.hpp for raw parseable_packet entries
grep -n "clif_parse_MyAction" ~/personal/rathena/src/map/clif_packetdb.hpp
```

If the packet only appears as a `parseable_packet(0xNNNN, LENGTH, clif_parse_X, pos0, pos1, ...)` entry with no struct, add a `SYNTH_CZ_*` struct to `internal/codegen/stubs/synthetic_structs.hpp` (see §5 for the pattern).

**Step 2 — Check the shuffle table**

C→S packets use shuffled IDs at some PACKETVER ranges. The shuffle is already handled by the codegen when the semantics DB implementation has a packetver range — the generated function emits the shuffled wire ID. For manual implementations, use:

```go
wireID := session.ShuffledCtoSID(packetver, baseID)
```

**Step 3 — Determine return type**

- Fixed packet size → `[N]byte` — no heap allocation, preferred
- Variable packet size (e.g., chat with user-typed text) → `[]byte` — one allocation per call

**Step 4 — Implement and test**

```go
func EncodeMyAction(req send.MyAction, packetver uint32) []byte {
    // Example: variable-length chat packet
    // 0x008C CZ_REQUEST_CHAT: int16 PacketType + int16 PacketLength + char msg[] + null
    msgLen := len(req.Message)
    total := 4 + msgLen + 1
    p := make([]byte, total)
    p[0] = 0x8C; p[1] = 0x00
    leU16Put(p[2:], uint16(total))
    copy(p[4:], req.Message)
    _ = packetver
    return p
}
```

---

## 5. Kind C — Adding a new packet entirely

This is the most involved operation. A new packet requires changes in: SemanticDB (via MCP), event struct, decode function, session lengths table, and optionally encode function and send struct.

**Do NOT hand-edit the generated files for new packets.** The correct path is:

1. Add the packet mapping to the SemanticDB via the `gokore-semantics` MCP server
2. Regenerate with `go run ./internal/codegen/main.go`
3. Hand-fix any generated code issues (stubs, complex expressions)

However, when the codegen pipeline cannot be run (e.g., no rAthena checkout), a packet can be added manually. This section covers both paths.

### Path 1 — Via codegen (preferred)

**Step 1 — Add the packet to SemanticDB via MCP**

```
# Create the packet mapping
semantics_add(
    packet_id="0x0NEW",
    direction="receive",
    rathena_struct="PACKET_ZC_EXAMPLE",
    openkore_name="example_action",
    category="status_update",
    description="Example packet description"
)

# Add fields (from GCC preprocessor output, position 0 = PacketType)
semantics_add_field(packet_id="0x0NEW", position=0,
    rathena_name="PacketType", rathena_type="int16",
    openkore_name="PacketType", semantic="Packet ID header",
    omit_from_openkore=True)

semantics_add_field(packet_id="0x0NEW", position=1,
    rathena_name="someField", rathena_type="uint32",
    openkore_name="someField", semantic="Description of field")
```

**Step 2 — Create the semantic action (if new)**

If this packet represents a completely new semantic action that doesn't exist yet:

```
semantics_create_action(
    action_name="example_action",
    description="What this action represents",
    openkore_name="example_action"
)

semantics_add_canonical_param(action_name="example_action",
    name="SomeField", type="uint32", semantic="Description")

semantics_add_implementation(action_name="example_action",
    packet_id="0x0NEW",
    struct_name="PACKET_ZC_EXAMPLE",
    field_mapping={"SomeField": "packet.someField"}
)
```

**Step 3 — Regenerate**

```bash
go run ./internal/codegen/main.go \
    --rathena ~/personal/rathena \
    --semantics semantics/mappings.yaml \
    --out .
```

This regenerates:
- `pkg/events/example_action.go` — new event struct
- `pkg/decode/example_action.go` — new decode function(s)
- `pkg/session/lengths_map.go` (or login/char) — new length entry
- Optionally `pkg/send/` and `pkg/encode/` if the packet is C→S

**Step 4 — Fix any codegen output issues**

Check for "complex expression" stubs and fix them following §3.

---

### Path 2 — Fully manual (when codegen cannot run)

Use this when you cannot run the codegen pipeline but need to add a packet.

**Step 1 — GCC verify the struct, update SemanticDB via MCP**

Same as §1. Do not skip this.

**Step 2 — Add the event struct (if new)**

Create `pkg/events/my_action.go`:

```go
// Code generated by internal/codegen. DO NOT EDIT.
// (or: Hand-written — DO NOT regenerate without reviewing changes.)

package events

// MyAction is the event emitted when a my_action packet is received.
// Description of what this packet represents.
type MyAction struct {
    ID     uint32  // Actor ID (rAthena: GID)
    Status uint16  // Status code (rAthena: status)
    // ... other fields
}
```

Rules:
- All fields must be Go primitive types or `[N]byte` fixed arrays — no pointers, no slices
- Field names use the SemanticDB canonical name (not the rAthena C field name)
- Every field has a comment citing the rAthena field name

**Step 3 — Add the decode function**

Create `pkg/decode/my_action.go`:

```go
// Code generated by internal/codegen. DO NOT EDIT.
// (or: Hand-written — packet 0x0NEW added manually, regenerate when codegen is available.)

package decode

import "github.com/lenaxia/rathena-client/pkg/events"

// MyAction_0x0NEW decodes a 0x0NEW packet (struct PACKET_ZC_EXAMPLE).
func MyAction_0x0NEW(data []byte, packetver uint32) events.MyAction {
    var e events.MyAction
    // GCC verified at PACKETVER 20180307:
    // g++ -E -P -DPACKETVER=20180307 ... packets_struct.hpp | grep -A 20 "struct PACKET_ZC_EXAMPLE "
    // struct PACKET_ZC_EXAMPLE { int16 PacketType; uint32 GID; uint16 status; } = 8 bytes
    e.ID = leU32(data, 2)      // rAthena: GID (offset 2, size 4)
    e.Status = leU16(data, 6)  // rAthena: status (offset 6, size 2)
    return e
}
```

**Step 4 — Add the session length entry**

In `pkg/session/lengths_map.go` (or `lengths_login.go`, `lengths_char.go`), add the packet length. This file is generated and marked DO NOT EDIT, but a manual addition is necessary when not regenerating:

```go
// In populateMapLengths():
t[0x0NEW] = 8  // PACKET_ZC_EXAMPLE: fixed 8 bytes

// Or for variable-length:
t[0x0NEW] = -1  // PACKET_ZC_EXAMPLE: variable (length in bytes [2:4])

// Or with a PACKETVER condition:
if pv >= 20180307 {
    t[0x0NEW] = 10  // PACKET_ZC_EXAMPLE2: 10 bytes from 20180307
}
```

**CRITICAL**: If you skip this step, the server sending this packet will cause `Feed()` to return `ErrUnknownPacket` and close the connection.

**Step 5 — Register in tests**

Add a golden test in `pkg/decode/my_action_test.go` (see §8 for the full pattern).

**Step 6 — Build and test**

```bash
go build ./...
go test ./...
go test -bench=. -benchmem ./pkg/decode/...  # 0 allocs/op
```

---

## 6. Adding a new PACKETVER range

rAthena adds packet variants as new struct breakpoints. When a new PACKETVER introduces a different struct layout for an existing packet:

**Step 1 — Identify the new breakpoint**

```bash
# Run the preprocessor at both the old and new PACKETVER
g++ -E -P -DPACKETVER=20200401 ... packets_struct.hpp | grep -A 60 "struct packet_idle_unit " > /tmp/old.h
g++ -E -P -DPACKETVER=20210101 ... packets_struct.hpp | grep -A 60 "struct packet_idle_unit " > /tmp/new.h
diff /tmp/old.h /tmp/new.h
```

The diff shows exactly which fields changed, were added, or moved.

**Step 2 — Update the decode function**

Add a new `if packetver >= YYYYMMDD` branch **above** the existing branches in the correct position (highest version first):

```go
func ActorExists_0x09FF(data []byte, packetver uint32) events.ActorExists {
    var e events.ActorExists
    if packetver >= 20210101 {  // ← NEW: add above existing branches
        // New field at new offset
        e.ID = leU32(data, 11)      // rAthena: GID (offset 11, size 4) — shifted by new field
        e.NewField = leU16(data, 9) // rAthena: newField (offset 9, size 2) — NEW
        // ... other fields at new offsets
    } else if packetver >= 20181121 {  // ← existing
        e.ID = leU32(data, 9)   // rAthena: GID (offset 9, size 4)
        // ...
    }
    // ...
}
```

**CRITICAL ordering rule**: branches must be ordered highest-to-lowest `packetver`. Go evaluates `if/else if` top-to-bottom; an older condition placed first will incorrectly match newer packetvers.

**Step 3 — Update the lengths table**

If the new PACKETVER changes the packet's length, add a conditional update:

```go
// In populateMapLengths():
if pv >= 20210101 {
    t[0x09FF] = -1  // was fixed in older version, now variable
}
```

**Step 4 — Update the SemanticDB via MCP**

```
semantics_add_type_variant(
    packet_id="0x09FF",
    field_name="someField",
    condition="PACKETVER >= 20210101",
    type="uint32",
    size=4,
    openkore_ids="0x09FF"
)
```

**Step 5 — Add a golden test for the new PACKETVER**

```go
func TestActorExists_0x09FF_Golden_20210101(t *testing.T) {
    // GCC struct layout at PACKETVER=20210101:
    // ... paste GCC output here ...
    data := make([]byte, 120)
    putU16LE(data, 0, 0x09FF)
    putU32LE(data, 11, 1001)  // GID at new offset 11
    putU16LE(data, 9, 42)     // newField at offset 9

    e := ActorExists_0x09FF(data, 20210101)
    if e.ID != 1001 { t.Errorf("ID = %d, want 1001", e.ID) }
    if e.NewField != 42 { t.Errorf("NewField = %d, want 42", e.NewField) }
}
```

---

## 7. Known incomplete areas — inventory of gaps

As of v0.2.4, the library has **full coverage** for the main kRO client. The gaps below are the only known remaining items.

### 7.1 Decode function gaps — Zero-client SKIP stubs (3 files)

Three packets exist only in the Ragnarok Zero client (`PACKETVER_ZERO_NUM >= 20210721`). The codegen does not define `PACKETVER_ZERO_NUM`, so their structs are invisible to the VersionTable and these files are empty SKIP stubs:

| File | Packet | Reason |
|---|---|---|
| `pkg/decode/quest_dialog.go` | 0x0BA6 | `PACKET_ZC_QUEST_DIALOG` — Zero-only |
| `pkg/decode/quest_dialog_list.go` | 0x0BA7 | `PACKET_ZC_QUEST_DIALOG_MENU_LIST` — Zero-only |
| `pkg/decode/zc_monolog_dialog.go` | 0x0BA9 | `PACKET_ZC_MONOLOG_DIALOG` — Zero-only |

These are irrelevant for main or RE kRO. Resolution plan in `docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`.

### 7.2 RE-client skill packet variants (3 packets, no decode)

For `PACKETVER_RE_NUM >= 20190807`, the RE client uses different packet IDs and a different `SKILLDATA` layout for three skill packets:

| RE Packet ID | Main Packet ID | Action |
|---|---|---|
| `0x0B31` | `0x0111` | `ZC_ADD_SKILL` |
| `0x0B32` | `0x010F` | `ZC_SKILLINFO_LIST` |
| `0x0B33` | `0x07E1` | `ZC_SKILLINFO_UPDATE2` |

The library decodes the main-client IDs only. On a RE-flavor server in the affected date range, these three packet IDs would trigger `ErrUnknownPacket`. Resolution plan in `docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`.

### 7.3 Session length table — one known gap

- `0x07FB`: codegen emits length=0 for `pv >= 20191120` but the live server sends it as 25 bytes. Workaround: `SetLength(0x07FB, 25)` before entering map. Documented in worklog 0023.

### 7.4 Homunculus / mercenary

Generated decode stubs exist for homunculus packets but have field-type truncation bugs (`hp`/`maxHp` `uint32→uint16`, `exp`/`expNext` `int64→uint32`). Mercenary packets are absent entirely. Both are out of scope for Phase 7.

### 7.5 Character list parsing

`OnCharList` in `pkg/fsm` delivers raw `CHARACTER_INFO` bytes rather than a parsed slice. The struct layout varies significantly by PACKETVER and a fully robust decoder is not yet implemented. Consumers must parse the raw bytes with their own codec for now.

**Finding any remaining gaps:**

```bash
# Zero-client SKIP stubs
grep -rn "^// SKIP" pkg/decode/

# Any remaining "implement manually" placeholders (should be 0)
grep -rn "implement manually" pkg/decode/ | grep -v "_test.go"

# Zero-length entries in the map length table (potential missing packets)
grep "t\[0x" pkg/session/lengths_map.go | grep "= 0$"
```

---

## 8. Testing requirements

Every packet implementation must have tests at these levels:

### Tier A — Byte-level golden test (mandatory for every decode function)

```go
// Template for pkg/decode/my_action_test.go
package decode

import "testing"

// putU16LE / putU32LE are defined in phase1_golden_test.go (same package)

func TestMyAction_0x0NEW_Golden_20180307(t *testing.T) {
    // GCC struct layout at PACKETVER=20180307:
    // g++ -E -P -DPACKETVER=20180307 ... packets_struct.hpp | grep -A 20 "struct PACKET_ZC_EXAMPLE "
    // struct PACKET_ZC_EXAMPLE {
    //   int16 PacketType;   off=0  size=2
    //   uint32 GID;         off=2  size=4
    //   uint16 status;      off=6  size=2
    // } total=8 bytes

    data := make([]byte, 8)
    putU16LE(data, 0, 0x0NEW)  // PacketType
    putU32LE(data, 2, 12345)   // GID = 12345
    putU16LE(data, 6, 7)       // status = 7

    e := MyAction_0x0NEW(data, 20180307)

    if e.ID != 12345 {
        t.Errorf("ID = %d, want 12345", e.ID)
    }
    if e.Status != 7 {
        t.Errorf("Status = %d, want 7", e.Status)
    }
}
```

**Rules:**
- Test file is in package `decode` (not `decode_test`) — access to unexported decode functions
- Golden bytes MUST be constructed from GCC struct layout output — not from examining existing code
- Paste GCC output into the test as a comment block
- Test every field that the decode function reads
- Add a test for each distinct PACKETVER branch (each `if packetver >= N` arm)

### Tier B — Benchmark test (mandatory for decode functions)

```go
func BenchmarkMyAction_0x0NEW(b *testing.B) {
    data := make([]byte, 8)
    putU16LE(data, 0, 0x0NEW)
    putU32LE(data, 2, 12345)
    putU16LE(data, 6, 7)

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _ = MyAction_0x0NEW(data, 20180307)
    }
}
```

Run with:
```bash
go test -bench=BenchmarkMyAction -benchmem ./pkg/decode/
```

Must show `0 allocs/op`. If it shows allocations, the event struct is escaping to the heap — fix the decode function (likely a pointer is being taken somewhere).

### Tier C — Encode test (for encode functions)

```go
func TestEncodeMyRequest_ContainsFields(t *testing.T) {
    pkt := EncodeMyRequest(send.MyRequest{ID: 9999}, 20180307)
    if len(pkt) != 6 { t.Fatalf("len=%d, want 6", len(pkt)) }
    // verify packet ID
    if pkt[0] != 0x42 || pkt[1] != 0x00 {
        t.Errorf("packet ID wrong: %02x %02x", pkt[0], pkt[1])
    }
    // verify field
    got := leU32(pkt, 2)
    if got != 9999 { t.Errorf("ID field = %d, want 9999", got) }
}
```

### Full repository validation (after every task)

```bash
go build ./...
go test ./...
go test -race ./...
go test -bench=. -benchmem ./pkg/...     # 0 allocs/op required
grep -r "^\s*go " pkg/                   # must be empty
./validation/preprocess_check.sh 20180307
```

---

## 9. Common mistakes

### Mistake 1 — Wrong PACKETVER branch order

```go
// WRONG: older condition is checked first
if packetver >= 20150101 {  // ← this matches 20181121 too!
    e.ID = leU32(data, 5)   // wrong offset for 20181121
} else if packetver >= 20181121 {  // ← never reached for modern packetver
    e.ID = leU32(data, 9)   // correct offset for 20181121
}

// CORRECT: highest PACKETVER first
if packetver >= 20181121 {
    e.ID = leU32(data, 9)
} else if packetver >= 20150101 {
    e.ID = leU32(data, 5)
}
```

### Mistake 2 — Using the C struct offset directly as the Go byte offset

The rAthena C struct field offset (from `offsetof(struct, field)`) IS the wire byte offset — because rAthena uses `#pragma pack` or equivalent and the packet ID is the first field of every struct. There is no padding to worry about.

**However**: `offsetof` gives you the offset from the start of the C struct. The first field `PacketType` (int16) is at C offset 0, which is wire offset 0. So C offsets = wire offsets = Go decode offsets. This is consistent.

```go
// C: struct packet_example { int16 PacketType; uint32 AID; };
// GCC: PacketType at offset 0, AID at offset 2
e.ID = leU32(data, 2)  // correct: GCC offset 2 = wire offset 2 = Go offset 2
```

### Mistake 3 — Treating MoveData as PosDir

Walking-unit packets have a 6-byte `MoveData[6]` at the position field, not 3-byte `PosDir[3]`. The bytes encode from/to coordinates. Byte 5 is `sx0`/`sy0`, NOT a direction.

```go
// WRONG — misidentifies MoveData as PosDir
e.PosDir = [3]byte(data[off:off+3])
x, y, dir := packing.DecodePosDir(e.PosDir[:])

// CORRECT — use MoveData and DecodeMoveData
e.MoveData = [6]byte(data[off:off+6])
fromX, fromY, toX, toY, _, _ := packing.DecodeMoveData(e.MoveData[:])
```

### Mistake 4 — Allocating inside a decode function

```go
// WRONG — slice literal escapes to heap
func MyAction_0x0NEW(data []byte, packetver uint32) events.MyAction {
    var e events.MyAction
    e.Items = make([]uint32, 5)  // HEAP ALLOCATION — violates 0 allocs contract
    return e
}

// CORRECT — event struct must use only fixed-size fields
// If the packet has a variable-length array, either:
// (a) use a fixed [MAX]type array in the event struct, or
// (b) use a []byte raw field and let the caller parse it
```

### Mistake 5 — Skipping the GCC verification step

The SemanticDB has 306+ known validation errors. Do not trust the DB field offsets without verifying against GCC output. A wrong offset silently reads garbage into the event struct field.

### Mistake 6 — Editing semantics/mappings.yaml directly

Always use the `gokore-semantics` MCP server tools. Direct edits break the MCP server's understanding of the file and may corrupt the YAML format.

```bash
# WRONG
vim semantics/mappings.yaml

# CORRECT — use MCP tools
# semantics_add(...), semantics_update_field_metadata(...), etc.
```

### Mistake 7 — Forgetting to add the session length entry

After adding a new decode function, the packet ID must also appear in the correct lengths table (`populateMapLengths`, `populateCharLengths`, or `populateLoginLengths`) with the correct frame length. Without it, `Feed()` returns `ErrUnknownPacket` when the server sends the packet.

### Mistake 8 — Not citing the rAthena field name in comments

Every field read in a decode function must have a comment citing the C field name and GCC-verified offset:
```go
e.ID = leU32(data, 9)  // rAthena: GID (offset 9, size 4)
```
This is not optional. It is the only machine-verifiable link between Go code and the C source.

---

## 10. Quick reference: helper functions

### pkg/decode/helpers.go

| Function | Signature | Use for |
|---|---|---|
| `leU16` | `func leU16(data []byte, off int) uint16` | `uint16` field |
| `leI16` | `func leI16(data []byte, off int) int16` | `int16` / `short` field |
| `leU32` | `func leU32(data []byte, off int) uint32` | `uint32` field |
| `leI32` | `func leI32(data []byte, off int) int32` | `int32` field |
| `leU64` | `func leU64(data []byte, off int) uint64` | `uint64` field |
| `leI64` | `func leI64(data []byte, off int) int64` | `int64` field |
| `nullTermString` | `func nullTermString(b []byte) string` | `char name[N]` — null-terminated fixed string. **Zero-alloc via unsafe**. Slice must remain valid (it does — it points into the session's receive buffer). |

For single-byte fields: `data[off]` (for `uint8`) or `int8(data[off])` (for `int8`).  
For `[3]byte` PosDir: `[3]byte(data[off : off+3])`.  
For `[6]byte` MoveData: `[6]byte(data[off : off+6])`.

### pkg/encode/helpers.go

| Function | Signature | Use for |
|---|---|---|
| `leU16Put` | `func leU16Put(b []byte, v uint16)` | Write `uint16` to `b[0:2]` |
| `leU32Put` | `func leU32Put(b []byte, v uint32)` | Write `uint32` to `b[0:4]` |
| `leU64Put` | `func leU64Put(b []byte, v uint64)` | Write `uint64` to `b[0:8]` |

For `int16` / `int32`: cast to unsigned first: `leU16Put(b, uint16(v))`.  
For `uint8` / `int8`: `b[off] = v` / `b[off] = uint8(v)`.  
For fixed strings: `copy(b[off:off+N], req.Field)` — note that `make` zero-fills the null terminator automatically.

### pkg/packing (position helpers)

| Function | Signature | Use for |
|---|---|---|
| `DecodePosDir` | `func DecodePosDir(data []byte) (x, y uint16, dir uint8)` | Decode `[3]byte` PosDir to coordinates |
| `EncodePosDir` | `func EncodePosDir(x, y uint16, dir uint8) [3]byte` | Encode coordinates to `[3]byte` PosDir |
| `DecodeMoveData` | `func DecodeMoveData(data []byte) (fromX, fromY, toX, toY uint16, sx0, sy0 uint8)` | Decode `[6]byte` MoveData to from/to coordinates |
| `EncodeMoveData` | `func EncodeMoveData(fromX, fromY, toX, toY uint16, sx0, sy0 uint8) [6]byte` | Encode from/to coordinates to `[6]byte` MoveData |

### Direction values (from `src/map/path.hpp`)

| Value | Direction |
|---|---|
| 0 | N (north) |
| 1 | NW |
| 2 | W |
| 3 | SW |
| 4 | S |
| 5 | SE |
| 6 | E |
| 7 | NE |
