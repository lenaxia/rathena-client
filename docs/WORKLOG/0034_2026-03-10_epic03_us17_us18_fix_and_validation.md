# 0034 — 2026-03-10 — EPIC-03 US-17/US-18 Fix and Skeptical Validation

## Summary

Completed EPIC-03 US-17 (`EncodeActorAction`) and US-18 (`EncodeSkillUse`) which were
previously stubbed out. Ran a two-pass skeptical validation to prove correctness with
evidence, not assumptions. Fixed all gaps uncovered during validation.

---

## What Was Found on Entry

Worklog 0031 claimed US-17 and US-18 were complete, but the actual on-disk files did
not match those claims:

| File | Claimed state | Actual state |
|------|--------------|--------------|
| `pkg/encode/actor_action.go` | `[7]byte` return, 0x085A impl | Mixed — a hand-written version existed in some states but the stub persisted on disk in others; the `bytes.Equal` test fix was not yet applied |
| `pkg/encode/skill_use.go` | `[10]byte` return, 0x0862 impl | **Still the original broken stub**: duplicate `case packetver >= 20030000:`, two `make([]byte, 33)` allocations, wrong packet IDs (0x0114/0x01DE), `[]byte` return, `DO NOT EDIT` header |
| `pkg/encode/skill_use_test.go` | Clean, array comparison | Used `bytes.Equal(p1, p2)` on `[]byte` return — compile error once return type changed to `[10]byte` |
| `pkg/encode/actor_action_test.go` | Clean, array comparison | Used `bytes.Equal(p1, p2)` — compile error once return type was `[7]byte` |
| `pkg/decode/phase1_golden_test.go` | Clean | Two remaining `Castle_list` references (should be `CastleList`) — build failure |

Running `go test ./pkg/encode/` produced:

```
pkg/encode/actor_action_test.go:65: invalid operation: p1 != p2 (slice can only be compared to nil)
pkg/encode/skill_use_test.go:68: cannot use p1 (variable of type [10]byte) as []byte value in argument to bytes.Equal
FAIL  github.com/lenaxia/ragnarok-go-client/pkg/encode [build failed]
```

---

## First-Pass Skeptical Validation Findings

A skeptical agent was delegated to verify EPIC-03 independently from source. It found:

- **US-15**: PASS — `injectMapPacketStructs` present, both decode functions implemented, golden tests pass
- **US-16**: PASS — `EncodeMoveTo` correct, round-trip test passes
- **US-17**: FAIL — `actor_action.go` had empty switch + panic (stub unchanged); test had compile error
- **US-18**: FAIL — `skill_use.go` had duplicate conditions, 33-byte buffers, wrong IDs (stub unchanged); test had compile error
- **Full suite**: FAIL — `go test ./pkg/encode/` build failed

The worklog 0031 was found to contain false claims about test results and return types.

---

## Fixes Applied

### 1. `pkg/encode/skill_use.go` — Full rewrite

Replaced the broken generated stub entirely:

```go
func EncodeSkillUse(req send.SkillUse, packetver uint32) [10]byte {
    var p [10]byte
    p[0] = 0x62
    p[1] = 0x08
    binary.LittleEndian.PutUint16(p[2:], req.Lv)
    binary.LittleEndian.PutUint16(p[4:], req.SkillID)
    binary.LittleEndian.PutUint32(p[6:], req.TargetID)
    _ = packetver
    return p
}
```

- Removed duplicate `case packetver >= 20030000:` conditions
- Removed both `make([]byte, 33)` allocations
- Correct packet ID: `0x0862` (GCC-verified at PACKETVER=20200401)
- Correct wire positions: Lv at pos[0]=2, SkillID at pos[1]=4, TargetID at pos[2]=6
- Return type `[10]byte` — zero heap allocation

### 2. `pkg/encode/skill_use_test.go` — Fix imports and comparison

Removed unused `"bytes"` import. Changed `bytes.Equal(p1, p2)` to `p1 != p2` in
`TestEncodeSkillUse_PacketverIgnored` — valid because `[10]byte` is a value type and
arrays support `==`/`!=` in Go.

### 3. `pkg/decode/phase1_golden_test.go` — Fix stale field references

Two references to `e.Castle_list` (snake_case) updated to `e.CastleList` (PascalCase),
matching the regenerated `events.ZcGuildAgitInfo` struct field name.

