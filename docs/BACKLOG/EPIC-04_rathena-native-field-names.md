# EPIC-04: Replace Canonical Field Renames with rAthena-Native Field Names

**Date**: 2026-03-10  
**Status**: Approved for implementation  
**Priority**: High — blocks Phase 7 goKore integration (Phase 7 handlers would be written against
renamed names that we then change; doing this now costs nothing, doing it after costs
a full handler rewrite)

---

## Problem Statement

The semantic DB (`semantics/mappings.yaml`) stores a `field_mapping` for every canonical
parameter in every action implementation. These mappings contain two kinds of entries:

**Kind A — identity (no rename):** `"GID": "packet.GID"` — the canonical name is already
the rAthena field name. 1,566 generated field assignments (48%) are this kind. The DB adds
no information here; the codegen could derive these directly from the struct layout.

**Kind B — rename:** `"Lowhead": "packet.accessory"` — the canonical name differs from
the rAthena field name. 1,716 generated field assignments (52%) are this kind. These are
**human judgments encoded in the DB**, and 306 of them are currently wrong (validation
errors). The errors are concentrated in renames: malformed expressions like `"[3]byte{}"`,
wrong field names, `GID→ID` vs `GID→CharID` inconsistencies across actions, and pointer
types (`*uint32`) that are invalid Go.

The rename layer adds complexity without adding correctness. It is the **primary source of
every `// implement manually` gap** in the generated decode functions — each gap is a field
mapping expression that `extractFieldName()` could not parse.

Under the rename model, the codegen's `extractFieldName()` function must parse arbitrary
Go expressions from the DB (`"packet.accessory"`, `"[3]byte(packet.PosDir[:])"`,
`"int16(packet.robe)"`, `"func() *uint32 { ... }()"`) to extract which rAthena field to
read. Every novel expression pattern is a new failure mode. Removing renames eliminates
`extractFieldName()` entirely for the straight-through case.

---

## Proposed Change

**Use rAthena field names directly as event struct field names.** The DB `canonical_params`
list becomes the list of rAthena field names the action exposes, with their Go types derived
from the C types in the struct layout. The `field_mapping` block is eliminated for all
straight-through cases.

### Before (current)

```yaml
actor_exists:
  canonical_params:
    - name: ID          # renamed from GID/AID
      type: uint32
    - name: Lowhead     # renamed from accessory
      type: int16
    - name: Opt1        # renamed from bodyState
      type: int16
  implementations:
    - packet_id: "0x09FF"
      struct_name: packet_idle_unit
      field_mapping:
        ID: "packet.AID"
        Lowhead: "packet.accessory"
        Opt1: "packet.bodyState"
```

Generated event struct:
```go
type ActorExists struct {
    ID       uint32  // Actor unique identifier
    Lowhead  int16   // Lower headgear sprite
    Opt1     int16   // Body state (stone, freeze, stun, sleep)
}
```

### After (proposed)

```yaml
actor_exists:
  canonical_params:
    - name: AID         # rAthena field name, present in >= 20131223
      type: uint32
    - name: GID         # rAthena field name, present in all versions
      type: uint32
    - name: accessory   # rAthena field name
      type: int16
    - name: bodyState   # rAthena field name
      type: int16
  implementations:
    - packet_id: "0x09FF"
      struct_name: packet_idle_unit
      # no field_mapping needed — param names ARE rAthena field names
```

Generated event struct:
```go
type ActorExists struct {
    AID        uint32  // rAthena: AID — actor block ID (>= PACKETVER 20131223, else zero)
    GID        uint32  // rAthena: GID — char ID (>= 20131223) or actor block ID (< 20131223)
    accessory  int16   // rAthena: accessory — lower headgear sprite ID
    bodyState  int16   // rAthena: bodyState — body state flags
}
```

---

## Key Design Decisions

### 1. Field name casing

rAthena field names are mixed-case (`bodyState`, `GID`, `isPKModeON`, `accessory`).
Go convention is PascalCase for exported fields. Options:

**Option A — keep rAthena case, export with PascalCase conversion:**
`bodyState` → `BodyState`, `GID` → `GID`, `isPKModeON` → `IsPKModeON`

**Option B — keep rAthena names verbatim, use rAthena case directly:**
`bodyState`, `GID`, `isPKModeON` (unexported — not viable for public API)

