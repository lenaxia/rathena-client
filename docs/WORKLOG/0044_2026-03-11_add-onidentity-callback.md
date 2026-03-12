# Worklog 0044: Add OnIdentity Callback to FSM

**Date**: 2026-03-11
**Package**: `pkg/fsm`
**Task**: Add identity callback for self-actor initialization

## Problem

goKore's connector.go was calling `f.OnAccountID(...)` which doesn't exist in rathena-client v0.2.5. The consumer needed access to account ID, character ID, selected slot, and sex before map phase begins to initialize the self-actor in GameState.

## Solution

Added `OnIdentity` callback to `ConnectionFSM` that fires after char selection completes (after parsing 0x0081/0x0AC5 zone server response) and before the map phase begins.

## Changes

### pkg/fsm/fsm.go

1. Added `IdentityInfo` struct:
```go
type IdentityInfo struct {
    AccountID    uint32
    CharID       uint32
    SelectedSlot uint8
    Sex          uint8
}
```

2. Added `onIdentity` field to `ConnectionFSM` struct

3. Added `OnIdentity(func(IdentityInfo))` method following existing callback pattern

4. Fire callback in `runCharPhase` after zone server response is parsed, before returning mapAddr:
```go
if f.onIdentity != nil {
    f.onIdentity(IdentityInfo{
        AccountID:    f.accountID,
        CharID:       f.charID,
        SelectedSlot: res.selectedSlot,
        Sex:          f.sex,
    })
}
```

## Test Results

```
=== RUN   TestConnect_FullFlow_Pre20170315
--- PASS: TestConnect_FullFlow_Pre20170315 (0.00s)
=== RUN   TestConnect_FullFlow_Post20170315
--- PASS: TestConnect_FullFlow_Post20170315 (0.00s)
... (all 55 tests pass)
PASS
ok      github.com/lenaxia/rathena-client/pkg/fsm      0.133s
```

## Consumer Usage

```go
f.OnIdentity(func(id fsm.IdentityInfo) {
    dispatcher.Trigger(ctx, hook.EventMapLoginAccepted, hook.MapLoginAcceptedEvent{
        AccountID: id.AccountID,
        CharID:    id.CharID,
        CharSlot:  id.SelectedSlot,
        Sex:       id.Sex,
    })
})
```

## Files Changed

- `pkg/fsm/fsm.go`: Added IdentityInfo struct, OnIdentity callback, and callback invocation

## Build Status

- `go build ./pkg/fsm/...`: PASS
- `go test ./pkg/fsm/...`: PASS (55 tests)