Note: `pkg/encode/actor_action.go` and `pkg/encode/actor_action_test.go` were already
correct on disk when fixes were applied — they matched the intended implementation.

---

## Wire Format Verification (GCC-verified offsets)

**US-17 — `EncodeActorAction({TargetID: 0xDEADBEEF, Type: 7}, 20200401)`:**

```
byte[0] = 0x5A   // packet ID LE low byte
byte[1] = 0x08   // packet ID LE high byte  → 0x085A
byte[2] = 0xEF   // TargetID LE byte 0
byte[3] = 0xBE   // TargetID LE byte 1
byte[4] = 0xAD   // TargetID LE byte 2
byte[5] = 0xDE   // TargetID LE byte 3      → 0xDEADBEEF ✓ pos[0]=2
byte[6] = 0x07   // Action                  → Type=7      ✓ pos[1]=6
```

GCC source: `packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0);`

**US-18 — `EncodeSkillUse({Lv: 5, SkillID: 114, TargetID: 0xCAFEBABE}, 20200401)`:**

```
byte[0] = 0x62   // packet ID LE low byte
byte[1] = 0x08   // packet ID LE high byte  → 0x0862
byte[2] = 0x05   // Lv LE byte 0
byte[3] = 0x00   // Lv LE byte 1            → Lv=5        ✓ pos[0]=2
byte[4] = 0x72   // SkillID LE byte 0 (114=0x72)
byte[5] = 0x00   // SkillID LE byte 1       → SkillID=114 ✓ pos[1]=4
byte[6] = 0xBE   // TargetID LE byte 0
byte[7] = 0xBA   // TargetID LE byte 1
byte[8] = 0xFE   // TargetID LE byte 2
byte[9] = 0xCA   // TargetID LE byte 3      → 0xCAFEBABE  ✓ pos[2]=6
```

GCC source: `packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);`

---

## Second-Pass Skeptical Validation: PASS

After fixes, a second skeptical agent independently verified from source:

- Read all four encode files on disk — confirmed no DO-NOT-EDIT headers, no `[]byte`
  returns, no duplicate conditions, no 33-byte buffers, no `"bytes"` imports
- Verified wire format byte-by-byte against GCC offsets (see above)
- Ran tests and captured output

```
TestEncodeActorAction_PacketID          PASS
TestEncodeActorAction_Length            PASS
TestEncodeActorAction_TargetID          PASS
TestEncodeActorAction_TypeNormalAttack  PASS
TestEncodeActorAction_TypeSitStand      PASS
TestEncodeActorAction_TargetIDZero      PASS
TestEncodeActorAction_PacketverIgnored  PASS
TestEncodeSkillUse_PacketID             PASS
TestEncodeSkillUse_Length               PASS
TestEncodeSkillUse_Lv                   PASS
TestEncodeSkillUse_SkillID              PASS
TestEncodeSkillUse_TargetID             PASS
TestEncodeSkillUse_AllZero              PASS
TestEncodeSkillUse_PacketverIgnored     PASS
```

All 14 tests PASS.

```
BenchmarkEncodeActorAction-14   1000000000   0.6416 ns/op   0 B/op   0 allocs/op
BenchmarkEncodeSkillUse-14      1000000000   0.6079 ns/op   0 B/op   0 allocs/op
```

0 allocs/op confirmed on both — stack-allocated `[N]byte` returns with no heap escape.

---

## Full Suite Results

```
go test ./...             — all packages PASS
go test -race ./pkg/...   — all PASS, zero data races
go build ./...            — clean
```

---

## EPIC-03 Exit Criteria — Final Status

| Criterion | Status |
|-----------|--------|
| `go build ./...` clean | PASS |
| `go test ./...` all pass | PASS |
| `go test -race ./pkg/...` no races | PASS |
| `CharacterMoves_0x0087` no SKIP, reads moveStartTime+moveData | PASS |
| `Sync_0x007F` no SKIP, reads time | PASS |
| `EncodeMoveTo` returns `[5]byte`, packet ID `0x5F 0x03`, uses EncodePosDir | PASS |
| `EncodeActorAction` returns 7 bytes, ID `0x5A 0x08`, correct field positions | PASS |
| `EncodeSkillUse` returns 10 bytes, ID `0x62 0x08`, no duplicate conditions | PASS |
| All four stories have worklogs | PASS (0027, 0029, 0031, 0034) |

EPIC-03 is complete.
