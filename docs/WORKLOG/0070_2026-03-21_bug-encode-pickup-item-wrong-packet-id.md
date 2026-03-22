# 0070 — Bug: EncodePickupItem sends wrong packet ID at pv >= 20101124

**Date**: 2026-03-21  
**Scope**: `pkg/encode/pickup_item.go`  
**Reporter**: goKore integration  
**Severity**: BLOCKING — server disconnects client on every item pickup attempt

---

## Observed Behaviour

goKore sends `ActionPickupItem` (ground item pickup). The server immediately
disconnects the client. The pickup never reaches the item list; DB inventory
unchanged despite pickup packet being sent.

From `connector.maploop` log at the moment of disconnect:
```
→ wire outbound action=ActionItemPickup bytes=6
← conn.Read error — closing map loop err=EOF
```

---

## Root Cause

`EncodePickupItem` hardcodes `0x009F` for all packetvers and ignores `packetver`:

```go
// pkg/encode/pickup_item.go (current — broken)
func EncodePickupItem(req send.PickupItem, packetver uint32) [6]byte {
    var p [6]byte
    p[0] = 0x9f  // ← hardcoded 0x009F, ignores packetver
    p[1] = 0x00
    leU32Put(p[2:], req.ITID)
    _ = packetver
    return p
}
```

Per `semantic.go` line 126: **"Send does NOT call ShuffledCtoSID — shuffle is the
encode function's responsibility."**

At `pv > 20180307`, `shuffledCtoSID(pv, 0x009F) = 0x0362` (from
`pkg/encode/shuffle_map.go:28`). The encode function must write `0x0362`, not
`0x009F`. Sending `0x009F` causes the server to interpret it as a completely
different packet (a non-TakeItem handler) → disconnect.

This is the same bug class as worklog 0069 (`EncodeItemUse`), just a different
action. The fix pattern is identical to how `EncodeItemUse` was fixed in v0.5.5.

---

## rAthena Source

From `src/map/clif_packetdb.hpp`:

```cpp
// Active at pv >= 20101124 (includes pv=20200401):
parseable_packet(0x0362, 6, clif_parse_TakeItem, 2);  // 0x009F shuffled → 0x0362

// Base table (pv < 20040705):
parseable_packet(0x009f, 6, clif_parse_TakeItem, 2);  // original 0x009F

// pv >= 20040713:
parseable_packet(0x009f, 10, clif_parse_TakeItem, 6); // 0x009F, 10 bytes
```

From `pkg/encode/shuffle_map.go` (pv > 20180307 block):
```go
case 0x009F:
    return 0x0362
```

---

## Required Fix

`EncodePickupItem` must write the shuffled packet ID based on `packetver`.
Pattern identical to `EncodeItemUse` fix in v0.5.5:

```go
func EncodePickupItem(req send.PickupItem, packetver uint32) [6]byte {
    var p [6]byte
    switch {
    case packetver > 20180307:       // shuffled: 0x009F → 0x0362
        p[0] = 0x62; p[1] = 0x03
    case packetver >= 20101124:      // shuffled: 0x009F → (check exact mapping)
        // Verify via shuffle_map.go for pv range 20101124-20180307
        p[0] = 0x9f; p[1] = 0x00    // placeholder — confirm correct shuffled ID
    default:                          // pv < 20101124: 0x009F, 6 bytes
        p[0] = 0x9f; p[1] = 0x00
    }
    leU32Put(p[2:], req.ITID)
    return p
}
```

**Note**: The struct size also changes at `pv >= 20040713` (6→10 bytes, field at
offset 6 instead of 2). Verify whether modern pv uses the 6-byte or 10-byte form.
From `clif_packetdb.hpp:1384`: at `pv >= 20101124`, `0x0362` is 6 bytes with
field at offset 2. So the 6-byte layout IS correct for `pv >= 20101124`.

Full version dispatch needed:
- `pv < 20040713`: `0x009F`, 6 bytes, field at 2
- `pv >= 20040713` and `pv < 20101124`: `0x009F`, 10 bytes, field at 6
- `pv >= 20101124`: shuffled ID, 6 bytes, field at 2 (re-uses original layout)

Exact shuffled IDs for each packetver range must be read from `shuffle_map.go`.

---

## rAthena Source References

| Claim | File | Line |
|-------|------|------|
| `0x0362` = `clif_parse_TakeItem` from `pv >= 20101124` | `src/map/clif_packetdb.hpp` | 1384 |
| `0x009F` → `0x0362` for `pv > 20180307` | `pkg/encode/shuffle_map.go` | 28 |
| Send does NOT apply shuffle | `pkg/session/semantic.go` | 126 |

---

## Tests

Add to `pkg/encode/pickup_item_test.go`:

```go
func TestEncodePickupItem_PacketID(t *testing.T) {
    req := send.PickupItem{ITID: 99999}

    // pv < 20040713: 0x009F
    old := EncodePickupItem(req, 20030000)
    assert.Equal(t, byte(0x9f), old[0])
    assert.Equal(t, byte(0x00), old[1])

    // pv >= 20101124 (modern): 0x0362
    modern := EncodePickupItem(req, 20200401)
    assert.Equal(t, byte(0x62), modern[0])
    assert.Equal(t, byte(0x03), modern[1])
}
```
