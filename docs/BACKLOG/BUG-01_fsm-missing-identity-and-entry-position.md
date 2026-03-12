# BUG-01: FSM silently discards MapName and map entry position

**Status**: Open  
**Identified**: 2026-03-11  
**Priority**: High (blocks goKore self-actor initialisation — creates actor at (0,0) with empty map name)  
**File**: `pkg/fsm/fsm.go`

---

## Summary

Two related defects in `ConnectionFSM`:

1. `IdentityInfo` is missing `MapName` — `runCharPhase` parses `res.mapName` from
   `HC_NOTIFY_ZONESVR` but never forwards it to the `OnIdentity` callback.
2. `OnReady`'s callback receives no entry position — the `(x, y, dir)` decoded from
   `ZC_ACCEPT_ENTER` inside `onMapEnter` is consumed internally and never passed to
   the caller.

Both cause goKore's connector.go to initialise the self-actor with zero/empty values,
making the bot believe it is standing at tile (0,0) on an unnamed map.

---

## Bug 1 — `IdentityInfo` is missing `MapName`

### Problem

`runCharPhase` parses the map name from `HC_NOTIFY_ZONESVR` (packet `0x0081` /
`0x0AC5`) into a local result field. The `onIdentity` call at the end of the function
does not include it:

```go
// pkg/fsm/fsm.go — current (broken)
f.onIdentity(IdentityInfo{
    AccountID:    f.accountID,
    CharID:       f.charID,
    SelectedSlot: res.selectedSlot,
    Sex:          f.sex,
    // MapName silently absent
})
```

### Root cause

`IdentityInfo` struct has no `MapName` field. `charPhaseResult` does not store the
map name either — the 0x0081/0x0AC5 handlers read the name bytes from the packet but
do not save them.

### Fix

**Step 1** — Add `MapName string` to `IdentityInfo`:

```go
type IdentityInfo struct {
    AccountID    uint32
    CharID       uint32
    SelectedSlot uint8
    Sex          uint8
    MapName      string // map name without .gat suffix, from HC_NOTIFY_ZONESVR
}
```

**Step 2** — Add `mapName string` to `charPhaseResult`:

```go
type charPhaseResult struct {
    // ... existing fields ...
    mapName string
}
```

**Step 3** — Populate `res.mapName` in both `0x0081` (PACKETVER < 20170315) and
`0x0AC5` (PACKETVER >= 20170315) handlers. The map name occupies bytes `[6:22]`
(16-byte fixed char field) in both packets — strip the `.gat` suffix and any null
padding:

```go
// In both handlers, after extracting ip/port:
rawName := data[6:22]
n := bytes.IndexByte(rawName, 0)
if n < 0 {
    n = len(rawName)
}
name := string(rawName[:n])
name = strings.TrimSuffix(name, ".gat")
res.mapName = name
```

**Step 4** — Forward it in the `onIdentity` call:

```go
f.onIdentity(IdentityInfo{
    AccountID:    f.accountID,
    CharID:       f.charID,
    SelectedSlot: res.selectedSlot,
    Sex:          f.sex,
    MapName:      res.mapName,
})
```

---

## Bug 2 — `OnReady` callback receives no entry position

### Problem

`onReady`'s callback signature is `func(*session.MapSession, net.Conn)`. The
player's initial `(x, y, dir)` and related fields decoded from `ZC_ACCEPT_ENTER`
are available only inside the `onMapEnter` closure in `runMapPhase` and are never
surfaced to the caller. By the time `OnReady` fires the packet bytes are gone.

### Root cause

`mapResult` only holds `done bool` and `err error`. `onMapEnter` decodes the entry
packet to confirm successful entry but discards position data immediately after.

### Fix

**Step 1** — Add `ReadyInfo` struct alongside `IdentityInfo` in `fsm.go`:

