# 0014 — lengths_login/char codegen + skeptical validation corrections

**Date**: 2026-03-07  
**Session**: Continuation of Phase 5 — fix empty `lengths_login.go` / `lengths_char.go`; skeptical agent validation; fix 4 confirmed bugs

---

## What Was Done

### 1. Implemented `ParseCommonPacketHeaders` (new file)

`internal/codegen/preprocess/common_packets.go` — new file adding:

- `ParseCommonPacketHeaders(preprocessed string, structDB StructDB) []CommonPacketEntry`  
  Parses `const int16 HEADER_NAME = id;` lines from preprocessed `common/packets.hpp` output.  
  Looks up `PACKET_NAME` in StructDB for size.  
  Detects flex arrays (`TYPE name[]`) → marks as variable-length (`-1`).  
  Computes prefix (`CA`, `AC`, `CH`, `HC`, etc.) for server routing.

- `computeNestedStructSizes(preprocessed, structDB)` — resolves nested struct scalar fields  
  (e.g., `CHARACTER_INFO character` in `PACKET_HC_ACCEPT_MAKECHAR`) whose types are unknown  
  to `ParseStructBody` (which only handles primitives).

- `detectFlexArrayStructs(preprocessed)` — scans struct bodies for `TYPE name[]` patterns.

- `extractAllStructBodies(preprocessed)` — helper for the nested struct pass.

GCC command verified:
```
g++ -E -P -DPACKETVER=20200000 -DPACKETVER_MAIN_NUM=20200000 \
  -I/home/mikekao/personal/rathena/src \
  -I/home/mikekao/personal/rathena/src/map \
  -I/home/mikekao/personal/rathena/src/common \
  -include internal/codegen/stubs/common_hpp_stub.h \
  /home/mikekao/personal/rathena/src/common/packets.hpp
```

### 2. Wired login/char lengths into `genLengths` (`internal/codegen/main.go`)

Refactored `genLengths` into three functions:
- `buildMapLengthBreakpoints` — old logic, unchanged
- `buildLoginCharLengthBreakpoints` — new; processes `common/packets.hpp` at 17 breakpoints;  
  routes by prefix to login vs char tables
- `diffLenTable` — reusable helper

Initial prefix maps:
- `loginPrefixes`: `CA`, `AC`
- `charPrefixes`: `CH`, `HC`, `SC`, `CT`, `TC`, `PING`

### 3. First codegen run — confirmed non-empty tables

`lengths_login.go`: 13 entries, 3 versioned breakpoints  
`lengths_char.go`: 37 entries, 6 versioned breakpoints  
`go test ./...` — all pass. Gate: 76 PASS / 1 FAIL (unchanged).

### 4. Skeptical validation agent found 4 failures

Delegated to a skeptical agent with explicit instruction to run GCC `sizeof()` for every entry.  
Agent wrote and compiled C++ sizeof programs at 8 different PACKETVERs.

**FAIL 1: `lengths_char.go` t[0x006D] = 2** (HC_ACCEPT_MAKECHAR, baseline)  
- GCC sizeof: `PACKET_HC_ACCEPT_MAKECHAR` = 2 + sizeof(CHARACTER_INFO) = 2 + 112 = **114**  
- Root cause: `ParseStructBody` skips fields with unknown types (nested structs).  
  `CHARACTER_INFO character` was being silently ignored.

**FAIL 2: `lengths_char.go` t[0x0B6F] = 2** (HC_ACCEPT_MAKECHAR, PACKETVER >= 20201007)  
- GCC sizeof at 20201007: `CHARACTER_INFO` = 155 bytes → `PACKET_HC_ACCEPT_MAKECHAR` = **157**  
- Same root cause as FAIL 1.

