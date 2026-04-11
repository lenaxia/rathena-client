# Work Log 0084 — Fix ZC_ACCEPT_ENTER handler: 0x0A18 era starts at pv >= 20141022, not 20141016

**Date:** 2026-04-11
**Commit:** 3f0b61b
**Release:** v0.6.4
**Type:** Bug fix — `pkg/session/fsm.go`

---

## Problem

Bots connecting to a map server with `packetver ∈ [20141016, 20141021]` timed
out waiting for map entry. The FSM never advanced past the `ZC_ACCEPT_ENTER`
wait state.

---

## Root Cause

`fsm.go` registered the `onMapEnter` handler for packet ID `0x0A18` when:

```go
// WRONG — was:
if f.server.Packetver >= 20141016 && f.server.Packetver < 20160330 {
    registerHandler(0x0A18, onMapEnter)
}
```

The ground truth is `src/map/packets.hpp:545-575`:

```cpp
#if PACKETVER < 20080102
    DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x73)    // 11 bytes
#elif PACKETVER < 20141022 || PACKETVER >= 20160330
    DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0x2eb)   // 13 bytes
#else                                               // >= 20141022 && < 20160330
    DEFINE_PACKET_HEADER(ZC_ACCEPT_ENTER, 0xa18)   // 14 bytes
#endif
```

The `0x0A18` era starts at `>= 20141022`, not `>= 20141016`. For `pv ∈
[20141016, 20141021]`, rAthena sends `0x02EB` (13 bytes) but the FSM had
registered a handler for `0x0A18`. The `0x02EB` packet had no handler → it was
discarded by the framer → `onMapEnter` never fired → the FSM timed out.

The `lengths_map.go` framer was correct for this range (it correctly parses
`0x02EB` at 13 bytes), so no framing error was visible — the packet was received
and silently dropped.

## Affected range

`pv ∈ [20141016, 20141021]` — 6 consecutive calendar days in October 2014.

---

## Fix

### `pkg/session/fsm.go`

The inline `if/else if/else` block was extracted into `zcAcceptEnterID(packetver
uint32) uint16` to make the condition directly testable:

```go
func zcAcceptEnterID(packetver uint32) uint16 {
    switch {
    case packetver >= 20141022 && packetver < 20160330:
        return 0x0A18
    case packetver >= 20080102:
        return 0x02EB
    default:
        return 0x0073
    }
}
```

Call site:
```go
mapSess.core.registerHandler(zcAcceptEnterID(f.server.Packetver), onMapEnter)
```

---

## Tests

### `pkg/session/fsm_map_enter_test.go` (new, `package session`)

`TestZcAcceptEnterID_Boundaries` — 8 cases covering:
- `20080101` → `0x0073` (one before 0x02EB era)
- `20080102` → `0x02EB` (first 0x02EB)
- `20141021` → `0x02EB` (previously registered 0x0A18 — the bug)
- `20141022` → `0x0A18` (exact lower boundary)
- `20150101` → `0x0A18` (mid-range)
- `20160329` → `0x0A18` (one before upper boundary)
- `20160330` → `0x02EB` (0x02EB resumes)
- `20200401` → `0x02EB` (modern MAIN)

`TestZcAcceptEnterID_PreBugRange` — all 6 broken packetvers
(`20141016`–`20141021`) must return `0x02EB`.

---

## Origin

Identified during a systematic audit of version-conditional packet length
changes across `clif_packetdb.hpp` and `clif_shuffle.hpp`. The audit confirmed
that all 37 "Group 2" C→S multi-registration cases in `clif_packetdb.hpp` are
handled correctly by the shuffle system for modern packetvers. The `ZC_ACCEPT_ENTER`
off-by-six-days bug was the only actionable client-side issue found.

---

## Test results

```
go test -race -count=1 ./...
ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session
```
