# 0049 — Fix 0x0ADD length (24 bytes at PACKETVER >= 20181121) + lengths_map_overrides.go infrastructure

**Date**: 2026-03-18  
**Scope**: `pkg/session/lengths_map_overrides.go`, `pkg/session/map.go`, `pkg/session/lengths_regression_test.go`, `README-LLM.md`

---

## Problem

`t[0x0ADD]` was set to 22 for all `pv >= 20180418`. The correct wire length for `PACKETVER_MAIN_NUM >= 20181121` is 24 bytes. With the wrong length of 22, the framer under-read the item drop frame by 2 bytes, leaving `0x0000` as the next apparent packet ID, which triggered the unknown-packet handler and cleared the receive buffer on every kill.

## Root cause

`clif_packetdb.hpp:1921` hardcodes `packet(0x0ADD, 22)` as an integer literal, not a `sizeof()` expression. The codegen's Part 1 diff pass reads this value at every PACKETVER breakpoint and always gets `22` — it never detects the struct size change. Parts 2–4 use `mergeBreakpointsFillOnly` and cannot override a value already claimed by Part 1.

The struct change is in `packets_struct.hpp:600`:
```c
#if PACKETVER_MAIN_NUM >= 20181121 || PACKETVER_RE_NUM >= 20180704 || PACKETVER_ZERO_NUM >= 20181114
    uint32 ITID;   // 4 bytes — was uint16 (2 bytes)
```

## GCC verification

```bash
g++ -E -P -DPACKETVER=20180418 -DPACKETVER_MAIN_NUM=20180418 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp | grep -A 15 "struct packet_dropflooritem"
# Result: uint16 ITID → 2+4+2+2+1+2+2+1+1+2+1+2 = 22 bytes ✓

g++ -E -P -DPACKETVER=20181121 -DPACKETVER_MAIN_NUM=20181121 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp | grep -A 15 "struct packet_dropflooritem"
# Result: uint32 ITID → 2+4+4+2+1+2+2+1+1+2+1+2 = 24 bytes ✓
```

OpenKore kRO confirmation:
- `RagexeRE_2018_06_21a/recvpackets.txt`: `0ADD 22`
- `RagexeRE_2020_04_01b/recvpackets.txt`: `0ADD 24`

Live TCP stream (goKore, pay_dun00, PACKETVER 20200401): 24-byte frame parses with zero remainder.

## Fix

**Immediate**: `pkg/session/lengths_map_overrides.go` — a new hand-maintained file with `applyMapLengthOverrides(pv, t)` applied after `populateMapLengths` in `NewMapSession`. The override for `0x0ADD`:

```go
if pv >= 20181121 {
    t[0x0ADD] = 24
}
```

**Wiring**: `pkg/session/map.go` — `NewMapSession` now calls `applyMapLengthOverrides` after `populateMapLengths`.

**Test helper**: `mapTableAt` in `lengths_regression_test.go` updated to call `applyMapLengthOverrides` so regression tests reflect the real table seen by production code.

**Regression test**: `TestLengthRegression_PacketID0ADD` added to `lengths_regression_test.go` — verifies 22 bytes before 20181121 and 24 bytes from 20181121 onward.

**Documentation**: README-LLM.md updated in two places:
- "Known open issues" — describes the codegen blind spot and the override file as workaround
- Codegen section — documents the `mergeBreakpointsFillOnly` limitation and the future Part 5 cross-check pass

## Long-term fix (not implemented, tracked in README-LLM.md)

Add a Part 5 cross-check pass to the codegen: after Part 1, for every packet ID where the SemanticDB has a struct mapping, compare `TotalSize` from the VersionTable at each breakpoint against the Part 1 length. Where they diverge, emit a correcting breakpoint. This would catch `0x0ADD` and any future cases automatically without manual entries in the overrides file.

## Test results

```
go build ./...   ✓
go test ./...    ✓  (all packages pass including new TestLengthRegression_PacketID0ADD)
```
