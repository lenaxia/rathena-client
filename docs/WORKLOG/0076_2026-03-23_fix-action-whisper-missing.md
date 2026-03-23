# 0076 — Fix: ActionWhisper missing from semantic action enum (goKore bug report 0799)

**Date**: 2026-03-23
**Status**: COMPLETE
**Scope**: `pkg/session/actions.go`, `pkg/encode/register.go`, `semantics/mappings.yaml`,
           `internal/codegen/stubs/synthetic_structs.hpp`
**Severity**: BLOCKING — `session.ActionWhisper` undefined; goKore Story 4 (whisper send) could not compile

---

## Problem

`session.ActionWhisper` was undefined. `EncodeWhisper` (`pkg/encode/whisper.go`) and
`send.Whisper` (`pkg/send/whisper.go`) both existed, but there was no `ActionWhisper`
SemanticAction constant and no `RegisterSendEncoder` call for it. Any goKore code calling
`session.Send(..., session.ActionWhisper, send.Whisper{...})` would fail at compile time.

## Root Cause

`pkg/session/actions.go` and `pkg/encode/register.go` are fully generated from the
semantics DB. The `whisper` action **did not exist in the DB at all** — there was no
entry in `semantic_actions`. With no DB entry, codegen emits no constant and no
`RegisterSendEncoder` call, regardless of whether the encoder file exists.

This is the same class of gap that affected `enter_world` before v0.5.8. The bug report
correctly identified the symptom but described the root cause as "encoder created but
never connected" — the actual cause is the DB entry was missing entirely.

The register.go codegen logic (gen/register.go:117) does detect hand-written encoder
files via `existingEncoders`, but `actions.go` generation (gen/actions.go:41) is driven
**only** by the DB — no DB entry → no constant → compile error regardless.

## Fix

### 1. Added `SYNTH_CZ_WISPER` stub to `synthetic_structs.hpp`

CZ_WISPER (0x0096) has no C struct in rAthena (variable-length, parsed manually via
`clif_process_message` with `whisperFormat=true`). A 2-byte stub was added following
the same pattern as `SYNTH_CZ_NOTIFY_ACTORINIT`:

```cpp
struct SYNTH_CZ_WISPER {
    int16 PacketType;
} __attribute__((packed));
```

This allows the VersionTable to resolve the struct so codegen's `isSendStruct` check
passes and the action is not silently dropped.

### 2. Added `whisper` action to semantics DB via MCP

```
semantics_create_action("whisper", "Send a private message (whisper) to another player via CZ_WISPER")
semantics_add_implementation("whisper", "0x0096", struct="SYNTH_CZ_WISPER")
```

### 3. Ran codegen

Codegen emitted:
- `ActionWhisper SemanticAction = 246` in `pkg/session/actions.go`
- `case ActionWhisper: return "ActionWhisper"` in the `String()` switch
- `session.RegisterSendEncoder(session.ActionWhisper, ...)` in `pkg/encode/register.go`
  using the variable-length (`[]byte`) path since `EncodeWhisper` returns `[]byte`

Note: `ActionWhisperSent` shifted from 246 to 247 as a result.

### 4. Tests

`pkg/encode/whisper_test.go` (new, TDD):
- `TestEncodeWhisper_WireFormat` — packet ID `0x0096`, length field, target at `[4:28]`
  NUL-padded, message at `[28:]` NUL-terminated
- `TestEncodeWhisper_EmptyTarget` — empty target field is all zeroes
- `TestEncodeWhisper_LongTarget` — copy() truncates gracefully at 24 bytes
- `TestActionWhisper_Registered` — **the regression test**: compiles only if
  `session.ActionWhisper` exists; asserts non-zero value and correct `String()` output
- `BenchmarkEncodeWhisper` — 1 alloc/op, 54 ns/op (expected: single `make([]byte)`)

## Wire Format Verified

rAthena source: `src/map/clif.cpp` — `clif_parse_WisMessage` and `clif_process_message`
with `whisperFormat=true`. `clif_packetdb.hpp:46`:
```
parseable_packet(0x0096, -1, clif_parse_WisMessage, 2, 4, 28)
```
Wire layout: `[packetType:2][packetLength:2][target:24][message:varlen+NUL]`

## Codegen Note

The `SYNTH_CZ_WISPER` stub's 2-byte `TotalSize` causes `commonSize = 2` in
`generateRegisterFileInner`, but the `encodeDir` scan overrides this at line 163–164
(`isFixed = fixedReturnEncoders["EncodeWhisper"]`). Since `EncodeWhisper` returns
`[]byte` (not `[N]byte`), `fixedReturnEncoders["EncodeWhisper"] = false` and
`isFixed = false` — the generated registration correctly uses the variable-length
`return EncodeWhisper(r, pv), nil` path.

## Test Results

```
--- PASS: TestEncodeWhisper_WireFormat
--- PASS: TestEncodeWhisper_EmptyTarget
--- PASS: TestEncodeWhisper_LongTarget
--- PASS: TestActionWhisper_Registered

BenchmarkEncodeWhisper: 18498612 ops, 54.64 ns/op, 48 B/op, 1 allocs/op
  (1 alloc expected — variable-length []byte output)
```

`go test ./...` — all packages pass.
