# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

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
