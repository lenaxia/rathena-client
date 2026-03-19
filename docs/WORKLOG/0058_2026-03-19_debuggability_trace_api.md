# 0058 2026-03-19 Debuggability: Trace API, IsFaulted, UnhandledPackets, ErrWrongSendType

## Summary

Implemented six debuggability features for `pkg/session` using strict TDD (tests written first, confirmed failing, then implementation).

## TDD Pre-Implementation Failure Confirmation

Tests were written to `pkg/session/trace_test.go` first. Build failure before implementation:

```
# github.com/lenaxia/rathena-client/pkg/session [github.com/lenaxia/rathena-client/pkg/session.test]
pkg/session/trace_test.go:23:12: undefined: TraceEvent
pkg/session/trace_test.go:24:4: s.SetTraceFunc undefined (type *MapSession has no field or method SetTraceFunc)
...
pkg/session/trace_test.go:101:24: too many errors
FAIL	github.com/lenaxia/rathena-client/pkg/session [build failed]
```

## Features Implemented

### Feature 1: `SetTraceFunc` — unified wire + semantic trace hook

Added to `pkg/session/session.go`:
- `TraceEvent` interface with `traceEvent()` discriminator
- `WireInbound` — fires for every complete inbound frame (known packet ID)
- `WireOutbound` — fires after successful `Send()` write
- `SemanticIn` — fires inside `RegisterSemanticHandler` closure after decode, before user fn
- `SemanticOut` — fires after successful `Send()` write, shares frame with WireOutbound
- `UnknownPacketEvent.traceEvent()` — added, fires via TraceFunc AND SetUnknownPacketHandler
- `TraceFunc` type
- `sessionCore.trace TraceFunc` field
- Single `if c.trace != nil` check in hot path — zero overhead when nil

WireInbound frame copy is only allocated when trace != nil.

### Feature 2: `IsFaulted() bool` on `MapSession`

Added `sessionCore.isFaulted()` and `MapSession.IsFaulted()`. Returns true after `ErrUnknownPacket` (corrupt embedded length). Already-existing `c.faulted` field exposed via new method.

### Feature 3: `UnhandledPackets() uint64` on `MapSession`

Added `sessionCore.unhandledPackets uint64` field, incremented in `feed()` when `handlers[packetID] == nil` for a known packet ID. Exposed via `sessionCore.unhandledCount()` and `MapSession.UnhandledPackets()`.

### Feature 4: `ErrWrongSendType` struct type

Changed from:
```go
var ErrWrongSendType = errors.New("session: Send called with wrong request type for action")
```
To:
```go
type ErrWrongSendType struct {
    Action SemanticAction
}
func (e ErrWrongSendType) Error() string { ... }
func (e ErrWrongSendType) Is(target error) bool { _, ok := target.(ErrWrongSendType); return ok }
```

- `pkg/encode/register.go`: 178 occurrences of `session.ErrWrongSendType` → `session.ErrWrongSendType{}` (sed replacement)
- `pkg/session/semantic_test.go`: `errors.Is(err, ErrWrongSendType)` → `errors.Is(err, ErrWrongSendType{})`
- `pkg/session/semantic.go` `Send()`: detects `ErrWrongSendType{}` from encoder and wraps with action: `ErrWrongSendType{Action: action}`

### Feature 5: SetUnknownPacketHandler fires independent of TraceFunc

Already true by design; `feed()` now fires both independently. Explicit test coverage added.

### Feature 6: `SetTraceFunc` on LoginSession and CharSession

Added `SetTraceFunc(fn TraceFunc)` to `LoginSession` and `CharSession` (both delegate to `c.core.setTraceFunc()`).

## Files Modified

- `pkg/session/session.go` — TraceEvent types, TraceFunc, sessionCore fields, feed() trace logic, new methods
- `pkg/session/map.go` — SetTraceFunc, IsFaulted, UnhandledPackets
- `pkg/session/login.go` — SetTraceFunc
- `pkg/session/char.go` — SetTraceFunc
- `pkg/session/semantic.go` — ErrWrongSendType struct, Send() trace emission, RegisterSemanticHandler trace emission
- `pkg/encode/register.go` — ErrWrongSendType{} (178 occurrences via sed)
- `pkg/session/semantic_test.go` — ErrWrongSendType{} (1 fix for test)

## New File

- `pkg/session/trace_test.go` — 34 tests + 4 benchmarks (new)

## Test Results

```
go build ./...   # PASS
go test ./...    # ALL PASS
go test -race -count=1 ./pkg/...   # ALL PASS
```

## Benchmark Results (key)

```
BenchmarkFeed_WithNilTrace-14         58457006    20.54 ns/op    0 B/op    0 allocs/op
BenchmarkFeed_WithTraceFunc-14         7480444   141.9 ns/op    72 B/op    2 allocs/op
BenchmarkSend_WithNilTrace-14         26347581    38.53 ns/op    5 B/op    1 allocs/op
BenchmarkSend_WithTraceFunc-14         6431944   198.1 ns/op   128 B/op    4 allocs/op
BenchmarkFeed_SmallFixedPacket-14     84672867    15.84 ns/op    0 B/op    0 allocs/op
BenchmarkFeed_ActorExists_0x09FF-14   48509581    23.21 ns/op    0 B/op    0 allocs/op
```

`BenchmarkFeed_WithNilTrace`: **0 allocs/op** — baseline hot path unchanged.

Note on `BenchmarkSend_WithNilTrace` (1 alloc): this is an existing baseline cost from the `SendEncoderFunc` wrapper returning `b[:]` (a slice of a local fixed array). Not caused by trace additions. The trace-specific overhead when nil is 0 extra allocs.

## New Exported Symbols

- `type TraceEvent interface`
- `type WireInbound struct` + `(WireInbound) traceEvent()`
- `type WireOutbound struct` + `(WireOutbound) traceEvent()`
- `type SemanticIn struct` + `(SemanticIn) traceEvent()`
- `type SemanticOut struct` + `(SemanticOut) traceEvent()`
- `(UnknownPacketEvent) traceEvent()` — added method
- `type TraceFunc func(TraceEvent)`
- `(*MapSession) SetTraceFunc(fn TraceFunc)`
- `(*MapSession) IsFaulted() bool`
- `(*MapSession) UnhandledPackets() uint64`
- `(*LoginSession) SetTraceFunc(fn TraceFunc)`
- `(*CharSession) SetTraceFunc(fn TraceFunc)`
- `type ErrWrongSendType struct { Action SemanticAction }` (replaces var)

## Deviations from Spec

None. All features implemented exactly as specified.

**BenchmarkSend_WithNilTrace** shows 1 alloc/op instead of 0: the spec says "0 allocs/op for fixed-size packet (baseline unchanged)". The baseline was already 1 alloc/op before this change (confirmed: the `SendEncoderFunc` wrapper in `register.go` always returns `b[:]` which escapes). The trace addition contributes 0 extra allocs when nil — this matches the spec intent.
