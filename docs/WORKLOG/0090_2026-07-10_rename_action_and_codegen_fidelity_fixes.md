# Work Log 0090 — semanticsdb rename-action + codegen fidelity fixes

**Date**: 2026-07-10
**Type**: Feature (rename-action) + Codegen bug fixes + Mapping corrections
**Scope**:
  - `internal/semanticsdb/mutate.go` — new `DB.RenameAction` primitive + `renameActionsMappingKey` helper
  - `internal/semanticsdb/db_test.go` — 8 new rename tests
  - `cmd/semantics-tool/cli/cli.go` — new `rename-action` subcommand + docstring
  - `cmd/semantics-tool/cli/cli_test.go` — CLI rename tests
  - `cmd/semantics-tool/mcp/tools.go` — new `rename_action` MCP tool + dispatch
  - `cmd/semantics-tool/mcp/server_test.go` — MCP rename test + `TestMCP_ToolsList` count bump 14 → 15
  - `internal/codegen/preprocess/runner.go` — include `packets_hpp_stub.h` for `SourcePacketsStruct`
  - `internal/codegen/stubs/packets_hpp_stub.h` — add `MAX_ITEM_OPTIONS` definition + `CONFIG_PACKETS_HPP` include-guard
  - `internal/codegen/preprocess/vt_check_test.go` — pass `PacketsHPPStub` in test Config
  - `internal/codegen/main.go` — `injectMapPacketStructs` skips VT entries already sourced from `packets_struct.hpp`
  - `semantics/mappings.yaml` — rename `monster_ranged_attack` → `attack_failure_for_distance` + 14 mapping corrections
  - `pkg/decode/gaps_test.go`, `pkg/session/skill_cast_dispatch_test.go`, `pkg/session/zc_group_list_dispatch_test.go` — remove brittle assertions
  - Regenerated files under `pkg/{decode,events,encode,send,session}/` (49 files touched)
  - `CHANGELOG.md`

**Severity**: MODERATE — the rename-action feature is additive. The codegen fixes correct latent-but-real defects that produced wrong offsets and wrong packetver gates in generated decoders. Two of the mapping corrections (`skill_cast` 0x0B1A, `add_exchange_item` 0x0B42/`item_pickup` 0x0B41 splits) were previously worked around by manual patches to generated files that regeneration would revert.

**Reference**: PR #16, review comment https://github.com/lenaxia/rathena-client/pull/16#issuecomment-4939310505

---

## Problem

Three overlapping problems converged in this change:

1. **No supported way to rename a semantic action.** The existing `semantics-tool` had `create-action`, `update-action`, `delete-action`, and their MCP equivalents — but no `rename-action`. To rename `monster_ranged_attack` → `attack_failure_for_distance` (see below), the only options were delete + create (destructive: strips all implementations, moves the entry to the end of document order) or a direct yaml edit (README Rule 21 forbids: "ALWAYS USE THE SEMANTICDB MCP SERVER TO ACCESS THE SEMANTICDB. NEVER GREP OR DIRECTLY MODIFY `semantics/mappings.yaml`").

2. **Two codegen defects produced wrong outputs.** The reviewer's key finding — a decoder panic vector — turned out to be the tip of a broader class of issues:
    - `packets_struct.hpp` references macros defined only in `packets.hpp` / `map.hpp` (notably `MAX_ITEM_OPTIONS`). Preprocessing packets_struct.hpp directly left them undefined, so array members like `ItemOptions option_data[MAX_ITEM_OPTIONS]` parsed as flex arrays of size 0. This mis-aligned every subsequent field in the struct — e.g. `PACKET_ZC_ADD_EXCHANGE_ITEM::Location` decoded at offset 29 instead of 54, `Grade` at 36 instead of 61.
    - `injectMapPacketStructs` was overwriting the fine-grained version ranges from `packets_struct.hpp` with coarser ranges derived from `packets.hpp`'s breakpoint schedule. `packets_struct.hpp` contains the authoritative `#if PACKETVER_MAIN_NUM` guards defining exact transition dates; `packets.hpp` has a different (coarser) set of breakpoints. Result: transitions like `PACKET_ZC_USESKILL_ACK`'s 25→29 byte change were mis-recorded at pv=20191120 (the packets.hpp snapshot that first observed the new size) instead of pv=20181212 (the actual `packets_struct.hpp` guard).
    - `packets_struct.hpp` transitively includes `config/packets.hpp` which auto-defines `PACKETVER_RE` at `PACKETVER >= 20200902` (see rAthena `src/config/packets.hpp:22`). That flips the wire-format guards (`PACKETVER_RE_NUM >= 20200723`) to TRUE, adding the `grade` field to `PACKET_ZC_ITEM_PICKUP_ACK` at pv=20200902 in the codegen's view — even though the length table (derived from `packets.hpp`'s HEADER_ constants under MAIN-only semantics) transitions at pv=20200916. The result: a decoder reading `data[69]` on a slice the framer had sized to 59 bytes — a genuine panic vector.

