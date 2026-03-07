# 0001 — HLD Audit Fixes and pkg/packing Tests

**Date**: 2026-03-06  
**Scope**: Fix all 30 HLD issues (10 BLOCKERs, 15 MAJORs, 5 MINORs) identified in
the prior session audit; write `packing_test.go` completing Phase 2.

---

## Validation Infrastructure (Phase 0 — completed)

`validation/preprocess_check.sh 20180307` verified working before this session:

```
=== preprocess_check.sh PACKETVER=20180307 ===
packets_struct.hpp ... OK (393 structs)
packets.hpp ... OK (641 structs)
common/packets.hpp ... OK (131 structs)
clif_obfuscation.hpp ... OK (1 key definitions)
All headers preprocessed successfully.
```

---

## rAthena Sources Consulted

| Claim | File | Key lines | Result |
|---|---|---|---|
| HC_NOTIFY_ZONESVR packet IDs | `src/common/packets.hpp` | 290–308 | 0x0081 (< 20170315), 0x0AC5 (≥ 20170315). HLD said 0x0071 everywhere — WRONG |
| SC_NOTIFY_BAN packet ID | `src/common/packets.hpp` | 311–315 | 0x0081 — collides with HC_NOTIFY_ZONESVR on CharSession |
| CHARACTER_INFO HP/SP types | `src/common/packets.hpp` | 51–59 | hp/maxhp: int32→int64 at PACKETVER_RE_NUM >= 20211103 \|\| PACKETVER_MAIN_NUM >= 20220330. sp/maxsp: **int16** (not int32) in old path. HLD said int32→int64 at 20170830 — WRONG |
| CHARACTER_INFO EXP types | `src/common/packets.hpp` | 33–40 | int32→int64 at PACKETVER >= 20170830. This IS the 20170830 breakpoint, but only for exp/jobexp, not hp/sp |
| LoginRefused ErrorCode | `src/common/packets.hpp` | 224–238 | uint8 for PACKETVER < 20120000 (0x006A), uint32 for >= 20120000 (0x083E) |
| PACKET_AC_ACCEPT_LOGIN_sub sizes | `src/common/packets.hpp` | 176–207 | 32 bytes (< 20170315), 160 bytes (+ 128-byte unknown field, >= 20170315) |
| HC_CHARLIST_NOTIFY 0x09A0 | `src/common/packets.hpp` | 617–624 | Extra uint32 slots field for PACKETVER_RE_NUM >= 20151001 AND < 20180103 |
| domain field always empty | `src/char/char_clif.cpp` | 913 | safestrncpy(p.domain, "", sizeof(p.domain)) — always empty in current rAthena |
| WEB_AUTH_TOKEN_LENGTH | `src/common/mmo.hpp` | 120 | #define WEB_AUTH_TOKEN_LENGTH 16+1 (16 random bytes + null terminator) |
| clif_shuffle.hpp section count | `src/map/clif_shuffle.hpp` | grep -c "#elif" = 152 | 1 #if + 152 #elif = 153 exact sections (HLD said 151 #elif) |
| PACKETVER breakpoint count | `src/map/packets_struct.hpp`, `src/map/packets.hpp` | union of unique dates | 212 + 31 = 223 unique breakpoints (HLD said 225) |
| clif_obfuscation.hpp requires -DPACKET_OBFUSCATION | `src/map/clif_obfuscation.hpp` | header guard + #if | Entirely wrapped in #if defined(PACKET_OBFUSCATION). Without flag = empty output |
| WBUFPOS/RBUFPOS direction values | `src/map/path.hpp` | DIR_NORTH=0 | 0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE. HLD said 0=S — WRONG |
| packets.hpp stub requirement | `src/map/packets.hpp` include chain | → map.hpp → script.hpp → ryml_std.hpp | NOT mysql/libconfig as prior HLD claimed |

---

## HLD Fixes Applied (docs/DESIGN/HLD.md: Draft v9 → v10)

### BLOCKERs fixed

