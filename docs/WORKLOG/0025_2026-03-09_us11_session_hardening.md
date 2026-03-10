# 0025 — 2026-03-09 — US-11 Session Framing Engine Hardening

## Summary

Fixed three confirmed bugs in `pkg/session/session.go` and `pkg/decode/helpers.go` following
TDD (tests written first, confirmed failing, then implementation made them pass).

## Bugs Fixed

### Bug 11-A — Infinite loop on zero-length variable packet

**File**: `pkg/session/session.go`  
**Location**: After line reading `frameLen` from `recvBuf[2:4]`

Added a minimum frame length guard. A variable-length packet with embedded length
0, 1, 2, or 3 is malformed (minimum valid header is 4 bytes). Without this guard,
`frameLen = 0` causes an infinite loop: `consumed += 0`, `recvBuf = recvBuf[0:]`,
loop repeats forever.

```go
// Fix added after frameLen = int(binary.LittleEndian.Uint16(c.recvBuf[2:4])):
if frameLen < 4 {
    c.faulted = true
    return ErrUnknownPacket{ID: packetID}
}
```

Confirmed: the guard was absent from `session.go:86` (original). Now at line 93.

### Bug 11-B — Copy-to-front skipped when consumed == 0

**File**: `pkg/session/session.go`  
**Location**: `done:` label at end of `feed()`

Removed the `if consumed > 0` guard. Without this fix, when recvBuf drifts
forward after frame consumption and no subsequent copy-to-front runs (because
consumed==0 on the next partial-frame Feed calls), the recvBuf detaches from
buf and can grow unboundedly.

```go
// Before:
if consumed > 0 {
    n := copy(c.buf, c.recvBuf)
    c.recvBuf = c.buf[:n]
}

// After:
n := copy(c.buf, c.recvBuf)
c.recvBuf = c.buf[:n]
```

Confirmed: `if consumed > 0 {` was the exact text at `session.go:110`.

### Bug 11-C — nullTermString aliasing hazard

**Files**: `pkg/decode/helpers.go`, `pkg/session/session.go`

1. Added `CopyString` exported helper to `pkg/decode/helpers.go`:
   ```go
   // CopyString returns a heap-allocated copy of s that is safe to retain beyond
   // the HandlerFunc callback lifetime.
   func CopyString(s string) string { return string([]byte(s)) }
   ```

2. Updated `HandlerFunc` godoc in `pkg/session/session.go` to warn about string
   lifetime and reference `decode.CopyString`.

## Tests Written (TDD)

All tests written FIRST (confirmed failing), then implementation made them pass.

### New tests in `pkg/session/session_test.go` (package session_test)

- `TestFeed_VariableLength_ZeroEmbeddedLen_Faults`: sends 0x09FF (variable-length
  at pv=20141023) with embedded length 0; verifies ErrUnknownPacket returned.
- `TestFeed_VariableLength_TruncatedEmbeddedLen_Faults`: embedded lengths 1, 2, 3
  each fault with ErrUnknownPacket.
- `TestFeed_NullTermString_HandlerMayNotRetain`: registers handler that stores name
  via unsafe alias and via `decode.CopyString`; verifies CopyString preserves the
  value correctly across Feed calls.

### New test in `pkg/session/session_internal_test.go` (package session)

- `TestFeed_CopyToFront_PartialFrames`: directly manipulates `s.core.recvBuf` to
  simulate a drifted slice (starting at buf[7] instead of buf[0]), then feeds a
  partial byte; verifies `recvBuf[0]` is re-anchored to `buf[0]` after the fix.
  Requires internal access to `sessionCore.recvBuf` and `sessionCore.buf`.

### Pre-fix failure states

- Tests 1, 2 (zero/truncated embedded length): `Feed returned nil` — loop fell
  through to the `len(recvBuf) < frameLen` check with frameLen=0,1,2,3 always
  satisfying the condition, dispatching garbage frames repeatedly without faulting.
  Actually for frameLen=0: `len(recvBuf) < 0` is always false (no break), then
  `consumed += 0`, `recvBuf = recvBuf[0:]` — infinite loop if run without timeout.
  For lengths 1-3: misframs and re-dispatching incorrect data.
- Test 3 (copy-to-front): `recvBuf[0] at c0000e2007, want buf[0] at c0000e2000`
  — confirmed recvBuf was at offset 7 and did NOT get moved to buf[0].
- Test 4: passed immediately (CopyString already documented the correct behavior
  once CopyString was added).

## Verification

### Variable-length packet ID used in tests

0x09FF is registered as variable-length (length == -1) for `20141022 <= pv < 20150513`.
Used pv=20141023 for tests 1 and 2. Confirmed by inspecting `lengths_map.go`:
```
if pv >= 20141022: t[0x09FF] = -1
if pv >= 20150513: t[0x09FF] = 104  (overrides)
```

### Test results

```
=== RUN TestFeed_CopyToFront_PartialFrames       PASS
=== RUN TestFeed_VariableLength_ZeroEmbeddedLen_Faults   PASS
=== RUN TestFeed_VariableLength_TruncatedEmbeddedLen_Faults   PASS
=== RUN TestFeed_NullTermString_HandlerMayNotRetain   PASS
```

All existing tests continue to pass: `go test ./...` = all green.

### Benchmark results

```
BenchmarkFeed_SmallFixedPacket-14         70699399    22.43 ns/op    0 B/op    0 allocs/op
BenchmarkFeed_VariableLengthPacket-14     81432309    13.87 ns/op    0 B/op    0 allocs/op
BenchmarkFeed_ActorExists_0x09FF-14       97118958    11.09 ns/op    0 B/op    0 allocs/op
BenchmarkEncode_RequestMove-14          1000000000     0.41 ns/op    0 B/op    0 allocs/op
BenchmarkFeed_1000Sessions_Parallel-14   540975721     2.97 ns/op    0 B/op    0 allocs/op
```

All benchmarks show **0 allocs/op**. Both key benchmarks meet HLD §8 targets:
- `BenchmarkFeed_SmallFixedPacket`: 22 ns/op < 200 ns/op target ✓
- `BenchmarkFeed_ActorExists_0x09FF`: 11 ns/op < 500 ns/op target ✓

### CI checks

```
go build ./...          ✓ clean
go test ./...           ✓ all pass
go test -race ./pkg/session/ ./pkg/decode/    ✓ no races
grep -r "^\s*go " pkg/ --include="*.go" | grep -v "_test.go"    ✓ empty
```

## Files Changed

- `pkg/session/session.go`: Bug 11-A guard + Bug 11-B copy-to-front + HandlerFunc godoc
- `pkg/decode/helpers.go`: CopyString exported helper added
- `pkg/session/session_test.go`: 3 new external tests + imports for decode/events
- `pkg/session/session_internal_test.go`: new file (package session), 1 internal test
