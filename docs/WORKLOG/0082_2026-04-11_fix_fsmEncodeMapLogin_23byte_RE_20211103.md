# Work Log 0082 — Fix fsmEncodeMapLogin: 23-byte packet for PACKETVER_RE >= 20211103

**Date:** 2026-04-11
**Type:** Bug fix — rathena-client pkg/session

---

## Summary

Fixed `fsmEncodeMapLogin` in `pkg/session/fsm.go` to emit a 23-byte map login
packet for PACKETVER in [20211103, 20211118] (i.e. the RE builds where
`PACKETVER_RE_NUM >= 20211103`). Previously the function always returned 19 bytes,
causing rAthena's `clif_parse_WantToConnection_sub` length check to fire and
immediately disconnect every connecting bot.

---

## Root Cause Analysis

### Ingress pipeline (verified in clif.cpp:25646–25784)

rAthena's `clif_parse` reads `cmd = RFIFOW(fd, 0)`, optionally XOR-decrypts it
under `PACKET_OBFUSCATION`, then looks up `packet_db[cmd]`. No further ID
transformation occurs before handler dispatch. `clif_parse_WantToConnection_sub`
checks:

```cpp
// clif.cpp:10625
if( packet_len != packet_db[cmd].len )
    return 1; /* wrong length → set_eof(fd) → disconnect */
```

### packet_db population

`packetdb_readdb()` (clif.cpp:25819) `#include`s `clif_packetdb.hpp` then
`clif_shuffle.hpp`. Both use `parseable_packet(cmd, len, func, ...)` which calls
`packetdb_addpacket` — **last write wins**.

### GCC verification

Commands run with stubs from `internal/codegen/stubs/packets_hpp_stub.h`:

```bash
# RE=20211103 config
g++ -E -P -DPACKETVER=20211103 -DPACKETVER_RE_NUM=20211103 -DPACKETVER_MAIN_NUM=0 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/packets_hpp_stub.h \
    /tmp/clif_trace.cpp 2>/dev/null | grep 0x0436
# → last write: clif_shuffle.hpp:4745  0x0436 23 clif_parse_WantToConnection

# MAIN=20211103 config
g++ -E -P -DPACKETVER=20211103 -DPACKETVER_MAIN_NUM=20211103 -DPACKETVER_RE_NUM=0 ...
# → last write: clif_shuffle.hpp:4747  0x0436 19 clif_parse_WantToConnection
```

### Final packet_db state for RE=20211103 (relevant entries)

| ID     | len | func                         | Source                      |
|--------|-----|------------------------------|-----------------------------|
| 0x0436 | **23** | clif_parse_WantToConnection | clif_shuffle.hpp:4745      |
| 0x0437 | 7   | clif_parse_ActionRequest     | clif_shuffle.hpp:4749       |
| 0x0362 | 6   | clif_parse_TakeItem          | clif_shuffle.hpp:4732       |

Note: `0x0437` (attack) and `0x0362` (pickup) are identical for RE=20211103 and
MAIN=20211103. The work log 0888 claim that these are wrong for RE was incorrect —
both are correct final values after last-write-wins resolution.

### The single real bug

`fsmEncodeMapLogin` hardcoded a `[19]byte` return for all packetvers.
For RE builds with packetver in [20211103, 20211118], rAthena registers
`0x0436` at length 23 (sex at offset 22). Sending 19 bytes triggered
`clif_parse_WantToConnection_sub` to return `1` (wrong length) → `set_eof(fd)` →
every bot disconnected immediately on map entry.

### Field layout of 23-byte variant

```
Source: clif_shuffle.hpp:4745
parseable_packet( 0x0436, 23, clif_parse_WantToConnection, 2, 6, 10, 14, 22 );

offset  0-1:  packet ID   0x0436
offset  2-5:  AID                  (pos[0]=2)
offset  6-9:  GID                  (pos[1]=6)
offset 10-13: AuthCode             (pos[2]=10)
offset 14-17: clientTime (zero)    (pos[3]=14)
offset 18-21: tick (zero)          (extra field vs 19-byte)
offset 22:    sex                  (pos[4]=22)
```

Confirmed by OpenKore `RagexeRE_2021_11_03.pm`:
```perl
'0436' => ['map_login', 'a4 a4 a4 V2 C', [qw(accountID charID sessionID unknown tick sex)]]
```

---

## Changes

### `pkg/session/fsm.go`

- `fsmEncodeMapLogin`: signature changed from `(aid, gid, authCode uint32, sex uint8) [19]byte`
  to `(aid, gid, authCode uint32, sex uint8, packetver uint32) []byte`.
- Returns 23 bytes when `packetver >= 20211103 && packetver <= 20211118`;
  returns 19 bytes otherwise.
- Call site in `runMapPhase` updated: `enterArr := fsmEncodeMapLogin(...); enterPkt := enterArr[:]`
  simplified to `enterPkt := fsmEncodeMapLogin(...)`.

### `pkg/session/fsm_map_login_test.go` (new file)

Internal tests (`package session`) covering:

- `TestFsmEncodeMapLogin_Length_19bytes` — 6 packetver cases that must produce 19 bytes
- `TestFsmEncodeMapLogin_Length_23bytes` — 3 packetver cases that must produce 23 bytes
- `TestFsmEncodeMapLogin_PacketID` — wire ID is always 0x0436
- `TestFsmEncodeMapLogin_Fields_19byte` — field layout at pv=20200401
- `TestFsmEncodeMapLogin_Fields_23byte` — field layout at pv=20211103
- `TestFsmEncodeMapLogin_Boundary` — exact boundary at pv=20211102/20211103/20211118
- `TestFsmEncodeMapLogin_SexPreserved_23byte` — sex byte at offset 22 (not 18)

---

## Test Results

```
go test -count=1 -v -run TestFsmEncodeMapLogin ./pkg/session/...
--- PASS: TestFsmEncodeMapLogin_Length_19bytes (0.00s)  [6 subtests]
--- PASS: TestFsmEncodeMapLogin_Length_23bytes (0.00s)  [3 subtests]
--- PASS: TestFsmEncodeMapLogin_PacketID (0.00s)
--- PASS: TestFsmEncodeMapLogin_Fields_19byte (0.00s)
--- PASS: TestFsmEncodeMapLogin_Fields_23byte (0.00s)
--- PASS: TestFsmEncodeMapLogin_Boundary (0.00s)
--- PASS: TestFsmEncodeMapLogin_SexPreserved_23byte (0.00s)

go test -count=1 ./...
ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session

go test -race -count=1 ./pkg/session/... ./pkg/encode/...
ok  github.com/lenaxia/rathena-client/pkg/session
ok  github.com/lenaxia/rathena-client/pkg/encode

grep -r "^\s*go " pkg/  → (only in test files)
```

---

## Scope Corrections vs Work Log 0888

Work log 0888 claimed:
- `0x0437` is wrong for RE=20211103 (should be `0x088E`) — **INCORRECT**
- `0x0362` is wrong for RE=20211103 (should be `0x07E4`) — **INCORRECT**
- Map login is `0x0888` for RE (different ID) — **INCORRECT**

Actual GCC-verified state for RE=20211103:
- `0x0437` → `clif_parse_ActionRequest` (7 bytes) — **correct** for both RE and MAIN
- `0x0362` → `clif_parse_TakeItem` (6 bytes) — **correct** for both RE and MAIN
- Map login uses `0x0436` for both RE and MAIN post-20180307; the difference is
  **packet length** (23 vs 19 bytes), not packet ID

No changes required to `pkg/encode/shuffle_map.go`, `EncodeActorAction`, or `EncodePickupItem`.