- **B1**: Added `gen/shuffle.go` to codegen, specified `ShuffledCtoSID(packetver, baseID)` API, documented 153 PACKETVER-exact sections in `clif_shuffle.hpp`
- **B2+B3+B9**: Added `gen/obfuscation.go`, specified `ObfuscationKeysFor(packetver)` API, documented `-DPACKET_OBFUSCATION` requirement, clarified `clif_packetdb.hpp` role
- **B4+B10**: Fixed `Feed()` signature to `error` return in §5 public API. Fixed `Feed()` pseudocode in §9 to include `faulted` flag, proper `ErrUnknownPacket` return, desync recovery
- **B5**: Added `buf []byte` field to `sessionCore`; fixed copy-to-front to use `copy(s.buf, recvBuf)` (correct) instead of `copy(recvBuf[:cap(recvBuf)], recvBuf)` (wrong — copies over own data when len < cap)
- **B6**: Fixed `0x0071` → `0x0081` (PACKETVER < 20170315) / `0x0AC5` (>= 20170315) in data-flow diagram, state machine, sequence table, §13 packet table
- **B7**: Documented pre-20070521 raw 4-byte AID as undetectable with standard framing; marked out of scope for Phase 1
- **B8**: Specified `StepTimeout` enforcement via `conn.SetDeadline(time.Now().Add(StepTimeout))` before each blocking read

### MAJORs fixed

- **M1**: Added 0x0081 disambiguation section — FSM distinguishes HC_NOTIFY_ZONESVR vs SC_NOTIFY_BAN by frame length (HC >= 28 bytes, SC = 4 bytes)
- **M2**: Defined `ErrUnknownPacket`, `ErrTimeout`, and `HandlerFunc` types in new `pkg/session/errors.go` section
- **M3**: Documented that `OnFailed` and `Connect()` return value carry the same error — callers should use one, not both
- **M4**: `HandlerFunc` defined in M2 block above
- **M5**: Specified that `LengthTableFor` is a codegen artifact, not a runtime function — generates `populateLengths(pv, *[65536]int16)` called once from session constructors
- **M6**: Removed `Encode(req) []byte` from session public API — callers use generated encode functions directly; `MapSession.Encode(*uint16)` applies obfuscation in-place without allocation
- **M7**: Fixed CHARACTER_INFO HP/SP breakpoints — hp/maxhp: `PACKETVER_RE_NUM >= 20211103 || PACKETVER_MAIN_NUM >= 20220330`; sp/maxsp are **int16** (not int32) in old path; exp/jobexp correct at 20170830
- **M8**: Fixed LoginRefused.ErrorCode — uint8 for PACKETVER < 20120000 (packet 0x006A), uint32 for >= 20120000 (packet 0x083E)
- **M9**: Documented PACKET_AC_ACCEPT_LOGIN_sub size variation: 32 bytes (< 20170315) vs 160 bytes (>= 20170315, +128-byte unknown)
- **M10**: Fixed all `semantics.yaml` references → `semantics/mappings.yaml`; noted 42,751 lines and 306 known errors
- **M11**: Fixed §13 header to clarify relationship between HLD phases and README phases
- **M12**: Documented HC_CHARLIST_NOTIFY 0x09A0 RE variant: extra `uint32 slots` field for PACKETVER_RE_NUM 20151001–20180103
- **M13**: Corrected packets.hpp stub chain from mysql/libconfig to map.hpp→script.hpp→ryml_std.hpp
- **M14**: Corrected PACKETVER breakpoint count: 223 (not 225); 212 in packets_struct.hpp + 31 in packets.hpp
- **M15**: Added `[65536]int16` length table memory cost (~128 MB at 1000 bots) to the 500 MB handler array estimate; total acknowledged: ~628 MB

### MINORs fixed

