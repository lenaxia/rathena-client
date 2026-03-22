# 0073 — Audit: C→S Send Encoder Packet ID Bugs (Systematic)

**Date**: 2026-03-21
**Status**: PLANNED — implementation pending (see fix plan at bottom)
**Scope**: `pkg/encode/` — all C→S send encoders
**Severity**: BLOCKING for 5 encoders; non-blocking (accidentally correct) for 3 more

---

## Background

Following worklogs 0069 (`EncodeItemUse` wrong ID) and 0070/0071 (`EncodePickupItem` wrong ID),
a systematic audit was run across all 172 C→S send actions in the semantics DB to identify any
remaining encoders with the same class of bug: hardcoded packet ID that ignores `packetver`.

---

## Audit Methodology

### Step 1 — Machine scan

Every C→S action with a single `[null, null]` packetver range was extracted from
`semantics/mappings.yaml`. Of 172 total C→S actions, 166 had a single null-null range (i.e. no
packetver dispatch). A Python script cross-referenced each action's `packet_id` against
`clif_packetdb.hpp` to determine whether the handler was ever reassigned to a different wire ID,
and against `pkg/encode/shuffle_map.go` to determine whether the ID is in the shuffle table.

Results:
- **37 stable** — handler assignment never changed; ID correct for all packetvers
- **5 shuffle-needed** — base ID is in the shuffle table; encoder must call `shuffledCtoSID(pv, base)` for pv >= 20130515
- **7 broken/reassigned** — base ID was permanently reassigned to a different handler
- **115 not in clif_packetdb** — login/char server packets (CA_/CH_), modern packets (0x0800+) registered via `DEFINE_PACKET_HEADER`, or old packets using `HEADER_*` symbolic constants that the initial grep missed

### Step 2 — Deep-dive delegation (three batches in parallel)

**Batch A (5 shuffle-needed):**
Full `clif_packetdb.hpp` and `clif_shuffle.hpp` history traced for each handler. Every
intermediate wire ID from pv=20101124 to pv=20130515 recorded. Confirmed stable post-20180307
wire IDs via `clif_shuffle.hpp` stable block.

**Batch B (7 broken/reassigned):**
Each handler traced to its correct modern wire ID. Two were found to be non-issues:
- `character_move` — accidentally correct at pv=20200401; duplicate of `ActionMoveTo` which
  already has the correct `move_to.go` hand-written dispatcher. No fix needed.
- `map_login` — FSM-owned; `fsmEncodeMapLogin` in `fsm.go` is the live code path with
  obfuscation support. `pkg/encode/map_login.go` is dead code for the connection sequence.
Two were found to be structural protocol gaps:
- `close_storage` — `clif_parse_CloseKafra` removed from packet tables after pv=20050110;
  not in `clif_shuffle.hpp`; no valid wire ID for modern servers.
- `public_chat` — `clif_parse_GlobalMessage` absent from shuffle table; last known valid ID
  `0x00F3` confirmed only to ~pv=20080910. Already partially hand-written.
Three are accidentally correct for pv=20200401 but wrong for shuffle era:
- `cz_party_join_req`, `friends_add`, `homunculus_menu` — post-shuffle stable IDs happen to
  coincide with original base IDs. Deferred (not currently used in goKore).

**Batch C (115 not-in-clif_packetdb):**
The initial grep used literal hex values but `clif_packetdb.hpp` registers many old-range packets
via `HEADER_*` symbolic constants (e.g. `HEADER_CZ_REQ_EMOTION = 0x00BF`). The literal hex never
appears — only the constant name. After accounting for this, all 115 are correctly classified:
- 9 `CA_CH_STABLE` — login/char server packets; never map-shuffled
- 69 `PACKETS_HPP_STABLE` — IDs >= 0x0800 via `DEFINE_PACKET_HEADER`; never shuffled
- 37 `OLD_CZ_STABLE` — old-range packets registered via `HEADER_*`; handler never reassigned

**Zero new bugs found in Batch C.** The 115 are all correctly encoded.

### Step 3 — Validation against OpenKore and rAthena

Both sources cross-checked for all 8 actionable encoders (5 shuffle-needed + 3 accidentally-correct).

**OpenKore** (`RagexeRE_2020_04_01b` inheritance chain → `RagexeRE_2018_11_21`):
- All 8 wire IDs confirmed via handler LUT entries and `ServerType0.pm` packet definitions
- `look` double-bug confirmed: OpenKore `actor_look_at` uses pack `'v C'` = 5 bytes, wire ID `0361`
- `skill_use_location` has an OpenKore LUT gap (missing explicit assignment post-2018_04_18a)
  but wire format at `0366` confirmed via `ServerType0.pm`. OpenKore bug, not a rAthena contradiction.

