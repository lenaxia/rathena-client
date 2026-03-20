# WORKLOG 0062 — EPIC-08 Open Questions: Answers and DB Corrections

**Date**: 2026-03-20
**Status**: COMPLETE (analysis + corrections documented — no code changes yet)
**Follows**: EPIC-08 implementation plan (`docs/BACKLOG/EPIC-08_implementation_plan.md`)

---

## Summary

The EPIC-08 implementation plan ended with 6 open questions that needed hard answers
before touching code. All 6 were resolved by reading rAthena source directly. One
pre-existing DB bug was discovered in the process (`0x02EC` misclassified).

---

## Q1: `item_pickup` — what is the exact packetver range for `0x0A37`?

**Source**: `packets_struct.hpp` lines 582–594, `DEFINE_PACKET_HEADER` chain for
`ZC_ITEM_PICKUP_ACK`.

**Answer**: `0x0A37` is active when `PACKETVER >= 20160921`. It slots between `0x0A0C`
(`>= 20150226`) and `0x0B41` (MAIN `>= 20200916`).

The complete chain in rAthena order (first match wins):

| PID | Condition | packetver_min | packetver_max |
|---|---|---|---|
| `0x0B41` | MAIN `>= 20200916` / RE `>= 20200723` | 20200723 RE / 20200916 MAIN | — |
| `0x0A37` | `>= 20160921` | 20160921 | 20200722 RE / 20200915 MAIN |
| `0x0A0C` | `>= 20150226` | 20150226 | 20160920 |
| `0x0990` | `>= 20120925` | 20120925 | 20150225 |
| `0x02D4` | `>= 20071002` | 20071002 | 20120924 |
| `0x029A` | `>= 20061218` | 20061218 | 20071001 |
| `0x00A0` | else | — | 20061217 |

**Action required**: The existing `0x00A0` impl needs `packetver_max: 20061217`. The
existing `0x0A37` impl needs its `packetver_range` narrowed from `[20150226, null]` to
`[20160921, <20200916>]`. Both are currently `[null, null]` / `[20150226, null]`.

---

## Q2: `zc_accept_enter` — what are the exact ranges for `0x0073`, `0x02EB`, `0x0A18`?

**Source**: `packets.hpp` lines 553–574, `DEFINE_PACKET_HEADER` chain for
`ZC_ACCEPT_ENTER`.

**Answer**: `0x02EB` covers two **discontinuous** ranges. `0x0A18` (which we already
have) was only active for a narrow window:

```
0x0073: PACKETVER < 20080102
0x02EB: PACKETVER < 20141022 || PACKETVER >= 20160330   (discontinuous)
0x0A18: else  (= 20141022 <= PACKETVER < 20160330)
```

This means `0x0A18` was used only from **2014-10-22 to 2016-03-29** (~18 months), then
rAthena switched back to `0x02EB`.

**Implementation**: Two separate `implementations` entries for `0x02EB`:

```yaml
- packet_id: "0x0073"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range: [null, 20080101]       # < 20080102
  
- packet_id: "0x02EB"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range: [null, 20141021]       # < 20141022

- packet_id: "0x0A18"                    # UPDATE EXISTING — add max
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range: [20141022, 20160329]   # 20141022 <= pv < 20160330

- packet_id: "0x02EB"
  struct_name: PACKET_ZC_ACCEPT_ENTER
  packetver_range: [20160330, null]       # >= 20160330 (returns)
```

The same PID appears twice with non-overlapping ranges. The codegen handles this: the
first entry whose range contains the session packetver is used.

---

## Q3: `0x00AA` — is it already in the DB?

**Answer**: Yes. `0x00AA` is already present under `zc_req_wear_equip_ack` with
`packetver_range: [null, null]`.

That action already exists with one implementation covering all packetvers. The rAthena
chain for `ZC_REQ_WEAR_EQUIP_ACK` (`packets_struct.hpp` lines 1269–1293):

