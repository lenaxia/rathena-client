# Work Log 0089 — Implement ZC_GROUP_LIST decode (issue #13)

**Date**: 2026-07-10
**Type**: Feature — packet coverage
**Scope**:
  - `semantics/mappings.yaml` (new `zc_group_list` action — added via the
    in-repo `cmd/semantics-tool` MCP/CLI from worklog 0088; first user of
    Rule 9's now-actually-available MCP server)
  - `pkg/session/actions.go` (new constant `ActionZcGroupList = 464`)
  - `pkg/session/receive_dispatch.go` (3 new entries)
  - `pkg/events/zc_group_list.go` (new event struct + member struct)
  - `pkg/decode/zc_group_list.go` (3 new decoders)
  - `pkg/decode/zc_group_list_test.go` (new — golden tests + benchmarks)
  - `pkg/session/zc_group_list_dispatch_test.go` (new — end-to-end dispatch tests)
  - `CHANGELOG.md`
**Severity**: BLOCKING (goKore) — without this packet decoded, the party roster
              is invisible when a bot joins an existing party. Pre-existing
              members never trigger per-member spawn packets
              (`ZC_NOTIFY_MEMBERINFO_TO_GROUPM` 0x00AB / 0x0ABD) — they only
              appear in the roster packet, which was silently dropped.

**Reference**: GitHub issue #13 —
  "ZC_GROUP_LIST (partyinfo, 0x00FB/0x0A44) not decoded — party roster
   invisible when joining existing party"

---

## Problem

`PACKET_ZC_GROUP_LIST` (`partyinfo` in rAthena's enum) is the packet a rAthena
server sends to a client to deliver the complete party roster at the moment
the client joins an existing party (rAthena `src/map/party.cpp:676`,
`party_member_added` → `clif_party_info`). Without decoding it, the joining
client has no way to learn about party members who were already in the party
when the local player joined — those members do not trigger per-member spawn
packets (`ZC_NOTIFY_MEMBERINFO_TO_GROUPM`), they only appear in the roster.

Before this fix, all three wire IDs (`0x00FB`, `0x0A44`, `0x0AE5`) were
registered in `lengths_map.go` so framing worked (the bytes were not
mis-parsed), but no decoder existed. Incoming packets were silently dropped
as "unknown" — no semantic event fired, downstream consumers waiting for party
roster data received nothing. The goKore motivating use case (Epic 56's
world-building cognitive layer) needs accurate party-roster data to track
party composition, compute per-member relationship deltas, and surface party
context to the reactive LLM.

The original issue #13 mentioned only two wire IDs (`0x00FB`, `0x0A44`); this
fix covers all three including the production-target `0x0AE5` (the wire ID at
`pv=20200401` and beyond).

## Pre-Implementation Gate

### GCC verification — wire layouts

rAthena `src/map/packets_struct.hpp:271-283` selects the wire ID by PACKETVER:

```cpp
#if PACKETVER >= 20171207
    partyinfo = 0x0ae5,
#elif PACKETVER_MAIN_NUM >= 20170524 || PACKETVER_RE_NUM >= 20170502 || defined(PACKETVER_ZERO)
    partyinfo = 0x0a44,
#else
    partyinfo = 0x00fb,
#endif
```

The SUB struct (`src/map/packets_struct.hpp:2071-2084`) adds two fields in
stages:

```cpp
struct PACKET_ZC_GROUP_LIST_SUB {
    uint32 AID;
#if PACKETVER >= 20171207
    uint32 GID;
#endif
    char playerName[NAME_LENGTH];        // NAME_LENGTH = 24
    char mapName[MAP_NAME_LENGTH_EXT];   // = 16
    uint8 leader;
    uint8 offline;
#if PACKETVER_MAIN_NUM >= 20170524 || PACKETVER_RE_NUM >= 20170502 || defined(PACKETVER_ZERO)
    int16 class_;
    int16 baseLevel;
#endif
} __attribute__((packed));
```

GCC preprocessor confirmed three SUB sizes:

| pv range | packet ID | SUB size | SUB fields |
|---|---|---|---|
| `< 20170524` (MAIN) | `0x00FB` | 46 | AID + playerName[24] + mapName[16] + leader + offline |
| `20170524 ≤ pv < 20171207` | `0x0A44` | 50 | + class_(2) + baseLevel(2) |
| `≥ 20171207` | `0x0AE5` | 54 | + GID(4) after AID |

The outer `PACKET_ZC_GROUP_LIST` header is constant: `packetType(2) +
packetLen(2) + partyName[24] = 28 bytes` before `members[]`.

### rAthena emit code (leader/offline byte polarity)

The leader/offline bytes are encoded inverted relative to the intuitive Go
bool. rAthena `src/map/clif.cpp:7892-7893`:

```cpp
member.leader  = (m.leader) ? 0 : 1;   // 0 = leader, 1 = normal
member.offline = (m.online) ? 0 : 1;   // 0 = online, 1 = offline
```

The decoder flips both back to the intuitive Go bool (`Leader bool` = true
when byte == 0, `Offline bool` = true when byte != 0).

### Wire trace from issue

At production packetver (`pv=20200401`), the wire ID is `0x0AE5`. The issue
only mentioned `0x00FB` and `0x0A44` because the issue author inspected the
older packetdb registrations; `0x0AE5` is registered at `pv >= 20171207` in
`packets_struct.hpp:276` and is the wire-effective ID for every modern
rAthena build.

## Implementation

### 1. `semantics/mappings.yaml` — added via the new MCP tool

Per Rule 9 (now actually enforceable thanks to PR #14), the new action was
added via the in-repo `semantics-tool` CLI rather than a hand-edit:

```bash
semantics-tool --file semantics/mappings.yaml create-action \
    -description "Server sends the full party roster when a player joins an existing party (ZC_GROUP_LIST / partyinfo)" \
    -openkore party_users \
    zc_group_list

semantics-tool --file semantics/mappings.yaml add-implementation -id 0x00FB -struct PACKET_ZC_GROUP_LIST -max 20170501 zc_group_list
semantics-tool --file semantics/mappings.yaml add-implementation -id 0x0A44 -struct PACKET_ZC_GROUP_LIST -min 20170502 -max 20171206 zc_group_list
semantics-tool --file semantics/mappings.yaml add-implementation -id 0x0AE5 -struct PACKET_ZC_GROUP_LIST -min 20171207 zc_group_list
```

Result: +24 lines, 0 deletions. `semantics-tool validate` reports clean.

### 2. `pkg/session/actions.go` — `ActionZcGroupList = 464`

Appended at the next free ID (464) rather than slotted alphabetically —
inserting in the middle would renumber every subsequent constant and break
consumers. Same pattern as `ActionZcPartyJoinReq` (worklog 0078).
`maxSemanticAction` bumped to `ActionZcGroupList`.

### 3. `pkg/events/zc_group_list.go` — typed event structs

- `ZcGroupList{ PacketLength, PartyName, Members []ZcGroupListMember }`
- `ZcGroupListMember{ AID, GID uint32; Name, MapName string; Leader, Offline bool; Class, BaseLevel int16 }`

Fields absent at older PACKETVERs decode to zero values (documented on each
field's godoc).

### 4. `pkg/decode/zc_group_list.go` — three decoders

One function per packet ID (`ZcGroupList_0x00FB`, `ZcGroupList_0x0A44`,
`ZcGroupList_0x0AE5`). All three share two helpers:
- `zcGroupListMemberSize(pv)` — returns 46 / 50 / 54 by packetver
- `decodeZcGroupListMember(dst, b, pv)` — reads one SUB with packetver-aware
  field offsets and flips the leader/offline byte polarity

Trailing partial members (when wire length isn't a clean multiple of the
per-member size) are dropped rather than read past the buffer.

### 5. `pkg/session/receive_dispatch.go` — three entries

All three packet IDs map to `ActionZcGroupList`:

```go
ActionZcGroupList: {
    {id: 0x00FB, fn: ...ZcGroupList_0x00FB...},
    {id: 0x0A44, fn: ...ZcGroupList_0x0A44...},
    {id: 0x0AE5, fn: ...ZcGroupList_0x0AE5...},
},
```

The dispatcher selects by packet ID on the wire, not by packetver, so any
rAthena build will route to the correct decoder for the wire ID it emits.

## Allocation note (documented exception)

Each decoder calls `make([]ZcGroupListMember, n)` — one heap alloc per packet,
unavoidable for a variable-count roster. This is a documented exception to
the 0-alloc decode hot-path contract, matching the inventory list events
(worklog 0066, `InventoryItemsEquip.Items`, `InventoryItemsStackable.Items`).
The event struct itself does not escape to the heap; only the members slice
allocates. Benchmark numbers:

```
BenchmarkZcGroupList_0x0AE5-8    4340487    347.3 ns/op    288 B/op    1 allocs/op
BenchmarkZcGroupList_0x00FB-8    4520715    293.7 ns/op    288 B/op    1 allocs/op
```

## Validation

| Check | Result |
|---|---|
| `go build ./...` | ✓ pass |
| `go test -race ./...` | ✓ all packages pass |
| Zero goroutines in `pkg/` production code | ✓ |
| `BenchmarkZcGroupList_0x0AE5` | 347 ns/op, 1 allocs/op (documented exception) |
| `BenchmarkZcGroupList_0x00FB` | 294 ns/op, 1 allocs/op (documented exception) |
| `semantics-tool validate` | ✓ clean |
| No new external deps | ✓ (no `go.mod` change) |

### New test coverage

`pkg/decode/zc_group_list_test.go` (golden tests):
- `TestZcGroupList_0x00FB_OldestLayout` — pre-20170524 layout (no GID, no
  class/baseLevel), 2 members, validates leader/offline byte polarity flip
- `TestZcGroupList_0x0A44_MidLayout` — 20170524..20171206 layout, 1 member,
  validates class/baseLevel fields present
- `TestZcGroupList_0x0AE5_NewestLayout` — production layout, 3 members
  (leader / normal-online / normal-offline), validates GID populated
- `TestZcGroupList_EmptyRoster` — zero members (party with no online members
  at the moment of roster send)
- `TestZcGroupList_PacketLengthPropagated` — packetLen field is read
- `TestZcGroupList_TruncatedMemberSlice` — partial trailing member is
  dropped, not read past the buffer
- `BenchmarkZcGroupList_0x0AE5` / `_0x00FB` — hot-path allocation check

`pkg/session/zc_group_list_dispatch_test.go` (end-to-end):
- `TestZcGroupList_Dispatch_HasAllThreeVariants` — receiveDispatch entry
  count and contents
- `TestZcGroupList_0x0AE5_FiresAt_20200401` — exact issue #13 reproduction
  at production packetver: 0x0AE5 frame fires the handler once, leaves
  `UnhandledPackets() == 0`, decodes all fields correctly
- `TestZcGroupList_0x00FB_FiresAt_LegacyPacketver` — legacy wire ID at a
  legacy packetver
- `TestZcGroupList_ActionConstant` — compile-time regression guard for
  `ActionZcGroupList = 464` and its `String()` method

## Sources

- rAthena `src/map/packets_struct.hpp:271-283` — partyinfo enum (packet ID
  selection by PACKETVER)
- rAthena `src/map/packets_struct.hpp:2071-2091` — PACKET_ZC_GROUP_LIST_SUB
  and PACKET_ZC_GROUP_LIST struct definitions
- rAthena `src/map/clif.cpp:7844-7904` — `clif_party_info` (the emitter;
  leader/offline byte polarity confirmed at lines 7892-7893)
- rAthena `src/map/party.cpp:676` — `party_member_added` (the caller that
  triggers the roster send when a player joins an existing party)
- rAthena `src/common/mmo.hpp:154,164` — NAME_LENGTH=24,
  MAP_NAME_LENGTH_EXT=16
- Issue #13 — original bug report and field-spec proposal
