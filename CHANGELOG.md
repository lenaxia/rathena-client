# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [v0.5.6] — 2026-03-21

### Fixed

- **16 packet ID → action mappings missing from semantics DB** — `semantics/mappings.yaml`
  had no entries for 9 "middle generation" actor packets and 7 other packet IDs added
  in v0.5.2. Because these were absent from the DB, every `go run ./internal/codegen`
  invocation regenerated `receive_dispatch.go` without them, silently dropping all the
  work from v0.5.2 and breaking the dispatch for those packet IDs.

  All 16 implementations added via the semantics MCP tools:

  **Actor "middle generation" (pv 20091103–20131222) — 9 IDs:**
  - `actor_connected`: `0x07F8` (pv 20091103–20101123), `0x0858` (20101124–20120220),
    `0x090F` (20120221–20131222) — `struct packet_spawn_unit`
  - `actor_exists`: `0x07F9` (pv 20091103–20101123), `0x0857` (20101124–20120220),
    `0x0915` (20120221–20131222) — `struct packet_idle_unit`
  - `actor_moved`: `0x07F7` (pv 20091103–20101123), `0x0856` (20101124–20120220),
    `0x0914` (20120221–20131222) — `struct packet_unit_walking`

  **Dispatch-only IDs — 7 IDs:**
  - `actor_status_active`: `0x0983` (pv >= 20120618) — `struct packet_status_change`
  - `area_spell`: `0x099F` (pv 20121212–20130730), `0x09CA` (pv >= 20130731) —
    `struct packet_skill_entry`
  - `inventory_items_equip`: `0x0295` (pv 20071002–20080101), `0x02D0`
    (pv 20080102–20120924) — `struct packet_itemlist_equip`
  - `item_appeared`: `0x084B` (pv 20130001–20180417), `0x0ADD` (pv >= 20180418) —
    `struct packet_dropflooritem`

### Tests

- **`internal/codegen/semantics/validate_test.go`** — updated `actor_exists` expected
  implementation count from 6 to 9 to reflect the three new middle-generation entries.

---

## [v0.5.5] — 2026-03-21

### Fixed

- **`EncodeItemUse` sent wrong packet ID at `pv >= 20080910`** — the encoder
  hardcoded `0x00A7` and silently ignored `packetver`. At `pv >= 20080910`,
  rAthena reassigns `0x00A7` to `clif_parse_SolveCharName` (9 bytes) and moves
  `clif_parse_UseItem` to `0x0439` (8 bytes). Sending `0x00A7` caused an immediate
  server disconnect: `"received packet 0x00a7 with expected length 9, only 8 bytes"`.
  Every item use attempt on any modern server was broken.

  Fix: `semantics/mappings.yaml` `item_use` action now has two implementations —
  `0x0439` for `pv >= 20080910` and `0x00A7` for `pv < 20080910`. Codegen
  regenerates `pkg/encode/item_use.go` as a packetver dispatcher. Both variants
  share the same 8-byte `SYNTH_CZ_USE_ITEM` layout (`index.W`, `AID.L`).
  (worklog 0069, rAthena source: `src/map/clif_packetdb.hpp:1151`)

### Tests

- **`pkg/encode/item_use_test.go`** (new) — covers packet ID at `pv=20200401`
  (must be `0x0439`), boundary at `pv=20080910` (must be `0x0439`), legacy at
  `pv=20040705` (must be `0x00A7`), wire length (8 bytes), `Index` at `[2:4]`,
  and `AID` at `[4:8]`.

---

## [v0.5.4] — 2026-03-21

### Added

- **`ConnectionFSM.OnMapSessionCreated`** — new callback hook that fires after the
  `MapSession` is created and the FSM's own auth handlers are registered, but
  **before** `feedUntil` processes any bytes from the map server. This is the
  correct place for callers to register receive-direction semantic handlers that
  must capture packets co-delivered with `ZC_ACCEPT_ENTER` in the same TCP
  segment (e.g., the inventory burst, stat updates, actor spawns sent by
  `clif_parse_LoadEndAck` in response to `0x007D CZ_NOTIFY_ACTORINIT`).

  Previously, any handler registered in `OnReady` would silently miss these
  packets: `sessionCore.feed()` drains all complete frames in a single call, so
  the entire initial burst was dispatched — with no handlers registered — before
  `feedUntil` returned and `OnReady` fired. (worklog 0068)

### Fixed

- **Inventory burst and initial-map-login packets silently dropped** — root cause
  of the above. On loopback (and in practice on LAN), rAthena co-delivers
  `ZC_ACCEPT_ENTER` and the full post-`LoadEndAck` burst in one TCP segment.
  `feedUntil` consumed all frames before `OnReady` fired, dropping every packet
  that arrived without a registered handler. `OnMapSessionCreated` closes this
  window. (worklog 0068, rAthena source: `clif.cpp:10744 clif_parse_LoadEndAck`,
  `pc.cpp:2241 clif_authok`)

### Tests

- **`TestConnect_OnMapSessionCreated_HandlersFire`** — regression test that
  co-delivers `ZC_AID` (0x0283) with `ZC_ACCEPT_ENTER` in one `conn.Write`.
  Asserts a handler registered in `OnMapSessionCreated` fires (Part A) and a
  handler registered only in `OnReady` does not (Part B — packet already consumed).
  The test failed before the fix and passes after.

