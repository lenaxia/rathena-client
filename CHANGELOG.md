# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

### Added

- **`semantics-tool rename-action` — rename a semantic action in-place.**
  Adds `DB.RenameAction(oldName, newName)` to `internal/semanticsdb` plus a
  CLI subcommand and MCP tool (`rename_action`). Preserves the action's
  document position, all implementations, canonical_params, and metadata;
  updates the inner `name:` field only when it currently mirrors the old
  key so unrelated diffs stay minimal. Rejects blank/existing/missing
  targets; a no-op when old == new.

  Previously the only way to rename an action was delete + create (which
  destroyed implementation history and moved the entry to the end of the
  file) or a direct yaml edit (which the README's Rule 21 forbids).
  `TestMCP_ToolsList_ReturnsAll15Tools` reflects the tool count bump from
  14 → 15.

### Changed

- **Renamed `monster_ranged_attack` → `attack_failure_for_distance`.**
  Packet 0x0139 is rAthena's `ZC_ATTACK_FAILURE_FOR_DISTANCE`, sent by
  `clif_movetoattack` (`src/map/clif.cpp:8171-8188`) when a player's attack
  target is out of range. It is not a monster attack and never involves a
  monster on the wire; the OpenKore name is misleading. The rename is
  in-place — position and implementation preserved. Event/decoder Go
  identifiers now use `AttackFailureForDistance` (both files renamed
  under `pkg/decode/` and `pkg/events/`); the `SemanticAction` enum
  constant is `ActionAttackFailureForDistance`. `openkore_name:
  monster_ranged_attack` is preserved as documentation of the (also
  misleading) OpenKore name.

- **Corrected two latent mapping errors caught by the codegen fidelity
  fixes below**:
  - `skill_cast` gains the missing `0x0B1A` implementation (pv >= 20181212;
    `PACKET_ZC_USESKILL_ACK` binds to 0x0B1A at that pv per
    `packets_struct.hpp:3951-3964`). Previously worked around by manual
    edits to generated files (commit 657c728), which regeneration would
    revert.
  - `add_exchange_item` splits the previously-unbounded `0x0A96` into
    `0x0A96 [20161026..20200915]` + new `0x0B42 [20200916..∞]`, matching
    rAthena's `DEFINE_PACKET_HEADER(ZC_ADD_EXCHANGE_ITEM, ...)` schedule
    at `packets_struct.hpp:2641-2648`.

### Fixed

- **Codegen: `packets_struct.hpp` array sizes now resolve correctly.**
  `packets_struct.hpp` references macros (notably `MAX_ITEM_OPTIONS`)
  that are only defined by `packets.hpp`, so preprocessing
  `packets_struct.hpp` directly left them undefined and turned array
  members like `ItemOptions option_data[MAX_ITEM_OPTIONS]` into flex
  arrays of size 0. This mis-aligned every subsequent field in the
  struct (e.g. `PACKET_ZC_ADD_EXCHANGE_ITEM` had `Location` at offset 29
  instead of 54, `Grade` at 36 instead of 61).

  Fix: include `packets_hpp_stub.h` when preprocessing
  `SourcePacketsStruct` too, and define `MAX_ITEM_OPTIONS` in the stub.

- **Codegen: `injectMapPacketStructs` no longer overwrites the
  authoritative packets_struct.hpp version table.**
  `packets_struct.hpp` contains fine-grained `#if PACKETVER_MAIN_NUM`
  guards that identify the exact transition dates for each struct
  layout. `packets.hpp` has a different (coarser) breakpoint schedule.
  The previous `injectMapPacketStructs` re-derived version ranges from
  `packets.hpp` snapshots for every struct name with a `PACKET_ZC_*` /
  `PACKET_SC_*` / `PACKET_CZ_*` prefix — overwriting the accurate
  packets_struct.hpp ranges with less-precise ones. For example,
  `PACKET_ZC_USESKILL_ACK` really transitions from 25→29 bytes at
  pv 20181212 (per `packets_struct.hpp:3951`) but the previous inject
  recorded the transition at 20191120 (the packets.hpp snapshot that
  first observed the new size).

  Fix: `injectMapPacketStructs` now skips structs that already have a
  VT entry from `packets_struct.hpp`. It continues to inject structs
  that only exist in `packets.hpp` (login/char server packets) — its
  original purpose. Emits a summary log line for skipped structs.

- **Tests: stopped enshrining wire-behavior that doesn't happen and
  codegen implementation details.**
  Three tests were latent-passing only because of the codegen bugs
  above:
  - `TestSkillCast_0x07FB_StillFires` asserted `lengths[0x07FB] == 29`
    at pv=20200401 — but rAthena never sends 0x07FB at pv >= 20181212;
    it uses 0x0B1A. Updated to test at pv=20180101 (0x07FB's real
    active range), expected length 25.
  - `TestZcGroupList_ActionConstant` hardcoded the exact integer value
    of a codegen-emitted enum constant, which shifts whenever actions
    are added/renamed alphabetically. Replaced with the `!= ActionUnknown`
    + `String() == "ActionZcGroupList"` invariants that match the actual
    API contract.
  - `TestGap_AddExchangeItem_Grade_20200401_IsZero` used
    `AddExchangeItem_0x0A96` at pv=20200916 — but rAthena sends 0x0B42
    at that pv. Split into two cases: 0x0A96 at pv=20200401 (no grade)
    and 0x0B42 at pv=20200916 (grade at offset 61).

---

## [v0.8.0] — 2026-07-10

### Added

- **`cmd/semantics-tool` — MCP server + CLI for editing `semantics/mappings.yaml`**
  (worklog 0088). Migrates the goKore `gokore-semantics` MCP server into
  rathena-client and adapts it to the rathena-client `semantic_actions:`
  schema. Exposes 14 tools (8 read-only, 6 mutating) over JSON-RPC stdio for
  AI clients, plus a 14-subcommand CLI for humans. Round-trips the YAML
  byte-identically when no mutations are applied (enforced by
  `TestProductionMappings_RoundTripByteIdentical`); mutations produce minimal
  diffs that match the surrounding formatting. This unblocks the README Rule
  9 "use the MCP server, never edit mappings.yaml directly" workflow, which
  had been aspirational — every prior mappings.yaml change was a hand-edit.

  Read-only tools: `list_actions`, `get_action`, `list_implementations`,
  `get_implementation`, `search_actions`, `validate`, `stats`, `export`.
  Mutating tools: `create_action`, `update_action`, `delete_action`,
  `add_implementation`, `update_implementation`, `delete_implementation`.

- **`internal/semanticsdb/` — editor layer** over `semantics/mappings.yaml`
  with load/mutate/save and structural validation. Built on `gopkg.in/yaml.v3`
  using a `yaml.Node` tree that preserves source formatting (comments, quote
  style, key order) for unchanged content.

### Changed (breaking — policy)

- **README Rule 5 reworded**: was "No External Runtime Dependencies (repo-wide,
  `go.mod` must have zero `require` entries)". Now scopes zero-deps to `pkg/`
  only. `internal/` and `cmd/` developer tooling may use stdlib plus
  `gopkg.in/yaml.v3`. The original "embeddable with no transitive dependency
  surprises" property is preserved for `pkg/` because Go's module graph
  excludes `internal/`/`cmd/` deps from the importer's closure.

  `go.mod` now contains `require gopkg.in/yaml.v3 v3.0.1`. Consumers of
  `pkg/` are unaffected.

