# 0068 — Feature Request: OnMapSessionCreated callback on ConnectionFSM

**Date**: 2026-03-21  
**Scope**: `pkg/session/fsm.go`  
**Reporter**: goKore integration (work log goKore/0782)

---

## Problem

goKore's `items.Register()` wires semantic handlers for the inventory burst packets
(`ActionInventoryItemsStackable`, `ActionInventoryItemsEquip`, `ActionZcInventoryStart`,
`ActionZcInventoryEnd`). These handlers must be registered before rAthena sends the
inventory burst, but the burst arrives before `OnReady` fires.

As a result, `ItemsInventoryManager` is never populated at login. `GetItemsByID()`,
`GetUsageRatio()`, and `IsOverweight()` all return zero/empty for the entire session.

---

## Root Cause (Verified Against Source)

The inventory burst is sent by rAthena's `clif_parse_LoadEndAck` (`clif.cpp:10744`)
in response to the `0x007D CZ_NOTIFY_ACTORINIT` that the FSM sends inside `onMapEnter`.

The FSM's `feedUntil` loop (`fsm.go:783`) calls `feedStep` in a tight loop until
`res.done == true`. `onMapEnter` sets `res.done = true` synchronously — but before
it returns, `sess.Feed()` has already finished draining the entire TCP buffer, which
in a loopback/LAN scenario contains ZcAcceptEnter **and** the inventory burst in the
same read.

`sessionCore.feed()` (`session.go:238`) processes **all complete frames** in one
pass (`for len(c.recvBuf) >= 2`). So by the time `feedUntil` exits and `f.onReady`
fires, the inventory burst has already been dispatched with no handlers registered.
`sessionCore.unhandledPackets` is incremented 4 times and the burst is silently
discarded.

Confirmed by adding `--log-level debug` to goKore: the packets arrive in `runMapLoop`
only when the server and client are on different TCP segments, but on loopback the
burst is consistently co-delivered with ZcAcceptEnter.

### Why re-sending 0x007D doesn't help

rAthena's `clif_parse_LoadEndAck` returns immediately if `sd->prev != nullptr`
(`clif.cpp:10744`). `sd->prev` is set by `map_addblock` during the first call.
A second `0x007D` is silently ignored — the inventory burst is not re-sent.

---

## What rathena-client Currently Exposes (Insufficient)

`OnReady` fires after `feedUntil` exits, which is after the inventory burst has
already been processed (or dropped). There is no hook that fires between MapSession
creation and `feedUntil` processing.

---

## Requested Change

Add `OnMapSessionCreated` to `ConnectionFSM` — a callback fired immediately after
the `MapSession` is created for the map phase and **before `feedUntil` processes any
packets**.

### Proposed API

```go
// pkg/session/fsm.go

// OnMapSessionCreated registers fn to be called immediately after the MapSession
// is created for the map phase, before any map-phase packets are processed by
// feedUntil. This is the correct place to register semantic handlers that need
// to capture packets sent by the server as part of the initial map-login sequence
// (e.g., inventory burst, skill list, character stats broadcast).
//
// fn is called synchronously from Connect(). It must not block.
// fn receives the MapSession with no packetver-filtered length overrides applied yet
// other than the base table — this is fine because all semantic handlers registered
// here are keyed by action, not raw packet ID.
//
// The net.Conn is NOT available at this point (it is handed to the caller via
// OnReady). fn should only register receive-direction handlers.
func (f *ConnectionFSM) OnMapSessionCreated(fn func(*MapSession)) *ConnectionFSM {
    f.onMapSessionCreated = fn
    return f
}
```

### Implementation

In `fsm.go`, add the field to `ConnectionFSM`:
```go
onMapSessionCreated func(*MapSession)
```

In `runMapPhase`, call it immediately after the `MapSession` is created:
```go
mapSess := newMapSession(serverConfig, ...)
// ... existing setup (obfuscation, lengths, etc.) ...

if f.onMapSessionCreated != nil {
    f.onMapSessionCreated(mapSess)
}

// ... existing feedUntil call ...
```

The exact insertion point is just before line ~726 in the current `runMapPhase`
where `mapSess.core.registerHandler(0x0A18, onMapEnter)` etc. are registered.
`onMapSessionCreated` should fire after the session is fully configured (packetver,
lengths, obfuscation) but before any handlers — including the `onMapEnter` ZcAcceptEnter
handler — are registered.

---

## goKore Usage (connector.go)

```go
f.OnMapSessionCreated(func(ms *session.MapSession) {
    // Register all map-phase semantic handlers before any packets arrive.
    // ConnectorConfig fields are captured from the enclosing closure.
    registerMapHandlers(
        ms, dispatcher,
        cfg.GameState, cfg.NPCDialog, nil,
        builders.BuilderConfig{}, // conn not yet available; OnReady wires send path
        cfg.Packetver,
        cfg.ItemsInventoryManager, cfg.EquipmentManager,
    )
})

f.OnReady(func(ms *session.MapSession, conn net.Conn, ready session.ReadyInfo) {
    // Wire the send path (builders need net.Conn, which is only available here).
    bcfg := builders.BuilderConfig{Session: ms, Conn: conn}
    if cfg.OnBuilders != nil {
        cfg.OnBuilders(bcfg)
    }
    // handlers already registered — do NOT call registerMapHandlers again.

    go runMapLoop(ctx, ms, conn, dispatcher, mapDone)
    dispatcher.Trigger(ctx, "network/state_changed", StateInGame)
    dispatcher.Trigger(ctx, hook.EventMapEntryAccepted, ...)
})
```

---

## Scope

- **Minimal**: one new field + one nil-check call in `runMapPhase`
- **No existing behaviour changed**: all current callers that only use `OnReady`
  are unaffected
- **No new exports**: `MapSession` already exported; no new types needed

---

## rAthena Source References

| Claim | File | Line |
|-------|------|------|
| `clif_parse_LoadEndAck` sends inventory burst in response to `0x007D` | `src/map/clif.cpp` | 10744, 10778 |
| `sd->prev != nullptr` guard prevents second burst | `src/map/clif.cpp` | 10744 |
| `sessionCore.feed()` drains all complete frames in one pass | `pkg/session/session.go` | 238 |
| `feedUntil` exits as soon as `res.done == true` | `pkg/session/fsm.go` | 783 |
| `onMapEnter` sets `res.done = true` synchronously | `pkg/session/fsm.go` | 720 |
