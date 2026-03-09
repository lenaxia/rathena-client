# Worklog 0020 — US-08: Live Server Integration Test

**Date**: 2026-03-09  
**Session focus**: Implement US-08 — live integration test that connects to real rAthena
Docker container, authenticates, navigates login→char→map, and asserts at least one
`ActorExists` or `StatUpdate` event fires within 10 seconds.

---

## Context

Previous sessions implemented the FSM (`pkg/fsm/`), session framing (`pkg/session/`),
and packet decode/event pipeline. US-08 is the first test that exercises the full stack
against a running server.

---

## Key Discovery: 4-byte Account ID Echo from Char Server

The most critical bug: **rAthena's char server sends a raw 4-byte account ID immediately
after receiving `0x0065` (CH_ENTER)**, before any normal framed packets.

Source: `char_clif.cpp:851-853`:
```c
WFIFOHEAD(fd,4);
WFIFOL(fd,0) = account_id;
WFIFOSET(fd,4);
```

The FSM's `runCharPhase()` was feeding these 4 bytes directly into the char session
framer, which interpreted them as a 2-byte packet ID `0x8483` (= first 2 bytes of
account ID 2000003 = `0x001E8483` in little-endian). This caused `ErrUnknownPacket`
and immediate test failure.

**Fix**: call `io.ReadFull(conn, make([]byte, 4))` to consume and discard the echo
**before** starting the CharSession framer, in `pkg/fsm/fsm.go:runCharPhase()`.

The existing unit tests in `fsm_test.go` did NOT send this echo (so they passed), but
needed to be updated to send it too, to match real server behavior.

---

## Changes Made

### 1. `pkg/fsm/fsm.go` — consume 4-byte echo in `runCharPhase()`

Added `io.ReadFull` call immediately after dialing the char server and before creating
the CharSession framer.

### 2. `pkg/fsm/fsm_test.go` — add echo to all 7 charScript functions

Added `writeAccountIDEcho(t, conn, aid)` helper and called it in every charScript
(and `mkChar` closure in `TestConnect_Reconnect`) after draining the CH_ENTER packet.

The missing call in `TestConnect_Reconnect.mkChar` caused `TestConnect_Reconnect` to
fail with `unknown packet 0x0000` — fixed in this session.

### 3. `pkg/fsm/fsm.go` — add missing `SetLength` calls to `runMapPhase()`

The map session faults permanently after the first unknown packet ID. Many packets
arrive in the initial burst before `OnReady` fires, so all needed lengths must be
registered before `feedUntil()` is called.

Added `SetLength` calls for all packets observed in DUMP8_movement and rAthena struct
analysis:

| Packet | Length | Description |
|--------|--------|-------------|
| 0x007F | 6 | ZC_NOTIFY_TIME |
| 0x0087 | 12 | movement ack |
| 0x0091 | 22 | ZC_NPCACK_MAPMOVE |
| 0x00B0 | 8 | ZC_PAR_CHANGE (StatUpdate) |
| 0x00B1 | 8 | ZC_LONGPAR_CHANGE |
| 0x00BD | 44 | ZC_STATUS (initial stats) |
| 0x008E | -1 | variable (NPC chat) |
| 0x010F | -1 | ZC_SKILLINFO_LIST (variable) |
| 0x013A | 4 | ZC_ATTACK_RANGE |
| 0x0141 | 14 | ZC_STATUS_CHANGE2 |
| 0x02C9 | 3 | (observed in dump) |
| 0x02DA | 3 | (observed in dump) |
| 0x0ACB | 12 | ZC_LONGLONGPAR_CHANGE (fixed) |
| 0x09A0 | 6 | (observed in dump) |
| 0x0ADE | 6 | (observed in dump) |
| 0x0ADF | 58 | ZC_ACK_REQNAMEALL_NPC (fixed) |
| 0x0B08 | -1 | ZC_INVENTORY_START (variable) |
| 0x0B09 | -1 | ZC_INVENTORY_DATA (variable) |
| 0x0B0A | -1 | ZC_INVENTORY_DATA_EQUIP (variable) |
| 0x0B0B | 4 | ZC_INVENTORY_END |
| 0x0B1B | 2 | ZC_NOTIFY_ACTORINIT (fixed) |
| 0x0B20 | 271 | ZC_SHORTCUT_KEY_LIST (fixed) |
| 0x0A23 | -1 | ZC_ACHIEVEMENT_LIST (variable) |

Note: EPIC-01 listed 0x0B1B, 0x0ACB, 0x0B20, 0x0ADF as variable-length; struct
analysis and dump verification show they are **fixed-length**. Used correct fixed sizes.

### 4. `pkg/fsm/live_integration_test.go` — NEW FILE (created in prior session)

Build tag `//go:build integration`. Test function: `TestLiveServer_FullAuthSequence`.
Connects to `127.0.0.1:6900`, authenticates as `botijo1`/`Melon.77`, packetver
`20200401`, waits up to 10 seconds for any `ActorExists` or `StatUpdate` event.

---

## Test Results

```
--- PASS: TestLiveServer_FullAuthSequence (5.07s)
    live_integration_test.go:209: Feed calls: 2, feed errors: 0, gotActorExists: false, gotStatUpdate: true
```

`gotStatUpdate: true` — a `ZC_PAR_CHANGE` (0x00B0) stat packet arrived and was decoded
into a `StatUpdate` event within 5 seconds of connecting.

All non-integration tests also pass:

```
ok  github.com/lenaxia/ragnarok-go-client/pkg/fsm   1.079s
```

---

## US-08 Status: COMPLETE