### Changed (non-breaking)

- **`internal/codegen/semantics/loader.go` rewritten** on `gopkg.in/yaml.v3`.
  327 lines of hand-rolled `bufio.Scanner` indent-counting parsing replaced
  with ~200 lines of yaml.v3 decode plus a `tolerantRange` custom unmarshaler
  that preserves null positions in `packetver_range:` sequences (yaml.v3
  skips null items when decoding into `[]int` directly) and accepts both
  bare integers and quoted strings (mappings.yaml is inconsistent on this;
  worklog-0087 used quoted strings like `"20121009"`). All 6 existing tests
  in `internal/codegen/semantics/*_test.go` pass unchanged.

### Fixed

- **Deleted empty duplicate action `received_character_ID_and_Map`** from
  `semantics/mappings.yaml`. Pre-existing validation finding surfaced by the
  new `validate` tool: a stub entry with capital letters (a leftover from
  worklog 0009's case-collision fix) existed alongside the canonical
  lowercase `received_character_id_and_map`. Both had `implementations: []`
  so neither was wired to anything; the duplicate is removed. 6 lines deleted.

### Tests

- `internal/semanticsdb/db_test.go` — 16 tests covering load, list, get,
  create/delete/update action, add/update/delete impl, validation (min>max,
  duplicate detection, bad action name), search, statistics, and production
  round-trip.
- `internal/semanticsdb/roundtrip_test.go` — strongest guarantee: no-op
  `Save` produces zero bytes of change to production mappings.yaml.
- `cmd/semantics-tool/cli/cli_test.go` — 11 CLI end-to-end tests including
  the flow-style regression test.
- `cmd/semantics-tool/mcp/server_test.go` — 7 MCP JSON-RPC integration tests
  via subprocess (initialize, tools/list, stats, create_action,
  add_implementation, validate, error paths).

### Added (further)

- **`ActionZcGroupList` / `ZC_GROUP_LIST` decode (issue #13, worklog 0089)**.
  The packet a rAthena server sends to a client to deliver the full party
  roster when the client joins an existing party (rAthena
  `src/map/party.cpp:676`, `party_member_added` → `clif_party_info`).
  Without this packet decoded, pre-existing party members remain invisible
  to the joining client — they do not trigger per-member spawn packets
  (`ZC_NOTIFY_MEMBERINFO_TO_GROUPM`), they only appear in this roster.

  Three wire IDs cover three PACKETVER-conditional SUB layouts, all
  dispatched under `ActionZcGroupList`:

  | PACKETVER range | Packet ID | SUB size | Fields added |
  |---|---|---|---|
  | `< 20170524` (MAIN) | `0x00FB` | 46 | AID + playerName + mapName + leader + offline |
  | `20170524 ≤ pv < 20171207` | `0x0A44` | 50 | + class_ + baseLevel |
  | `≥ 20171207` (production) | `0x0AE5` | 54 | + GID after AID |

  The third variant (`0x0AE5`) is the wire ID at production packetver
  `20200401`; the original issue #13 mentioned only the first two. The
  production-target decoder is `ZcGroupList_0x0AE5`.

  rAthena encodes the leader/offline bytes inverted relative to the
  intuitive bool (`clif.cpp:7892-7893`: leader byte 0 = leader,
  offline byte 0 = online). The decoder flips both back to intuitive Go
  bool field values.

  **Allocation note**: each decoder calls `make([]ZcGroupListMember, n)`
  — one heap alloc per packet, unavoidable for a variable-count roster.
  Documented exception to the 0-alloc decode hot-path contract, matching
  the inventory list events (worklog 0066).
  `BenchmarkZcGroupList_0x0AE5`: 347 ns/op, 1 allocs/op.

### Changed (non-breaking, further)

- `pkg/session/actions.go`: new constant `ActionZcGroupList SemanticAction
  = 464`. Appended at the next free ID (not slotted alphabetically) to
  avoid renumbering existing constants. `maxSemanticAction` bumped.
- `pkg/session/receive_dispatch.go`: three new entries under
  `ActionZcGroupList` for `0x00FB`, `0x0A44`, `0x0AE5`.
- `semantics/mappings.yaml`: new `zc_group_list` action with three
  packetver-bounded implementations. **Added via the in-repo
  `cmd/semantics-tool` CLI** (worklog 0088) — first Rule 9 DB edit done
  through the MCP/CLI tooling rather than a hand-edit.
- `.github/workflows/ci.yml`: `BenchmarkZcGroupList` added to the
  benchmark allocs allowlist (variable-length roster — 1 alloc/op
  documented exception, same class as `BenchmarkDecode(Normal|Equip)Items`).

### Tests (further)

- `pkg/decode/zc_group_list_test.go` — 7 golden tests covering all three
  layouts, empty roster, packetLen propagation, partial-trailing-member
  edge case, and a truncated-header robustness test (3 sub-tests × 3
  decoders = 9 cases asserting no panic on malformed frames). Plus 2
  benchmarks (1 allocs/op each, documented exception).
- `pkg/session/zc_group_list_dispatch_test.go` — 5 end-to-end dispatch
  tests including the exact issue #13 reproduction (0x0AE5 frame at
  `pv=20200401` fires the handler once, leaves `UnhandledPackets() == 0`),
  the legacy 0x00FB variant, the mid-layout 0x0A44 variant, the dispatch
  entry-count guard, and a compile-time regression guard for the new
  `ActionZcGroupList` constant.

---

## [v0.7.0] — 2026-07-09

### Fixed

- **`ActionZcNotifyMapproperty2` never fired at production packetvers** — at modern
  packetvers rAthena emits the map-property packet as `0x099B` (`ZC_MAPPROPERTY_R2`,
  8-byte layout: `type` + `flags` bitfield) via `clif_map_property()`, not `0x01D6`
  (`ZC_NOTIFY_MAPPROPERTY2`, 4-byte layout) via `clif_map_type()`. rathena-client's
  `semantics/mappings.yaml` only registered `0x01D6` with an unbounded packetver range,
  so on modern servers the `0x099B` packet arrived on the wire, was framed correctly
  (lengths_map.go already had `t[0x099B] = 8`), but never dispatched to any semantic
  handler. Downstream consumers gating on `ActionZcNotifyMapproperty2` (e.g. goKore's
  "server is ready after map entry" signal — `sd->prev` becomes set immediately before
  `clif_map_property` is emitted) never received the event. (GitHub issue #9)

  **Boundary nuance:** the source-level `#if PACKETVER >= 20121010` in
  `clif.cpp:6873` (which controls which `cmd` rAthena compiles in) is *not* the
  wire-effective boundary. The packetdb registration
  (`clif_packetdb.hpp:1600-1645`, `#if PACKETVER >= 20130320`) is what governs
  when `0x099B` actually appears on the wire. In the `20121010 ≤ pv < 20130320`
  gap, `clif_map_property()` calls `clif_send(buf, packet_len(0x99b), ...)` but
  `packet_len` returns 0 — a zero-length no-op. The dispatch entry is therefore
  dead (but harmless) in the gap. The mapping registers the implementation at
  `pv >= 20121010` (matching the source-level boundary); the test exercises it
  at `pv=20200401` (well past the wire-effective boundary).

  The mapping now declares both variants with packetver bounds:

  | PACKETVER range | Packet ID | Struct | Bytes | Fields |
  |---|---|---|---|---|
  | `< 20121010` | `0x01D6` | `PACKET_ZC_NOTIFY_MAPPROPERTY2` | 4 | `Type` |
  | `≥ 20121010` | `0x099B` | `SYNTH_ZC_MAPPROPERTY_R2` | 8 | `Type`, `Flags` |

  `0x099B` has no C struct in rAthena (`clif_map_property` builds it with raw
  `WBUFW`/`WBUFL` calls), so a new `SYNTH_ZC_MAPPROPERTY_R2` packed stub was added to
  `internal/codegen/stubs/synthetic_structs.hpp` documenting the wire layout derived
  from `src/map/clif.cpp:6871-6903` and confirmed by `clif_packetdb.hpp:1642`
  (`packet(0x099b,8); //maptypeproperty2`). OpenKore names this packet `map_property3`
  (`ServerType0.pm:589`); the rAthena comment in `clif.cpp:6870` calls it
  `ZC_MAPPROPERTY_R2`. rathena-client keeps both `0x01D6` and `0x099B` under the
  existing `ActionZcNotifyMapproperty2` so the action doubles as a "map-ready" signal
  across all packetvers.

  Sources: `src/map/clif.cpp:6868-6903` (`clif_map_property` — both layouts),
  `src/map/clif.cpp:10811-10844` (`clif_parse_LoadEndAck` — where `map_addblock` and
  the subsequent `clif_map_property` call live),
  `src/map/clif.cpp:25784` (silent-drop rule when `sd->prev == nullptr`),
  `src/map/packets.hpp:966-970` (`PACKET_ZC_NOTIFY_MAPPROPERTY2`, the 0x01D6 struct).

### Changed (breaking)

- **`events.ZcNotifyMapproperty2` gains a `Flags uint32` field** to model the 8-byte
  `0x099B` layout. Code constructing the struct with named fields is unaffected; code
  using positional struct literals (`events.ZcNotifyMapproperty2{value}`) must add the
  second field. The `0x01D6` decoder leaves `Flags` at the zero value (the 4-byte
  layout has no flags bitfield), preserving backward compatibility for legacy-packetver
  consumers that only inspect `Type`.

### Tests

- Added `pkg/session/mappropr2_dispatch_test.go` covering: (a) both `0x01D6` and
  `0x099B` are present in `receiveDispatch[ActionZcNotifyMapproperty2]`, (b) the exact
  issue #9 reproduction — feeding an 8-byte `0x099B` frame at `pv=20200401` fires the
  handler once with `Type=1`/`Flags=0x467` and leaves `UnhandledPackets=0`, and (c) a
  regression guard that `0x01D6` still fires and leaves `Flags=0`.
- Added `pkg/decode/zc_notify_mapproperty2_test.go` with zero-value edge cases,
  packetver-agnostic decoder checks, and `BenchmarkZcNotifyMapproperty2_0x099B` /
  `BenchmarkZcNotifyMapproperty2_0x01D6` verifying 0 allocs/op on the decode hot path
  (Rule 1). Locally: 0.37 ns/op, 0 allocs/op for `_0x099B`; 0.36 ns/op, 0 allocs/op for
  `_0x01D6`.

---

## [v0.6.8] — 2026-07-06

### Fixed

- **`EncodeRepairItem` emitted wrong wire layout at `PACKETVER >= 20181121`** — the
  encoder was a codegen artifact that discarded `packetver` (`_ = packetver`) and
  always emitted a fixed `[15]byte` packet with `uint16 itemId` and `uint16[4]` card
  slot. At production packetvers (`20200401`), rAthena expects `uint32 itemId` and
  `uint32[4]` card slot (25 bytes for `0x01FD`, or 26 bytes + new packet ID `0x0B66`
  for `PACKET_CZ_REQ_ITEMREPAIR2`). Sending 15 bytes was rejected by the server (or
  worse, tripped the anti-exploit guard in `clif_parse_RepairItem`). (GitHub issue #7,
  worklog 0086)

  The encoder now branches on PACKETVER across three regimes, all verified by GCC
  preprocessor output and empirical `sizeof`/`offsetof` against the actual rAthena
  structs:

  | PACKETVER range | Packet ID | Bytes | Layout |
  |---|---|---|---|
  | `< 20181121` | `0x01FD` | 15 | narrow: `uint16` itemId, `uint16[4]` card |
  | `20181121 ≤ pv < 20191224` | `0x01FD` | 25 | wide: `uint32` itemId, `uint32[4]` card |
  | `≥ 20191224` | `0x0B66` | 26 | REPAIR2: slot before refine, `grade` appended |

  PACKETVER boundary reconciliation: the `clif_packetdb.hpp` registration (the binding
  contract for which packet IDs the server accepts) gates `0x0B66` at `>= 20191224`,
  matching the struct definition. The `clif.cpp` dispatcher's `>= 20200916` cast
  boundary only affects an internal pointer cast and does not affect wire correctness
  (the server only reads `p->item.index` at offset 2 in both structs).

  Sources: `src/map/packets_struct.hpp:410-416` (EQUIPSLOTINFO),
  `src/map/packets_struct.hpp:2901-2948` (REPAIRITEM_INFO1/2, PACKET_CZ_REQ_ITEMREPAIR1/2),
  `src/map/clif_packetdb.hpp:256,1975-1978` (packetdb registrations),
  `src/map/clif.cpp:13265-13287` (clif_parse_RepairItem dispatcher).

- **CI benchmark allowlist incomplete (pre-existing failure on main)** — the
  0-allocs/op benchmark check in `.github/workflows/ci.yml` was missing
  `BenchmarkEncodeBattleChat`, `BenchmarkEncodePartyChat`, and `BenchmarkEncodeWhisper`
  from its allowlist of legitimately-allocating variable-length encoders. These three
  encoders return `[]byte` and allocate 1/op on the CI runner (Go's escape analysis
  cannot keep the slice on the stack when the length depends on runtime input). CI had
  been failing on every `main` push since 2026-07-04 because of this. Added
  `BattleChat`, `PartyChat`, `Whisper`, and `RepairItem` (new in this release) to the
  allowlist and clarified the comment.

### Changed (breaking)

- **`send.RepairItem` struct field types changed** to accommodate all three wire
  layouts. Update any struct literals:

  ```go
  // Before
  type RepairItem struct {
      Index  int16
      ItemId uint16
      Refine uint8
      Card   []byte
  }

  // After
  type RepairItem struct {
      Index  int16
      ItemId uint32   // narrowed to uint16 on wire at pv < 20181121
      Refine uint8
      Card   [4]uint32 // narrowed to uint16[4] on wire at pv < 20181121
      Grade  uint8     // REPAIR2 only (pv >= 20191224); ignored earlier
  }
  ```

  `Card` is now a typed `[4]uint32` array (was untyped `[]byte`). `Grade` is new
  (only emitted on the REPAIR2 wire layout). No production code in this repo or
  goKore references `send.RepairItem` yet, so the blast radius is zero today.

### Tests

- **`pkg/encode/repair_item_test.go`** (new, TDD) — 10 tests + 3 benchmarks covering:
  - All three wire layouts at boundary packetvers (20180307, 20181120, 20181121,
    20190000, 20191223, 20191224, 20200401, 20200916)
  - Hand-synthesized golden bytes for narrow (15B), wide (25B), and REPAIR2 (26B)
  - REPAIR2 field order (slot before refine, grade appended) — the critical layout
    difference from REPAIR1
  - Card and itemId uint32→uint16 truncation on narrow wire
  - Adjacent-day boundary tests (20181120→20181121, 20191223→20191224)
  - `index` field survives at offset [2..3] in all three layouts (the only field
    the server dispatcher reads)

  Benchmarks: 24–33 ns/op, 1 alloc/op (variable-length `[]byte` return — same pattern
  as `EncodeBattleChat`, `EncodeWhisper`).

---

## [v0.6.4] — 2026-04-11

### Fixed

- **Map entry timeout for `pv ∈ [20141016, 20141021]`** — `ZC_ACCEPT_ENTER` handler
  was registered for `0x0A18` when the condition used `>= 20141016`, but
  `src/map/packets.hpp:554` defines the `0x0A18` era as `>= 20141022 && < 20160330`.
  For those six packetvers rAthena sends `0x02EB`; with the wrong registration
  `onMapEnter` never fired and the FSM timed out waiting for map entry.
  Fixed by extracting `zcAcceptEnterID(packetver)` which mirrors the exact
  `packets.hpp` condition. Source: `src/map/packets.hpp:545-575`.

---

## [v0.6.3] — 2026-04-11

### Fixed

- **Map login disconnected immediately on RE `[20211103, 20211118]` and MAIN `>= 20220330`** —
  `fsmEncodeMapLogin` always sent a 19-byte `0x0436` packet regardless of packetver.
  For `PACKETVER_RE_NUM >= 20211103` or `PACKETVER_MAIN_NUM >= 20220330`, rAthena registers
  `0x0436` at 23 bytes (`sex` at offset 22, extra 4-byte `tick` field at offset 18).
  `clif_parse_WantToConnection_sub` (`clif.cpp:10625`) performs a strict length check and
  calls `set_eof(fd)` on mismatch, disconnecting every bot immediately on map entry.
  Both `fsmEncodeMapLogin` (`pkg/session/fsm.go`) and `EncodeMapLogin` (`pkg/encode/map_login.go`)
  now emit 23 bytes for the affected packetver windows and 19 bytes otherwise.
  Source: `clif_shuffle.hpp:4744-4747`, GCC-verified at all boundary points.

---

## [v0.6.2] — 2026-04-07

### Fixed

- **`CharacterInfoEntry.Money` never populated** — `decodeCharacterInfoEntry` skipped
  the `money` field in both PACKETVER branches. For `pv >= 20170830` the field sits at
  offset 12 (int32, between `exp` int64 and `jobexp` int64); for `pv < 20170830` it sits
  at offset 8 (int32, between `exp` int32 and `jobexp` int32). The decoder now reads it
  correctly in both cases. Real-capture test confirms `Almarc.Money = 1,000,000`.
  Source: `rAthena src/common/packets.hpp:38`.

---

## [v0.6.1] — 2026-03-23

### Breaking Changes

- **`OnFailed` signature changed** — was `func(error)`, now `func(FailInfo)`.
  `FailInfo{Phase AuthPhase, Err error}` identifies which auth phase (login/char/map) failed.

- **`OnServerNotify` signature changed** — was `func(uint8)`, now `func(NotifyInfo)`.
  `NotifyInfo{Phase AuthPhase, Code uint8}` tags the phase that received the ban notify.

### Added

- **`AuthPhase`** — new type (`PhaseLogin`, `PhaseChar`, `PhaseMap`) with `String()`.
  Used in `FailInfo` and `NotifyInfo` to identify which auth stage triggered a callback.

- **`CharacterInfoEntry.JobLevel int32`** — rAthena: `joblevel`; OpenKore: `lv_job`.
  Offsets: 16 (pv<20170830), 24 (pv>=20170830). Required for skill point budget before map entry.

- **`CharacterInfoEntry.Speed int16`** — rAthena: `speed`; OpenKore: `walkspeed`.
  Offsets: 54 / 62 / 82 by breakpoint. Required for movement timing prediction.

- **`IdentityInfo.MapDomain string`** — rAthena: `domain[128]` in `PACKET_HC_NOTIFY_ZONESVR`
  (0x0AC5, pv >= 20170315). Format: `"hostname"` or `"hostname:port"` (OpenKore: `mapUrl`).
  When non-empty, callers should prefer this over `MapIP`. Empty for pv < 20170315 (0x0081).

- **`CharServerInfo.Users uint16`** — rAthena: `PACKET_AC_ACCEPT_LOGIN_sub.users`.
  Present in both 32-byte and 160-byte sub-entry variants.
  OpenKore: `users` field in `parse_account_server_info`. Enables population-based server selection.

- **`SlotInfo`** — new struct `{Normal, Premium, Billing, Producible, Total uint8}`.
  rAthena: `PACKET_HC_ACCEPT_ENTER2` (0x082D, pv >= 20130000), `packets.hpp:508–517`.
  OpenKore: `$charSvrSet{normal_slot}`, `{billing_slot}`, etc. in `received_characters_slots_info`.

- **`OnSlotInfo(func(SlotInfo))`** — new optional callback, fires when 0x082D is received
  before the char list. Previously the 0x082D handler was a no-op.

### Tests

- `TestDecodeCharacterInfoEntry_JobLevelAndSpeed` — 5 breakpoints × sentinel values
- `TestDecodeCharacterInfoEntry_B8_RealCapture_JobLevelSpeed` — 4 real captured characters
- `TestConnect_OnIdentity_MapDomain` — domain present (0x0AC5) and empty (0x0081) subtests
- `TestConnect_OnSlotInfo` — slot counts round-trip via 0x082D
- Extended `TestConnect_OnCharServerList` with `Users` assertions
- Extended `TestConnect_LoginRefused`, `TestConnect_CharServerRefused`, `TestConnect_MapRefused` with `FailInfo.Phase` assertions
- Extended `TestConnect_ServerNotifyBan_Login` with `NotifyInfo.Phase` assertion

---

## [v0.6.0] — 2026-03-23

### Breaking Changes

- **`ConnectionFSM.OnCharList` signature changed** — was `func([]byte) uint8`, now
  `func([]events.CharacterInfoEntry) uint8`. Callers must update their callback.
  No backwards-compat `OnCharListRaw` alias; migrate directly.

### Added

- **`events.CharacterInfoEntry`** — decoded form of one `CHARACTER_INFO` element
  from char-server packets (`HC_ACCEPT_ENTER` 0x006B, `HC_ACK_CHARINFO_PER_PAGE`
  0x099D / 0x0B72). All PACKETVER-conditional field widths normalised to widest form.

  Fields: `GID uint32`, `Exp int64`, `JobExp int64`, `HP int64`, `MaxHP int64`,
  `SP int64`, `MaxSP int64`, `Job int16`, `Level int16`, `Name string`,
  `MapName string`, `CharNum uint8`, `Sex uint8`.

  Source: `rAthena src/common/packets.hpp:31–105`. GCC-verified at 10 PACKETVER
  snapshots (B0 112 B → B9 175 B). Cross-checked against real DUMP17 capture
  (4 characters, pv=20200401).

- **`decode.DecodeCharacterInfoList`** (exported) — decodes a `CHARACTER_INFO[]`
  byte slice into `[]events.CharacterInfoEntry` at the given packetver. Handles all
  10 PACKETVER breakpoints including the `exp`/`jobexp` int32→int64 widening at
  20170830 and the `hp`/`sp` widening at 20220330 (MAIN).

- **`IdentityInfo.MapIP uint32` and `IdentityInfo.MapPort uint16`** — map server
  address previously parsed by the FSM from `HC_NOTIFY_ZONESVR` and silently
  discarded. Now surfaced to `OnIdentity` callers.

### Changed

- **`events.ReceivedCharacters.Characters`** — type changed from `[]byte` to
  `[]CharacterInfoEntry`. Same for `events.ReceivedCharactersPage.Characters`.
  Decode functions for 0x006B, 0x099D, 0x0B72 updated accordingly.

### Tests

- **`pkg/decode/character_info_test.go`** (new, TDD, 13 tests) — covers B0/B2/B5/B7
  (synthesised golden bytes from GCC-verified offsets), B8 (synthesised + 4 real
  captured characters from DUMP17), B9 (widened hp/sp beyond old widths),
  list decode, empty/partial-trailing/too-short edge cases,
  null-terminated name and mapName.

---

## [v0.5.15] — 2026-03-23

### Added

- **`ActionZcPartyJoinReq`** — new receive-direction semantic action for
  `PACKET_ZC_PARTY_JOIN_REQ`. Server sends this when another player invites the
  client to join their party. (goKore bug report 0807, worklog 0078)

  Two packet IDs supported:
  - `0x02C6` — modern (`pv >= 20110718`), `DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ, 0x02c6)`
  - `0x00FE` — legacy (`pv < 20110718`), same struct layout

  Generated artifacts: `pkg/events/zc_party_join_req.go` (`ZcPartyJoinReq{GRID []byte, GroupName string}`),
  `pkg/decode/zc_party_join_req.go` (two decoder functions), dispatch entries in
  `receive_dispatch.go`, `ActionZcPartyJoinReq SemanticAction = 396` in `actions.go`.

  GCC-verified wire layout at pv=20200401 (`packets_struct.hpp:5082`):
  `int16 PacketType` + `int GRID` (4B LE, party ID) + `char groupName[24]` = 30 bytes.

  Note: `GRID` is `[]byte` in the Go event struct (standard codegen pattern for C `int`
  fields). Callers use `binary.LittleEndian.Uint32(e.GRID[:4])` to extract the party ID.

### Tests

- **`pkg/decode/zc_party_join_req_test.go`** (new, TDD) — decodes both packet IDs,
  verifies NUL-padding stripped from `GroupName`, `TestActionZcPartyJoinReq_Exists`
  compile-time regression guard. `BenchmarkZcPartyJoinReq_0x02C6`: 0 allocs/op, 7 ns/op.

---

## [v0.5.14] — 2026-03-23

### Fixed

- **`ActionBattleChat`, `ActionPartyChat`, `ActionSetWhisperState` missing** — three
  hand-written send encoders existed but had no semantics DB entries, so codegen never
  emitted SemanticAction constants or `RegisterSendEncoder` calls. Same root cause as
  `ActionWhisper` (v0.5.13 / worklog 0076). (worklog 0077)

  | Encoder | Packet | Wire |
  |---|---|---|
  | `EncodeBattleChat` | `CZ_BATTLEFIELD_CHAT` `0x02DB` (variable-length) | rAthena `clif_packetdb.hpp:921` |
  | `EncodePartyChat` | `CZ_PARTY_MESSAGE` `0x0108` (variable-length) | rAthena `clif_packetdb.hpp:108` |
  | `EncodeSetWhisperState` | `CZ_SETTING_WHISPER_PC` `0x00CF` (fixed 27 bytes) | rAthena `clif_packetdb.hpp:78` |

  All three are stable IDs — not in `clif_shuffle.hpp`. Fix: SYNTH stubs added to
  `synthetic_structs.hpp`, three actions added to semantics DB via MCP, codegen
  regenerated. `ActionSetWhisperState` correctly uses the fixed-array `b[:]` registration
  path (codegen detects `[27]byte` return via AST scan).

  Note on `set_whisper_state`: `0x00CF` (`clif_parse_PMIgnore`, nick-specific ignore, 27 bytes)
  is distinct from `0x00D0` (`PACKET_CZ_SETTING_WHISPER_STATE`, 3-byte bulk state setter).

### Tests

- **`pkg/encode/chat_actions_test.go`** (new, TDD) — wire format and
  `TestActionXxx_Registered` compile-time regression tests for all three actions.
  `BenchmarkEncodeSetWhisperState`: 0.2 ns/op, 0 allocs/op ✓

---

## [v0.5.13] — 2026-03-23

### Fixed

- **`session.ActionWhisper` was missing** — `EncodeWhisper` and `send.Whisper` existed
  but there was no `ActionWhisper` SemanticAction constant and no `RegisterSendEncoder`
  call. Any goKore code calling `session.Send(..., session.ActionWhisper, ...)` failed
  to compile. (goKore bug report 0799, worklog 0076)

  Root cause: `actions.go` is fully generated from the semantics DB. The `whisper` action
  had no DB entry at all — codegen emits no constant without one, regardless of whether
  the encoder file exists.

  Fix:
  - Added `SYNTH_CZ_WISPER` 2-byte stub to `synthetic_structs.hpp` (same pattern as
    `SYNTH_CZ_NOTIFY_ACTORINIT` — CZ_WISPER has no C struct in rAthena)
  - Added `whisper` action to semantics DB via MCP with `0x0096` / `SYNTH_CZ_WISPER`
  - Ran codegen: emits `ActionWhisper SemanticAction = 246` in `actions.go` and
    `RegisterSendEncoder(ActionWhisper, ...)` in `register.go` using variable-length
    `[]byte` path (since `EncodeWhisper` returns `[]byte`)

  Note: `ActionWhisperSent` shifted from 246 → 247 as a result.

### Tests

- **`pkg/encode/whisper_test.go`** (new, TDD) — wire format, empty target, long target
  truncation, and `TestActionWhisper_Registered` (the direct regression test: compile
  fails if `session.ActionWhisper` is undefined). `BenchmarkEncodeWhisper`: 1 alloc/op,
  54 ns/op (expected — single `make([]byte)` for variable-length output).

---

## [v0.5.12] — 2026-03-22

### Fixed

- **Receive-direction packetver range gaps** — systematic audit found wrong
  `packetver_min`/`packetver_max` values causing packets to be silently unhandled
  at certain packetvers. All fixes sourced directly from `packets_struct.hpp`
  enum definitions verified by GCC preprocessor:

  **`actor_connected` / `actor_exists` — 33-month gap closed**
  - `0x09FE` (`actor_connected`) `packetver_min`: `20181121` → `20150513`
  - `0x09FF` (`actor_exists`) `packetver_min`: `20181121` → `20150513`
  - Source: `spawn_unitType = 0x9fe` and `idle_unitType = 0x9ff` for
    `PACKETVER >= 20150513` in `packets_struct.hpp`
  - Impact: any server at `20150513 <= pv < 20181121` (including `pv=20170315`)
    had actor spawn/idle packets silently dropped

  **`actor_moved` — wrong range on `0x09DB`**
  - `0x09DB` was `[20181121, null]` — completely wrong. Correct range is
    `[20131223, 20150512]` (`unit_walkingType = 0x9db` for `PACKETVER < 20150513`)
  - `0x09FD` `packetver_min`: `20141022` → `20150513`
    (`unit_walkingType = 0x9fd` for `PACKETVER >= 20150513`)
  - Source: `unit_walkingType` enum in `packets_struct.hpp`

  **`zc_req_wear_equip_ack` — 2-year gap closed**
  - `0x00AA` extended from `[null, 20101122]` to `[null, 20121204]`
  - `0x0999` `packetver_min`: `20121107` → `20121205`
  - Source: `PACKETVER_MAIN_NUM >= 20121205` condition in `packets_struct.hpp`
    (the previous `20121107` was the RE boundary, not MAIN)

### Tests updated

- `internal/codegen/semantics/validate_test.go` — `actor_exists 0x09FF` expected
  `pvMin` updated `20181121` → `20150513`
- `internal/codegen/semantics/epic08_test.go` — `zc_req_wear_equip_ack` boundaries
  updated to reflect MAIN boundary (`20121204`/`20121205`)
- `pkg/decode/phase1_golden_test.go` — `makeActorMoved0x09DB_20181121` updated to
  108-byte layout (pv=20131223, no shield/body fields); added `makeActorMoved_114byte`
  for 0x007B golden test; benchmarks updated to use correct pvs
- `pkg/decode/gaps_test.go` — `0x09DB` name test updated to 108-byte layout with
  name at offset 84 (was offset 90 in 114-byte layout)

---

## [v0.5.11] — 2026-03-21

### Fixed

- **Codegen shuffle overlap lint rule now handles false positives correctly** — the
  `genEncodeWithShuffleCheck` allowlist was previously empty, causing three false-positive
  violations on every codegen run:
  - `friends_add` (0x0202): hand-written encoder already uses `shuffledCtoSID` — allowlisted
    by scanning hand-written files before `cleanGeneratedDir` runs
  - `homunculus_menu` (0x022D): out-of-scope (homunculus/mercenary not supported) — explicit
    exception added to allowlist
  - `master_login` (0x0064): CA_ login-server packet that shares an ID with a map-server
    shuffle entry by coincidence (different server, never shuffled) — explicit exception added

  The lint rule now passes cleanly on every codegen run and correctly rejects only genuinely
  broken generated encoders.

- **`cz_party_join_req` semantics DB range corrected** — the implementation was `[null, null]`
  implying `0x02C4` = party invite across all packetvers. In fact `0x02C4` was reassigned to
  `clif_parse_PartyInvite2` (party invite, size=26) only from `pv >= 20111102`. Before that,
  `0x02C4` was `clif_parse_UseSkillToId` (size=10). Tightened to `packetver_min=20111102`.
  Cross-validated: `clif_shuffle.hpp` stable post-20180307 block confirms `0x02C4 → clif_parse_PartyInvite2 size=26`; OpenKore `RagexeRE_2018_11_21.pm` confirms `party_join_request_by_name 02C4`.

- **`close_storage.go`** promoted to hand-written with a protocol-removal warning.
  `clif_parse_CloseKafra` was removed from rAthena after `pv=20050110`. The encoder
  still produces `0x00F7` but is clearly documented as legacy-only. The semantics DB
  was already bounded to `pv <= 20050110` (v0.5.6).

### Verified (no fix needed)

- **`friends_add` (0x0202) shuffle era cross-validation** — all 152 weekly shuffle
  blocks in `shuffle_map.go` cross-checked against OpenKore kRO modules for `friend_request`
  (the OpenKore name for this packet). OpenKore explicitly defines `0x0962` at `pv=20130515`
  and `0x0362` at `pv=20130522`, both matching `shuffledCtoSID(pv, 0x0202)`. Zero mismatches
  across all verifiable weekly entries. The `friends_add` encoder is correct.

- **`cz_party_join_req`, `friends_remove`, `friends_reply`** — all confirmed correct,
  no fixes needed (see v0.5.10 for full analysis).

### Cleanup

- Deleted root-level integration test binaries (`gokore_api_check`, `new_api_check`,
  `verify_new_api`) — these are now `.gitignored` since v0.5.10.

---

## [v0.5.10] — 2026-03-21

### Changed

- **`pkg/encode/character_move.go`** — promoted from generated to hand-written so the
  packetver scope limitation comment (`pv >= 20101124` only) survives future codegen runs.
  Content is functionally identical; header changed from `// Code generated` to
  `// Manually maintained`.

### Confirmed no-fix-needed

Deeper investigation of the three "accidentally-correct" encoders from worklog 0073/0074
confirmed all three are actually fully correct:

- **`cz_party_join_req` (0x02C4)** — `0x02C4` was reassigned from `clif_parse_UseSkillToId`
  to `clif_parse_PartyInvite2` (party invite) at `pv >= 20111102`. Checking all weekly shuffle
  blocks (20130515–20180307): `0x02C4` is never remapped — every block returns `baseID`.
  Confirmed by OpenKore `RagexeRE_2018_11_21.pm`: `party_join_request_by_name 02C4`.
- **`friends_remove` (0x0203)** — single stable entry, never shuffled. Correct.
- **`friends_reply` (0x0208)** — stable at `pv >= 20040705` (14 bytes). Never shuffled.
  Correct for all production servers.

### Cleanup

- **`.gitignore`** — added `gokore_api_check`, `new_api_check`, `verify_new_api` to ignore
  integration test binaries built at repo root.
- **`docs/WORKLOG/0068–0070, 0072`** — committed untracked original bug report worklogs
  (historical record for fixes in v0.5.4–v0.5.8).
- **`docs/WORKLOG/0074`** — addendum documenting the confirmed-correct findings for the
  three "accidentally-correct" encoders.

---

## [v0.5.9] — 2026-03-21

### Fixed

- **`EncodeFriendsAdd` sent wrong wire ID for shuffle-era packetvers (20130515–20180307)** —
  `0x0202` base ID appears in `clif_shuffle.hpp` and is remapped every weekly block
  (e.g. `0x0962` at pv=20130515, `0x08AA` at pv=20180307). The encoder hardcoded `0x0202`
  and ignored `packetver`. Now uses `shuffledCtoSID(pv, 0x0202)` for `pv >= 20130515`;
  pre-shuffle and post-20180307 stable both return `0x0202`. (worklog 0074)

### Added

- **Codegen shuffle overlap lint rule** (`internal/codegen/gen/encode.go:GenerateEncodeDirFilesWithShuffleCheck`) —
  the root cause of all send-encoder ID bugs in worklogs 0069–0074. When the codegen
  runs, it now validates that no generated encoder hardcodes a packet ID that appears in
  `clif_shuffle.hpp`. If a violation is found, the codegen **fails** with an actionable
  error message rather than silently shipping wrong wire bytes.

  This check would have caught `drop_item`, `look`, `move_from_storage`, `move_to_storage`,
  `skill_use_location`, `pickup_item`, `item_use`, and `friends_add` before they shipped.

  `main.go` wires the check into `genEncodeWithShuffleCheck` (called for step 8). Shuffle
  base IDs are pre-built from `clif_shuffle.hpp` + `clif_packetdb.hpp` before encoding.
  If `clif_shuffle.hpp` is unavailable, the check is skipped with a warning (non-fatal).

### Tests

- **`TestGenerateEncodeDirFilesWithShuffleCheck_DetectsOverlap`** — asserts that a
  generated encoder for `drop_item 0x00A2` (a shuffled ID) is detected and rejected
- **`TestGenerateEncodeDirFilesWithShuffleCheck_AllowlistSuppresses`** — asserts that
  allowlisted IDs are not flagged
- **`TestGenerateEncodeDirFilesWithShuffleCheck_StableIDPassesThrough`** — asserts that
  stable IDs (not in shuffle map) pass through cleanly
- **`TestGenerateEncodeDirFilesWithShuffleCheck_RealDB`** — end-to-end regression guard:
  loads the real semantics DB and asserts no current action would generate a shuffle overlap.
  Allowlists `0x022D` (homunculus_menu) and `0x0233` (homunculus_attack) as explicitly
  out-of-scope (homunculus/mercenary not supported).
- **`pkg/encode/friends_add_test.go`** (new, TDD) — covers all boundary transitions
  including `pv=20130515` → `0x0962`, `pv=20130522` → `0x0362`, `pv=20180307` → `0x08AA`,
  and post-shuffle `pv=20200401` → `0x0202`.

### Documentation

- **`pkg/encode/character_move.go`** — added limitation note: only supports
  `pv >= 20101124`; callers needing full packetver coverage should use `ActionMoveTo`.
- **`pkg/send/look.go`** — hand-written, adds field godoc for `HeadDir` (valid 0–2,
  rAthena `headdir`, wire offset 2) and `Dir` (valid 0–7 clockwise from N, rAthena `dir`,
  wire offset 4 after padding byte at offset 3).
- **`semantics/mappings.yaml`** — `friends_add` 0x0202 bounded to `pv <= 20130514`;
  `character_move` 0x035F bounded to `pv >= 20101124`.
- **`docs/WORKLOG/0074`** — full validated analysis: confirmed decode layer has zero gaps
  (the earlier "109 missing decoders" count was a false positive), confirmed `cz_party_join_req`/
  `friends_remove`/`friends_reply` are NOT in shuffle map (no fix needed), documented
  `character_move` as structural duplicate of `move_to` with a packetver scope limitation.

---

## [v0.5.8] — 2026-03-21

### Fixed (worklog 0072 — `enter_world`)

- **`ActionEnterWorld` had no registered send encoder** — `send.EnterWorld{}` and
  `ActionEnterWorld = 141` existed but `EncodeEnterWorld` was absent, forcing goKore
  to use a raw `conn.Write([]byte{0x7D, 0x00})` workaround. The encoder is now
  hand-written (`enter_world.go`) and codegen picks it up via `existingEncoders`
  into `register.go`'s `init()`. `0x007D` is stable across all packetvers (single
  entry in `clif_packetdb.hpp:32`, absent from `clif_shuffle.hpp`).

  Note: worklog 0072 proposed `ActionCzNotifyActorinit` / `send.CzNotifyActorinit` —
  both are incorrect names. The correct action is `ActionEnterWorld` / `send.EnterWorld`
  which already existed. No new constants or structs were needed.

### Fixed (worklog 0073 — 5 shuffle-era send encoder ID bugs)

All five encoders hardcoded the pre-shuffle base packet ID and ignored `packetver`.
Each has been replaced with a hand-written dispatcher following the `move_to.go` /
`pickup_item.go` pattern (semantics DB codegen cannot express `shuffledCtoSID()` calls).
Cross-validated against rAthena `clif_shuffle.hpp` stable post-20180307 block and
OpenKore `RagexeRE_2018_11_21.pm`.

- **`drop_item.go`** (`0x00A2` → `shuffledCtoSID(pv, 0x00A2)`, stable `0x0363` post-20180307)
  — 6 bytes, `index(u16)@[2:4]`, `amount(u16)@[4:6]`

- **`look.go`** — triple bug: wrong ID (`0x009B`), wrong size (`[4]byte` instead of `[5]byte`),
  wrong `Dir` offset (`p[3]` instead of `p[4]`). The `Dir` field was silently dropped entirely.
  Fixed: `shuffledCtoSID(pv, 0x009B)`, stable `0x0361`, `[5]byte`,
  `headDir(u8)@[2]`, `padding(0x00)@[3]`, `dir(u8)@[4]`. Layout confirmed by rAthena
  `RFIFOB(pos[0]=2)` / `RFIFOB(pos[1]=4)` and OpenKore pack `'v C'` (5 bytes total).

- **`move_from_storage.go`** (`0x00F5` → `shuffledCtoSID(pv, 0x00F5)`, stable `0x0365`)
  — 8 bytes, `index(u16)@[2:4]`, `amount(u32)@[4:8]`

- **`move_to_storage.go`** (`0x00F3` → `shuffledCtoSID(pv, 0x00F3)`, stable `0x0364`)
  — 8 bytes, `index(u16)@[2:4]`, `amount(u32)@[4:8]`

- **`skill_use_location.go`** (`0x0116` → `shuffledCtoSID(pv, 0x0116)`, stable `0x0366`)
  — 10 bytes, `skillLevel(u16)@[2:4]`, `skillID(u16)@[4:6]`, `xPos(u16)@[6:8]`, `yPos(u16)@[8:10]`

### Fixed (semantics DB — packetver bounds corrected)

All via MCP (no direct YAML edits):

- `drop_item`, `look`, `move_from_storage`, `move_to_storage`, `skill_use_location`:
  existing `[null, null]` implementation replaced with `[null, 20101123]` (legacy)
  + `[20101124, null]` (modern stable wire ID)
- `close_storage`: `0x00F7` bounded to `[null, 20050110]` — `clif_parse_CloseKafra`
  removed from packet tables after pv=20050110 and is absent from `clif_shuffle.hpp`
- `public_chat`: `0x008C` bounded to `[null, 20080909]` — `clif_parse_GlobalMessage`
  absent from shuffle table; last confirmed valid ID is `0x00F3` up to ~pv=20080909

### Tests

- **`drop_item_test.go`** — 18 boundary sub-tests, field encoding, 0 allocs/op (9.8 ns/op)
- **`look_test.go`** — 14 ID boundary sub-tests, length (5 bytes), HeadDir@[2], Padding@[3],
  Dir@[4], DirNotAtOffset3 regression test, 0 allocs/op (4.2 ns/op)
- **`move_from_storage_test.go`** — 14 boundary sub-tests, field encoding, 0 allocs/op (9.3 ns/op)
- **`move_to_storage_test.go`** — 14 boundary sub-tests, field encoding, 0 allocs/op (9.1 ns/op)
- **`skill_use_location_test.go`** — 13 boundary sub-tests, all-fields, 0 allocs/op (10.5 ns/op)
- **`enter_world_test.go`** — wire format across all packetvers, 0 allocs/op (0.26 ns/op)

All tests written before implementation (TDD red → green). `go test ./...` passes.

---

## [v0.5.7] — 2026-03-21

### Fixed

- **`EncodePickupItem` sent wrong packet ID at `pv >= 20101124`** — the encoder
  hardcoded `0x009F` and ignored `packetver`. At `pv >= 20101124`, `clif_parse_TakeItem`
  is reassigned away from `0x009F` through a sequence of 7 explicit packet ID changes
  before entering the shuffle era at `pv >= 20130515`. Sending `0x009F` caused an
  immediate server disconnect on every item pickup attempt.

  Fix: `pkg/encode/pickup_item.go` rewritten as a hand-written packetver dispatcher
  following the `move_to.go` pattern (codegen cannot express the shuffle-era
  `shuffledCtoSID(pv, 0x009F)` runtime call). Full packetver dispatch table:

  | pv range | Wire ID | Source |
  |---|---|---|
  | < 20101124 | `0x009F` | `clif_packetdb.hpp:50` |
  | >= 20101124 | `0x0362` | `clif_packetdb.hpp:1384` |
  | >= 20111005 | `0x0815` | `clif_packetdb.hpp:1402` |
  | >= 20120307 | `0x0865` | `clif_packetdb.hpp:1441` |
  | >= 20120410 | `0x0938` | `clif_packetdb.hpp:1494` |
  | >= 20120418 | `0x07E4` | `clif_packetdb.hpp:1560` |
  | >= 20120702 | `0x089F` | `clif_packetdb.hpp:1587` |
  | >= 20130320 | `0x0933` | `clif_packetdb.hpp:1631` |
  | >= 20130515 | `shuffledCtoSID(pv, 0x009F)` | `clif_shuffle.hpp` |
  | > 20180307 | `0x0362` (stable) | `clif_shuffle.hpp:4723+` |

  Cross-validated against OpenKore kRO Send modules: 57 direct matches, 0 mismatches
  across the full shuffle era. Production `pv=20200401` emits `0x0362` — confirmed
  by both rAthena and OpenKore `RagexeRE_2018_11_21.pm`.
  (worklog 0070/0071)

- **`semantics/mappings.yaml` `pickup_item` action corrected** via MCP:
  `0x009F` implementation upper-bounded to `pv <= 20101123`; `0x0362` implementation
  added for `pv >= 20101124` with `struct SYNTH_CZ_ITEM_PICKUP2` (stable modern wire ID).

### Tests

- **`pkg/encode/pickup_item_test.go`** (new, TDD) — 24 tests covering all 7 explicit
  boundary transitions (both sides), shuffle era entry (`pv=20130515 → 0x08A1`),
  mid-shuffle weekly (`pv=20130522 → 0x095E`), post-shuffle stable (`pv=20200401 → 0x0362`),
  wire length (6 bytes), ITID at `[2:6]`, zero ITID, and cross-packetver ITID
  preservation. Red phase: 19 failures. Green phase: 24/24 pass.

- **`BenchmarkEncodePickupItem`**: 151M ops/s, 8.4 ns/op, **0 allocs/op** ✓

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
