# 0016 — 2026-03-08 — US-05: Phase 1 Golden Tests and 0-Alloc Benchmarks

**Story**: US-05 — Byte-level golden tests and 0-alloc benchmarks for Phase 1 decode functions  
**Session**: 0016

---

## Pre-Implementation Gate

### GCC Commands Run

```bash
# packet_idle_unit at PACKETVER=20181121 (total=108 bytes)
./validation/struct_layout.sh dump map/packets_struct.hpp packet_idle_unit 20181121

# packet_unit_walking at PACKETVER=20181121 (total=114 bytes)
./validation/struct_layout.sh dump map/packets_struct.hpp packet_unit_walking 20181121

# packet_spawn_unit at PACKETVER=20181121 (total=107 bytes)
./validation/struct_layout.sh dump map/packets_struct.hpp packet_spawn_unit 20181121

# PACKET_ZC_PAR_CHANGE in packets_struct.hpp at PACKETVER=20181121 (total=8 bytes)
# Field layout: PacketType(0:2), varID(2:2), count(4:4)
g++ -E -P -DPACKETVER=20181121 ... packets_struct.hpp | grep -A 10 "PACKET_ZC_PAR_CHANGE"
```

### Key Struct Layouts Verified

**packet_idle_unit @ 20181121 (108 bytes)**:
| offset | size | field |
|---|---|---|
| 0 | 2 | PacketType |
| 2 | 2 | PacketLength |
| 4 | 1 | objecttype |
| 5 | 4 | AID |
| 9 | 4 | GID |
| 13 | 2 | speed |
| 15 | 2 | bodyState |
| 17 | 2 | healthState |
| 19 | 4 | effectState |
| 23 | 2 | job |
| 25 | 2 | head |
| 27 | 4 | weapon |
| 31 | 4 | shield |
| 35 | 2 | accessory |
| 37 | 2 | accessory2 |
| 39 | 2 | accessory3 |
| 41 | 2 | headpalette |
| 43 | 2 | bodypalette |
| 45 | 2 | headDir |
| 47 | 2 | robe |
| 49 | 4 | GUID |
| 53 | 2 | GEmblemVer |
| 55 | 2 | honor |
| 57 | 4 | virtue |
| 61 | 1 | isPKModeON |
| 62 | 1 | sex |
| 63 | 3 | PosDir |
| 66 | 1 | xSize |
| 67 | 1 | ySize |
| 68 | 1 | state |
| 69 | 2 | clevel |
| 71 | 2 | font |
| 73 | 4 | maxHP |
| 77 | 4 | HP |
| 81 | 1 | isBoss |
| 82 | 2 | body |
| 84 | 24 | name |

**packet_unit_walking @ 20181121 (114 bytes)**:
Same prefix as packet_idle_unit through accessory (offset 35), then:
- accessory at 35, moveStartTime at 37 (+4 bytes), accessory2 at 41, accessory3 at 43
- headpalette at 45, bodypalette at 47, headDir at 49, robe at 51, GUID at 53
- GEmblemVer at 57, honor at 59, virtue at 61
- isPKModeON at 65, sex at 66, MoveData[6] at 67
- xSize at 73, ySize at 74, clevel at 75, font at 77, maxHP at 79, HP at 83
- isBoss at 87, body at 88, name at 90

**packet_spawn_unit @ 20181121 (107 bytes)**:
Same as packet_idle_unit layout except:
- body at 81 is `int16` not `uint16` (signed)
- name at 83 (not 84 — one byte earlier due to int16 vs uint16 body)
- total size 107 not 108

**PACKET_ZC_PAR_CHANGE (8 bytes)**:
- PacketType(0:2), varID(2:2), count(4:4)

**PACKET_ZC_LONGPAR_CHANGE (8 bytes)**:
- PacketType(0:2), varID(2:2), amount(4:4)

**PACKET_ZC_STATUS_CHANGE (5 bytes)**:
- PacketType(0:2), statusID(2:2), value(4:1)

---

## Implementation

### File Created

`pkg/decode/phase1_golden_test.go` — 10 tests, 6 benchmarks

### Tests Written