**Decision: Option A.** Apply the same PascalCase conversion used today
(`actionNameToGoIdent`), but the *input* to that conversion is now the rAthena field name,
not the DB canonical name. The comment on each field cites the original rAthena name.

Result:
- `bodyState` → `BodyState` (was `Opt1`)
- `healthState` → `HealthState` (was `Opt2`)
- `accessory` → `Accessory` (was `Lowhead`)
- `accessory2` → `Accessory2` (was `Tophead`)
- `accessory3` → `Accessory3` (was `Midhead`)
- `GID` → `GID` (was `ID` in some actions, `CharID` in others)
- `AID` → `AID` (was `ID` in some actions)
- `isPKModeON` → `IsPKModeON` (was `Stance`)
- `effectState` → `EffectState` (was `Option`)
- `virtue` → `Virtue` (was `Opt3`)
- `body` → `Body` (was `Opt4`)

### 2. The AID/GID dual-role problem

For `packet_idle_unit` (stationary actors), before PACKETVER 20131223 only `GID` exists
and it holds the actor block ID. From 20131223 onward, `AID` holds the actor block ID and
`GID` holds the character ID.

**Under the old model:** the DB hid this by mapping `AID → ID` and `GID → CharID` for
new versions, and `GID → ID` for old versions. The event struct had one `ID` field that
was always the actor block ID.

**Under the new model:** the event struct has both `AID uint32` and `GID uint32`.
For old packetvers, `AID` is zero and `GID` is the actor block ID.
For new packetvers, `AID` is the actor block ID and `GID` is the character ID.

goKore handlers must use: `id := e.AID; if id == 0 { id = e.GID }` — a two-liner that
is explicit about the protocol behavior. This is **more correct** than hiding it via a
rename, because the handler now knows which packetver path it received.

This pattern recurs for several rAthena packets where fields changed roles across versions.
Document it clearly in `docs/USAGE.md`.

### 3. The `field_mapping` block under the new model

For straight-through cases (param name = rAthena field name), no `field_mapping` is needed.
The codegen derives the offset directly from the struct layout by field name lookup.

`field_mapping` is retained only for the **three genuinely non-trivial cases**:

1. **Bool fields:** `IsIdentified`, `IsDamaged` — rAthena type is `uint8`, event type is
   `bool`. The DB records `"IsIdentified": "bool"` as a type hint, not a mapping expression.
   The codegen generates `data[N] != 0`.

2. **Absent fields with non-zero defaults:** rare; handled by the existing `nil`/`null`
   convention.

3. **Action-grouping mismatches like 0x02EC:** fix the grouping (move `0x02EC` to
   `actor_moved`) rather than expressing it via a field mapping.

Everything else — type widening, array slices, string fields — is handled by `fieldReadExpr`
already based on the Go type and field metadata from the struct layout.

### 4. The `FieldMapping` block is eliminated from the DB for most actions

The DB `field_mapping` is replaced by a simple list of field names (= rAthena names) the
action exposes. For actions where all params are identity, the entire `field_mapping` block
is dropped. The DB shrinks significantly.

---

## Impact Assessment

### What changes

| Layer | Change | Effort |
|---|---|---|
| `semantics/mappings.yaml` | Rename `canonical_params.name` values from DB names to rAthena names; drop `field_mapping` for straight-through cases | Automated via codegen pass |
| `internal/codegen/gen/decode.go` | Remove `extractFieldName()`; replace with direct field-name lookup by param name against struct layout | ~50 lines removed |
| `internal/codegen/gen/events.go` | PascalCase conversion input changes from DB name to rAthena name; no logic change | Trivial |
| `internal/codegen/gen/send.go` | Same as events.go | Trivial |
| `pkg/events/*.go` | All 417 generated event struct files regenerated | Automated |
| `pkg/decode/*.go` | All 442 generated decode functions regenerated; `// implement manually` gaps eliminated for bool/string cases | Automated |
| `pkg/send/*.go` | All 163 generated send structs regenerated | Automated |
| `pkg/decode/*_test.go` | `phase1_golden_test.go` references field names — regenerate or update | Manual, ~1,133 lines |
| `docs/PHASE7_SPEC.md` | Handler examples use `e.ID`, `e.ObjectType`, `e.Speed` — update to `e.AID`/`e.GID`, `e.objecttype`, `e.speed` | Manual, ~10 lines |
| `docs/USAGE.md` | Add note on AID/GID dual-role pattern | Manual, ~20 lines |