```
0x0999: MAIN >= 20121205 || RE >= 20121107 || ZERO     — adds wItemSpriteNumber, u32 wearLocation
0x00AA: >= 20101123 MAIN / >= 20100629 RE               — adds wItemSpriteNumber, u16 wearLocation
0x00AA: else                                            — no wItemSpriteNumber, u16 wearLocation
```

`0x00AA` appears in **two `#elif` branches** with different struct layouts. The `else`
branch (pre-2010) is what our current impl covers with `[null, null]`.

**Actions required**:
1. Narrow existing `0x00AA` to `packetver_range: [null, 20101122]` (the `else` layout)
2. Add a second `0x00AA` entry: `packetver_range: [20100629, 20121204]` (the `>=20101123`
   layout with sprite number — RE date 20100629 applies)
3. Add `0x0999`: `packetver_range: [20121107, null]`

No new action needed — `zc_req_wear_equip_ack` already exists.

---

## Q4: `actor_exists 0x02EC` — confirmed pre-existing DB bug

**Answer**: `0x02EC` is a **bug** in our `semantic_actions`. It must be in `actor_moved`,
not `actor_exists`.

### Evidence

`packets_struct.hpp` defines three separate enum constants for actor unit packet types:
- `idle_unitType` → used by `clif_set_unit_idle()` → `actor_exists`
- `spawn_unitType` → used by `clif_spawn_unit()` → `actor_connected`
- `unit_walkingType` → used by `clif_set_unit_walking()` → `actor_moved`

The complete assignment tables (with packetver conditions):

**idle_unitType** (actor_exists / `packet_idle_unit`):
```
0x0078 [< 4]         0x01D8 [< 7]          0x022A [< 20080102]
0x02EE [< 20091103]  0x07F9 [< 20101124]   0x0857 [< 20120221]
0x0915 [< 20131223]  0x09DD [< 20150513]   0x09FF [else]
```

**spawn_unitType** (actor_connected / `packet_spawn_unit`):
```
0x0079 [< 4]         0x01D9 [< 7]          0x022B [< 20080102]
0x02ED [< 20091103]  0x07F8 [< 20101124]   0x0858 [< 20120221]
0x090F [< 20131223]  0x09DC [< 20150513]   0x09FE [else]
```

**unit_walkingType** (actor_moved / `packet_unit_walking`):
```
0x007B [< 4]         0x01DA [< 7]          0x022C [< 20080102]
0x02EC [< 20091103]  0x07F7 [< 20101124]   0x0856 [< 20120221]
0x0914 [< 20131223]  0x09DB [< 20150513]   0x09FD [else]
```

`0x02EC` = `unit_walkingType` → belongs in `actor_moved`. Our DB currently has it under
`actor_exists` — the action and struct are both wrong for each other (the struct
`packet_unit_walking` is correct, but the action `actor_exists` is wrong).

### Fix

```yaml
# actor_exists — REMOVE 0x02EC impl, ADD missing idle PIDs:
# Remove: 0x02EC (packet_unit_walking) — wrong action
# Add:    0x022A [20050411, 20080101], 0x02EE [20080102, 20091102]
#         0x09DD [20131223, 20150512]

# actor_moved — MOVE 0x02EC here:
# Add: 0x02EC [20080102, 20091102] struct=packet_unit_walking
```

### Cascade: length-0 shuffle aliases (no decode needed)

Several entries in both spawn and walk enum tables have `length=0` in OpenKore's
`recvpackets.txt`. These are **shuffle alias slots** — the client receives bytes tagged
with these IDs but the actual data is framed under a non-zero-length PID. They must
appear in our **length tables** (for the framer) but do not need decode functions or
dispatch entries:

