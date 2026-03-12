# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [v0.3.0] — 2026-03-12

### Added

- **`pkg/encode`** — 22 new generated encode functions covering all FSM C→S
  packets and additional auth variants: `EncodeMasterLogin`, `EncodeGameLogin`,
  `EncodeCharLogin`, `EncodeRequestCharacterPage`, `EncodeMapLogin`,
  `EncodeMapLoaded`, `EncodeTimeSyncResponse` (packetver-dispatched
  `0x007E`/`0x0360`), plus `EncodeCA*`, `EncodeCharCreate`, `EncodeCharDelete`,
  `EncodePinCodeResponse`, `EncodeSelectAccessibleMap`, `EncodeSelectCharacter`.

- **`pkg/send`** — corresponding request structs populated with all fields for
  the 22 new encode functions.

### Fixed

- **`internal/codegen/gen/encode.go`** — `map_loaded`, `time_sync_response`, and
  `game_login` were incorrectly listed in `fsmOwnedActions` and suppressed from
  codegen; all three removed.

- **`internal/codegen/main.go`** — `injectCommonPacketStructs` excluded
  `PACKET_CH_` and `PACKET_CA_` prefixes; both added so char/auth structs are
  injected into the VersionTable correctly.

- **`semantics/mappings.yaml`** — five action entries had wrong `rathena_struct`
  values (`PACKET_CZ_*` / `PACKET_CH_ENTER_0x0065` instead of the correct
  `SYNTH_*` names); all fixed. Stale duplicate `request_time` action deleted.

### Changed

- **`pkg/fsm/fsm.go`** — all seven `build*` handwritten packet builders replaced
  with generated `encode.Encode*` calls. The FSM no longer contains any
  hand-encoded packet bytes.

### Removed

- **`pkg/fsm/packets.go`** — all `build*` functions deleted (dead code after FSM
  migration). Only `copyStr` helper retained.

- **`pkg/fsm/fsm_test.go`** — `TestBuild*` unit tests for the now-deleted
  `build*` functions removed; equivalent coverage exists in `packets_test.go`
  via the generated encode functions.

- **`pkg/encode/request_time.go`** / **`pkg/send/request_time.go`** — stale
  files generated from the duplicate `request_time` action deleted.

### Tests

- **`internal/codegen/preprocess/vt_check_test.go`** — extended to verify all
  8 FSM structs are present in the VersionTable.

- **`pkg/fsm/packets_test.go`** — rewritten as external `package fsm_test`
  golden tests for all 7 generated encode functions (field layout, null padding,
  packetver dispatch boundaries).

- **`pkg/fsm/live_integration_test.go`** — field references updated to match
  current `events.ActorExists` (`GID`) and `events.StatUpdate` (`VarID`,
  `Count`) struct names. Integration test passes against live rAthena server
  (packetver 20200401).

---

## [v0.2.9] — 2026-03-11

### Fixed

- **`pkg/fsm`** — `IdentityInfo.MapName` was always empty. `runCharPhase` now
  parses the 16-byte map name field from `HC_NOTIFY_ZONESVR` (0x0081 / 0x0AC5),
  strips null padding and the `.gat` suffix, and forwards it to `OnIdentity`.

- **`pkg/fsm`** — `OnReady` callback received no entry position. The
  `(x, y, dir)`, `startTime`, `font`, and `sex` fields decoded from
  `ZC_ACCEPT_ENTER` (0x0073 / 0x02EB / 0x0A18) are now captured in `onMapEnter`
  and passed to the caller via the new `ReadyInfo` struct.

### Breaking API change

`OnReady` callback signature changed:

```go
// Before
OnReady(func(*session.MapSession, net.Conn))

// After
OnReady(func(*session.MapSession, net.Conn, ReadyInfo))
```

All callers must add the `ReadyInfo` parameter (can be ignored with `_`).

---

## [v0.2.8] — 2026-03-11

### Added

- **`pkg/fsm`** — `OnIdentity` callback fires after char selection with
  `IdentityInfo{AccountID, CharID, SelectedSlot, Sex}`.

### Fixed

- **`pkg/session`** — `Feed()` now recovers from unknown packet IDs instead of
  faulting permanently; unknown packets are skipped and feeding continues.

---

## [v0.2.7] — 2026-03-11

### Fixed

- **`internal/codegen`** — `lengths_map.go` correctness fixes (EPIC-05): wrong
  lengths for several S→C packets corrected against GCC-verified rAthena source.

---

## [v0.2.6] — 2026-03-11

### Added

- **`pkg/encode`** — 4 new encode functions; undocumented skips listed.

---

## [v0.2.5] — 2026-03-11

### Added

- **`pkg/encode`** — 32 new encode functions (28 codegen + 4 manual chat variants).

### Changed

- `send_chat` / `public_chat` duplication eliminated; merged into `public_chat`.

---

## [v0.2.4] — 2026-03-11

### Added

- Complete gameplay packet encode/decode coverage.

---

## [v0.2.3] — 2026-03-11

### Added

- **`internal/codegen`** — CZ struct injection.

### Fixed

- Duplicate action cleanup; encode panic fixes.

---

## [v0.2.2] — 2026-03-11

### Fixed

- **`internal/codegen`** — all codegen gaps closed.

---

## [v0.2.1] — 2026-03-10

### Fixed

- **`internal/codegen`** — enum-assigned packet IDs now resolved; `0x01D7` size
  gap corrected.

---

## [v0.2.0] — 2026-03-10

### Changed

- **`pkg/decode`** / **`pkg/encode`** — rAthena-native field names throughout
  (EPIC-04). Breaking change for any code using old camelCase field names.

---

## [v0.1.0] — initial release