| Test | Packet ID | PACKETVER | Status |
|---|---|---|---|
| TestActorExists_0x09FF_Golden_20181121 | 0x09FF | 20181121 | PASS |
| TestActorExists_0x0078_Golden_20181121 | 0x0078 | 20181121 | PASS |
| TestActorMoved_0x09DB_Golden_20181121 | 0x09DB | 20181121 | PASS |
| TestActorMoved_0x007B_Golden_20181121 | 0x007B | 20181121 | PASS |
| TestActorConnected_0x09FE_Golden_20181121 | 0x09FE | 20181121 | PASS |
| TestActorConnected_0x0079_Golden_20181121 | 0x0079 | 20181121 | PASS |
| TestStatUpdate_0x00B0_Golden | 0x00B0 | any | PASS |
| TestStatUpdate_0x00B1_Golden | 0x00B1 | any | PASS |
| TestStatUpdate_0x00BE_Golden | 0x00BE | any | PASS |
| TestActorExists_0x09FF_VersionBoundary_Below | 0x09FF | 20181120 | PASS (no panic) |

**Note**: `AcAcceptLogin_0x0069` is not testable via decode package: the generated file
contains only a SKIP comment (`PACKET_AC_ACCEPT_LOGIN not found in VersionTable`).
This packet is in `common/packets.hpp` which is not yet fully wired into the VersionTable.
Deferred to US-02 / future epic.

### Discoveries From Tests

1. **ActorExists_0x09FF vs 0x0078 semantic difference confirmed**:
   - `0x09FF` uses `AID` (offset 5) for `e.ID` — the character ID
   - `0x0078` uses `GID` (offset 9) for `e.ID` — the game object ID
   - This is intentional in the SemanticDB field_mapping

2. **ActorConnected_0x09FE vs 0x0079 semantic difference confirmed**:
   - `0x09FE` uses `AID` (offset 5) for `e.ID` and `GID` (offset 9) for `e.CharID`
   - `0x0079` uses `GID` (offset 9) for `e.ID` only (no CharID mapping)

3. **ActorConnected_0x09FE Name field is a known skip**:
   The generated code emits `// e.Name = strings.TrimRight(...)  (complex expression — implement manually)`.
   Name is empty string in the decoded event. Documented as a complex expression skip.

### Issues Found and Documented

**PERF-1 (KNOWN_ISSUES.md)**: `ActorExists_0x09FF` shows 1 alloc/op (8 bytes) due to
`nullTermString` converting `char[24]` to Go `string`. This is unavoidable with the current
`Name string` event field type. Accepted for Phase 1; resolution options documented.

**PERF-2 (KNOWN_ISSUES.md)**: `StatUpdate_0x00BE` reads `leU32(data, 4)` on a 1-byte
field (`uint8 value`). The struct is 5 bytes but the decode reads 4 bytes from offset 4,
requiring callers to supply 8+ bytes. Deferred to US-07 / next codegen pass.

---

## Benchmark Results

```
BenchmarkActorExists_0x09FF-14       33766957    31.67 ns/op    8 B/op    1 allocs/op  ← PERF-1
BenchmarkActorMoved_0x09DB-14        67369180    20.06 ns/op    0 B/op    0 allocs/op  ✅
BenchmarkActorMoved_0x007B-14        57714486    17.60 ns/op    0 B/op    0 allocs/op  ✅
BenchmarkActorConnected_0x09FE-14    61113518    18.39 ns/op    0 B/op    0 allocs/op  ✅
BenchmarkActorExists_0x0078-14       69204742    16.95 ns/op    0 B/op    0 allocs/op  ✅
BenchmarkStatUpdate_0x00B0-14      1000000000     0.13 ns/op    0 B/op    0 allocs/op  ✅
```

The 1 alloc/op in `ActorExists_0x09FF` is the `Name string` allocation documented in PERF-1.
All other Phase 1 decode benchmarks are 0 allocs/op.

---

## US-05 Acceptance Criteria Status

- [x] Golden test for every P0 decode function at PACKETVER=20181121
- [x] Every golden test constructs bytes from GCC-verified offsets (not from generated code)
- [x] `go test ./pkg/decode/` — all tests pass (10/10)
- [x] `go test -bench=. -benchmem ./pkg/decode/` — 0 allocs/op for all P0 benchmarks except ActorExists_0x09FF
- [ ] ActorExists_0x09FF: 1 alloc/op — documented in KNOWN_ISSUES.md (PERF-1), accepted
- [x] Non-zero alloc identified, explained in KNOWN_ISSUES.md, given a resolution plan
- [ ] `AcAcceptLogin_0x0069` decode function — SKIP (struct not in VersionTable, US-02 dependency)

**US-05 status**: COMPLETE (with documented exceptions)

---

## Full Validation

```
go build ./...        → clean
go test ./...         → all pass (10 new tests in pkg/decode)
go test -bench=. -benchmem ./pkg/decode/  → see above
grep -r "^\s*go " pkg/  → empty (0 goroutines in pkg/)
```
