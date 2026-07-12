# Work Log 0093 — Decoder audit: bound 123 unbounded slice reads, add ItemPickup golden tests

**Date**: 2026-07-12
**Type**: Defensive correctness + test coverage
**Scope**:
  - `pkg/decode/*.go` — 123 unbounded `data[N:]` slice reads bound to `data[N:N+M]` across 34 decoder files (1 false positive caught by `TestZcPositionIdNameInfo_0x0166_PosInfo_Decoded` and reverted)
  - `pkg/decode/item_pickup_golden_test.go` — NEW file, 9 golden tests + 7 benchmarks for the entire `ItemPickup_*` decoder family (previously only `0x029A` had any test coverage)
  - `CHANGELOG.md` — v0.9.3 entry

**Severity**: LOW (no caller-visible behavior change for the slice fixes; existing tests already passed). The golden tests are the high-value part: they catch future regressions in a decoder family that was largely untested.

**Reference**: triggered by goKore worklog 1001, which initially (and incorrectly) blamed an `ItemPickup_0x0A37` decoder bug for `TestLive_Loot_ItemPickup` failures. This worklog corrects that diagnosis and adds the regression-prevention tests that should have existed in the first place.

---

## 1. Background: the goKore misdiagnosis

goKore PR #183's `TestLive_Loot_ItemPickup` failed on the first E2E run that reached it, with `item added to inventory: itid=0 amount=0`. goKore worklog 1001 (filed against this repo) attributed the failure to a decoder bug in `ItemPickup_0x0A37` at pv >= 20181121, claiming the offsets were wrong.

**The diagnosis was wrong.** GCC preprocessor output at pv=20200401 (the goKore test server packetver) confirms the decoder matches the C struct exactly:

```c
struct PACKET_ZC_ITEM_PICKUP_ACK {           // preprocessed at PACKETVER=20200401
    int16 PacketType;                        // offset 0, size 2
    uint16 Index;                            // offset 2, size 2
    uint16 count;                            // offset 4, size 2
    uint32 nameid;                           // offset 6, size 4   <-- matches decoder leU32(data, 6)
    uint8 IsIdentified;                      // offset 10
    uint8 IsDamaged;                         // offset 11
    uint8 refiningLevel;                     // offset 12
    struct EQUIPSLOTINFO slot;               // offset 13, size 16  <-- 4×uint32 post-20181121
    uint32 location;                         // offset 29
    uint8 type;                              // offset 33
    uint8 result;                            // offset 34
    int32 HireExpireDate;                    // offset 35
    uint16 bindOnEquipType;                  // offset 39
    struct ItemOptions option_data[5];       // offset 41, size 25  (5 × {i16,i16,u8})
    uint8 favorite;                          // offset 66
    uint16 look;                             // offset 67
} __attribute__((packed));                   // total: 69 bytes
```

The Go decoder reads every field at the correct offset. **The actual failure mode is server-side:** rAthena's `clif_parse_TakeItem` (in `src/map/clif.cpp`) sends `clif_additem(sd, 0, 0, 6)` whenever `pc_takeitem` rejects the pickup for ANY reason — distance > 2 cells, loot-priority tick not elapsed, etc. The constant `6` is `ADDITEM_REFUSED_TIME`. The frame carries `Index=0`, `count=0`, `nameid=0`, `result=6` — those zeros are the server's rejection payload, not a decoder bug. goKore's handler checks `e.Result != PICKUP_SUCCESS` and early-returns, leaving the hook event's ITID/Amount at their default zero values.

goKore worklog 1001 will be corrected in a follow-up.

## 2. Audit methodology

Built a Python audit harness (`/tmp/opencode/rathena-audit/`) that:

1. Preprocesses `~/personal/rathena/src/map/packets_struct.hpp` at 18 representative packetvers spanning 2006–2023.
2. Resolves nested struct sizes (EQUIPSLOTINFO, ItemOptions, etc.) via recursive computation.
3. Parses every decoder in `pkg/decode/*.go` and classifies each slice read as:
   - `CORRECT_BOUNDED` — `data[N:N+M]` with M matching struct size
   - `HARMLESS_UNBOUNDED` — `data[N:]` where struct size M > 0 (over-permissive, no caller reads trailing bytes)
   - `UNKNOWN_SIZE` — `data[N:]` where struct size is 0 (variable-length trailing data, correctly unbounded)
   - `SIZE_MISMATCH` — `data[N:N+M]` where M ≠ struct size

### 3. Findings

| Category | Count | Notes |
|---|---|---|
| CORRECT_BOUNDED | 28 | Already correct |
| HARMLESS_UNBOUNDED | 123 | Fixed in this PR — 34 files affected |
| UNKNOWN_SIZE | 53 | Correct as-is (variable-length trailing data, e.g. server lists) |
| SIZE_MISMATCH | 0 | No bugs of this class |

**Zero real decoder bugs found.** The codegen pipeline produces correct output. The 123 unbounded slices were a style/defensive-correctness issue, not a correctness bug — they included trailing bytes that no caller ever read, but the data those callers needed was at the right offset.

### 4. The slice-bounds cleanup