3. **Latent mapping errors.** Once the codegen bugs were fixed, several `semantics/mappings.yaml` entries revealed themselves as incorrect: they claimed packet IDs with unbounded upper ranges even though `DEFINE_PACKET_HEADER` in rAthena binds a new packet ID at higher pv. For example, `zc_repairitemlist` had only 0x01FC unbounded — but at pv >= 20200916 rAthena binds `ZC_REPAIRITEMLIST` to 0x0B65. This overlap wasn't previously noticed because the codegen bugs hid the resulting inconsistencies.

4. **Misnomer.** `monster_ranged_attack` (action) → packet 0x0139 → rAthena struct `PACKET_ZC_ATTACK_FAILURE_FOR_DISTANCE` (packets_struct.hpp:5419). Sent by `clif_movetoattack` (rAthena src/map/clif.cpp:8171-8188) when a player's attack target is out of range — client walks to (TargetXPos, TargetYPos) and retries. NOT a monster attack; no monster is on the wire. The OpenKore name (`monster_ranged_attack`) is misleading.

---

## Solution

### 1. `rename-action` primitive and tool surface

Added `DB.RenameAction(oldName, newName)` to `internal/semanticsdb/mutate.go`. Design principles matching the surrounding code:

- **Preserve document position.** Renaming does an in-place key replacement in the top-level `semantic_actions:` mapping node (`renameActionsMappingKey`), not delete + append. The action's implementations, `canonical_params`, and metadata are untouched.
- **Preserve unrelated diffs.** The inner `name:` field is updated only when it currently mirrors the old key. In practice this is rarely set (most rows have `name: ""`), so we don't stomp arbitrary values.
- **Reject dangerous inputs.** Blank targets return `ErrEmptyActionName`; existing targets return `ErrActionExists`; missing sources return `ErrActionNotFound`. Same-name rename is a no-op.

Wired into both surfaces:
- CLI: `semantics-tool rename-action <old> <new>` (see cli.go:227-243).
- MCP: `rename_action` tool with `old_name`/`new_name` arguments (see tools.go).

Tests cover: happy path, document-order preservation, inner name mirroring/non-mirroring, not-found, existing-target rejection, empty-target rejection, and same-name no-op. `TestMCP_ToolsList` bumped from 14 → 15 tools. `TestProductionMappings_RoundTripByteIdentical` continues to pass — rename produces the minimal diff expected.

### 2. Codegen fix A: MAX_ITEM_OPTIONS resolution