**rAthena** (GCC preprocessor at PACKETVER=20200401 + `clif_shuffle.hpp` stable block):
- All wire IDs confirmed via `parseable_packet(0xNNNN, SIZE, HANDLER, field_offsets...)` lines
  in the post-20180307 stable block of `clif_shuffle.hpp`
- `look` triple-bug confirmed via `RFIFOB` handler reads (headDir=u8@2, padding@3, dir=u8@4)
- Field types and sizes for all 5 broken encoders verified against rAthena handler code

---

## Confirmed Bugs

### BUG-1: `drop_item.go` — wrong packet ID

| | Value |
|---|---|
| Current wire ID | `0x00A2` (`CZ_ITEM_THROW`) |
| Correct wire ID at pv=20200401 | `0x0363` (`CZ_ITEM_THROW2`) |
| rAthena source | `clif_shuffle.hpp` post-20180307: `parseable_packet(0x0363, 6, clif_parse_DropItem, 2, 4)` |
| OpenKore source | `RagexeRE_2018_11_21.pm`: `item_drop 0363` |
| Return type correct | Yes — `[6]byte` |
| Field encoding correct | Yes — `index(u16)@[2:4]`, `amount(u16)@[4:6]` |

Pre-shuffle intermediate IDs (pv >= 20101124, field layout unchanged throughout):

| pv range | wire ID |
|---|---|
| [20101124, 20111005) | `0x0363` |
| [20111005, 20120307) | `0x0885` |
| [20120307, 20120418) | `0x02C4` |
| [20120418, 20120702) | `0x0362` |
| [20120702, 20130320) | `0x089E` |
| [20130320, 20130515) | `0x0438` |
| >= 20130515 | `shuffledCtoSID(pv, 0x00A2)` |
| > 20180307 | `0x0363` (stable) |

---

### BUG-2: `look.go` — wrong packet ID + wrong size + wrong layout (triple bug)

| | Value |
|---|---|
| Current wire ID | `0x009B` (`CZ_CHANGE_DIRECTION`) |
| Correct wire ID at pv=20200401 | `0x0361` (`CZ_CHANGE_DIRECTION2`) |
| rAthena source | `clif_shuffle.hpp` post-20180307: `parseable_packet(0x0361, 5, clif_parse_ChangeDir, 2, 4)` |
| OpenKore source | `RagexeRE_2018_11_21.pm`: `actor_look_at 0361`; pack `'v C'` = 5 bytes total |
| Current return type | `[4]byte` — **WRONG** |
| Correct return type | `[5]byte` |
| Current field encoding | `headDir(u8)@[2]`, `dir(u8)@[3]` — **WRONG** |
| Correct field encoding | `headDir(u8)@[2]`, `padding(0x00)@[3]`, `dir(u8)@[4]` |
| Consequence | Dir field silently dropped entirely (truncated at 4 bytes); server reads dir at offset 4 which is missing |

rAthena handler evidence: `RFIFOB(fd, pos[0])` reads headDir at byte 2; `RFIFOB(fd, pos[1])` reads dir
at byte 4. Total 5 bytes. Offset 3 is never read — it is a padding byte. OpenKore pack `'v C'` means
`headDir` is packed as a `v` (uint16 LE, 2 bytes at offsets 2–3), making offset 3 the high byte of the
uint16. Since headDir is always 0–2, the high byte is always 0x00. Functionally equivalent to
`headDir(u8)@[2] + pad(0x00)@[3] + dir(u8)@[4]`.

Pre-shuffle intermediate IDs (pv >= 20101124):

| pv range | wire ID |
|---|---|
| [20101124, 20111005) | `0x0361` |
| [20111005, 20120307) | `0x0366` |
| [20120307, 20120410) | `0x0890` |
| [20120410, 20120418) | `0x0871` |
| [20120418, 20120702) | `0x0202` |
| [20120702, 20130320) | `0x0960` |
| [20130320, 20130515) | `0x0897` |
| >= 20130515 | `shuffledCtoSID(pv, 0x009B)` |
| > 20180307 | `0x0361` (stable) |

---

### BUG-3: `move_from_storage.go` — wrong packet ID