Applied programmatically. For each `e.Field = data[N:]  // rAthena: ... size M` with M > 0, rewrote to `e.Field = data[N:N+M]`. One false positive caught by `TestZcPositionIdNameInfo_0x0166_PosInfo_Decoded`: the field `positionID` is annotated `size 4` but is actually `posInfo[MAX_GUILDPOSITION]` (variable-length array of structs). Reverted that one case and added an inline comment.

Files touched (34): `add_exchange_item.go`, `item_pickup.go`, `skill_add.go`, `skill_update.go`, `zc_ack_add_item_rodex.go`, `zc_ack_apply_macro_detector.go`, `zc_ack_preview_macro_detector.go`, `zc_ack_upload_macro_detector.go`, `zc_add_item_to_cart.go`, `zc_add_item_to_store.go`, `zc_adventurer_agency_join_req.go`, `zc_adventurer_agency_join_result.go`, `zc_change_item_option.go`, `zc_changestate_pet.go`, `zc_close_macro_detector.go`, `zc_delete_member_from_group.go`, `zc_dialog_window_pos.go`, `zc_dialog_window_pos2.go`, `zc_dialog_window_size.go`, `zc_grade_enchant_ack.go`, `zc_grade_enchant_material_list.go`, `zc_guild_info.go`, `zc_hoskillinfo_update.go`, `zc_inventory_tab.go`, `zc_item_pickup_party.go`, `zc_notify_chat_party.go`, `zc_notify_memberinfo_to_groupm.go`, `zc_notify_position_to_groupm.go`, `zc_party_join_req.go`, `zc_position_id_name_info.go` (reverted, comment added), `zc_req_answer_macro_detector.go`, `zc_req_takeoff_equip_ack.go`, `zc_update_gdid.go`, `zc_view_camerainfo.go`.

## 5. ItemPickup golden tests

Added `pkg/decode/item_pickup_golden_test.go` with 9 golden tests + 7 benchmarks covering the entire `ItemPickup_*` decoder family:

| Test | Decoder | Struct size | Packetver range |
|---|---|---|---|
| `TestItemPickup_0x00A0` | baseline | 23 B | pre-20061218 |
| `TestItemPickup_0x029A` (pre-existing) | +HireExpire | 27 B | 20061218–20071001 |
| `TestItemPickup_0x02D4` | +bindOnEquip | 29 B | 20071002–20120924 |
| `TestItemPickup_0x0990` | location uint16→uint32 | 31 B | 20120925–20150225 |
| `TestItemPickup_0x0A0C` | +option_data[5] | 56 B | 20150226–20160920 |
| `TestItemPickup_0x0A37_PreModernNameid` | +favorite+look | 59 B | 20160921–20181120 |
| `TestItemPickup_0x0A37_ModernNameid` | nameid uint16→uint32, slot uint16→uint32 | 69 B | 20181121–20200915 |
| `TestItemPickup_0x0A37_ModernNameid_RejectionPayload` | server rejection (Fail=6) | 69 B | same range |
| `TestItemPickup_0x0B41` | refiningLevel moves to end, +grade | 70 B | 20200916– |

The `RejectionPayload` test specifically verifies that a server-sent `clif_additem(sd,0,0,6)` frame decodes to `Result=6, Nameid=0, Count=0` — documenting the behavior goKore's test server actually exhibits when `pc_takeitem` rejects a pickup. This is the regression test for the misdiagnosis in worklog 1001.

All tests pass. All benchmarks report **0 allocs/op** (preserves the decode hot-path invariant).

## 6. Validation

```bash
$ go build ./...
$ go test -timeout 120s -race ./...
# all packages pass, race clean

$ go test -bench=. -benchmem -benchtime=1s ./pkg/decode/...
BenchmarkItemPickup_0x029A-8                    46199878     23.14 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x00A0-8                    50359406     23.49 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x02D4-8                    50002587     23.08 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x0990-8                    47734542     22.87 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x0A0C-8                    58075431     22.60 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x0A37_PreModernNameid-8    49119063     25.10 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x0A37_ModernNameid-8       49581060     22.97 ns/op     0 B/op   0 allocs/op
BenchmarkItemPickup_0x0B41-8                    44036850     24.82 ns/op     0 B/op   0 allocs/op

$ grep -rE "^\s*go\s+\w" pkg/ --include="*.go" | grep -v "_test.go"
# empty — zero goroutines in pkg/ non-test code (Rule 3)

$ go vet ./...
# clean
```

## 7. Codegen follow-up (deferred)

The slice-bounds cleanup was applied by hand to the generated decoder files. The codegen template (`internal/codegen/gen/decode.go:469`) still emits `data[%d:]` for `[]byte`-typed fields where the codegen doesn't know the nested struct size. A proper fix would teach the codegen to resolve nested struct sizes (EQUIPSLOTINFO, ItemOptions, etc.) and emit the bounded slice at generation time. That's a separate story — the hand-edits are stable and a regression test (`TestItemPickup_*`) now guards the highest-traffic decoder family.

## 8. Rule 0 note

Worklog 0093; latest prior 0092.