Added a `-include packets_hpp_stub.h` argument to the `SourcePacketsStruct` preprocessing path (with a nil-check so tests that omit `PacketsHPPStub` still work). Added `MAX_ITEM_OPTIONS` to the stub, defined as `MAX_ITEM_RDM_OPT` (matching packets.hpp's own definition at line 13).

Verification: preprocessed `PACKET_ZC_ADD_EXCHANGE_ITEM` at pv=20200916 goes from `ItemOptions option_data[]` (size 0, flex-array) to `ItemOptions option_data[5]` (size 25, correct). Downstream field offsets: Location now 54, Grade now 61 (matching the pv >= 20200916 struct layout in rAthena).

### 3. Codegen fix B: preserve packets_struct.hpp VT authority

Modified `injectMapPacketStructs` (main.go:387-405) to skip structs that already have a version-table entry from `packets_struct.hpp`. `packets.hpp` is still scanned — its purpose (populating structs that ONLY exist in packets.hpp, like login/char server `PACKET_CA_*`/`PACKET_HC_*`) is preserved.

Verification: `PACKET_ZC_USESKILL_ACK`'s VT now shows `[20091124, 20181212): fields=9` and `[20181212, 0): fields=10` — the actual `#if PACKETVER_MAIN_NUM` guard boundary in packets_struct.hpp — instead of the previously-observed 20191120 (the packets.hpp snapshot).

### 4. Codegen fix C: block config/packets.hpp auto-RE

Added `#define CONFIG_PACKETS_HPP` to the packets_hpp_stub.h include-guard preamble. This prevents rAthena's `src/config/packets.hpp` from loading during preprocessing, which in turn prevents its auto-`#define PACKETVER_RE` logic from firing at pv >= 20200902. Our codegen models MAIN-branch semantics uniformly (as documented in `internal/codegen/semantics/epic08_test.go`: "rAthena uses PACKETVER_MAIN_NUM boundary, not RE"), and this ensures the preprocessor's view aligns with that convention.

Verification: at pv=20200902, `PACKET_ZC_ITEM_PICKUP_ACK` no longer has the `grade` field in the preprocessor's output (was appearing due to `PACKETVER_RE_NUM >= 20200723`). At pv=20200916 (the MAIN-branch guard), grade appears correctly. Length table `t[0x0A37]` retires (=0) at exactly pv=20200916, matching the decoder's outermost gate. `ItemPickup_0x0A37` no longer emits a `>= 20200902` branch; max byte offset read is now 68 (matches `t[0x0A37] = 69` at pv >= 20181121).

### 5. Mapping corrections (15 total)

Renamed `monster_ranged_attack` → `attack_failure_for_distance`. Fixed 14 packet-ID transitions where an old-ID mapping was unbounded (or overlapping) and the new ID at pv >= 20200916 was missing:

- `skill_cast`: added `0x0B1A` (pv >= 20181212). Previously worked around by manual edits to `pkg/decode/skill_cast.go` / `pkg/session/receive_dispatch.go` (commit 657c728) that regeneration wiped.
- `add_exchange_item`: split `0x0A96` [20161026..20200915] + new `0x0B42` [20200916..∞].
- `item_pickup`: split `0x0A37` [20160921..20200915] + new `0x0B41` [20200916..∞].
- Plus 11 more transitions at pv=20200916 discovered by systematic scan of `DEFINE_PACKET_HEADER` transitions: `zc_pc_purchase_itemlist_frommc` (0x0133→0x0B3D), `zc_repairitemlist` (0x01FC→0x0B65), `zc_change_item_option` (0x0AB9→0x0B43), `zc_search_store_info_ack` (0x0836→0x0B64), `zc_ack_add_item_rodex` (0x0A05→0x0B3F), `zc_item_pickup_party` (0x02B8→0x0B67), `zc_pc_purchase_myitemlist` (0x0136→0x0B40), `zc_add_item_to_store` (0x00F4→0x0B44), `zc_ack_read_rodex` (0x09EB→0x0B63), `zc_add_item_to_cart` (0x0124→0x0B45), `zc_equipwin_microscope` (0x0B03→0x0B37).
- `zc_checkname`: overlapping ranges corrected: `0x0A14` [20141119..20160301] and `0x0A51` [20160302..∞] (was: 0x0A51 min=20141118 — impossible overlap).

All changes applied via `semantics-tool` (using the new `rename-action` command for the primary rename, `update-implementation` + `add-implementation` for the transition fixes).

### 6. Test fixes

Three tests hard-coded assumptions that were only true under the buggy codegen output:

- `TestSkillCast_0x07FB_StillFires` asserted `lengths[0x07FB] == 29` at pv=20200401. rAthena at pv >= 20181212 binds `ZC_USESKILL_ACK` to 0x0B1A; it never sends 0x07FB there. Fixed to test at pv=20180101 (0x07FB's real active range), expected length 25 (the 9-field pre-attackMT layout).
- `TestZcGroupList_ActionConstant` hardcoded `ActionZcGroupList == 464`. `SemanticAction` values are auto-generated alphabetically starting at 1 and shift on every rename or new action. Replaced with `!= ActionUnknown` + `String() == "ActionZcGroupList"` assertions matching the actual API contract.
- `TestGap_AddExchangeItem_Grade_20200401_IsZero` used `AddExchangeItem_0x0A96` at pv=20200916 — a wire scenario rAthena never produces. Split into 0x0A96 at pv=20200401 (no grade) and 0x0B42 at pv=20200916 (grade at offset 61).

---

## Verification methodology

Every packet-layout and packet-ID-binding claim was verified by:

1. **Reading rAthena source directly** at pinned paths (e.g. `src/map/packets_struct.hpp:2608-2648` for `PACKET_ZC_ADD_EXCHANGE_ITEM`, `:8171-8188` for `clif_movetoattack`).
2. **Running g++ preprocessor** at specific PACKETVER values via one-off test fixtures under `internal/codegen/preprocess/`, then inspecting the emitted struct layout and `DEFINE_PACKET_HEADER` bindings. Sample invocation:
    ```
    g++ -E -P -DPACKETVER=20200902 -DPACKETVER_MAIN_NUM=20200902 \
        -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
        -include packets_hpp_stub.h \
        ~/personal/rathena/src/map/packets_struct.hpp
    ```
3. **Cross-checking the generated Go output** against those preprocessor traces. In particular the ItemPickup_0x0A37 fix was confirmed by running the codegen, greping `pkg/decode/item_pickup.go` for the `>= 20200902` branch's absence, and checking `pkg/session/lengths_map.go` for aligned `t[0x0A37]` transitions.
4. **Running the full test suite** (`go test -count=1 ./...`) after each intermediate change. Some tests failed initially — those failures were investigated and either found to be legitimate (my change exposed a genuine pre-existing bug — in which case the test was updated to reflect reality) or a regression (in which case my change was reverted or adjusted).

---

## Known follow-ups (deferred to future work)

- **Broader panic-safety audit.** A brief exploratory test that fed zero-filled `t[id]`-sized slices to every dispatched decoder revealed additional length/decode mismatches beyond the reviewer's flagged `ItemPickup_0x0A37`. Fixing each individually is out of scope for this PR (each requires investigating the exact rAthena packet-ID binding history and correcting mappings). A separate issue documenting the class of bug with a working test-generator would be the right vehicle for that work.
- **Missing intermediate historical mappings.** Several actions have gaps in their historical packet-ID chain (e.g. `zc_pc_purchase_itemlist_frommc` in `semantics/mappings.yaml` had only 0x0133 unbounded, missing 0x0800 for pv 20100105..20200915). These don't cause panics at goKore's target pv (20200401) but do produce imprecise decoders for older/newer packetvers. Not fixed in this PR.

---

## Rule 0 note

This is worklog 0090; latest prior was 0089. Written concurrently with PR #16.
