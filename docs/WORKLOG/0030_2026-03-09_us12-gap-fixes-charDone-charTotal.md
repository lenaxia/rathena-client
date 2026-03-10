# 0030 — US-12 Gap Fixes: G1 (charDone comment) + G2 (charTotal dead field)

**Date**: 2026-03-09
**Story**: US-12 code-reviewer gaps G1 and G2

## What was done

### G1 — Remove dead comment referencing `charDone` (pkg/fsm/fsm.go)

**Before** (line 493):
```go
// Inner feed loop: runs until charList ready, then sends 0x0066, then waits for zone info.
// charDone dead code removed (Bug 12-C): the loop condition uses !res.done directly.
slotSent := false
```

**After**:
```go
// Inner feed loop: runs until charList ready, then sends 0x0066, then waits for zone info.
slotSent := false
```

The second comment line was dead prose referencing a removed variable. It added no information beyond what the code already expresses.

### G2 — Remove dead `charTotal` field from `charPhaseResult` (pkg/fsm/fsm.go)

**Before** (struct definition, ~line 307):
```go
rawChars      []byte // accumulated CHARACTER_INFO bytes
charTotal     uint32 // total characters expected (from 0x09A0)
charsExpected uint32 // total from 0x09A0; we send one CH_CHARLIST_REQ per character
```

**After**:
```go
rawChars      []byte // accumulated CHARACTER_INFO bytes
charsExpected uint32 // total from 0x09A0; we send one CH_CHARLIST_REQ per character
```

**Before** (0x09A0 handler, ~line 458):
```go
total := binary.LittleEndian.Uint32(data[2:6])
res.charTotal = total
// total is the character count from 0x09A0; we send one CH_CHARLIST_REQ per
```

**After**:
```go
total := binary.LittleEndian.Uint32(data[2:6])
// total is the character count from 0x09A0; we send one CH_CHARLIST_REQ per
```

`charTotal` was set from the same `total` value as `charsExpected` and never read anywhere else. It was fully redundant.

## Test results

```
go build ./...          — exit 0, no output
go test ./pkg/fsm/...   — ok  github.com/lenaxia/ragnarok-go-client/pkg/fsm  0.123s
grep -n "charDone" pkg/fsm/fsm.go  — empty (PASS)
grep -n "charTotal" pkg/fsm/fsm.go — empty (PASS)
```
