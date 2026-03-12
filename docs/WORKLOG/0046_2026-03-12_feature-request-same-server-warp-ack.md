# 0046 — Feature Request: Public API for Same-Server Warp Acknowledgement

**Date**: 2026-03-12  
**Scope**: `pkg/encode/`, `pkg/fsm/` (or new `pkg/handshake/`)  
**Reporter**: goKore integration

---

## Problem

goKore is unable to keep its TCP connection alive after a same-server warp (`0x0091
ZC_NPCACK_MAPMOVE`). The TCP connection closes approximately 4 seconds after the
`0x0091` is received, preventing any further gameplay.

---

## Root Cause (Verified Against rAthena Source)

The Ragnarok Online same-server warp sequence works as follows:

1. rAthena sends `0x0091 ZC_NPCACK_MAPMOVE` (`clif_changemap`, `clif.cpp:2154`)
2. rAthena then **waits** for the client to send `0x007D CZ_NOTIFY_ACTORINIT`
3. Only after receiving `0x007D` does `clif_parse_LoadEndAck` (`clif.cpp:10744`) run and
   send the burst of map-entry packets (inventory list, spawn, etc.)
4. If `0x007D` is never received, the server eventually closes the connection (observed:
   ~4 seconds in testing against rAthena with `PACKETVER=20200401`)

**Critically**: rAthena does **not** re-send `ZC_ACCEPT_ENTER` (`0x02EB` / `0x0A18`)
after a same-server warp. `clif_authok` (which sends `ZC_ACCEPT_ENTER`) is only called
from `pc_reg_authentication` (`pc.cpp:2241`) during initial login, not from `pc_setpos`.

This was confirmed by tracing `pc_setpos` (`pc.cpp:6925`) → `clif_changemap`
(`clif.cpp:2154`), and separately tracing `clif_parse_LoadEndAck` (`clif.cpp:10744`)
as the `0x007D` handler.

---

## What goKore Currently Does (Incorrect)

goKore's `handleMapReEntry` handler in `internal/network/connector.go` is registered on
`0x02EB` and `0x0A18`, expecting rAthena to re-send `ZC_ACCEPT_ENTER` after a warp.
It is never called because rAthena does not send that packet after a same-server warp.

The `0x0091` handler fires `EventMapChanged` but sends nothing back to the server.
The server waits for `0x007D`, times out, and closes the TCP connection.

---

## What Must Be Sent After Receiving `0x0091`

Immediately after receiving `0x0091 ZC_NPCACK_MAPMOVE`, the client must send:

1. **`0x007D CZ_NOTIFY_ACTORINIT`** — 2-byte packet (header only, no fields)  
   Source: `clif.cpp:10742`, `clif_parse_LoadEndAck`

2. **`CZ_REQUEST_TIME`** — 6-byte packet (header + `uint32 clientTick`)  
   - For `PACKETVER < 20080102`: wire ID `0x007E`  
   - For `PACKETVER >= 20080102`: wire ID `0x0360`  
   Source: `clif.cpp:11196–11197`, `fsm.go:buildTickSyncPacket`

Both packets must have C→S obfuscation applied via `MapSession.Encode` before writing
to the socket (no-op for `PACKETVER > 20180307` where obfuscation is discontinued).

---

## What rathena-client Currently Exposes (Insufficient)

### `pkg/encode/request_time.go`

```go
func EncodeRequestTime(req send.RequestTime, packetver uint32) [6]byte {
    var p [6]byte
    p[0] = 0x7e   // hardcoded 0x007E regardless of packetver
    p[1] = 0x00
    leU32Put(p[2:], req.ClientTime)
    _ = packetver  // packetver is ignored
    return p
}
```

This always emits `0x007E`. For `PACKETVER >= 20080102` the server expects `0x0360`
(`CZ_REQUEST_TIME2`). The `packetver` parameter is silently ignored.

### `pkg/fsm/packets.go` (package-private)

