# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [v0.5.1] — 2026-03-20

### Fixed

- **`ActionInventoryItemsStackable` never fired at pv≥20181002** — `0x0B09`
  (`inventorylistnormalType` at `PACKETVER_MAIN_NUM >= 20181002`) was present in
  `lengths_map.go` but absent from `receive_dispatch.go`. At production packetver
  `20200401` the server sends `0x0B09`; `0x0991` is disabled (`length=0`) at that
  version. The action now fires correctly via a new
  `InventoryItemsStackable_0x0B09` decoder and dispatch entry.
  (worklog 0066, rAthena source: `packets_struct.hpp:138`)

- **`InventoryItemsStackable` and `InventoryItemsEquip` exposed raw `[]byte`** —
  the inner `NORMALITEM_INFO` / `EQUIPITEM_INFO` array was left as an opaque
  `List []byte`, forcing consumers to implement their own packetver-aware binary
  parser. Both events now expose fully decoded typed slices:
  - `InventoryItemsStackable.Items []NormalItemEntry`
  - `InventoryItemsEquip.Items []EquipItemEntry`

  New types `NormalItemEntry`, `EquipItemEntry`, and `ItemOption` added to
  `pkg/events/`. Decoders handle all packetver-conditional field widths internally
  (`NORMALITEM_INFO`: 4 breakpoints; `EQUIPITEM_INFO`: 8 breakpoints covering
  pv < 20071002 through pv ≥ 20200916). One heap allocation per packet
  (`make([]T, n)`) — documented in HLD §known-exceptions.

  Also added decoders `InventoryItemsEquip_0x0295` and `InventoryItemsEquip_0x02D0`
  which were present in the rAthena enum but had no corresponding decode functions.
  (worklog 0066)

### Changed (breaking)

- **`events.InventoryItemsStackable.List []byte` removed** — replaced by
  `Items []NormalItemEntry`. Update handlers:
  ```go
  // Before
  session.RegisterSemanticHandler(ms, session.ActionInventoryItemsStackable,
      func(e events.InventoryItemsStackable) {
          raw := e.List  // []byte
      })

  // After
  session.RegisterSemanticHandler(ms, session.ActionInventoryItemsStackable,
      func(e events.InventoryItemsStackable) {
          for _, item := range e.Items {
              _ = item.ITID      // uint32
              _ = item.Count     // uint16
              _ = item.InvType   // on e, not item
          }
      })
  ```

- **`events.InventoryItemsEquip.List []byte` removed** — replaced by
  `Items []EquipItemEntry`. Same pattern as above.

---

## [v0.5.0] — 2026-03-20

### Added

- **EPIC-08 receive coverage** — 10 new decode actions (worklogs 0059-0062, 0064):
  `char_created`, `exp`, `inventory_items_equip`, `inventory_items_stackable`,
  `mail_receive`, `zc_el_par_change`, `zc_ho_par_change`, `zc_req_takeoff_equip_ack`
  plus updated decoders for `actor_connected`, `actor_exists`, `actor_moved`,
  `add_exchange_item`, `area_spell`, `character_server_refused`, `item_pickup`,
  `pin_code_request`, `received_characters_page`, `received_map_server_info`,
  `skill_add`, `skills_list`, `stat_update`, `zc_accept_enter`,
  `zc_equipwin_microscope`, `zc_guild_info`, `zc_hoskillinfo_list`,
  `zc_req_wear_equip_ack`, `zc_shortcut_key_list`, `zc_skillinfo_update2`.
  New corresponding event types added to `pkg/events/`.

- **`ActionRequestBuySellList`** — new send action for `0x00C5`
  (`PACKET_CZ_ACK_SELECT_DEALTYPE`): `send.RequestBuySellList{GID, Type}`.

- **`ActionGuildChat` send encoder** — `session.Send(ActionGuildChat, ...)` now works;
  `EncodeGuildChat` was already implemented but not registered in `register.go`.

### Fixed

- **13 broken variable-length send encoders** returned a fixed `[N]byte` and silently
  dropped the flex-array payload field (items, sell list, text value, etc.).
  Root cause: `internal/codegen/gen/encode.go` did not check `IsFlexArray` when
  choosing the return type. All 13 encoders now return `[]byte` and correctly write
  the full payload (worklogs 0061, 0063):
  `EncodeShopBuy`, `EncodeShopSell`, `EncodeNpcTalkText`, `EncodeMarketPurchase`,
  `EncodeCzNpcBarterMarketPurchase`, `EncodeCzNpcExpandedBarterMarketPurchase`,
  `EncodeCzPcPurchaseItemlistFrommc`, `EncodeCzPcPurchaseItemlistFrommc2`,
  `EncodeCzReqChangeMemberpos`, `EncodeCzReqMergeItem`,
  `EncodeCzReqRandomCombineItem`, `EncodeCzSePcBuyCashitemList`,
  `EncodeCzUploadMacroDetectorCaptcha`, `EncodeCaSsoLoginReq`.

### Changed (breaking)

- **14 send structs** no longer expose `PacketLength` / `PacketSize` as caller-visible
  fields. The encoders compute the wire length internally (`uint16(len(p))`). Remove
  these fields from any `send.X{PacketLength: n, ...}` struct literals:
  `ShopBuy`, `ShopSell`, `NpcTalkText`, `MarketPurchase`,
  `CzNpcBarterMarketPurchase`, `CzNpcExpandedBarterMarketPurchase`,
  `CzPcPurchaseItemlistFrommc`, `CzPcPurchaseItemlistFrommc2`,
  `CzReqChangeMemberpos`, `CzReqMergeItem`, `CzReqRandomCombineItem`,
  `CzSePcBuyCashitemList`, `CzUploadMacroDetectorCaptcha`, `CaSsoLoginReq`.

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
