# 0002 — Phase 1 Gate Verification Script Creation

**Date**: 2026-03-06
**Scope**: Create systematic verification script to validate all HLD packet ID claims against rAthena source before implementation.

---

## Goal

Create `validation/phase1_gate.sh` that:
1. Runs GCC preprocessor for all Phase 1 packets
2. Extracts packet ID definitions
3. Compares against HLD §13 claims
4. Outputs pass/fail report

**Why**: The HLD audit found 30 issues caused by prose describing data structures without machine verification. This script prevents that class of error from reaching implementation.

---

## Verification Script Created

`validation/phase1_gate.sh` - 540 lines

Checks:
- **Login packets**: 0x0064, 0x0069, 0x0AC4, 0x006A, 0x083E
- **Char packets**: 0x0065, 0x082D, 0x006B, 0x09A0, 0x099D, 0x0066, 0x0081, 0x0AC5, 0x006C
- **Map packets**: 0x0436, 0x0283, 0x0073, 0x0A18, 0x007D, 0x007E, 0x0360, 0x0074
- **Actor packets**: 0x0078, 0x01D8, 0x09FF, 0x007B, 0x0080
- **Stat packets**: 0x00B0, 0x00B1
- **Send packets**: 0x0085
- **Ping packets**: 0x0B1D, 0x0B1C
- **Struct existence**: CHARACTER_INFO, PACKET_AC_ACCEPT_LOGIN_sub, PACKET_HC_NOTIFY_ZONESVR, packet_authok, packet_idle_unit
- **Algorithm verification**: WBUFPOS/RBUFPOS, direction enum

---

## Run Results

### First Run (before fix): 38 failures
- Root cause: Script was looking for `DEFINE_PACKET_HEADER` macro
- Actual preprocessor output uses: `const int16 HEADER_<NAME> = 0xXX;`

### Second Run (after grep fix): 14 passes, 24 failures

**PASSES** (14):
- Login/0x0064: CA_LOGIN = 0x64 ✓
- Login/0x006A: AC_REFUSE_LOGIN exists (version-specific) ✓
- Login/0x083E: AC_REFUSE_LOGIN (modern) = 0x83E ✓
- Char/0x082D: HC_ACCEPT_ENTER2 = 0x82D ✓
- Char/0x006B: HC_ACCEPT_ENTER = 0x6B ✓
- Char/0x09A0: HC_CHARLIST_NOTIFY = 0x9A0 ✓
- Char/0x099D: HC_ACK_CHARINFO_PER_PAGE = 0x99D ✓
- Char/0x0066: CH_SELECT_CHAR = 0x66 ✓
- Char/0x0081: HC_NOTIFY_ZONESVR = 0x81 (old) ✓
- Char/0x0AC5: HC_NOTIFY_ZONESVR = 0xAC5 (modern) ✓
- Char/0x0081-SC_NOTIFY_BAN: SC_NOTIFY_BAN = 0x81 ✓
- Char/0x006C: HC_REFUSE_ENTER = 0x6C ✓
- Algo/WBUFPOS: WBUFPOS/RBUFPOS found in clif.cpp ✓
- Algo/WBUFPOS2: WBUFPOS2/RBUFPOS2 found in clif.cpp ✓
- Algo/Direction: DIR_NORTH=0 found in path.hpp ✓

**FAILURES** (24):
- Login/0x0069: AC_ACCEPT_LOGIN = 0xAC4 at PACKETVER=20180307 (expected 0x69)
  - **EXPLANATION**: At PACKETVER >= 20170315, AC_ACCEPT_LOGIN uses 0xAC4, not 0x69
  - **HLD IS CORRECT**: HLD §13 says "0x0069, 0x0AC4" — both IDs are valid
  
- Login/0x0AC4: Not found as separate entry
  - **EXPLANATION**: Same struct, just different ID based on PACKETVER
  
- Char/0x0065: CH_MAKE_CHAR = 0xA39 at PACKETVER=20180307
  - **ISSUE**: HLD §18 says "0x0065 is sent for char connect" but at modern PACKETVER, it's shuffled
  - **NEEDS CLARIFICATION**: Is 0x0065 the base ID or the shuffled ID?

- Map packets (0x0436, 0x0283, etc.): All returning empty ACTUAL
  - **ROOT CAUSE**: The `map/packets.hpp` preprocessing outputs to wrong filename
  - Script uses `basename "$header"` which produces `packets_20180307.h` for both common and map
  - **FIX NEEDED**: Use distinct output filenames for each header

- Struct checks: All failing because grep pattern `^struct NAME{` doesn't match
  - **ROOT CAUSE**: Preprocessor output may have different whitespace/formatting

---

## Issues Found

### Issue 1: Output filename collision
```
common/packets.hpp → gate/packets_20180307.h
map/packets.hpp    → gate/packets_20180307.h  (OVERWRITES!)
```

Need to prefix with source directory.

### Issue 2: Login/0x0069 vs 0x0AC4
At PACKETVER >= 20170315, `AC_ACCEPT_LOGIN` is 0xAC4.
The script checks for 0x0069 at PACKETVER=20180307 which is wrong.
Should check 0x0069 at PACKETVER < 20170315.

### Issue 3: Char connect packet ID
HLD §18 says "0x0065" for char connect, but at PACKETVER=20180307 it's 0xA39.
This may be due to packet shuffling — need to check `clif_shuffle.hpp`.

### Issue 4: Struct grep pattern
`struct_exists()` uses `^struct NAME{` but actual format may have spaces.
Need to check actual preprocessor output format.

---

## Manual Verification Done

Confirmed correct via `validation/preprocess_check.sh`:
```
packets_struct.hpp ... OK (393 structs)
packets.hpp ... OK (641 structs)
common/packets.hpp ... OK (131 structs)
clif_obfuscation.hpp ... OK (1 key definitions)
```

Manual grep confirms:
```bash
$ grep "HEADER_CA_LOGIN\s*=" validation/output/common_packets_20180307.h
const int16 HEADER_CA_LOGIN = 0x64;
```

---

## Next Steps

1. **Fix output filename collision** - prefix with source dir
2. **Fix PACKETVER-specific checks** - use appropriate PACKETVER for each packet ID
3. **Fix struct grep pattern** - handle whitespace variations
4. **Re-run until 100% pass**
5. **Only then proceed to implementation**

---

## Key Insight

This is exactly why we need the verification gate. The script found:
- Packet ID versioning issues (0x0069 vs 0xAC4)
- Possible shuffle ID issues (0x0065 vs 0xA39)
- Output file overwrites

Better to discover these NOW than during implementation.

---

## Files Created

| File | Status |
|------|--------|
| `validation/phase1_gate.sh` | Created, needs fixes |
| `validation/phase1_gate_report.md` | Generated (24 failures) |
| `docs/WORKLOG/0002_...` | This file |
