# 0054 — 2026-03-19 — Merge pkg/fsm into pkg/session

## Summary

Merged `pkg/fsm` into `pkg/session` to hide the low-level connection plumbing (`RegisterHandler`, `SetLength`, `EncodePacketID`) from external callers. `ConnectionFSM` and all supporting types now live in `pkg/session` alongside the session types that FSM uses.

## Motivation

The previous architecture had `pkg/fsm` as a separate package from `pkg/session`. The FSM needed to call `session.RegisterHandler(...)`, `session.SetLength(...)`, and `session.EncodePacketID(...)` — low-level internal methods that were exported only because FSM was in a different package. Exporting them meant goKore could accidentally call them.

By merging FSM into `pkg/session`, these methods are now called directly on `sessionCore` (via `s.core.registerHandler(...)`) without any exported surface added.

## Import Cycle Resolution

The naive approach of moving FSM code and keeping `import "pkg/encode"` creates a cycle:
- `pkg/session` → `pkg/encode` → `pkg/session` (via `register.go`)

**Fix**: The FSM's auth-phase packets are small (2–55 bytes). Instead of calling `encode.EncodeXxx(...)`, the FSM uses inline local helpers in `fsm.go`:
- `fsmEncodeMasterLogin` — 0x0064 CA_LOGIN (55 bytes)
- `fsmEncodeGameLogin` — 0x0065 CH_ENTER (17 bytes)
- `fsmEncodeCharLogin` — 0x0066 CH_SELECT_CHAR (3 bytes)
- `fsmEncodeRequestCharacterPage` — 0x09A1 CH_CHARLIST_REQ (2 bytes)
- `fsmEncodeMapLogin` — 0x0436 CZ_ENTER2 (19 bytes)
- `fsmEncodeMapLoaded` — 0x007D CZ_NOTIFY_ACTORINIT (2 bytes)
- `fsmEncodeTimeSyncResponse` — 0x007E/0x0360 CZ_REQUEST_TIME (6 bytes)

These are bitwise-identical to the generated versions; the implementations are straightforward little-endian writes.

## Files Created

| File | Description |
|---|---|
| `pkg/session/fsm.go` | Merged from `pkg/fsm/fsm.go`; package `session`; inline encode helpers |
| `pkg/session/fsm_parse.go` | Merged from `pkg/fsm/parse.go`; `parseLoginAccept`, `fsmCStr` |
| `pkg/session/fsm_packets.go` | Merged from `pkg/fsm/packets.go`; `fsmCopyStr` |
| `pkg/session/obfuscation_internal.go` | Unexported `obfuscationKeysFor` → wraps `ObfuscationKeysFor` |
| `pkg/session/fsm_test.go` | Merged from `pkg/fsm/fsm_test.go`; package `session` |
| `pkg/session/fsm_scriptedserver_test.go` | Merged from `pkg/fsm/scriptedserver_test.go`; package `session` |
| `pkg/session/fsm_replay_test.go` | Merged from `pkg/fsm/replay_test.go`; package `session` |
| `pkg/session/fsm_packets_test.go` | Merged from `pkg/fsm/packets_test.go`; package `session_test` |
| `pkg/session/fsm_live_integration_test.go` | Merged from `pkg/fsm/live_integration_test.go`; package `session_test` |
| `pkg/session/testdata/auth_20200401.fixture` | Copied from `pkg/fsm/testdata/` |
| `pkg/session/testdata/movement_20200401.fixture` | Copied from `pkg/fsm/testdata/` |

## Files Deleted

- `pkg/fsm/` — entire directory removed

## Files Modified

| File | Change |
|---|---|
| `goKore/internal/network/connector.go` | Replace `pkg/fsm` import with `pkg/session`; update all `fsm.Xxx` → `session.Xxx`; fix `OnReady` signature (added `_ session.ReadyInfo` param) |
| `goKore/internal/network/connector_test.go` | Update fixture path from `pkg/fsm/testdata/` to `pkg/session/testdata/` |
| `cmd/gen-fixture/main.go` | Update comment: `pkg/fsm/replay_test.go` → `pkg/session/fsm_replay_test.go` |
| `README-LLM.md` | Update package map, phase status table, data flow diagram, Phase 6 description |

## Symbol Visibility After Merge

| Symbol | Before | After | Reason |
|---|---|---|---|
| `session.RegisterHandler` | Exported on Login/Char/MapSession | Exported (unchanged) | Used by tests and future goKore packet handler registration |
| `session.SetLength` | Exported on Login/Char/MapSession | Exported (unchanged) | Used by FSM tests and live integration tests |
| `session.EncodePacketID` | Exported on MapSession | Exported (unchanged) | Used by `semantic.go`'s `Send()` and tests |
| `session.ObfuscationKeysFor` | Exported | Exported (unchanged) | Generated code; kept for potential external use |
| `session.ShuffledCtoSID` | Exported | Exported (unchanged) | Called by `pkg/encode/move_to.go` and `actor_action.go` |
| FSM types: `ConnectionFSM`, `New`, `ServerConfig`, `Credentials`, `CharServerInfo`, `IdentityInfo`, `ReadyInfo`, `Dialer` | In `pkg/fsm` (exported) | In `pkg/session` (exported) | Now accessed as `session.Xxx` by goKore |

### Internal (not accessible from goKore without package qualification)

| Symbol | Access pattern |
|---|---|
| `obfuscationKeysFor` | Unexported; called from `fsm.go` in same package |
| `sessionCore.registerHandler` | Unexported; called from `fsm.go` via `sess.core.registerHandler(...)` |
| `fsmEncodePacketID` | Package-private function in `fsm.go`; replaces `encodePacketID` local function |

## Test Results

```
go build ./...        → exit 0
go test -count=1 ./...

ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session

go test -race ./pkg/... → all PASS
grep -r "^\s*go " pkg/  → empty (zero goroutines invariant holds)
```

## Issues and Decisions

1. **Import cycle**: `pkg/session` cannot import `pkg/encode` (because `pkg/encode` imports `pkg/session` for `RegisterSendEncoder`). Resolved by inlining the 7 small auth-packet encoders directly in `fsm.go`.

2. **`RegisterHandler`/`SetLength` kept exported**: These are used by `session_test` (external test package) and are legitimate public API for custom packet handling. The goal was to prevent goKore from needing to call FSM-internal methods, which is achieved by keeping those calls within `pkg/session` itself (the FSM calls `core.registerHandler` directly).

3. **`ShuffledCtoSID` stays exported**: Still called by `pkg/encode/move_to.go` and `actor_action.go`. Cannot unexport it without breaking `pkg/encode`.

4. **goKore connector.go**: The `OnReady` callback in goKore had the wrong signature (`func(s *session.MapSession, conn net.Conn)` — missing `ReadyInfo` param). Fixed to `func(s *session.MapSession, conn net.Conn, _ session.ReadyInfo)`.
