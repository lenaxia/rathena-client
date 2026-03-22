# 0069 — Bug: EncodeItemUse sends wrong packet ID at pv >= 20080910

**Date**: 2026-03-21  
**Scope**: `pkg/encode/item_use.go`, `pkg/session/semantic.go` (send dispatch)  
**Reporter**: goKore integration  
**Severity**: BLOCKING — server disconnects client on every item use attempt

---

## Observed Behaviour

goKore sends `ActionItemUse` and the rAthena server immediately disconnects:

```
[Warning]: clif_parse: Received packet 0x00a7 with expected packet length 9,
           but only 8 bytes remaining, disconnecting session #8.
```

---

## Root Cause

`EncodeItemUse` hardcodes the wire packet ID as `0x00A7`:

```go
// pkg/encode/item_use.go (generated)
func EncodeItemUse(req send.ItemUse, packetver uint32) [8]byte {
    var p [8]byte
    p[0] = 0xa7   // ← hardcoded 0x00A7
    p[1] = 0x00
    leU16Put(p[2:], req.Index)
    leU32Put(p[4:], req.AID)
    _ = packetver  // ← packetver silently ignored
    return p
}
```

At `pv >= 20080910`, rAthena's `clif_packetdb.hpp` reassigns `0x00A7` to
`clif_parse_SolveCharName` (9 bytes) and moves `clif_parse_UseItem` to `0x0439`.

Verified in rAthena source (`src/map/clif_packetdb.hpp`):

```cpp
// 2008-09-10aSakexe
#if PACKETVER >= 20080910
    parseable_packet(0x0439, 8, clif_parse_UseItem, 2, 4);  // CZ_USE_ITEM2
#endif
```

This assignment persists through all subsequent packetver blocks — the last occurrence
at `pv >= 20120410` still maps `0x0439` to `clif_parse_UseItem`. At `pv=20200401`
(production), `0x0439` is the correct wire ID for `CZ_USE_ITEM`.

When goKore sends `0x00A7` (8 bytes), the server interprets it as
`clif_parse_SolveCharName` which expects 9 bytes → length mismatch → disconnect.

---

## Correct Wire Format at pv >= 20080910

```
0x0439 <index>.W <account_id>.L  (8 bytes total)
```

Fields (from `clif_packetdb.hpp` field offsets `2, 4`):
- `[0:2]` packet ID = `0x0439` (LE)
- `[2:4]` index (uint16 LE) — inventory slot index as stored in the burst packet
- `[4:8]` AID (uint32 LE) — account ID

---

## Required Fix

In `pkg/encode/item_use.go`, branch on `packetver`:

```go
func EncodeItemUse(req send.ItemUse, packetver uint32) [8]byte {
    var p [8]byte
    if packetver >= 20080910 {
        // CZ_USE_ITEM2 (0x0439): active from 2008-09-10 onward
        p[0] = 0x39
        p[1] = 0x04
    } else {
        // CZ_USE_ITEM (0x00A7): active before 2008-09-10
        p[0] = 0xa7
        p[1] = 0x00
    }
    leU16Put(p[2:], req.Index)
    leU32Put(p[4:], req.AID)
    return p
}
```

No struct size change is needed — both `0x00A7` (pre-20080910) and `0x0439`
(post-20080910) are 8 bytes with the same field layout.

Note: `0x00A7` is NOT in the C→S shuffle map for `pv > 20180307`
(`pkg/encode/shuffle_map.go`), confirming it is not the correct wire ID at
modern packetvers — the shuffle map only covers packets that retained their
original base ID. `CZ_USE_ITEM` migrated to `0x0439` and is not shuffled.

---

## rAthena Source References

| Claim | File | Line |
|-------|------|------|
| `0x0439` = `clif_parse_UseItem` from `pv >= 20080910` | `src/map/clif_packetdb.hpp` | 1151 |
| `0x00A7` reassigned to `clif_parse_SolveCharName` at same pv | `src/map/clif_packetdb.hpp` | ~304 |
| No `0x00A7` entry in shuffle map | `pkg/encode/shuffle_map.go` | — |

---

## Test

Add to `pkg/encode/item_use_test.go` (or relevant test file):

```go
func TestEncodeItemUse_PacketID(t *testing.T) {
    req := send.ItemUse{Index: 12, AID: 2000301}

    // pv < 20080910: must use 0x00A7
    old := EncodeItemUse(req, 20040705)
    assert.Equal(t, byte(0xa7), old[0])
    assert.Equal(t, byte(0x00), old[1])

    // pv >= 20080910: must use 0x0439
    modern := EncodeItemUse(req, 20200401)
    assert.Equal(t, byte(0x39), modern[0])
    assert.Equal(t, byte(0x04), modern[1])
}
```
