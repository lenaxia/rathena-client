# 0038 — BUG-01: FSM missing MapName in IdentityInfo and entry position in OnReady

**Date**: 2026-03-11  
**Files changed**: `pkg/fsm/fsm.go`, `pkg/fsm/fsm_test.go`, `pkg/fsm/replay_test.go`, `pkg/fsm/live_integration_test.go`, `docs/BACKLOG/BUG-01_fsm-missing-identity-and-entry-position.md`

---

## What was done

Fixed two bugs in `pkg/fsm/fsm.go` that caused goKore's self-actor to be created at
tile (0,0) on an unnamed map after the login → char → map auth sequence.

### Bug 1 — IdentityInfo.MapName was always empty

`runCharPhase` parses `HC_NOTIFY_ZONESVR` (0x0081 for PACKETVER < 20170315, 0x0AC5
for >= 20170315). Both handlers read the map name from bytes [6:22] of the packet
but never stored it. The `onIdentity` call at the end of `runCharPhase` therefore
always passed an empty `MapName`.

**Changes:**
- Added `MapName string` field to `IdentityInfo` struct
- Added `mapName string` field to `charPhaseResult` struct
- Both `0x0081` (>= 28 byte branch) and `0x0AC5` handlers now extract the 16-byte
  map name field, strip null padding (`bytes.IndexByte`) and the `.gat` suffix
  (`strings.TrimSuffix`), and store it in `res.mapName`
- The `onIdentity` call now forwards `MapName: res.mapName`

### Bug 2 — OnReady received no entry position

The `onMapEnter` closure in `runMapPhase` decoded `ZC_ACCEPT_ENTER`
(0x0073 / 0x02EB / 0x0A18) to send the map-loaded response packets but then
discarded the position data. `onReady`'s callback signature was
`func(*session.MapSession, net.Conn)` with no way to pass position.

**Changes:**
- Added `ReadyInfo` struct to `fsm.go` with fields `X`, `Y`, `Dir`, `StartTime`,
  `Font`, `Sex`
- Changed `onReady` field type to `func(*session.MapSession, net.Conn, ReadyInfo)`
- Updated `OnReady` method signature to match
- Expanded `mapResult` to hold the six position fields
- In `onMapEnter`, after the tick sync write succeeds and before setting `res.done`,
  reads position from the entry packet:
  - `res.startTime` from bytes [2:6]
  - `(x, y, dir)` via `packing.DecodePosDir(data[6:9])`
  - `res.font` from bytes [11:13] (only present in 0x02EB / 0x0A18)
  - `res.sex` from byte [13] (only present in 0x0A18)
- The `onReady` call at the bottom of `runMapPhase` now passes a populated
  `ReadyInfo` struct

### Imports added

- `bytes` — for `bytes.IndexByte` in map name null-stripping
- `strings` — for `strings.TrimSuffix` in `.gat` removal
- `github.com/lenaxia/rathena-client/pkg/packing` — for `packing.DecodePosDir`

---

## Tests updated

All existing `OnReady` callbacks in `fsm_test.go`, `replay_test.go`, and
`live_integration_test.go` updated to accept the new `ReadyInfo` argument.

Two new tests added to `fsm_test.go`:

- `TestConnect_OnReady_ReceivesEntryPosition` — builds a 0x02EB packet with known
  `startTime=0x12345678`, `x=150`, `y=200`, `dir=3`, `font=7`; asserts all five
  fields arrive correctly in `ReadyInfo`
- `TestConnect_OnIdentity_ReceivesMapName` — table-driven test covering both the
  0x0081 (pre-20170315) and 0x0AC5 (post-20170315) variants; sends `"prontera.gat"`
  as the map name and asserts `IdentityInfo.MapName == "prontera"` (suffix stripped)

Also added test helper functions:
- `buildZCAcceptEnterWithPos` — 13-byte 0x02EB packet with explicit position data
- `encodePosDir` — local bit-packing helper for test packet construction
- `buildHCNotifyZonesvrPreWithMap` / `buildHCNotifyZonesvrPostWithMap` — variants
  of the existing zone server notify builders that accept an explicit map name

---

## Test results

```
go build ./...   → PASS (clean)
go test ./...    → PASS (all packages)
go test -race ./pkg/fsm/  → PASS (0 races)
```

New tests:
```
TestConnect_OnReady_ReceivesEntryPosition        PASS
TestConnect_OnIdentity_ReceivesMapName           PASS
  └─ pre20170315_0x0081                          PASS
  └─ post20170315_0x0AC5                         PASS
```
