# 0078 — Fix: Add ActionZcPartyJoinReq (goKore bug report 0807)

**Date**: 2026-03-23
**Status**: COMPLETE
**Scope**: `pkg/session/actions.go`, `pkg/session/receive_dispatch.go`,
           `pkg/events/zc_party_join_req.go` (generated), `pkg/decode/zc_party_join_req.go` (generated),
           `semantics/mappings.yaml`
**Severity**: BLOCKING — party invite receive functionality unavailable; goKore Story 7
              accept/decline flow could not complete

---

## Problem

`ActionZcPartyJoinReq` was missing from rathena-client. `PACKET_ZC_PARTY_JOIN_REQ`
(both `0x02C6` modern and `0x00FE` legacy) was never dispatched — no event struct,
no decoder, no dispatch table entry, no SemanticAction constant.

goKore could not call `session.RegisterSemanticHandler(ms, session.ActionZcPartyJoinReq, ...)`
to receive party invites.

---

## Verification

### rAthena source (GCC at PACKETVER=20200401)

```
packets_struct.hpp:5082 — struct PACKET_ZC_PARTY_JOIN_REQ {
    int16 PacketType;    // offset 0, size 2
    int   GRID;          // offset 2, size 4 — party/group ID
    char  groupName[24]; // offset 6, size 24 — NUL-padded
}                        // total: 30 bytes
```

```
packets_struct.hpp:5088 — DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ, 0x00fe) // legacy (pv < 20110718)
packets_struct.hpp:5090 — DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ, 0x02c6) // modern (pv >= 20110718)
```

### Bug report correction

Bug report 0807 specified `PartyID uint32` and `GroupName string` for the event struct.
The codegen maps C `int` → `[]byte` (standard pattern, see `ZcDeleteMemberFromGroup.AID`,
`ZcNotifyMemberinfoToGroupm.AID`). The generated field is `GRID []byte`; callers use
`binary.LittleEndian.Uint32(e.GRID[:4])` to extract the party ID. `GroupName string`
is correct — codegen emits `nullTermString(data[6:30])`.

---

## Fix

Added `zc_party_join_req` action to semantics DB via MCP with two implementations:

```
create_action("zc_party_join_req", "Server notifies client of a party invite")
add_implementation("0x02C6", struct=PACKET_ZC_PARTY_JOIN_REQ, packetver_min=20110718)
add_implementation("0x00FE", struct=PACKET_ZC_PARTY_JOIN_REQ, packetver_max=20110717)
```

Codegen generated:

**`pkg/events/zc_party_join_req.go`** (new):
```go
type ZcPartyJoinReq struct {
    GRID      []byte  // rAthena: GRID (4-byte LE int, party/group ID)
    GroupName string  // rAthena: groupName (NUL-padded char[24])
}
```

**`pkg/decode/zc_party_join_req.go`** (new): `ZcPartyJoinReq_0x02C6` and `ZcPartyJoinReq_0x00FE`

**`pkg/session/actions.go`**: `ActionZcPartyJoinReq SemanticAction = 396`

**`pkg/session/receive_dispatch.go`**: dispatch entries for both `0x02C6` and `0x00FE`

---

## goKore Usage

```go
session.RegisterSemanticHandler(ms, session.ActionZcPartyJoinReq,
    func(e events.ZcPartyJoinReq) {
        partyID := binary.LittleEndian.Uint32(e.GRID[:4])
        groupName := e.GroupName
        // fire hook.EventPartyInviteReceived
    })
```

---

## Test Results

```
--- PASS: TestZcPartyJoinReq_0x02C6_Decode
--- PASS: TestZcPartyJoinReq_0x00FE_Decode
--- PASS: TestZcPartyJoinReq_NulPaddedName
--- PASS: TestActionZcPartyJoinReq_Exists

BenchmarkZcPartyJoinReq_0x02C6: 165970576 ops, 7.066 ns/op, 0 B/op, 0 allocs/op ✓
```

`go test ./...` — all packages pass.
