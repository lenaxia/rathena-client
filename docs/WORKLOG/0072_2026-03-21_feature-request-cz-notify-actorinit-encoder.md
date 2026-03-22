# 0072 — Feature Request: ActionCzNotifyActorinit send encoder (CZ_NOTIFY_ACTORINIT, 0x007D)

**Date**: 2026-03-21  
**Scope**: `pkg/encode/`, `pkg/session/semantic.go` (send dispatch), `semantics/mappings.yaml`  
**Reporter**: goKore integration  
**Priority**: LOW — hygiene/correctness; current raw-write workaround is functionally correct

---

## Problem

goKore's `connector.go` must send `CZ_NOTIFY_ACTORINIT` (0x007D) in two places:

1. **FSM-internal** (already handled): Inside `fsm.go`'s `onMapEnter`, the FSM itself
   sends `0x007D` to trigger `clif_parse_LoadEndAck` and start the inventory burst.
   This is already correct.

2. **Same-server warp ack** (`connector.go:344`): When `ActionMapChanged` fires
   (same-server warp, `0x0091`), goKore must re-send `0x007D` so rAthena unblocks the
   character on the new map. Currently implemented as a raw `conn.Write`:

```go
// connector.go:353
if _, err := bcfg.Conn.Write([]byte{0x7D, 0x00}); err != nil { ... }
```

This is the **only place in goKore** that bypasses `session.Send()` with a hardcoded
raw byte slice. All other outbound packets go through `session.Send()`.

---

## Why raw write is currently used

`send.MapLoaded` was removed in rathena-client v0.5.0 during the Epic 08 semantic API
migration (worklog `0065`). `ActionMapLoaded` exists as a semantic constant (ID 168)
but has no registered send encoder in `pkg/encode/register.go`.

---

## Why this is safe today (but still wrong)

`0x007D` is never shuffled — it appears in `clif_packetdb.hpp:32` with a single
base-table entry and no overrides, and is absent from `clif_shuffle.hpp`. At
`pv >= 20180308`, obfuscation (rolling XOR) is disabled, so the raw two-byte write
`[0x7D, 0x00]` reaches rAthena unmodified. For `pv < 20180308` (obfuscation active),
the raw write would send the wrong bytes — this is a latent bug at older packetvers.

---

## What goKore needs

```go
// connector.go
if err := session.Send(ms, bcfg.Conn, session.ActionCzNotifyActorinit,
    send.CzNotifyActorinit{}); err != nil { ... }
```

This eliminates the only raw `conn.Write` call in goKore's network layer.

---

## Required change

### `pkg/send/types.go` (or new file)

```go
// CzNotifyActorinit is the payload for CZ_NOTIFY_ACTORINIT (0x007D).
// The packet has no fields beyond the 2-byte header — it is a pure signal.
type CzNotifyActorinit struct{}
```

### `pkg/encode/cz_notify_actorinit.go` (new)

```go
// EncodeCzNotifyActorinit encodes a 0x007D (CZ_NOTIFY_ACTORINIT) packet.
// This packet is a fixed 2-byte signal with no fields.
// 0x007D is never shuffled (single base-table entry in clif_packetdb.hpp:32,
// absent from clif_shuffle.hpp) and remains 0x007D at all packetvers.
func EncodeCzNotifyActorinit(_ send.CzNotifyActorinit, _ uint32) [2]byte {
    return [2]byte{0x7D, 0x00}
}
```

### `pkg/encode/register.go`

```go
session.RegisterSendEncoder(session.ActionCzNotifyActorinit,
    func(req interface{}, pv uint32) ([]byte, error) {
        r, ok := req.(send.CzNotifyActorinit)
        if !ok { return nil, session.ErrWrongSendType{Action: session.ActionCzNotifyActorinit} }
        b := EncodeCzNotifyActorinit(r, pv)
        return b[:], nil
    },
)
```

### `semantics/mappings.yaml`

Add `cz_notify_actorinit` action with `direction: send`, single implementation
for `0x007D` (`struct SYNTH_CZ_NOTIFY_ACTORINIT`, no fields).

---

## rAthena Source References

| Claim | File | Line |
|-------|------|------|
| `0x007D` single entry, never overridden | `src/map/clif_packetdb.hpp` | 32 |
| `0x007D` absent from shuffle | `src/map/clif_shuffle.hpp` | — |
| `0x007D` length = 2 in goKore session | `pkg/session/lengths_map.go` | `t[0x007D] = 2` |

---

## Test

```go
func TestEncodeCzNotifyActorinit(t *testing.T) {
    b := EncodeCzNotifyActorinit(send.CzNotifyActorinit{}, 20200401)
    assert.Equal(t, [2]byte{0x7D, 0x00}, b)
    // Same at any packetver
    b2 := EncodeCzNotifyActorinit(send.CzNotifyActorinit{}, 20030000)
    assert.Equal(t, [2]byte{0x7D, 0x00}, b2)
}
```
