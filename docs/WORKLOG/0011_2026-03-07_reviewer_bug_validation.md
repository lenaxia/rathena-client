# Worklog 0011 — Skeptical Reviewer Bug Validation

**Date**: 2026-03-07  
**Status**: Completed

---

## Summary

A skeptical reviewer identified four potential bugs in the previous session's work. This
session systematically validated each one against GCC output and actual rAthena source.

---

## Bug 1: synthetic_structs.hpp sizes claimed wrong

**Reviewer claim**: Synthetic struct sizes may not match `clif_packetdb.hpp` lengths.

**Validation method**: Wrote a GCC test program that computes `sizeof()` for each struct
and compared against the `parseable_packet(id, length, ...)` entries.

```
g++ -o /tmp/verify_synth /tmp/verify_synthetic_sizes.cpp && /tmp/verify_synth
```

**Result**: All 19 structs verified correct:
- SYNTH_CZ_CONCLUDE_EXCHANGE_ITEM: 2 ✓  (clif_packetdb:92 = 2)
- SYNTH_CZ_CLOSE_STORE: 2 ✓            (clif_packetdb:97 = 2)
- SYNTH_CZ_NOTIFY_ACTORINIT: 2 ✓       (clif_packetdb:32 = 2)
- SYNTH_CZ_ITEM_PICKUP: 6 ✓            (clif_packetdb:50 = 6)
- SYNTH_CZ_ITEM_PICKUP2: 6 ✓           (clif_packetdb:1384 = 6)
- SYNTH_CZ_REQ_NEXT_SCRIPT: 6 ✓        (clif_packetdb:63 = 6)
- SYNTH_CZ_REQUEST_TIME: 6 ✓           (clif_packetdb:33 = 6)
- SYNTH_CZ_REQUEST_TIME2: 6 ✓          (clif_packetdb:1382 = 6)
- SYNTH_CZ_REQUEST_MOVE: 5 ✓           (clif_packetdb:37 = 5)
- SYNTH_CZ_REQUEST_MOVE2: 5 ✓          (clif_packetdb:1381 = 5)
- SYNTH_CZ_USE_ITEM: 8 ✓               (clif_packetdb:56 = 8)
- SYNTH_CZ_MOVE_ITEM_FROM_BODY_TO_STORE: 8 ✓ (clif_packetdb:95 = 8)
- SYNTH_CZ_MOVE_ITEM_FROM_STORE_TO_BODY: 8 ✓ (clif_packetdb:96 = 8)
- SYNTH_CZ_ITEM_THROW2: 6 ✓            (clif_packetdb:1385 = 6)
- SYNTH_CZ_USE_SKILL_TOGROUND: 10 ✓    (clif_packetdb:114 = 10)
- SYNTH_CZ_ENTER: 19 ✓                 (clif_packetdb:1148 = 19)
- SYNTH_CH_ENTER_0x0065: 17 ✓          (clif_packetdb:11 = 17)
- SYNTH_ZC_AID: 6 ✓                    (clif_packetdb:800 = 6)
- SYNTH_ZC_PC_SELL_RESULT: 3 ✓         (clif.cpp:12332-12337 = 3)

**Verdict**: No bug. All sizes were already correct.

---

## Bug 2: SemanticDB rathena_struct corrections wrong/incomplete

**Reviewer claim**: Some Category A phantom → real struct name corrections may be wrong,
and some entries may have been missed.

**Validation method**: GCC preprocessing to get complete struct name list, then verified
each Category A real struct name appears:

```
g++ -E -P -DPACKETVER=20181002 -I./src -I./src/common ./src/map/packets_struct.hpp \
  | grep -E 'packet_idle_unit|packet_spawn_unit|...'
```

**Result**: All 14 unique Category A real struct names confirmed present:
- `packet_idle_unit`, `packet_spawn_unit`, `packet_unit_walking` ✓
- `packet_dropflooritem`, `packet_itemlist_normal`, `packet_itemlist_equip` ✓
- `packet_sc_notick`, `packet_damage`, `packet_skill_entry` ✓
- `packet_status_change2`, `packet_monster_hp`, `PACKET_ZC_USE_ITEM_ACK` ✓

**Verdict**: No bug. All corrections were already correct.

---

## Bug 3: extractFieldName / isZeroLiteral logic bugs

