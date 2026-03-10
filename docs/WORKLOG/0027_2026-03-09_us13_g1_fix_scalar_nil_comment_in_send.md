# Work Log 0027 — US-13 G1: Fix scalar nil comment in gen/send.go

**Date**: 2026-03-09
**Story**: US-13 gap G1 — gen/send.go never calls fixScalarNilComment
**Status**: COMPLETE

## Problem

`internal/codegen/gen/send.go` was building field comments as:

```go
comment = " // " + sanitiseComment(p.Semantic)
```

This omitted the `fixScalarNilComment` call that `gen/events.go` already applied.
As a result, scalar fields like `uint32`, `uint16`, `uint8` in generated `pkg/send/`
files kept "may be nil" language even though Go scalars cannot be nil.

Example (before fix):
```go
// pkg/send/game_login.go
AccountID uint32 // Account ID from login server (may be nil for certain packet variants)
SessionID uint32 // Session ID 1 (login_id1 from 0x0AC4, may be nil)
Clienttype uint16 // Client type or user level (may be nil, often unused/0)
SessionID2 uint32 // Session ID 2 (login_id2 from login server, may be nil)
Sex uint8 // Character sex (0=female, 1=male, 99=account sex, may be nil)
```

## Fix

Single-line change in `internal/codegen/gen/send.go` line 53:

```diff
-			comment = " // " + sanitiseComment(p.Semantic)
+			comment = " // " + fixScalarNilComment(sanitiseComment(p.Semantic), goType)
```

This mirrors the exact pattern used in `gen/events.go` line 62.

## Verification

Re-ran codegen:
```
go run ./internal/codegen/main.go --rathena ~/personal/rathena --out .
→ pkg/send/ (163 files)
```

Grep for "may be nil" in pkg/:
```
grep -rn "may be nil" /home/mikekao/personal/rathena-client/pkg/
```
**Empty output** — all 5 occurrences removed.

## Build and Test Results

```
go build ./...   → OK
go test ./...    → all pass except pkg/encode (pre-existing slice comparison
                   issue in hand-written test files from US-17/US-18 work,
                   unrelated to this change)
```

The `pkg/encode` failure:
- `pkg/encode/actor_action_test.go:64`: `p1 != p2` (slice can only be compared to nil)
- `pkg/encode/skill_use_test.go:67`: `p1 != p2` (slice can only be compared to nil)

These are in hand-written test files from US-17/US-18 which changed the return type of
encode functions from fixed-size arrays to `[]byte`. That change is tracked in worklog
0031. Not caused by this G1 fix.
