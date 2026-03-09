# 0007 — Full Spot-Check Offset Revalidation

**Date**: 2026-03-07  
**Status**: COMPLETE

## Objective

Deep revalidation of every field spot-check offset assertion in `validation/phase1_gate.sh` section 6 ("Struct Layout Verification"). The previous session left this incomplete — only total-byte assertions had been independently verified; the per-field `name:offset:size` triplets had not been cross-checked against live GCC output.

The rule: **assume everything wrong until GCC confirms it**. Do not trust the existing hardcoded values.

## Method

For every struct and PACKETVER boundary that has field spot-checks in `phase1_gate.sh`, ran:

```bash
bash validation/struct_layout.sh dump <header> <struct> <packetver>
```

Then compared each `field:offset:size` triplet in the gate assertions against the live `dump` output.

## Results

### packet_idle_unit (8 breakpoints, ~12 spot-checks)

| PACKETVER | Assertions | GCC Output | Verdict |
|-----------|-----------|------------|---------|
| 20080101 | `effectState:12:2`, `PosDir:48:3` | effectState=12, PosDir=48 | ✅ CORRECT |
| 20080102 | `effectState:12:4`, `font:58:2`, `PosDir:50:3` | effectState=12, font=58, PosDir=50 | ✅ CORRECT |
| 20091103 | `PacketLength:2:2`, `objecttype:4:1`, `GID:5:4`, `PosDir:53:3` | 2, 4, 5, 53 | ✅ CORRECT |
| 20101124 | `robe:39:2`, `PosDir:55:3` | robe=39, PosDir=55 | ✅ CORRECT |
| 20120221 | `maxHP:65:4`, `HP:69:4`, `isBoss:73:1` | 65, 69, 73 | ✅ CORRECT |
| 20131223 | `AID:5:4`, `GID:9:4`, `name:78:24`, `PosDir:59:3` | 5, 9, 78, 59 | ✅ CORRECT |
| 20150513 | `body:78:2`, `name:80:24` | 78, 80 | ✅ CORRECT |
| 20181121 | `weapon:27:4`, `shield:31:4`, `accessory:35:2`, `PosDir:63:3`, `name:84:24` | 27, 31, 35, 63, 84 | ✅ CORRECT |

### packet_spawn_unit (8 breakpoints, ~8 spot-checks)

| PACKETVER | Assertions | GCC Output | Verdict |
|-----------|-----------|------------|---------|
| 20080101 | `effectState:12:2` | effectState=12 | ✅ CORRECT |
| 20080102 | `effectState:12:4` | effectState=12 | ✅ CORRECT |
| 20091103 | `PacketLength:2:2`, `objecttype:4:1`, `GID:5:4` | 2, 4, 5 | ✅ CORRECT |
| 20101124 | `robe:39:2` | robe=39 | ✅ CORRECT |
| 20120221 | `maxHP:64:4`, `HP:68:4`, `isBoss:72:1` | 64, 68, 72 | ✅ CORRECT |
| 20131223 | `AID:5:4`, `GID:9:4`, `name:77:24` | 5, 9, 77 | ✅ CORRECT |
| 20150513 | `body:77:2`, `name:79:24` | 77, 79 | ✅ CORRECT |
| 20181121 | `shield:31:4`, `accessory:35:2`, `name:83:24` | 31, 35, 83 | ✅ CORRECT |

### packet_unit_walking (9 breakpoints, ~12 spot-checks)

| PACKETVER | Assertions | GCC Output | Verdict |
|-----------|-----------|------------|---------|
| 20071105 | `PacketType:0:2`, `effectState:12:4`, `MoveData:54:6` | 0, 12, 54 | ✅ CORRECT |
| 20071106 | `objecttype:2:1`, `effectState:13:4`, `MoveData:55:6` | 2, 13, 55 | ✅ CORRECT |
| 20080102 | `objecttype:2:1`, `effectState:13:4`, `MoveData:55:6` | 2, 13, 55 | ✅ CORRECT |
| 20091103 | `PacketLength:2:2`, `objecttype:4:1`, `effectState:15:4`, `MoveData:57:6` | 2, 4, 15, 57 | ✅ CORRECT |
| 20101124 | `robe:43:2`, `MoveData:59:6` | 43, 59 | ✅ CORRECT |
| 20120221 | `robe:43:2`, `maxHP:71:4` | 43, 71 | ✅ CORRECT |
| 20131223 | `AID:5:4`, `robe:47:2`, `MoveData:63:6`, `name:84:24` | 5, 47, 63, 84 | ✅ CORRECT |
| 20150513 | `body:84:2`, `name:86:24` | 84, 86 | ✅ CORRECT |
| 20181121 | `shield:31:4`, `robe:51:2`, `MoveData:67:6`, `name:90:24` | 31, 51, 67, 90 | ✅ CORRECT |

### packet_authok (4 breakpoints, total-only, no field spot-checks beyond PacketType)

Gate only asserts `PacketType:0:2` at pre-20080102. GCC confirms: packetType at offset 0, size 2. ✅

### PACKET_ZC_ACCEPT_ENTER (4 breakpoints)

| PACKETVER | Assertions | GCC Output | Verdict |
|-----------|-----------|------------|---------|
| 20070101 | `packetType:0:2`, `startTime:2:4`, `posDir:6:3`, `xSize:9:1`, `ySize:10:1` | all match | ✅ CORRECT |
| 20080102 | `font:11:2` | font=11 | ✅ CORRECT |
| 20141022 | `font:11:2`, `sex:13:1` | 11, 13 | ✅ CORRECT |
| 20160330 | `font:11:2` | font=11 | ✅ CORRECT |

### common/packets.hpp structs (totals only, no field spot-checks)

All totals confirmed: PACKET_AC_ACCEPT_LOGIN_sub (32/160), CHARACTER_INFO (147/155), PACKET_HC_NOTIFY_ZONESVR (28/156). ✅

## Summary

**Zero errors found.** Every single `field:offset:size` triplet in `phase1_gate.sh` section 6 matches GCC ground truth exactly.

- Total spot-checks verified: ~50 individual field triplets across 29 struct/packetver combinations
- No corrections needed
- Gate script remains at 76 PASS / 1 FAIL (the 1 FAIL is the expected CH_MAKE_CHAR shuffle table issue, documented and deferred to Phase 3)

## Gate Status

**Phase 1 pre-implementation gate: READY TO PASS**

The single remaining FAIL (`Char/0x0065` CH_MAKE_CHAR) is a known, documented limitation of the shuffle table not being handled yet. It does not block Phase 1 implementation.

## Next Step

Proceed to Phase 1 implementation per `README-LLM.md`.
