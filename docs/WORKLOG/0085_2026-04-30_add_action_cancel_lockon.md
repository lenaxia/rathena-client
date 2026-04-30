# Work Log 0085 — Add ActionCancelLockon (CZ_CANCEL_LOCKON 0x0118)

**Date**: 2026-04-30
**Release**: v0.6.5 (pending)
**Type**: Feature — add missing send action
**Scope**: `semantics/mappings.yaml` (via MCP), `internal/codegen/stubs/synthetic_structs.hpp`,
           `pkg/session/actions.go` (regenerated), `pkg/encode/register.go` (regenerated),
           `pkg/encode/cancel_lockon.go` (generated), `pkg/send/cancel_lockon.go` (generated),
           `pkg/encode/cancel_lockon_test.go` (new)
**Severity**: BLOCKING (goKore) — goKore needs this packet to avoid crashing the rAthena
              map-server during concurrent combat. Without CZ_CANCEL_LOCKON sent at
              end-of-combat, the server's `ud->target` stays pointing at the dead mob ID,
              and the next `unit_attacktimer` callback null-derefs when `map_id2bl`
              returns NULL for the freed target. Documented extensively in goKore
              work logs `docs/work_logs/0883`–`0888` and `0890`.

---

## Problem

`CZ_CANCEL_LOCKON` (0x0118) had no SemanticAction constant in `pkg/session/actions.go`,
no `send.CancelLockon` struct, no `EncodeCancelLockon` function, and no
`RegisterSendEncoder` call. Any code calling
`session.Send(ms, conn, session.ActionCancelLockon, send.CancelLockon{})` would fail at
compile time because the symbols don't exist.

goKore's `CombatAction` was never sending this packet when combat ended (target dies,
switches, or gives up). The server-side effect: stale `ud->target` in
`map_session_data`. When `unit_attacktimer` fires next, `map_id2bl(ud->target)` returns
NULL for the dead target and the deref crashes with SIGSEGV.

OpenKore sends this packet extensively (found 8 call sites in `~/personal/openkore/src/`,
including `AI/Attack.pm:78,163,192,286` and `AI/CoreLogic.pm:2345`) — that's why
OpenKore-driven bots don't crash rAthena.

This is the same gap class as `ActionWhisper` (worklog 0076) and the chat actions
(worklog 0077): the DB had no entry, so codegen emitted nothing.

---

## Pre-Implementation Gate

### GCC verification

Packet ID and handler are registered in `clif_packetdb.hpp`:

```
~/personal/rathena/src/map/clif_packetdb.hpp:115
parseable_packet(0x0118, 2, clif_parse_StopAttack, 0);
```

Wire format: **2-byte header only, no payload, not shuffled**.

Server handler (`~/personal/rathena/src/map/clif.cpp:12575-12581`):

```cpp
void clif_parse_StopAttack(int32 fd, map_session_data *sd)
{
    unit_stop_attack( sd );
    sd->ud.state.attack_continue = 0;
}
```

The handler clears the target pointer and the attack-continue flag, which is exactly
what prevents the stale-target SIGSEGV on the next attack timer tick.

### Shuffle check

`grep "0x0118" ~/personal/rathena/src/map/clif_shuffle.hpp` — no entries. The packet
ID is stable across all PACKETVERs.

### DB query (via MCP)

`semantics_get("0x0118")` before implementation — **not found**. No packet entry
existed at all. Not even as a NOOP or receive-direction entry.
`semantics_noop_get("0x0118")` — also not found.

---

## Fix (TDD)

### 1. Tests FIRST (`pkg/encode/cancel_lockon_test.go`)

```go
func TestEncodeCancelLockon_WireFormat(t *testing.T) { ... }
func TestActionCancelLockon_Registered(t *testing.T) { ... }
func BenchmarkEncodeCancelLockon(b *testing.B) { ... }
```

Both tests fail initially (`ActionCancelLockon` undefined, `send.CancelLockon`
undefined, `encode.EncodeCancelLockon` undefined).

### 2. Added `SYNTH_CZ_CANCEL_LOCKON` stub

`internal/codegen/stubs/synthetic_structs.hpp` (added after the existing
`SYNTH_CZ_NOTIFY_ACTORINIT` cluster of 2-byte no-payload stubs):

```cpp
// 0x0118 CZ_CANCEL_LOCKON — Stop attacking / cancel target lock-on
// parseable_packet(0x0118, 2, clif_parse_StopAttack, 0)
// Source: clif_packetdb.hpp:115
// Server handler (clif.cpp:12577): unit_stop_attack(sd) + sd->ud.state.attack_continue = 0
// No C struct in rAthena (no payload — the packet is 2 bytes of header and nothing else).
// Stub present so codegen emits ActionCancelLockon constant and encoder registration.
// Length: 2
struct SYNTH_CZ_CANCEL_LOCKON {
    int16 PacketType;
} __attribute__((packed));
```