### What does NOT change

- `pkg/fsm/` — no event field references
- `pkg/session/` — no event field references  
- `pkg/packing/` — no event field references
- `internal/codegen/preprocess/` — no change
- goKore handlers — not yet written against rathena-client event fields (Phase 7 not started)

### Scope of field name changes (the rename table)

137 distinct rename pairs currently exist. After this change, all 137 become the rAthena
name. The most common:

| Old canonical | New (rAthena) | Occurrences |
|---|---|---|
| `Opt1` | `BodyState` | 81 |
| `Opt2` | `HealthState` | 81 |
| `HairColor` | `Headpalette` | 79 |
| `ClothesColor` | `Bodypalette` | 79 |
| `GuildID` | `GUID` | 79 |
| `ID` (actor) | `GID` or `AID` | 78 |
| `Option` | `EffectState` | 72 |
| `WalkSpeed` | `Speed` | 70 |
| `Manner` | `Honor` | 70 |
| `Lv` | `Clevel` | 70 |
| `Midhead` | `Accessory3` | 70 |
| `Tophead` | `Accessory2` | 70 |
| `Lowhead` | `Accessory` | 70 |
| `Type` (job) | `Job` | 65 |
| `HairStyle` | `Head` | 61 |
| `EmblemID` | `GEmblemVer` | 61 |

---

## User Stories

### Story 1: Codegen — eliminate `extractFieldName`, derive fields from layout directly

**Scope**: `internal/codegen/gen/decode.go`

**Current behavior**: `generateFieldReads` calls `extractFieldName(expr)` to parse a DB
expression string into a rAthena field name, then looks up the field in the struct layout.

**New behavior**: `generateFieldReads` receives the canonical param name (which IS the
rAthena field name), looks it up directly in the struct layout by name, and emits the read
expression. No expression parsing needed.

**Special case handling** (replaces `extractFieldName`):
- Param name found in layout → emit `fieldReadExpr(field, goType)` as today
- Param name not found in layout but known in other versions → emit `// e.X = zero (absent in this version)`
- Param name not found in any version → emit `// e.X: field not found in layout` (diagnostic)
- Bool type hint in `field_mapping` → `fieldReadExpr` already handles `case "bool": return "data[N] != 0"`

**Deliverables**:
- Remove `extractFieldName()` and `isZeroLiteral()` functions
- Update `generateFieldReads()` to do direct name lookup
- Keep `fieldReadExpr()` unchanged
- All 129 `// implement manually` gaps for bool/string cases close automatically
- `// implement manually` gaps for `[3]byte{}` close (they were DB expression errors — now absent-field zeros)

**Acceptance criteria**:
- `go build ./...` passes
- `go test ./internal/codegen/...` passes
- Zero `// implement manually` comments remain for bool and fixed-string fields
- `// implement manually` count drops from 129 to ≤ 10 (only genuinely complex cases remain)

---

### Story 2: DB migration — rename canonical params to rAthena names, drop field_mapping

**Scope**: `semantics/mappings.yaml` (via MCP only)

For each action in the DB:
1. Replace each `canonical_params[].name` with the corresponding rAthena field name
2. Drop the `field_mapping` block entirely for actions where all mappings are straight-through
3. For the three special cases (bool type hints, absent-field markers, action-grouping
   fixes), retain a minimal `field_mapping` with only those entries

**The migration is mechanical**: the current `field_mapping` already tells us the rAthena
field name for each canonical name (it's the field name extracted from expressions like
`"packet.accessory"` → `"accessory"`). A codegen pass can emit the new DB entries
automatically, which are then applied via MCP.

**Action-grouping fixes to make in this story**:
- Move `0x02EC` (`packet_unit_walking`) from `actor_exists` to `actor_moved`
- Audit other walking-unit packet IDs in standing-actor actions

**Deliverables**:
- All 477 action implementations in DB updated
- `field_mapping` blocks eliminated for straight-through actions (~95% of all actions)
- `field_mapping` retained only for bool-type-hint entries
- `0x02EC` moved to correct action

**Acceptance criteria**:
- `semantics_validate` error count drops from 306 to < 20
- `go run ./internal/codegen/main.go ...` regenerates without errors
- `go build ./...` passes after regeneration

---

### Story 3: Regenerate all generated packages