```go
// ReadyInfo carries the decoded ZC_ACCEPT_ENTER fields to the OnReady callback.
// The FSM consumes the entry packet before handing off to goKore; these fields
// let the caller read the initial position without re-parsing a consumed frame.
type ReadyInfo struct {
    X         uint16 // initial tile X coordinate
    Y         uint16 // initial tile Y coordinate
    Dir       uint8  // facing direction (0–7)
    StartTime uint32 // server tick from entry packet
    Font      uint16 // overhead font ID
    Sex       uint8  // character sex byte
}
```

**Step 2** — Change `onReady` field type and `OnReady` method signature:

```go
// field:
onReady func(*session.MapSession, net.Conn, ReadyInfo)

// method:
func (f *ConnectionFSM) OnReady(fn func(*session.MapSession, net.Conn, ReadyInfo)) *ConnectionFSM {
    f.onReady = fn
    return f
}
```

**Step 3** — Expand `mapResult` to hold position fields:

```go
type mapResult struct {
    done      bool
    err       error
    x         uint16
    y         uint16
    dir       uint8
    startTime uint32
    font      uint16
    sex       uint8
}
```

**Step 4** — In `onMapEnter`, capture the position from the entry packet before
setting `res.done`. The entry packet layout:
- bytes `[2:6]`  — `uint32 startTime`
- bytes `[6:9]`  — `uint8[3] posDir` (use `packing.DecodePosDir`)
- bytes `[11:13]` — `uint16 font` (only in 0x02EB / 0x0A18, i.e. len >= 13)
- byte  `[13]`    — `uint8 sex` (only in 0x0A18, i.e. len >= 14)

```go
// In onMapEnter, after the tick sync write succeeds, before setting res.done:
if len(data) >= 9 {
    x, y, dir := packing.DecodePosDir(data[6:9])
    res.x = x
    res.y = y
    res.dir = dir
}
if len(data) >= 6 {
    res.startTime = binary.LittleEndian.Uint32(data[2:6])
}
if len(data) >= 13 {
    res.font = binary.LittleEndian.Uint16(data[11:13])
}
if len(data) >= 14 {
    res.sex = data[13]
}
res.done = true
```

**Step 5** — Pass `ReadyInfo` in the `onReady` call at the bottom of `runMapPhase`:

```go
if f.onReady != nil {
    connTransferred = true
    f.onReady(mapSess, conn, ReadyInfo{
        X:         res.x,
        Y:         res.y,
        Dir:       res.dir,
        StartTime: res.startTime,
        Font:      res.font,
        Sex:       res.sex,
    })
}
```

---

## Impact on goKore

Once both fixes are in, goKore's `connector.go` will:

- Receive a non-empty `info.MapName` from `OnIdentity` (already wired in connector,
  currently always receives `""`)
- Receive real `(x, y, dir)` in `OnReady` and fire `EventMapEntryAccepted` with a
  correct initial position instead of `(0, 0)`

No other callers of the rathena-client FSM API exist outside of goKore.

---

## Acceptance Criteria

- [ ] `IdentityInfo.MapName` field added
- [ ] `charPhaseResult.mapName` field added; populated in both `0x0081` and `0x0AC5` handlers
- [ ] Map name `.gat` suffix and null bytes stripped before storing
- [ ] `onIdentity` call includes `MapName: res.mapName`
- [ ] `ReadyInfo` struct added to `fsm.go`
- [ ] `onReady` field type updated to `func(*session.MapSession, net.Conn, ReadyInfo)`
- [ ] `OnReady` method signature updated
- [ ] `mapResult` expanded with position fields
- [ ] `onMapEnter` captures `(x, y, dir)`, `startTime`, `font`, `sex` from entry packet bytes
- [ ] `onReady` call passes populated `ReadyInfo`
- [ ] `packing.DecodePosDir` used for position decode (no manual bit arithmetic)
- [ ] All existing FSM tests updated to pass the new `ReadyInfo{}` argument
- [ ] New FSM tests added: `TestConnect_OnReady_ReceivesEntryPosition` and
  `TestConnect_OnIdentity_ReceivesMapName` covering non-zero values
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -race ./pkg/fsm/` passes
- [ ] Worklog written in `docs/WORKLOG/`