`buildMapLoadedPacket()` and `buildTickSyncPacket(packetver)` implement the correct
logic but are unexported. goKore cannot call them.

### No public `EncodeMapLoaded` / `EncodeCzNotifyActorInit` exists

There is no exported encode function for `0x007D CZ_NOTIFY_ACTORINIT` in `pkg/encode/`.

---

## Requested Changes

### Option A — Minimal fix (two changes)

**1. Fix `encode.EncodeRequestTime` to respect `packetver`:**

```go
// pkg/encode/request_time.go
func EncodeRequestTime(req send.RequestTime, packetver uint32) [6]byte {
    var p [6]byte
    id := uint16(0x007E)
    if packetver >= 20080102 {
        id = 0x0360
    }
    p[0] = byte(id)
    p[1] = byte(id >> 8)
    leU32Put(p[2:], req.ClientTime)
    return p
}
```

**2. Add `encode.EncodeMapLoaded` (for `0x007D CZ_NOTIFY_ACTORINIT`):**

```go
// pkg/encode/map_loaded.go (new file, or add to existing)
func EncodeMapLoaded(_ send.MapLoaded, _ uint32) [2]byte {
    return [2]byte{0x7D, 0x00}
}
```

With these two changes, goKore's `0x0091` handler becomes:

```go
ms.RegisterHandler(0x0091, func(data []byte, pv uint32) {
    // ... decode mapName, x, y ...

    // Respond with warp acknowledgement
    notifyPkt := encode.EncodeMapLoaded(send.MapLoaded{}, pv)
    sendPacket(ms, conn, notifyPkt[:])

    timePkt := encode.EncodeRequestTime(send.RequestTime{ClientTime: 0}, pv)
    sendPacket(ms, conn, timePkt[:])

    // ... fire EventMapChanged ...
})
```

### Option B — Higher-level helper (preferred)

Export a single function that encapsulates both sends and handles all packetver
selection internally. goKore stays completely ignorant of wire packet IDs:

```go
// pkg/fsm or pkg/handshake
// SendSameServerWarpAck sends 0x007D + 0x007E/0x0360 in response to 0x0091.
// Must be called immediately after receiving ZC_NPCACK_MAPMOVE on an existing
// map connection (same-server warp). Applies C→S obfuscation via ms.Encode.
func SendSameServerWarpAck(ms *session.MapSession, conn net.Conn, packetver uint32) error
```

This mirrors what `ConnectionFSM.runMapPhase` already does internally
(`fsm.go:681–697`) for the initial connection.

---

## Additional Context

- Tested against rAthena with `PACKETVER=20200401`
- Obfuscation is a no-op for this version (`ObfuscationKeysFor(20200401)` returns
  `(0,0,0)`, so `EnableObfuscation` is never called)
- The `0x02EB`/`0x0A18` `handleMapReEntry` handlers in goKore are dead code and will
  be removed once this is fixed — rAthena never sends `ZC_ACCEPT_ENTER` on a
  same-server warp
- `clif_parse_LoadEndAck` (`clif.cpp:10744`) also handles the `rewarp` case
  (`sd->state.rewarp`), which sends another `0x0091` if the landing cell is occupied
  by an NPC. goKore must be prepared to receive a second `0x0091` and respond again.

---

## rAthena Source References

| Claim | File | Line |
|-------|------|------|
| `clif_changemap` sends `0x0091` | `src/map/clif.cpp` | 2154 |
| `clif_authok` (sends `0x02EB`) only called on initial login | `src/map/pc.cpp` | 2241 |
| `clif_parse_LoadEndAck` is the `0x007D` handler | `src/map/clif.cpp` | 10744 |
| `CZ_REQUEST_TIME2` uses `0x0360` for `pv >= 20080102` | `pkg/fsm/packets.go` | 121–123 |
| `EncodeRequestTime` ignores `packetver` | `pkg/encode/request_time.go` | 16 |
| `buildMapLoadedPacket` is unexported | `pkg/fsm/packets.go` | 101 |