| PID | Type | Reason: length=0 |
|---|---|---|
| `0x07F7` | unit_walkingType | shuffle alias |
| `0x07F8` | spawn_unitType | shuffle alias |
| `0x07F9` | idle_unitType | shuffle alias |
| `0x0856` | unit_walkingType | shuffle alias |
| `0x0857` | idle_unitType | shuffle alias |
| `0x0858` | spawn_unitType | shuffle alias |
| `0x0914` | unit_walkingType | shuffle alias |
| `0x0915` | idle_unitType | shuffle alias |
| `0x090F` | spawn_unitType | shuffle alias |

These are correctly absent from `receive_dispatch.go` and do not need to be added. The
length table (`lengths_map.go`) may already have them via codegen; if not, they are
harmless to omit since the framer falls back to the unknown-packet handler for length=0.

---

## Q5 & Q6: `0xB09` collision — `ZC_INVENTORY_START` vs `inventorylistnormalType`

**Answer**: No collision. They are different PIDs.

- `ZC_INVENTORY_START` = **`0x0B08`** (our DB has this correctly)
- `ZC_INVENTORY_END` = **`0x0B0B`** (our DB has this correctly)
- `inventorylistnormalType = 0xB09` = the **item list body** packet sent between START and END

Confirmed from `clif_inventorylist()` in `clif.cpp`:

```cpp
void clif_inventorylist(map_session_data *sd) {
#if PACKETVER_RE_NUM >= 20180912 || ...
    clif_inventoryStart(sd, INVTYPE_INVENTORY, "");  // → sends 0x0B08
#endif
    // loop:
    itemlist_normal.PacketType = inventorylistnormalType;  // → 0xB09 at 2018+
    clif_send(&itemlist_normal, ...);

    itemlist_equip.PacketType = inventorylistequipType;    // → 0xB0A at 2018+
    clif_send(&itemlist_equip, ...);
    // end loop
    clif_inventoryEnd(sd, INVTYPE_INVENTORY);  // → sends 0x0B0B
}
```

The 2018+ inventory sequence for one session login:
```
→ 0x0B08  ZC_INVENTORY_START  (session boundary open)
→ 0x0B09  packet_itemlist_normal × N  (stackable items)
→ 0x0B0A  packet_itemlist_equip × N   (equippable items)
→ 0x0B0B  ZC_INVENTORY_END    (session boundary close)
```

Our current `zc_inventory_start` (`0x0B08`) and `zc_inventory_end` (`0x0B0B`) are
correct. `0x0B09` and `0x0B0A` are the item list packets that need to be added under
`inventory_items_stackable` and `inventory_items_equip` respectively.

The `inventorylistnormalType = 0xB09` and `inventorylistequipType = 0xB0A` enum
assignments mean that when `packet_itemlist_normal.PacketType = inventorylistnormalType`
is sent at `>= 20181002`, the wire PID is `0xB09`. The struct used (`packet_itemlist_normal`)
already has the `invType` field conditionally for this packetver range.

---

## Revised EPIC-08 corrections summary

Based on these answers, the EPIC-08 implementation plan needs the following corrections
before any code changes begin:

### Corrections to existing implementations (no new PIDs)

| Action | PID | Change |
|---|---|---|
| `item_pickup` | `0x00A0` | Add `packetver_max: 20061217` |
| `item_pickup` | `0x0A37` | Change `packetver_range` to `[20160921, ~20200722]` |
| `zc_accept_enter` | `0x0A18` | Add `packetver_range: [20141022, 20160329]` |
| `zc_req_wear_equip_ack` | `0x00AA` | Split into two entries: `[null, 20101122]` and `[20100629, 20121204]` |
| `actor_exists` | `0x02EC` | **REMOVE** — wrong action (must move to `actor_moved`) |

### New bug: `0x02EC` must move from `actor_exists` to `actor_moved`

This is a pre-existing DB error, not introduced by EPIC-08. It should be fixed as the
first change when EPIC-08 work begins.

```yaml
# actor_exists: remove this impl
#   - packet_id: "0x02EC"
#     struct_name: packet_unit_walking

# actor_moved: add this impl
    - packet_id: "0x02EC"
      struct_name: packet_unit_walking
      packetver_range: [20080102, 20091102]
```

