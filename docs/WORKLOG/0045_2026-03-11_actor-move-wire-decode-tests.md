# 0045 — Actor Move Wire-Level Position Decode Tests

**Date**: 2026-03-11  
**Scope**: `pkg/decode/actor_move_positions_test.go` (new file)

---

## Objective

Add tests that verify the full decode pipeline for actor movement packets:  
decode function → `packing.DecodeMoveData` / `packing.DecodePosDir` → concrete map coordinates.

Existing tests in `phase1_golden_test.go` and `character_moves_test.go` verified that the raw `MoveData [6]byte` and `PosDir [3]byte` fields were copied correctly from the wire buffer, but none of them exercised the position unpacking step to confirm the actual (x, y) coordinates from real captured traffic.

---

## Sources

- `~/personal/goKore-test/docs/03_REFERENCE/dumps/DUMP8_movement` (2026-01-17): bot `botijo16` on `geffen.gat`, moving toward prontera from spawn (52, 128)
- `~/personal/goKore-test/docs/03_REFERENCE/dumps/DUMP2` (2025-04-30): bot `botijo0` on `gef_fild07.gat`, monster "Chonchon" visible

---

## Packet Formats Verified

All packet layouts verified by GCC preprocessor at `PACKETVER=20181121`:

```
PACKET_ZC_NOTIFY_PLAYERMOVE  (0x0087) — fixed 12 bytes
  offset 2: moveStartTime (uint32)
  offset 6: moveData[6]   (WBUFPOS2)

struct packet_unit_walking  (0x09FD) — 114 bytes at PACKETVER>=20181121
  offset 67: MoveData[6]  (WBUFPOS2)

struct packet_idle_unit     (0x09FF) — 108 bytes at PACKETVER>=20181121
  offset 63: PosDir[3]    (WBUFPOS)

SYNTH_ZC_NOTIFY_MOVE        (0x0086) — fixed 16 bytes
  offset 6: moveData[6]   (WBUFPOS2)
```

---

## Real-Wire Coordinate Verification

### 0x0087 (CharacterMoves) — 3 consecutive packets from DUMP8

| Packet | Wire bytes (MoveData) | fromX | fromY | toX | toY |
|--------|----------------------|-------|-------|-----|-----|
| pkt1 | `0D 08 00 D0 76 88` | 52 | 128 | 52 | 118 |
| pkt2 | `0D 07 F0 D0 76 88` | 52 | 127 | 52 | 118 |
| pkt3 | `0D 07 E0 D0 75 88` | 52 | 126 | 52 | 117 |

Cross-check: dump annotation "Your Coordinates: 52, 128" ✓ (fromX=52, fromY=128 in pkt1)

Byte 5 = `0x88` → sx0=8, sy0=8 (NOT direction — goKore v1 bug guard)

### 0x09FD (ActorMoved) — Chonchon monster from DUMP2

Wire bytes `40 8A E4 08 B1 88` at offset 67:

| Field | Value |
|-------|-------|
| fromX | 258 |
| fromY | 174 |
| toX   | 258 |
| toY   | 177 |
| AID   | 110035299 |
| speed | 200 |
| Name  | "Chonchon" |

Byte 5 = `0x88` → sx0=8, sy0=8 (sub-cell offsets, not direction)

### 0x09FF (ActorExists) — NPC from DUMP8

Wire bytes `0A C7 B6` at offset 63:

| Field | Value |
|-------|-------|
| x     | 43 |
| y     | 123 |
| dir   | 6 (DIR_EAST) |
| AID   | 110022517 |
| Name  | "Magician's Guild Guide#" |

Cross-check: dump annotation "(43, 123) (ID 110022517)" ✓

---

## Test Results

```
=== RUN   TestCharacterMoves_0x0087_WireDecode_FromTo_Packet1  PASS
=== RUN   TestCharacterMoves_0x0087_WireDecode_FromTo_Packet2  PASS
=== RUN   TestCharacterMoves_0x0087_WireDecode_FromTo_Packet3  PASS
=== RUN   TestCharacterMoves_0x0087_WireDecode_Byte5IsNotDirection  PASS
=== RUN   TestActorMoved_0x09FD_WireDecode_FromTo  PASS
=== RUN   TestActorMoved_0x09FD_WireDecode_MetaFields  PASS
=== RUN   TestActorMoved_0x09FD_WireDecode_Byte5IsNotDirection  PASS
=== RUN   TestActorExists_0x09FF_WireDecode_Position  PASS
=== RUN   TestActorExists_0x09FF_WireDecode_MetaFields  PASS
=== RUN   TestEntityMove_0x0086_WireDecode_Position  PASS
=== RUN   TestEntityMove_0x0086_WireDecode_MaxCoords  PASS
=== RUN   TestEntityMove_0x0086_WireDecode_ZeroCoords  PASS
```

All 12 tests pass. Full suite `go test ./...` passes.

---

## Benchmark Results

```
BenchmarkCharacterMoves_0x0087_WireDecode-14    92884863    14.38 ns/op    0 B/op    0 allocs/op
BenchmarkActorMoved_0x09FD_WireDecode-14        20310418    55.13 ns/op    0 B/op    0 allocs/op
BenchmarkActorExists_0x09FF_WireDecode-14       27131875    48.10 ns/op    0 B/op    0 allocs/op
BenchmarkEntityMove_0x0086_WireDecode-14        97347388    14.25 ns/op    0 B/op    0 allocs/op
```

Zero allocations on all four decode+unpack benchmarks. ✓
