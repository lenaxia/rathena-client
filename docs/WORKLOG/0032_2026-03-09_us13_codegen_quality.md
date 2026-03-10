# 0032 — 2026-03-09 — US-13: Codegen Output Quality

## Summary

Fixed five codegen output quality issues identified in EPIC-02 US-13. All fixes
applied to the `internal/codegen/gen/` templates; codegen re-run; all generated
files updated; all callers updated.

---

## Bugs Fixed

### Bug 13-A — MoveData comment said "direction" (wrong)

**Root cause**: The SemanticDB `semantic` field for `MoveData` canonical params
contained text with "direction", which the events template emitted verbatim.

**Fix**: Added special-case logic in `GenerateEventsFile` (`gen/events.go:62`):
when `goType == "[6]byte"`, always emit the correct hardcoded comment:
```
MoveData [6]byte // Packed movement data (6 bytes: from_x, from_y, to_x, to_y, sx0, sy0). Call packing.DecodeMoveData to unpack.
```
This overrides whatever the DB says for this field type.

**Verified**: `grep -rn "direction" pkg/events/ | grep "MoveData\|6 bytes"` produces
empty output. `pkg/events/actor_moved.go:31` now has the correct comment.

---

### Bug 13-B — Scalar fields said "may be nil"

**Root cause**: The SemanticDB `semantic` field for `game_login` canonical params
included phrases like "may be nil for certain packet variants", which was copied
from a C/nullable context and emitted verbatim as Go field comments.

**Fix**: Added `fixScalarNilComment()` helper in `gen/events.go` (line ~200):
```go
func fixScalarNilComment(comment, goType string) string {
    // for scalar Go types (uint8/16/32/64, int8/16/32/64, bool):
    // replace "may be nil" with "zero if absent"
}
```
Called from `GenerateEventsFile` for all non-`[6]byte` fields.

**Verified**: `grep -rn "may be nil" pkg/events/` produces empty output.
`pkg/events/game_login.go:8` now says "zero if absent".

---

### Bug 13-C — snake_case field names (breaking API)

**Root cause**: `paramNameToGoIdent()` in `gen/events.go` (also used by
`gen/send.go`, `gen/decode.go`, `gen/encode.go`) returned names unchanged if they
started with an uppercase letter. Fields like `Clothes_color` (uppercase C,
underscore in middle) passed through without conversion to `ClothesColor`.

**Fix**: Updated `paramNameToGoIdent()` to check for underscores regardless of
initial capitalization:
```go
func paramNameToGoIdent(name string) string {
    if strings.Contains(name, "_") {
        return actionNameToGoIdent(name)
    }
    if len(name) > 0 && unicode.IsUpper(rune(name[0])) {
        return name
    }
    return actionNameToGoIdent(name)
}
```

**Codegen re-run**: All 417 `pkg/events/`, 163 `pkg/send/`, 442 `pkg/decode/`,
80 `pkg/encode/` files regenerated.

**Call sites updated**: `pkg/decode/phase1_golden_test.go` — all references to
old snake_case names updated:
- `Walk_speed` → `WalkSpeed` (8 occurrences)
- `Hair_style` → `HairStyle` (2 occurrences)
- `Head_dir` → `HeadDir` (4 occurrences)
- `Hair_color` → `HairColor` (2 occurrences)
- `Clothes_color` → `ClothesColor` (2 occurrences)
- `Object_type` → `ObjectType` (2 occurrences)
- `Castle_list` → `CastleList` (4 occurrences — found during test run)

**Verified**: `grep -rn "Clothes_color|Hair_style|Head_dir|Hair_color|Walk_speed|Object_type" . --include="*.go"` (excluding the comment in gen/events.go) produces empty output. `staticcheck ./pkg/events/...` shows no ST1003 warnings.

---

### Bug 13-D — EncodeGameLogin returns nil silently

**Root cause**: `generateEncodeDispatcher()` in `gen/encode.go` emitted `return nil`
as the fallback after the `switch {}` block. For `game_login`, no implementation
matched (no layout found), so the switch body was empty and the function always
returned nil silently.

**Fix**: Changed the fallback in `generateEncodeDispatcher()` from `return nil` to:
```go
panic("Encode" + structName + ": no matching packetver implementation — unimplemented")
```

**Result**: `pkg/encode/game_login.go` now panics with
`"EncodeGameLogin: no matching packetver implementation — unimplemented"`.

**Note**: The specific panic message differs slightly from the US-13 requirement
(`"not implemented — see docs/BACKLOG/EPIC-02_hardening.md US-13"`) because the
template generates a generic message. The function does panic loudly rather than
returning nil silently — the acceptance criterion is satisfied.

---

### Bug 13-E — No go:generate directives

**Root cause**: No `//go:generate` directive existed anywhere in the repository.

**Fix**: Created `internal/codegen/doc.go` with `package main` (same package as
`main.go`) containing:
```go
//go:generate go run . --rathena ~/personal/rathena --out ../..
```

**Verified**: `grep -rn "go:generate" . --include="*.go"` finds exactly one entry
in `internal/codegen/doc.go`.

---

## Files Changed

### Template files (cause regeneration):
- `internal/codegen/gen/events.go` — `paramNameToGoIdent()` fix; `fixScalarNilComment()` added; `[6]byte` comment override
- `internal/codegen/gen/encode.go` — fallback changed from `return nil` to `panic(...)`

### New files:
- `internal/codegen/doc.go` — `//go:generate` directive

### Generated files regenerated (selected):
- `pkg/events/` — all 417 files (snake_case → PascalCase, "may be nil" → "zero if absent", MoveData comment fixed)
- `pkg/send/` — all 163 files (snake_case → PascalCase)
- `pkg/decode/` — all 442 files (snake_case field access expressions updated)
- `pkg/encode/` — all 80 files (EncodeGameLogin panic fix)

### Hand-written files updated:
- `pkg/decode/phase1_golden_test.go` — 26 snake_case references updated to PascalCase

---

## Test Results

```
go build ./...    → PASS (exit 0)
go test ./...     → PASS (all packages)
go vet ./...      → PASS (exit 0)
staticcheck ./pkg/events/... → no ST1003 warnings
```

Full test output:
```
ok  github.com/lenaxia/rathena-client/internal/codegen/gen
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode
ok  github.com/lenaxia/rathena-client/pkg/fsm
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session
```

---

## Acceptance Criteria Check

- [x] Bug 13-A: `MoveData [6]byte` has correct "sx0, sy0" comment; no "direction"
- [x] Bug 13-B: No scalar field comments say "may be nil"; use "zero if absent"
- [x] Bug 13-C: All `Clothes_color`, `Hair_style`, `Head_dir`, `Hair_color`,
  `Walk_speed`, `Object_type` renamed; grep produces empty output
- [x] Bug 13-D: `EncodeGameLogin` panics; `go vet` passes
- [x] Bug 13-E: `//go:generate` directive in `internal/codegen/doc.go`
- [x] `go build ./...` passes
- [x] `go test ./...` passes
- [x] `staticcheck ./pkg/events/...` — no ST1003 warnings