### Revised 55-gap PID table

After these corrections, the actor section of the gap list changes:

| PID | Action | Was | Now |
|---|---|---|---|
| `0x02EC` | `actor_moved` | bug: under `actor_exists` | correctly `actor_moved` |
| `0x02EE` | `actor_exists` | gap | add with `packet_idle_unit` |
| `0x022A` | `actor_exists` | gap | add with `packet_idle_unit` |
| `0x022B` | `actor_connected` | gap | add with `packet_spawn_unit` |
| `0x09DC` | `actor_connected` | gap | add with `packet_spawn_unit` |
| `0x09DD` | `actor_exists` | gap | add with `packet_idle_unit` |

The total gap count remains 55 (moving `0x02EC` doesn't add/remove a gap — it fixes
its action classification). The DB bug fix is a correction on top of the gap fills.

---

## What's next

The investigation work is complete. The plan is fully specified. Implementation can
begin immediately in this order:

### Step 1: Fix the pre-existing bug (one YAML change, no codegen needed for validation)

Move `0x02EC` from `actor_exists` to `actor_moved` in `semantic_actions`. This is a
correctness fix independent of the gap fills and should land first.

### Step 2: US-08-1 — Login/map-enter path (highest value, pure YAML)

Add to `semantic_actions`:
- `zc_accept_enter`: `0x0073`, `0x02EB` (×2 ranges), update `0x0A18` range
- Create `char_created` action: `0x006D`, `0x0B6F`
- `received_characters_page`: `0x0B72`, update `0x099D` range

Run codegen. Verify `receive_dispatch.go`.

### Step 3: US-08-3 — item_pickup (5 variants, pure YAML)

Add `0x029A`, `0x02D4`, `0x0990`, `0x0A0C`, `0x0B41`. Fix `0x00A0` and `0x0A37` ranges.
Run codegen.

### Step 4: US-08-5 — Equipment acks (pure YAML, 2 new actions)

Create `zc_req_takeoff_equip_ack`: `0x00AC`, `0x08D1`, `0x099A`.
Fix `zc_req_wear_equip_ack` `0x00AA` range, add `0x0999`.
Run codegen.

### Step 5: US-08-2 (actor PIDs, pure YAML)

Fix `0x02EC`, add `0x022A`, `0x022B`, `0x02EE`, `0x09DC`, `0x09DD`. Run codegen.

### Step 6: US-08-4 — EXP packets (SYNTH_ + new action)

Write two SYNTH_ structs (`SYNTH_ZC_LONG_PAR_CHANGE`, `SYNTH_ZC_LONG_PAR_CHANGE2`) in
`synthetic_structs.hpp`. Create `exp` action. Run codegen.

### Step 7: US-08-7 — Remaining Cat A (trade, skills, stats, hotkeys, guild, equip window)

12 pure-YAML adds plus 2 new actions (`zc_ho_par_change`, `zc_el_par_change`). Run codegen.

### Step 8: US-08-7 Cat B remainder (6 SYNTH_ structs)

`0x0071`, `0x02A2`, `0x02AD`, `0x02CA`, `0x08C7`, `0x0274` (mail). Write SYNTH_ structs,
add impls. Run codegen.

### Step 9: US-08-8 — Inventory lists (Cat D, most complex)

Create `inventory_items_stackable` and `inventory_items_equip`. Add all 9 PIDs. Confirm
no overlap with `zc_inventory_start` (`0x0B08`). Run codegen.

### After all steps: verification

Run the cross-reference audit script from worklog 0060 to confirm all 55 PIDs now
appear in `receive_dispatch.go`:

```python
# Quick check: count gap PIDs in dispatch
pids = [...]  # all 55 gap PIDs
with open('pkg/session/receive_dispatch.go') as f:
    d = f.read()
missing = [p for p in pids if f'0x{p.upper()}' not in d]
print(f"Still missing: {missing}")
```

Expected result: 0 missing.
