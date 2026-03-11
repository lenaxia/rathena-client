# Work Log 0038 — Encode Skip Root Cause Investigation

**Date**: 2026-03-11
**Tag**: (none — no code changes this session, investigation only)
**Status**: Investigation complete, fix not yet implemented

---

## Summary

Investigated the two open questions left from worklog 0037:
1. Why do duplicate action names exist in `semantic_actions`?
2. Why are gameplay-critical encode packets missing from `pkg/encode/`?

Both root causes are now confirmed. No code was changed this session.

---

## Question 1 — Duplicate action names

**Answer**: They are genuine duplicates in `semantics/mappings.yaml`. Multiple action names
(e.g., `item_equip` and `equip_item`) point to the same rAthena struct. This is a data
quality problem in the semantics DB — likely accumulated from multiple contributors adding
the same packet under different names. Since codegen uses action names as Go function names
and filenames, only one of each pair gets generated (whichever sorts first alphabetically).

The duplicates confirmed in `semantic_actions`:

| Action A | Action B | Shared struct |
|---|---|---|
| `equip_item` | `item_equip` | `PACKET_CZ_REQ_WEAR_EQUIP` |
| `unequip_item` | `item_unequip` | `PACKET_CZ_REQ_TAKEOFF_EQUIP` |
| `pickup_item` | `item_take` | `PACKET_CZ_ITEM_PICKUP` |
| `item_use` | `use_item` | `PACKET_CZ_USE_ITEM` |
| `skill_use` | `use_skill` | `PACKET_CZ_USE_SKILL` |
| `move_to_storage` | `storage_item_add` | `PACKET_CZ_MOVE_ITEM_FROM_BODY_TO_STORE` |
| `move_from_storage` | `storage_item_remove` | `PACKET_CZ_MOVE_ITEM_FROM_STORE_TO_BODY` |
| `cz_pc_sell_itemlist` | `shop_sell` | `PACKET_CZ_PC_SELL_ITEMLIST` |
| `enter_world` | `notify_actor_init` | `PACKET_CZ_NOTIFY_ACTORINIT` |
| `login_request` | `master_login` | `PACKET_CA_LOGIN` |
| `party_invite` | `cz_req_join_group` | same party invite struct |
| `party_kick` | `cz_req_expel_group_member` | same party expel struct |
| `party_leave` | `cz_req_leave_group` | same party leave struct |

**Fix required**: Remove the `_b` name from each pair (keep the canonical name, delete the
alias). Canonical names to keep (delete the other):
- Keep: `equip_item`, `unequip_item`, `pickup_item`, `item_use`, `skill_use`,
  `move_to_storage`, `move_from_storage`, `shop_sell`, `enter_world`, `party_invite`,
  `party_kick`, `party_leave`
- Delete: `item_equip`, `item_unequip`, `item_take`, `use_item`, `use_skill`,
  `storage_item_add`, `storage_item_remove`, `cz_pc_sell_itemlist`, `notify_actor_init`,
  `login_request` (or `master_login`), `cz_req_join_group`, `cz_req_expel_group_member`,
  `cz_req_leave_group`

---

## Question 2 — Missing gameplay encode packets (root cause confirmed)

**Answer**: `injectMapPacketStructs()` in `internal/codegen/main.go:295` only injects structs
with prefixes `PACKET_ZC_` and `PACKET_SC_` from `packets.hpp` into the VersionTable. It
**completely ignores `PACKET_CZ_` (client-to-server) structs**.

The gameplay CZ structs (`PACKET_CZ_NOTIFY_ACTORINIT`, `PACKET_CZ_ITEM_PICKUP`,
`PACKET_CZ_USE_ITEM`, `PACKET_CZ_CHOOSE_MENU`, `PACKET_CZ_CLOSE_DIALOG`,
`PACKET_CZ_REQUEST_ACT`, `PACKET_CZ_INPUT_EDITDLG`, `PACKET_CZ_INPUT_EDITDLGSTR`,
`PACKET_CZ_REQ_EMOTION`, `PACKET_CZ_CHANGE_DIRECTION`, etc.) all live in
`src/map/packets.hpp` — NOT in `packets_struct.hpp`.

`buildVersionTable()` only reads `packets_struct.hpp`. So these structs are never in the
VersionTable. When `GenerateEncodeFile()` calls `resolveLayout()` at
`internal/codegen/gen/encode.go:49`, it returns `nil`, and the action is added to the skip
list (`encode.go:51`).

**Struct file split confirmed**:
- `packets_struct.hpp`: 107 `PACKET_CZ_` structs (modern, packetver-versioned ones)
- `packets.hpp`: 81 `PACKET_CZ_` structs — **zero overlap** with `packets_struct.hpp`

The 81 CZ structs in `packets.hpp` that are NOT in `packets_struct.hpp` include all the
simple gameplay ones: equip, pickup, use skill, NPC menus, storage moves, emotion, look, etc.

---

## The fix (not yet implemented)

**Location**: `internal/codegen/main.go:327` — the `mapPrefixes` slice in
`injectMapPacketStructs()`.

**Change**:
```go
// Before:
mapPrefixes := []string{"PACKET_ZC_", "PACKET_SC_"}

// After:
mapPrefixes := []string{"PACKET_ZC_", "PACKET_SC_", "PACKET_CZ_"}
```

This single-line change causes all 81 CZ structs from `packets.hpp` to be injected into the
VersionTable, making them available for encode generation. After running codegen, the
gameplay actions (`enter_world`, `pickup_item`, `equip_item`, `npc_menu_response`,
`move_to_storage`, `skill_use_location`, `request_action`, `emote`, `look`, etc.) should
produce generated files in `pkg/encode/`.

**Expected outcome after fix + codegen re-run**:
- ~30+ new files in `pkg/encode/`
- The "83 skipped" count should drop significantly (remainder will be auth-flow and
  not-needed-for-bot packets)
- `go test ./...` must still pass
- `pkg/send/` types for the new structs will also be auto-generated

**Caution**: After adding `PACKET_CZ_` to injection, verify no struct name collisions between
`packets.hpp` CZ entries and `packets_struct.hpp` CZ entries. The dedup check at
`main.go:364–371` (same size + field count = extend range) should handle it, but confirm
with a test run.

---

## Files relevant to the fix

| File | What to change |
|---|---|
| `internal/codegen/main.go:327` | Add `"PACKET_CZ_"` to `mapPrefixes` |
| `semantics/mappings.yaml` | Remove ~13 duplicate alias action names (use MCP delete_action or direct yaml edit) |

---

## Validation checklist for next agent

1. Apply the one-line `mapPrefixes` fix
2. Run codegen: `go run ./internal/codegen/main.go --rathena ~/personal/rathena --semantics semantics/mappings.yaml --out .`
3. Check new encode file count: `ls pkg/encode/*.go | wc -l`
4. Check remaining skip count in codegen log output
5. Run `go test ./...` — must pass
6. Run encode benchmarks: `go test -bench=. -benchmem ./pkg/encode/` — must be 0 allocs/op
7. Optionally: clean up duplicate action names from `semantics/mappings.yaml`
8. Tag `v0.2.3`
