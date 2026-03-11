# Work Log 0039 — Encode Skip Fix: CZ Struct Injection + Duplicate Action Cleanup

**Date**: 2026-03-11
**Tag**: v0.2.3 (pending commit)
**Status**: Complete

---

## Summary

Implemented the two fixes identified in worklog 0038:

1. Added `PACKET_CZ_` prefix injection in codegen so the 81 CZ structs from
   `src/map/packets.hpp` are included in the VersionTable and available for
   encode generation.
2. Removed 13 duplicate alias action names from `semantics/mappings.yaml`.

Also fixed two secondary issues discovered during validation:
- The loader (`internal/codegen/semantics/loader.go`) did not handle the new
  `packetver_range:` YAML format written by the MCP server. Updated to parse
  both old `packetver_min:` / `packetver_max:` keys and the new list format.
- The MCP server had serialized all `packetver_range` values as `[null, null]`,
  losing 20 non-default min/max values. Restored all 20 via MCP
  `update_implementation_metadata`.

---

## Fix 1 — PACKET_CZ_ injection

**File**: `internal/codegen/main.go:327`

**Change**:
```go
// Before:
mapPrefixes := []string{"PACKET_ZC_", "PACKET_SC_"}

// After:
mapPrefixes := []string{"PACKET_ZC_", "PACKET_SC_", "PACKET_CZ_"}
```

**Result**: codegen now injects all PACKET_CZ_ structs from `packets.hpp` into
the VersionTable. The encode generator can then resolve layouts for CZ actions.

---

## Fix 2 — Duplicate action name removal

Deleted 13 alias actions that duplicated canonical names:

| Deleted alias | Canonical name kept |
|---|---|
| `item_equip` | `equip_item` |
| `item_unequip` | `unequip_item` |
| `item_take` | `pickup_item` |
| `use_item` | `item_use` |
| `use_skill` | `skill_use` |
| `storage_item_add` | `move_to_storage` |
| `storage_item_remove` | `move_from_storage` |
| `cz_pc_sell_itemlist` | `shop_sell` |
| `notify_actor_init` | `enter_world` |
| `login_request` | `master_login` |
| `cz_req_join_group` | `party_invite` |
| `cz_req_expel_group_member` | `party_kick` |
| `cz_req_leave_group` | `party_leave` |

Corresponding `pkg/send/` stub files for the deleted aliases were also cleaned
up by codegen re-run.

---

## Fix 3 — Loader: new packetver_range YAML format

**File**: `internal/codegen/semantics/loader.go`

The MCP server rewrote `semantics/mappings.yaml` using a new format:
```yaml
    ac_accept_login:
        implementations:
            - packet_id: "0x0069"
              packetver_range:
                - null      # min (null = 20030000 default)
                - null      # max (null = no upper bound)
              struct_name: PACKET_AC_ACCEPT_LOGIN
```

The old format was:
```yaml
    ac_accept_login:
        implementations:
            - packet_id: "0x0069"
              struct_name: PACKET_AC_ACCEPT_LOGIN
              packetver_min: 20030000
```

Updated `parse()` to handle indent-16 list items under `packetver_range:`,
with a default of 20030000 when the list item is `null`.

---

## Fix 4 — Restored 20 non-default pvMin/pvMax values

The MCP serializer had set all `packetver_range` values to `[null, null]`,
losing actual version bounds. Restored via MCP `update_implementation_metadata`:

| Action | Packet | pvMin | pvMax |
|---|---|---|---|
| `actor_action` | `0x085A` | 20200401 | — |
| `actor_connected` | `0x09FE` | 20181121 | — |
| `actor_exists` | `0x09FF` | 20181121 | — |
| `actor_moved` | `0x09DB` | 20181121 | — |
| `actor_moved` | `0x09FD` | 20141022 | — |
| `actor_status_effect_extended` | `0x043F` | 20090121 | — |
| `character_pages_notify` | `0x09A0` | 20100803 | — |
| `entity_spawn` | `0x007C` | 20030000 | 20080102 |
| `item_pickup` | `0x00A0` | 20030000 | 20181120 |
| `item_pickup` | `0x0A37` | 20150226 | — |
| `received_characters_page` | `0x099D` | 20100803 | — |
| `send_chat` | `0x008C` | 20030000 | 20040725 |
| `send_chat` | `0x00F3` | 20040726 | — |
| `skill_use` | `0x0862` | 20200401 | — |
| `time_sync_response` | `0x007E` | 20030000 | 20101123 |
| `time_sync_response` | `0x0360` | 20101124 | — |
| `zc_checkname` | `0x0A14` | 20141119 | — |
| `zc_checkname` | `0x0A51` | 20141118 | — |
| `zc_state_change` | `0x0229` | 20070000 | — |

---

## Results

**Encode file count**: 115 implementation files (up from ~80 before)

**Codegen output**:
```
→ pkg/encode/ (114 files, 38 skipped)
```
(114 generated + 1 hand-written actor_action_test.go; 38 remaining skips are
auth-flow and not-needed-for-bot packets)

**New encode files generated** (gameplay-critical actions now covered):
- `emote.go` — `PACKET_CZ_REQ_EMOTION`
- `equip_item.go` — `PACKET_CZ_REQ_WEAR_EQUIP`
- `npc_talk_close.go`, `npc_talk_number.go`, `npc_talk_text.go`
- `party_invite.go`, `party_kick.go`, `party_leave.go`, `party_create.go`
- `shop_sell.go` — `PACKET_CZ_PC_SELL_ITEMLIST`
- `deal_add_other.go` — `PACKET_CZ_ADD_EXCHANGE_ITEM`
- `cz_join_group.go`, `cz_join_guild.go`
- `cz_req_apply_bargain_sale_item.go`, `cz_req_ban_guild.go`
- `cz_req_cash_bargain_sale_item_info.go`, `cz_req_change_memberpos.go`
- `cz_req_disorganize_guild.go`, `cz_req_join_guild.go`
- And ~20 more CZ structs

**Test results**:
```
go test ./...   — all packages PASS
```

**Benchmark results** (all 0 allocs/op):
```
BenchmarkEncodeActorAction      0.27 ns/op   0 B/op  0 allocs/op
BenchmarkEncodeMoveTo          17.15 ns/op   0 B/op  0 allocs/op
BenchmarkEncodeSkillUse         0.39 ns/op   0 B/op  0 allocs/op
BenchmarkActorExists_0x09FF    29.82 ns/op   0 B/op  0 allocs/op
BenchmarkFeed_SmallFixedPacket 10.89 ns/op   0 B/op  0 allocs/op
BenchmarkEncode_RequestMove     0.38 ns/op   0 B/op  0 allocs/op
```