| | Value |
|---|---|
| Current wire ID | `0x00F5` (`CZ_MOVE_ITEM_FROM_STORE_TO_BODY`) |
| Correct wire ID at pv=20200401 | `0x0365` (`CZ_MOVE_ITEM_FROM_STORE_TO_BODY2`) |
| rAthena source | `clif_shuffle.hpp` post-20180307: `parseable_packet(0x0365, 8, clif_parse_MoveFromKafra, 2, 4)` |
| OpenKore source | `RagexeRE_2018_11_21.pm`: `storage_item_remove 0365`; pack `'a2 V'` = 8 bytes |
| Return type correct | Yes — `[8]byte` |
| Field encoding correct | Yes — `index(u16)@[2:4]`, `amount(u32)@[4:8]` |

Pre-shuffle intermediate IDs (pv >= 20101124):

| pv range | wire ID |
|---|---|
| [20101124, 20111005) | `0x0365` |
| [20111005, 20120307) | `0x0897` |
| [20120307, 20120410) | `0x0963` |
| [20120410, 20120418) | `0x08A6` |
| [20120418, 20120702) | `0x0364` |
| [20120702, 20130320) | `0x0861` |
| [20130320, 20130515) | `0x0874` |
| >= 20130515 | `shuffledCtoSID(pv, 0x00F5)` |
| > 20180307 | `0x0365` (stable) |

---

### BUG-4: `move_to_storage.go` — wrong packet ID

| | Value |
|---|---|
| Current wire ID | `0x00F3` (`CZ_MOVE_ITEM_FROM_BODY_TO_STORE`) |
| Correct wire ID at pv=20200401 | `0x0364` (`CZ_MOVE_ITEM_FROM_BODY_TO_STORE2`) |
| rAthena source | `clif_shuffle.hpp` post-20180307: `parseable_packet(0x0364, 8, clif_parse_MoveToKafra, 2, 4)` |
| OpenKore source | `RagexeRE_2018_11_21.pm`: `storage_item_add 0364`; pack `'a2 V'` = 8 bytes |
| Return type correct | Yes — `[8]byte` |
| Field encoding correct | Yes — `index(u16)@[2:4]`, `amount(u32)@[4:8]` |

Pre-shuffle intermediate IDs (pv >= 20101124):

| pv range | wire ID |
|---|---|
| [20101124, 20111005) | `0x0364` |
| [20111005, 20120307) | `0x0893` |
| [20120307, 20120410) | `0x093B` |
| [20120410, 20120418) | `0x086C` |
| [20120418, 20120702) | `0x07EC` |
| [20120702, 20130320) | `0x08A0` |
| [20130320, 20130515) | `0x08AC` |
| >= 20130515 | `shuffledCtoSID(pv, 0x00F3)` |
| > 20180307 | `0x0364` (stable) |

---

### BUG-5: `skill_use_location.go` — wrong packet ID

| | Value |
|---|---|
| Current wire ID | `0x0116` (`CZ_USE_SKILL_TOGROUND`) |
| Correct wire ID at pv=20200401 | `0x0366` (`CZ_USE_SKILL_TOGROUND2`) |
| rAthena source | `clif_shuffle.hpp` post-20180307: `parseable_packet(0x0366, 10, clif_parse_UseSkillToPos, 2, 4, 6, 8)` |
| OpenKore source | `ServerType0.pm`: `'0366' => ['skill_use_location', 'v4', ...]` = 10 bytes; LUT gap noted in 2018_04_18a but wire format unambiguous |
| Return type correct | Yes — `[10]byte` |
| Field encoding correct | Yes — `skillLevel(u16)@[2:4]`, `skillID(u16)@[4:6]`, `xPos(u16)@[6:8]`, `yPos(u16)@[8:10]` |

Pre-shuffle intermediate IDs (pv >= 20101124):

| pv range | wire ID |
|---|---|
| [20101124, 20111005) | `0x0366` |
| [20111005, 20120702) | `0x0369` then `0x0438` (20120307 block sets 0x0438; 20120418 block has no entry — inherits 0x0438) |
| [20120702, 20130320) | `0x0863` |
| [20130320, 20130515) | `0x0959` |
| >= 20130515 | `shuffledCtoSID(pv, 0x0116)` |
| > 20180307 | `0x0366` (stable) |

---

## Non-Bug Findings

### `character_move.go` — duplicate, no fix needed
`ActionCharacterMove` and `ActionMoveTo` both encode `clif_parse_WalkToXY`. `move_to.go` is the
correct, hand-written implementation with full `shuffledCtoSID(pv, 0x0085)` dispatch. `character_move.go`
is an inferior codegen stub that is accidentally correct for pv=20200401 only. Not used in goKore.
No fix needed; document as duplicate.