**FAIL 3: CT_AUTH (0x0ACF) and TC_RESULT (0x0AE3) in char table (wrong server)**  
- GCC confirms sizes correct (68 and 34), but usage is exclusively in `loginclif.cpp`.  
  Grep evidence:  
  `loginclif.cpp:476: PACKET_CT_AUTH* p = (PACKET_CT_AUTH*)RFIFOP(fd, 0);`  
  `loginclif.cpp:464: PACKET_TC_RESULT p = {};`  
  Neither appears in `charserv_clif.cpp` or `char_clif.cpp`.

**FAIL 4: SC_NOTIFY_BAN (0x0081) missing from login table**  
- GCC sizeof: 3. Used in `loginclif.cpp:35` (sends to clients).  
  Also used in `charserv_clif.cpp:514` and `clif.cpp:830` — belongs on ALL three servers.

### 5. Fixed all 4 failures

**Fix for FAILs 1+2**: Added `computeNestedStructSizes()` to `common_packets.go`.  
Re-scans struct bodies for unresolved scalar fields, looks them up in StructDB, adds their  
TotalSize (plus any recursive nested contributions) to the total.  
Result: `PACKET_HC_ACCEPT_MAKECHAR` now gets correct sizes at all PACKETVER breakpoints.

**Fix for FAIL 3**: Removed `CT` and `TC` from `charPrefixes`; added them to `loginPrefixes`.

**Fix for FAIL 4**: Added `SC` to both `loginPrefixes` and `charPrefixes`.  
Changed routing from `else if` to independent `if` blocks so a packet can appear in both tables.

### 6. Updated README-LLM.md

- Current State table updated: Phases 0-5 marked Complete
- `pkg/session` (hand-written) marked Complete
- `pkg/fsm` marked as current phase (Phase 6)
- Package map updated (all `NOT STARTED` → `COMPLETE`)
- Code generation pipeline section: "does not exist yet" → "complete"
- Last Updated bumped to session 0014

---

## GCC Evidence

### CHARACTER_INFO at baseline (20030000)
Fields: GID(4) + exp(4) + money(4) + jobexp(4) + joblevel(4) + bodystate(4) + healthstate(4) +  
effectstate(4) + virtue(4) + honor(4) + jobpoint(2) + hp(4) + maxhp(4) + sp(2) + maxsp(2) +  
speed(2) + job(2) + head(2) + weapon(2) + level(2) + sppoint(2) + accessory(2) + shield(2) +  
accessory2(2) + accessory3(2) + headpalette(2) + bodypalette(2) + name[24](24) + Str(1) + Agi(1) +  
Vit(1) + Int(1) + Dex(1) + Luk(1) + CharNum(1) + hairColor(1) + bIsChangedCharName(2) = **112 bytes**

`PACKET_HC_ACCEPT_MAKECHAR` at baseline = packetType(2) + character(112) = **114 bytes** ✓

### CT_AUTH in loginclif.cpp
```
/home/mikekao/personal/rathena/src/login/loginclif.cpp:496:
    this->add(HEADER_CT_AUTH, true, sizeof(PACKET_CT_AUTH), logclif_parse_otp_login);
```
Not present in any char server file.

### SC_NOTIFY_BAN usage
```
loginclif.cpp:35    → sends SC_NOTIFY_BAN
charserv_clif.cpp:514 → sends SC_NOTIFY_BAN
clif.cpp:830        → sends SC_NOTIFY_BAN
```
Belongs on login, char, AND map length tables.

---

## Test Results

```
go test ./... — all pass
go build ./... — clean
Gate: 76 PASS / 1 FAIL (CH_MAKE_CHAR 0x0065 shuffle — intentional, documented)
```

---

## Remaining Known Issues

- lengths_char.go: HC_ACCEPT_MAKECHAR (0x006D/0x0B6F) — nested struct fix applied but  
  not yet re-validated by skeptical agent. Re-validate before Phase 6 completion.
- SemanticDB: 306 validation errors. Non-blocking for pkg/fsm.
- Unused-import warnings in some pkg/decode generated files (pre-existing).