### 3. DB entries via MCP

```
semantics_create_action(
    "cancel_lockon",
    "Stop attacking / cancel target lock-on. Sent when combat ends...",
    "sendAttackStop")
semantics_add_implementation("cancel_lockon", "0x0118", "SYNTH_CZ_CANCEL_LOCKON")
semantics_add(
    packet_id="0x0118",
    direction="send",
    rathena_struct="SYNTH_CZ_CANCEL_LOCKON",
    openkore_name="stopattack",
    category="combat",
    description="...",
    validated_by="clif_packetdb.hpp:115 ...")
semantics_add_field(
    packet_id="0x0118", position=0,
    rathena_name="PacketType", rathena_type="int16",
    openkore_name="PacketType", semantic="Packet ID header (0x0118)",
    omit_from_openkore=true)
```

### 4. Ran codegen

```bash
go run ./internal/codegen/main.go \
    --rathena ~/personal/rathena \
    --semantics semantics/mappings.yaml \
    --out .
```

Emitted:

- `pkg/session/actions.go`: `ActionCancelLockon SemanticAction = 31` + `String()` case
- `pkg/send/cancel_lockon.go`: `type CancelLockon struct {}` (empty — no payload)
- `pkg/encode/cancel_lockon.go`: `func EncodeCancelLockon(req send.CancelLockon, packetver uint32) [2]byte`
  with the 2-byte header `[0x18, 0x01]` (little-endian 0x0118)
- `pkg/encode/register.go`: `session.RegisterSendEncoder(session.ActionCancelLockon, ...)`
  using the fixed-size `[2]byte` path

All other existing action constants stayed the same offset. `ActionCancelLockon`
took slot 31 (previously unused).

---

## Test Results

```
$ go test -run "TestEncodeCancelLockon_WireFormat|TestActionCancelLockon_Registered" -v ./pkg/encode/
=== RUN   TestEncodeCancelLockon_WireFormat
--- PASS: TestEncodeCancelLockon_WireFormat (0.00s)
=== RUN   TestActionCancelLockon_Registered
--- PASS: TestActionCancelLockon_Registered (0.00s)
PASS
```

## Benchmark

```
BenchmarkEncodeCancelLockon-14    1000000000    0.2546 ns/op    0 B/op    0 allocs/op
```

**0 allocs/op** — meets the performance contract for fixed-size encoders.

## Full Test Suite

```
$ go test ./...
ok    github.com/lenaxia/rathena-client/internal/codegen/gen          0.024s
ok    github.com/lenaxia/rathena-client/internal/codegen/preprocess   0.352s
ok    github.com/lenaxia/rathena-client/internal/codegen/semantics    0.098s
ok    github.com/lenaxia/rathena-client/pkg/decode                    0.019s
ok    github.com/lenaxia/rathena-client/pkg/encode                    0.019s
ok    github.com/lenaxia/rathena-client/pkg/packing                   0.005s
ok    github.com/lenaxia/rathena-client/pkg/session                   0.181s
```

`go test -race ./...` clean. `grep -r "^\s*go " pkg/` shows only test files (pre-existing,
not production code).

---

## goKore integration (next)

After this is tagged v0.6.5:

1. goKore bumps `go.mod`: `require github.com/lenaxia/rathena-client v0.6.5`
2. goKore adds `CombatBuilder.BuildStopAttackDirect() error` that calls
   `session.Send(cfg.Session, cfg.Conn, session.ActionCancelLockon, send.CancelLockon{})`
3. goKore's `CombatAction` calls `sender.StopAttack()` when:
   - Target dies (ActionComplete path)
   - Target becomes invalid (ActionFailed path)
   - Bot switches to a new target
   - Bot gives up on combat (timeout / AI state change)
4. 200-bot validation with loot enabled to confirm the crash no longer occurs.

---

## Design notes

### Why an empty send struct?

`send.CancelLockon` has no fields because the wire packet has no payload. This is
consistent with how the codegen handles other 2-byte no-payload send actions like
`SYNTH_CZ_CONCLUDE_EXCHANGE_ITEM` (ActionEndTrade) and `SYNTH_CZ_CLOSE_STORE`. An empty
struct value in Go is zero-sized — costs nothing at the call site.

### Why 0 allocs?

`EncodeCancelLockon` returns `[2]byte` (fixed-size array, not `[]byte` slice). The
Go compiler lays the 2-byte result on the caller's stack. No heap allocation is
needed. `session.Send` receives it via `b := Encode...(req, pv); return b[:], nil`
in the generated registration — the slice header references the stack array for the
lifetime of the call (which only spans the conn.Write).