- **`ScriptedServer` `maxChunk=16` workaround removed** — the scripted server
  previously throttled map-phase writes to 16-byte chunks to prevent the FSM from
  consuming early-burst packets before `OnReady`-registered handlers were in place.
  With `OnMapSessionCreated` available, `runReplayTest` now registers handlers via
  `OnMapSessionCreated` and the chunk throttle is removed (all phases use 4096).

---

## [v0.5.3] — 2026-03-21

### Fixed

- **`t[0x009E]` stale length at `pv == 20130000`** — `packet_dropflooritem` gains a
  `type` field at `PACKETVER >= 20130000`, making the wire size 19 bytes, but
  `dropflooritemType` only switches from `0x009E` to `0x084B` at `PACKETVER > 20130000`
  (strictly greater). At exactly `pv == 20130000` the server sends `0x009E` with a
  19-byte body while the length table had 17, causing `IsIdentified`, `xPos`, `yPos`,
  `subX`, `subY`, and `count` to all be read from wrong offsets (silent corruption).
  Fix: `lengths_map_overrides.go` sets `t[0x009E] = 19` at `pv == 20130000`;
  `ItemAppeared_0x009E` now branches on `pv >= 20130000` to read `type` at offset 8.
  GCC verified at pv=20120925 (17 bytes), pv=20130000 (19 bytes), pv=20130001
  (19 bytes, 0x084B). (goKore-test worklog 0778)

- **`0x08C7` length override was dead code** — the `t[0x08C7] = 19` correction
  (introduced in v0.5.2) was accidentally nested inside the `pv >= 20181121` block in
  `lengths_map_overrides.go`, so it was never applied. It is now a standalone condition
  (`pv >= 20110718 && pv < 20121212`), correctly fixing the packet length for the
  area spell packet at those versions.

---

## [v0.5.2] — 2026-03-21

### Fixed

- **`ActionInventoryItemsEquip` missing `0x0295` and `0x02D0` dispatch entries** —
  decoder functions `InventoryItemsEquip_0x0295` and `_0x02D0` existed but were not
  wired into the dispatch table. Equip inventory packets were silently dropped for
  `pv 20071002–20120924`. (worklog 0067)

- **`AreaSpell_0x08C7` out-of-bounds read and wrong field widths** — the generated
  decoder used an invented `SYNTH_ZC_SKILL_ENTRY3` struct with 20-byte reads on a
  19-byte packet (`data[19]` was OOB) and read `Range` as `uint16` where rAthena has
  `int8`. The length table also hardcoded 20 instead of 19.
  Fix: decoder now mirrors `AreaSpell_0x011F`'s `>= 20110718` branch (same struct);
  `lengths_map_overrides.go` corrects the length to 19 for `pv 20110718–20121211`.
  (worklog 0067, rAthena source: packets_struct.hpp:1434–1454)

- **`ActionAreaSpell` missing `0x099F` and `0x09CA`** — `skill_entryType` has four
  packet IDs across history; only two were dispatched. Area spells were invisible on
  servers with `pv >= 20121212`. New decoders `AreaSpell_0x099F` (22 bytes, pv
  20121212–20130730) and `AreaSpell_0x09CA` (23 bytes, pv >= 20130731) added and
  wired. (worklog 0067)

- **`ZcGuildInfo_0x0A84` else branch applied wrong layout** — the else branch
  (`pv 20161019–20170314`) used the `0x01B6` struct (with `masterName` before
  `manageLand`). The `0x0A84` struct has no `masterName` field; `manageLand` is at
  offset 70. Both date ranges now use the same correct layout. (worklog 0067)

- **`ActionItemAppeared` missing `0x084B` and `0x0ADD`** — `dropflooritemType` has
  three packet IDs; only `0x009E` was dispatched. All items dropped on the floor were
  invisible on **every modern server** (`pv > 20130000`). New decoders added:
  `ItemAppeared_0x084B` (19 bytes, pv 20130000–20180417) and `ItemAppeared_0x0ADD`
  (22 bytes at pv 20180418–20181120; 24 bytes at pv >= 20181121 via existing
  `lengths_map_overrides.go`). The original `ItemAppeared_0x009E` now handles only
  the pre-20130000 layout (dead branches removed). (worklog 0067)

- **`ActionActorStatusActive` missing `0x0983`** — `status_changeType = 0x0983`
  (pv >= 20120618) was never dispatched. Status effect events (buffs/debuffs) were
  invisible on all clients from mid-2012 onwards. New decoder
  `ActorStatusActive_0x0983` (29 bytes) reads `Total`, `Left`, `Val1–Val3` from
  `packet_status_change`. `events.ActorStatusActive` extended with those five fields
  (zero for the `0x0196` path). (worklog 0067)

- **Actor "middle generation" packet IDs missing (9 total)** — the dispatch table
  covered generation 1–3 (pre-20091103) and generation 7–9 (post-20131223) but
  skipped three consecutive generations covering `pv 20091103–20131222`:
  - `ActionActorExists`: `0x07F9`, `0x0857`, `0x0915`
  - `ActionActorConnected`: `0x07F8`, `0x0858`, `0x090F`
  - `ActionActorMoved`: `0x07F7`, `0x0856`, `0x0914`

  New decoder functions and dispatch entries added for all nine. `packet_idle_unit`,
  `packet_spawn_unit`, and `packet_unit_walking` layouts verified via GCC
  preprocessor at each of the three breakpoints. (worklog 0067)

### Changed (non-breaking)

- `events.ActorStatusActive` gains five new zero-initialized fields: `Total uint32`,
  `Left uint32`, `Val1 int32`, `Val2 int32`, `Val3 int32`. Existing handlers that
  only read `Index`, `AID`, `State` are unaffected.

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