**Scope**: `pkg/events/`, `pkg/decode/`, `pkg/send/`

Run codegen after Stories 1 and 2 complete:

```bash
go run ./internal/codegen/main.go \
    --rathena ~/personal/rathena \
    --semantics semantics/mappings.yaml \
    --out .
```

**Deliverables**:
- All 417 event struct files regenerated with rAthena field names
- All 442 decode functions regenerated; 0-alloc benchmark targets maintained
- All 163 send struct files regenerated
- Zero `// implement manually` for bool and fixed-string fields

**Acceptance criteria**:
- `go build ./...` passes
- `go test ./...` passes (golden tests will need updating — see Story 4)
- `go test -bench=. -benchmem ./pkg/...` shows 0 allocs/op on all benchmarks
- `grep -r "^\s*go " pkg/` produces empty output

---

### Story 4: Update tests and documentation

**Scope**: `pkg/decode/phase1_golden_test.go`, `docs/PHASE7_SPEC.md`, `docs/USAGE.md`

**`phase1_golden_test.go`** (1,133 lines): update all field name references from canonical
to rAthena-native. The byte layouts do not change — only the field names in assertions.
Example:
```go
// Before
assert(t, e.ID == 1001)
assert(t, e.Lowhead == 8)
assert(t, e.Opt1 == 1)

// After  
assert(t, e.AID == 1001)
assert(t, e.Accessory == 8)
assert(t, e.BodyState == 1)
```

**`docs/PHASE7_SPEC.md`** handler examples: update 10 field references.

**`docs/USAGE.md`**: add section documenting:
- AID/GID dual-role pattern and the `if AID == 0 { use GID }` idiom
- How to find the rAthena field name for any event struct field (it's the PascalCase of
  the C field name; the struct comment cites it explicitly)

**Acceptance criteria**:
- `go test ./...` passes with zero failures
- All field references in `phase1_golden_test.go` use rAthena-native names
- `docs/USAGE.md` has AID/GID pattern documented

---

## Story Ordering and Estimated Duration

| Story | Description | Days |
|---|---|---|
| 1 | Codegen: remove `extractFieldName`, direct layout lookup | 0.5 |
| 2 | DB migration: rename params, drop field_mapping | 1.0 |
| 3 | Regenerate all generated packages | 0.5 |
| 4 | Update tests and documentation | 0.5 |
| **Total** | | **~2.5 days** |

Stories 1 and 2 are independent and can be done in parallel. Story 3 requires both 1 and 2
complete. Story 4 requires Story 3 complete.

---

## Acceptance Criteria for EPIC-04 Complete

1. `go build ./...` passes
2. `go test ./...` passes
3. `go test -bench=. -benchmem ./pkg/...` shows 0 allocs/op on all decode/encode benchmarks
4. `grep -r "^\s*go " pkg/` produces empty output
5. `grep -rc "implement manually" pkg/decode/` — total count ≤ 10 (only genuinely
   irreducible cases; zero for bool and fixed-string fields)
6. `semantics_validate` error count < 20
7. Zero references to old canonical names (`Lowhead`, `Tophead`, `Midhead`, `Opt1`, `Opt2`,
   `Opt3`, `Opt4`, `WalkSpeed`, `ClothesColor`, `HairColor`, `HairStyle`, `EmblemID`,
   `GuildID`, `ObjectType`, `Stance`, `Costume`, `Manner`, `Lv`) in any non-WORKLOG file
8. Work log created (`docs/WORKLOG/0035_...`)

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| DB migration misses some actions | Low | Codegen pass generates the new DB entries mechanically from current `field_mapping`; validate with `semantics_validate` after |
| AID/GID dual-role confuses Phase 7 handler authors | Medium | Document the pattern in `USAGE.md` before Phase 7 starts; the PHASE7_SPEC handler examples will use the correct pattern |
| Bool type hint fields not recognized by new codegen | Low | The new codegen checks Go type `bool` in `fieldReadExpr` which already handles it; test explicitly in Story 1 |
| Some `// implement manually` gaps are genuinely irreducible | Low | Audit remaining gaps after Story 3; they will be the actual complex cases (multi-step transforms, pointer fields in `SkillUse`) |
| Phase 7 goKore handler code written against old names before this epic completes | None | Phase 7 has not started; this epic must complete before Phase 7 Story 4 (auth handlers) begins |