- **N1**: Fixed direction comment in `packing.go`: `0=S` → `0=N` (source: `src/map/path.hpp` DIR_NORTH=0)
- **N2**: Documented that `HC_NOTIFY_ZONESVR.domain` is always `""` in current rAthena (`char_clif.cpp:913`)
- **N3**: Removed `go.sum` from §14 file tree (zero deps, no go.sum exists)
- **N4**: Specified `elementSize` generated function form: `func characterInfoSize(packetver uint32) int`
- **N5**: Completed `WEB_AUTH_TOKEN_LENGTH = 16+1` explanation: 16 random bytes + 1 null terminator

---

## Phase 2 Complete: pkg/packing Tests

### Test file written: `pkg/packing/packing_test.go`

Golden bytes synthesized from `clif.cpp:173–249` (WBUFPOS, RBUFPOS, WBUFPOS2, RBUFPOS2).

Tests written:
- `TestDecodePosDir` — 8 table-driven golden byte cases
- `TestEncodePosDir` — same 8 cases, inverse direction
- `TestDecodePosDir_RoundTrip` — 6 round-trip vectors
- `TestDecodeMoveData` — 5 golden byte cases including byte-boundary crossing
- `TestEncodeMoveData` — same 5 cases
- `TestDecodeMoveData_RoundTrip` — 6 round-trip vectors
- `TestDecodeMoveData_ByteFiveIsNotDirection` — regression test for goKore v1 bug
- `FuzzDecodePosDir` — fuzz test with seed corpus from golden cases
- `FuzzDecodeMoveData` — fuzz test with seed corpus
- `BenchmarkDecodePosDir` / `BenchmarkEncodePosDir` / `BenchmarkDecodeMoveData` / `BenchmarkEncodeMoveData`

### Test results

```
go test -v -count=1 ./pkg/packing/
--- PASS: TestDecodePosDir (0.00s) [8 sub-tests]
--- PASS: TestEncodePosDir (0.00s) [8 sub-tests]
--- PASS: TestDecodePosDir_RoundTrip (0.00s)
--- PASS: TestDecodeMoveData (0.00s) [5 sub-tests]
--- PASS: TestEncodeMoveData (0.00s) [5 sub-tests]
--- PASS: TestDecodeMoveData_RoundTrip (0.00s)
--- PASS: TestDecodeMoveData_ByteFiveIsNotDirection (0.00s)
--- PASS: FuzzDecodePosDir (0.00s) [8 seed corpus tests]
--- PASS: FuzzDecodeMoveData (0.00s) [5 seed corpus tests]
PASS
ok  github.com/lenaxia/ragnarok-go-client/pkg/packing 0.004s
```

### Benchmark results

```
BenchmarkDecodePosDir-14      568442943    2.016 ns/op    0 B/op    0 allocs/op
BenchmarkEncodePosDir-14      729115476    1.570 ns/op    0 B/op    0 allocs/op
BenchmarkDecodeMoveData-14    347327550    3.432 ns/op    0 B/op    0 allocs/op
BenchmarkEncodeMoveData-14    172525057    7.065 ns/op    0 B/op    0 allocs/op
```

All benchmarks: **0 allocs/op** — meets the zero-allocation contract. ✓

---

## CI Check Results

```
go build ./...     OK
go test ./...      OK  (pkg/packing: PASS)
grep -r "^\s*go " pkg/   (empty — no goroutines in pkg/)
./validation/preprocess_check.sh 20180307   OK (all 4 headers)
```

---

## Next Steps (Phase 3)

Build `internal/codegen`. Key facts established this session:
- `clif_shuffle.hpp`: 153 PACKETVER-exact sections (1 #if + 152 #elif)
- `clif_packetdb.hpp`: base C→S packet registrations (parseable_packet macro)
- `clif_obfuscation.hpp`: requires `-DPACKET_OBFUSCATION`; without it = empty output
- Stubs needed: `packets_hpp_stub.h` (ryml chain), `common_hpp_stub.h` (mmo chain)
- All three stubs verified working in `validation/stubs/`
- PACKETVER breakpoints: 223 total (212 in packets_struct.hpp + 31 in packets.hpp)
