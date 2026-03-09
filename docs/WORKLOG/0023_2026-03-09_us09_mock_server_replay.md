# 0023 — 2026-03-09 — US-09: Mock Server Replay from Captured Dumps

## Summary

US-09 is complete. Two replay tests now drive `ConnectionFSM.Connect()` against
`ScriptedServer` — a fixture-based mock that replays captured rAthena wire bytes
over `net.Pipe`. Tests are deterministic, require no Docker container, and run as
part of the standard `go test ./pkg/fsm/` suite.

---

## Context

US-08 proved the FSM works against a live server but requires Docker. US-09 makes
the same coverage available offline by replaying real captured bytes from
DUMP1 and DUMP8_movement (packetver 20200401, captured against a real rAthena
instance at `192.168.5.105`).

---

## Architecture

### Fixture file format

Binary format written by `cmd/gen-fixture/main.go`:

```
[4 bytes: magic "RATF"]
[4 bytes: uint32 LE version = 1]
[4 bytes: uint32 LE packetver]
[phase block × 3:]
  [1 byte: phase tag  0x01=login  0x02=char  0x03=map]
  [4 bytes: uint32 LE byte count N]
  [N bytes: raw S→C bytes for this phase]
[4 bytes: magic "END "]
```

### Phase boundary detection

`cmd/gen-fixture` scans OpenKore dump files line by line:
- `>> Sent 0x0065` (CH_ENTER) → transition login → char phase
- `>> Sent 0x0436` (CZ_ENTER) → transition char → map phase
- `<< Received` lines accumulate bytes into the current phase's buffer

### ScriptedServer (`pkg/fsm/scriptedserver_test.go`)

`ScriptedServer` provides a `Dialer` that serves the three phases in order
over `net.Pipe` connections. For each phase:

1. **Drain initial C→S packet** (synchronous, to unblock the FSM's first `Write`
   on the synchronous `net.Pipe`). Sizes: login=55, char=17, map=19 bytes.
2. **Write 4-byte account ID echo** (char phase only, replicating `char_clif.cpp:851-853`)
3. **Start background drain goroutine** for remaining C→S traffic so writes don't deadlock
4. **Write S→C bytes** in random chunks (1–4096 bytes), seeded from `t.Name()` via FNV-64a
   hash for determinism

### Replay tests (`pkg/fsm/replay_test.go`)

**`TestReplay_FullAuth_20200401`** — uses `auth_20200401.fixture` (from DUMP1):
- Connects FSM, verifies `OnReady` fires
- Post-OnReady: feeds remaining S→C bytes for up to 5 seconds
- Asserts `stat_update` or `actor_exists` fires with non-zero fields
- Result: `gotStatUpdate=true, gotActorExists=true, feedErrors=0`

**`TestReplay_Movement_20200401`** — uses `movement_20200401.fixture` (from DUMP8_movement):
- Same auth flow, different map phase bytes
- Asserts `stat_update` or `actor_exists` fires
- Result: `gotStatUpdate=true, gotActorExists=false, feedErrors=0`
  (DUMP8_movement has `0x09FF` actor_exists but no `StatUpdate` handlers fired
  until post-OnReady feed which only gets `0x00B0` stat packets, not actor packets)

---

## Key Discovery: 0x07FB with length=0 at pv >= 20191120

`lengths_map.go` sets `0x07FB = 0` for packetver >= 20191120 (the codegen sets
zero = "disabled"). But DUMP1 (captured at packetver 20200401) contains a `0x07FB`
packet at 25 bytes — the real server sends it.

This is a codegen gap: the packets.hpp conditional for `0x07FB` says it was changed
or removed for new versions, but the actual server binary (Docker image
`ghcr.io/lenaxia/docker-rathena:packetver20200401-renewal-20251223`) still emits it.

**Fix**: pre-register `0x07FB = 25` via `sess.SetLength(0x07FB, 25)` inside
`setupHandlers` in `TestReplay_FullAuth_20200401`. Documented with source comment.

This is the only `SetLength` workaround needed for the replay tests.

---

## Fixture Verification

Both fixtures verified by hand:

| Fixture | Login | Char | Map | First login pkt | First char pkt | First map pkt |
|---------|-------|------|-----|-----------------|----------------|---------------|
| auth_20200401.fixture | 224B | 866B | 4201B | 0x0AC4 ✅ | 0x082D ✅ | 0x0283 ✅ |
| movement_20200401.fixture | 224B | 866B | 3857B | 0x0AC4 ✅ | 0x082D ✅ | 0x0283 ✅ |

---

## Acceptance Criteria — Verified

| Criterion | Result |
|-----------|--------|
| `cmd/gen-fixture/main.go` parses DUMP1; key packets verified | ✅ |
| `auth_20200401.fixture` and `movement_20200401.fixture` in testdata/ | ✅ |
| `ScriptedServer` replays each phase with random-chunk splitting | ✅ |
| `TestReplay_FullAuth_20200401` passes (no build tag) | ✅ |
| `TestReplay_Movement_20200401` passes | ✅ |
| `Feed()` does not fault (feedErrors=0 in both) | ✅ |
| SetLength workaround documented (0x07FB=25) | ✅ |
| Tests are deterministic: `go test -count=10 ./pkg/fsm/` passes | ✅ |
| `go test -race ./pkg/fsm/` passes | ✅ |
| `go build ./...` passes | ✅ |
| `go test ./...` passes | ✅ |

---

## Files Created/Modified

| File | Change |
|------|--------|
| `cmd/gen-fixture/main.go` | New — OpenKore dump parser + .fixture writer |
| `pkg/fsm/testdata/auth_20200401.fixture` | New — login+char+map S→C bytes from DUMP1 |
| `pkg/fsm/testdata/movement_20200401.fixture` | New — login+char+map S→C bytes from DUMP8_movement |
| `pkg/fsm/scriptedserver_test.go` | New — ScriptedServer, loadFixture, helpers |
| `pkg/fsm/replay_test.go` | New — TestReplay_FullAuth_20200401, TestReplay_Movement_20200401 |

---

## EPIC-01 Status

Both US-08 (live) and US-09 (replay) are complete. EPIC-01 exit criteria:

1. ✅ `go test -tags integration -timeout 60s ./pkg/fsm/` (US-08, requires Docker)
2. ✅ `go test -count=10 ./pkg/fsm/` (US-09, no Docker, deterministic)
3. ✅ `go test -race ./pkg/fsm/`
4. ✅ `go build ./...`
5. ✅ Non-zero decoded events asserted in both tests
6. ✅ All SetLength workarounds documented (0x07FB=25 in replay test)
7. ✅ Worklogs written for US-08 (0020) and US-09 (0023)
