# 0066 — Fix Inventory List Decoders: Decode Item Arrays + Add 0x0B09 Dispatch

**Date:** 2026-03-20  
**Type:** Bug Fix  
**Scope:** pkg/events, pkg/decode, pkg/session, semantics/mappings.yaml, docs/DESIGN/HLD.md  
**Reported in:** goKore-test docs/07_WORK_LOG/0775_2026-03-20_rathena_client_bug_report_inventory_list_decoders.md

---

## Summary

Two bugs fixed:

**Bug 1 (design limitation):** `InventoryItemsStackable` and `InventoryItemsEquip` exposed the
inner item array as raw `List []byte`, forcing consumers to implement their own packetver-aware
binary parser. This is the work the library is supposed to own.

**Bug 2 (functional regression):** `0x0B09` (`inventorylistnormalType` at `pv >= 20181002 MAIN`,
per `packets_struct.hpp:138`) was present in `lengths_map.go` but absent from
`receive_dispatch.go`. At production packetver `20200401`, `0x0991` is disabled (`lengths = 0`)
and `0x0B09` is the active packet, so `ActionInventoryItemsStackable` **never fired**. Inventory
was silently never populated.

---

## Pre-implementation validation

Struct layouts verified by reading `packets_struct.hpp` directly:

### NORMALITEM_INFO (packets_struct.hpp:418–448)

| pv range | size | key fields |
|---|---|---|
| `< 20080102` | **18** | index(2)+ITID(2)+type(1)+IsIdent(1)+count(2)+WearState(2)+slot.card[4×2=8] |
| `>= 20080102, < 20120925` | **22** | +HireExpireDate(4) |
| `>= 20120925, < 20181121` | **24** | WearState→uint32(+2), IsIdent→Flag bit, slot same(8)+Flag(1) |
| `>= 20181121` | **34** | ITID→uint32(+2), slot.card[4×4=16](+8) |

### EQUIPITEM_INFO (packets_struct.hpp:457–507)

| pv range | size |
|---|---|
| `< 20071002` | **20** |
| `>= 20071002, < 20080102` | **24** |
| `>= 20080102, < 20100629` | **26** |
| `>= 20100629, < 20120925` | **28** |
| `>= 20120925, < 20150226` | **31** |
| `>= 20150226, < 20181121` | **57** |
| `>= 20181121, < 20200916` | **67** |
| `>= 20200916` | **68** |

**Deep-dive validation (adversarial agent)** identified one original-plan omission:
`EquipItemEntry` was missing `PlaceETCTab uint8` (present in `EQUIPITEM_INFO.Flag` bit 2 at
pv >= 20120925, `packets_struct.hpp:503`). Fixed before implementation.

---

## Files changed

### New (hand-written)
- `pkg/events/normal_item_entry.go` — `NormalItemEntry` type
- `pkg/events/equip_item_entry.go` — `ItemOption` and `EquipItemEntry` types
- `pkg/decode/inventory_items_test.go` — golden tests for all 4 NORMALITEM_INFO breakpoints
  and all 8 EQUIPITEM_INFO breakpoints; edge cases (empty body, partial entry); alloc benchmarks
- `pkg/session/inventory_dispatch_test.go` — integration tests: 0x0B09 fires at pv=20200401;
  0x0991 is silent at pv=20200401

### Modified (hand-written replacement of generated files)
- `pkg/events/inventory_items_stackable.go` — `List []byte` → `Items []NormalItemEntry`
- `pkg/events/inventory_items_equip.go` — `List []byte` → `Items []EquipItemEntry`
- `pkg/decode/inventory_items_stackable.go` — rewrote: `decodeNormalItems`, `normalItemSize`,
  all 4 existing decoders + new `InventoryItemsStackable_0x0B09`
- `pkg/decode/inventory_items_equip.go` — rewrote: `decodeEquipItems`, `equipItemSize`,
  all 7 decoders (added `0x0295` and `0x02D0` which had no prior decoders)

### One-line fix
- `pkg/session/receive_dispatch.go` — added `0x0B09` entry to `ActionInventoryItemsStackable`

### SemanticDB (via MCP)
- Added `0x0B09` implementation to `inventory_items_stackable` action (packetver_min=20181002,
  struct_name=packet_itemlist_normal)

### Documentation
- `docs/DESIGN/HLD.md` — alloc-exceptions table updated: added
  `InventoryItemsStackable` and `InventoryItemsEquip` as known 1-alloc-per-call events

---

## Test results

```
go test ./...
ok  github.com/lenaxia/rathena-client/pkg/decode    0.012s
ok  github.com/lenaxia/rathena-client/pkg/session   0.209s
ok  (all other packages pass)
```

### Benchmarks

```
BenchmarkDecodeNormalItems_1Entry-14      13043910    84.98 ns/op    48 B/op   1 allocs/op
BenchmarkDecodeNormalItems_10Entries-14    2663764   577.0  ns/op   416 B/op   1 allocs/op
BenchmarkDecodeEquipItems_1Entry-14        6641348   179.2  ns/op    96 B/op   1 allocs/op
```

1 alloc/op in all cases — the single `make([]T, n)` per packet, consistent with `PetEggList` and
`ZcSkillSelectRequest` (documented in HLD §known-exceptions).

Zero-alloc benchmarks for fixed-size packets **unaffected**:
```
BenchmarkFeed_SmallFixedPacket-14        35589145   36.76 ns/op   0 B/op   0 allocs/op
BenchmarkFeed_ActorExists_0x09FF-14      35898584   38.92 ns/op   0 B/op   0 allocs/op
```

---

## Impact on goKore Epic 41

With this fix:
- `ActionInventoryItemsStackable` fires at `pv=20200401` (via `0x0B09`)
- `e.Items []NormalItemEntry` is fully decoded; no `parser.go` needed
- `ActionInventoryItemsEquip` now correctly decodes all 7 packet variants at all supported
  packetvers

The temporary `parser.go` workaround described in goKore-test work log 0775 is no longer
required.