### `map_login.go` — FSM-owned, dead code, no fix needed
`fsmEncodeMapLogin` in `pkg/session/fsm.go` is the live code path for map login. It hardcodes
`0x0436` and applies C→S obfuscation via `fsmEncodePacketID`. `pkg/encode/map_login.go` is a
codegen artifact that is never called in the actual connection sequence. No fix needed.

### `close_storage.go` — protocol removed from modern rAthena
`clif_parse_CloseKafra` was removed from the packet registration tables after pv=20050110 and is
absent from `clif_shuffle.hpp` entirely. No valid wire ID exists for pv=20200401. The current
encoder hardcodes `0x00F7` which at modern pv hits `clif_parse_MoveFromKafra`. Fix: add
`packetver_max=20050110` bound to the semantics DB entry and document in the encoder.

### `public_chat.go` — handler absent from shuffle table
`clif_parse_GlobalMessage` was dropped from `clif_packetdb.hpp` after ~pv=20080910 and is absent
from `clif_shuffle.hpp`. Last confirmed valid ID was `0x00F3` (pv < ~20080910). Already partially
hand-written — dispatches `0x00F3` for pv >= 20040726 and `0x008C` for baseline. Fix: add
`packetver_max=20080909` bound to the semantics DB entry and document the upper bound.

### `cz_party_join_req`, `friends_add`, `homunculus_menu` — accidentally correct, deferred
All three are accidentally correct for pv=20200401 because the post-20180307 stable shuffle block
recycled back to the original base IDs (`0x02C4`, `0x0202`, `0x022D`). Any shuffle-era packetver
(20111102–20180307) would receive the wrong wire ID. Not currently used in goKore. Deferred.

---

## Fix Plan

### All 5 fixes use the same pattern as `move_to.go` and `pickup_item.go`

**Why hand-written (not codegen):**  
The semantics DB codegen can emit a `switch { case packetver >= N }` dispatcher only when different
implementations have different structs. Here all variants use the same struct with the same field
layout — only the packet ID byte changes. More critically, the shuffle era (pv >= 20130515) requires
calling `shuffledCtoSID(pv, base)` at runtime, which codegen cannot emit. This is the established
pattern; `move_to.go` and `pickup_item.go` are precedents.

### Implementation sequence (all 5 in this PR)

```
1. Write failing tests (TDD) for all 5 encoders before any code changes
2. Implement drop_item.go
3. Implement look.go  (most complex — size + layout change; also update send.Look if needed)
4. Implement move_from_storage.go
5. Implement move_to_storage.go
6. Implement skill_use_location.go
7. Run full test suite + 0 allocs/op benchmarks
8. Update semantics DB via MCP (5 actions: bound existing null-null to pre-20101124; add modern impl)
9. Update semantics DB via MCP for close_storage and public_chat upper bounds
10. CHANGELOG, v0.5.8, push
```

### Semantics DB changes (via MCP only — no direct YAML edits)

For each of the 5 broken encoders:
- Delete existing `[null, null]` implementation
- Add `[null, 20101123]` implementation (baseline fallback with old wire ID)
- Add `[20101124, null]` implementation with the modern stable wire ID and correct struct

For `close_storage`: update existing implementation `packetver_max = 20050110`
For `public_chat`: update existing implementation `packetver_max = 20080909`

---

## References

| Source | File | Key lines |
|---|---|---|
| rAthena shuffle stable block | `src/map/clif_shuffle.hpp` | 4723–4761 (post-20180307) |
| rAthena explicit drop_item | `src/map/clif_packetdb.hpp` | 1385 (0x0363@20101124) |
| rAthena explicit look | `src/map/clif_packetdb.hpp` | 1383 (0x0361@20101124) |
| rAthena explicit move_from_storage | `src/map/clif_packetdb.hpp` | 1387 (0x0365@20101124) |
| rAthena explicit move_to_storage | `src/map/clif_packetdb.hpp` | 1386 (0x0364@20101124) |
| rAthena explicit skill_use_location | `src/map/clif_packetdb.hpp` | 1388 (0x0366@20101124) |
| OpenKore kRO 2020 | `src/Network/Send/kRO/RagexeRE_2018_11_21.pm` | item_drop, actor_look_at, storage_item_remove, storage_item_add |
| shuffle_map.go | `pkg/encode/shuffle_map.go` | stable block (pv > 20180307) |
| Reference encoder | `pkg/encode/move_to.go` | full pattern |
| Reference encoder | `pkg/encode/pickup_item.go` | full pattern |