**Reviewer claim**: Edge cases produce wrong Go expressions or incorrect field names.

**Testing method**: Enumerated all distinct `field_mapping` value patterns in
`semantics/mappings.yaml` and tested each against the current implementation.

**Real bugs found and fixed**:

### Bug 3a: `"nil"` matched plain-identifier branch → returned `"nil"` as field name
- The plain-identifier branch (`!ContainsAny(expr, ".()[]...")`) matched `"nil"` because
  it starts with a letter and has no special characters.
- Generator would then look for a field named `nil` in the layout, fail, and emit a
  "field nil not found in layout" comment instead of the correct "absent in this version" comment.

**Fix**: Added `goKeywordsAndLiterals` map that guards the plain-identifier branch.
`nil`, `true`, `false`, and all Go keywords are now rejected.

### Bug 3b: `func() *string { ... strings.TrimRight(packet.mapName, ...) }()` → extracted `"mapName, \"\\x00\""` as field name
- The type-cast branch `strings.Index(expr, "(packet.")` found `(packet.` inside the
  function literal and extracted garbage up to the next `.` or `)`.
- Generator would try to look up field `mapName, "\x00"` in the layout and fail.

**Fix**: The type-cast branch now requires:
1. No spaces in the expression (`!strings.Contains(expr, " ")`) — function literals always
   have spaces.
2. The cast type prefix (before `(packet.`) must be a simple identifier with no special chars.
3. The character after the field name must specifically be `)` — not just any `.[]()`
   character. This prevents extracting partial strings.

Additionally, early rejection: `strings.HasPrefix(expr, "func")` returns `""` immediately.

**All 26 test cases now pass**, covering:
- `packet.Field`, `packet.Field[:]`, `uint32(packet.Field)`, `[]byte(packet.Field)` — correct extraction
- `nil`, `true`, `false`, `"0"`, `p.Field`, `&packet.Field`, `func(){...}()` — correctly return `""`
- `packet.Flag != 0`, `[]byte{packet.Key}` — correctly return `""`

**Verdict**: Real bugs found and fixed in `gen/decode.go`.

---

## Bug 4: PHANTOM_STRUCTS.md categorization not fully validated (Rule 12)

**Reviewer claim**: Some Category assignments may be wrong. Rule 12 requires exhaustive
grep across ALL `.hpp` AND `.cpp` files.

**Validation method**: 
- Category B: `grep -rn "struct $name" /home/mikekao/personal/rathena/src/` for all 22 names
- Category A: confirmed all 12 real struct names have ≥1 occurrence in rAthena src
- Line numbers: confirmed against `packets_struct.hpp` directly

**Result**:
- All 22 Category B phantom names: zero struct occurrences across entire rAthena src ✓
- All 12 unique Category A real struct names: confirmed present with matching line numbers ✓

**Verdict**: No bug. Categorization was already correct.

---

## Files Changed

### `internal/codegen/gen/decode.go`
- Added `goKeywordsAndLiterals` map to guard plain-identifier extraction
- Rewrote `extractFieldName` to fix the two bugs above:
  - `nil`/`true`/`false` now correctly return `""`
  - Function literal expressions with `(packet.` inside now correctly return `""`
  - `[]byte(packet.X)` and `uint32(packet.X)` forms: require the character after field name
    to be specifically `)` (not any punctuation)
  - Added early rejection for `strings.HasPrefix(expr, "func")`
  - Removed the ambiguous `end < 0 → return rest` branch (unreachable in valid inputs)

---

## Gate Status

**76 PASS / 1 FAIL** — unchanged (expected failure: CH_MAKE_CHAR shuffle).  
All codegen tests pass: `go test ./internal/codegen/...` — OK.  
Full build clean: `go build ./...` — OK.

---

## Next Steps

The four reviewer bugs are resolved. Remaining work before Phase 4:

1. **Extend VersionTable pipeline** to process `synthetic_structs.hpp` — so SYNTH_* structs
   are available for decode codegen lookup.

2. **Extend VersionTable** to cover `packets.hpp` static structs (PACKET_ZC_* defined there).

3. **Re-run codegen** after pipeline extensions — expect fewer "struct not found" skips.

4. **Phase 4**: `pkg/session` — stateful packet dispatch and event emission.
