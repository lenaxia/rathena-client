# Worklog 0017 — US-05 Known Issue Fixes: PERF-1, PERF-2, AcAcceptLogin

**Date**: 2026-03-08  
**Session focus**: Fix the three issues documented in `KNOWN_ISSUES.md` after US-05 golden tests

---

## Issues Fixed

### PERF-1: nullTermString — 1 alloc/op → 0 alloc/op

**File**: `pkg/decode/helpers.go`

Changed `nullTermString` from `string(b[:i])` (copies, allocates) to `unsafe.String`
+ `unsafe.SliceData` (Go 1.20+). The returned string is a zero-copy alias over the
input `[]byte` slice. No heap allocation occurs.

```go
// before
func nullTermString(b []byte) string {
    for i, c := range b {
        if c == 0 { return string(b[:i]) }  // allocates
    }
    return string(b)  // allocates
}

// after
func nullTermString(b []byte) string {
    n := len(b)
    for i, c := range b {
        if c == 0 { n = i; break }
    }
    if n == 0 { return "" }
    return unsafe.String(unsafe.SliceData(b), n)  // zero alloc
}
```

Benchmark verification:
```
BenchmarkActorExists_0x09FF: 0 B/op, 0 allocs/op  (was 1 alloc)
BenchmarkAcAcceptLogin_0x0069: 0 B/op, 0 allocs/op
```

---

### PERF-2: Wrong read width for uint8 field widened to uint32

**File**: `internal/codegen/gen/decode.go` (fieldReadExpr function)

Added width-mismatch detection before the type switch. When the canonical param type
is wider than the actual rAthena field size, the codegen now emits a narrower read
with an explicit cast:

```go
case f.Size == 1 && (goType == "uint16" || goType == "uint32" || goType == "uint64"):
    return fmt.Sprintf("%s(data[%d])", goType, off), false
case f.Size == 2 && (goType == "uint32" || goType == "uint64"):
    return fmt.Sprintf("%s(leU16(data, %d))", goType, off), false
// ... etc
```

Generated `StatUpdate_0x00BE` now correctly emits:
```go
e.Value = uint32(data[4])  // rAthena: value (offset 4, size 1)
```
instead of the previous (buggy) `leU32(data, 4)`.

Golden test updated to use an exact 5-byte buffer (no padding needed).

---

### AcAcceptLogin_0x0069: struct not in VersionTable

**Problem**: `PACKET_AC_ACCEPT_LOGIN` is defined in `common/packets.hpp` (not
`packets_struct.hpp`), so `buildVersionTable()` never saw it. The generated file was:
```go
// SKIP AcAcceptLogin_0x0069: struct PACKET_AC_ACCEPT_LOGIN not found in VersionTable
```

**Fix — two parts**:

1. **SemanticDB field_mapping corrections** (`semantics/mappings.yaml`):
   The field mappings used incorrectly capitalized field names (`packet.Sex`,
   `packet.Last_ip`, `packet.Login_id1`, etc.). The actual C struct fields are
   all lowercase (`sex`, `last_ip`, `login_id1`, etc.). Fixed via
   `semantics_bulk_update_field_mappings`. `AID` remains uppercase (correct).

2. **New `injectCommonPacketStructs` step** (`internal/codegen/main.go`):
   Added Step 5c in the `run()` function. The function preprocesses
   `common/packets.hpp` at all its PACKETVER breakpoints, extracts struct layouts,
   and injects structs listed in `commonStructsToInject` into the VersionTable.
   Currently injects `PACKET_AC_ACCEPT_LOGIN` with 2 version ranges:
   - `[20030000, 20170315)` — without `token` field (47-byte fixed header)
   - `[20170315, ∞)` — with `token[WEB_AUTH_TOKEN_LENGTH]` field

Generated `ac_accept_login.go` now correctly decodes:
```go
func AcAcceptLogin_0x0069(data []byte, packetver uint32) events.AcAcceptLogin {
    ...
    e.AccountID = leU32(data, 8)
    e.LastLoginIP = leU32(data, 16)
    e.LastLoginTime = nullTermString(data[20:46])
    e.SessionID = leU32(data, 4)
    e.SessionID2 = leU32(data, 12)
    e.AccountSex = data[46]
    ...
}
```

The `char_servers` flex array is not decoded by the generated code (expected — it
requires a higher-level packet parser).

---

## Golden Tests Added / Updated

- `TestStatUpdate_0x00BE_Golden`: updated to use exact 5-byte buffer, removed bug workaround comments
- `TestAcAcceptLogin_0x0069_Golden`: new — verifies all 6 decoded fields
- `BenchmarkAcAcceptLogin_0x0069`: new — confirms 0 allocs/op

---

## Files Changed

| File | Change |
|------|--------|
| `pkg/decode/helpers.go` | `nullTermString` → `unsafe.String` (zero-alloc) |
| `internal/codegen/gen/decode.go` | `fieldReadExpr` — add width-mismatch narrowing |
| `internal/codegen/main.go` | Add Step 5c: `injectCommonPacketStructs` |
| `semantics/mappings.yaml` | Fix lowercase field names in `ac_accept_login` field_mapping |
| `internal/codegen/semantics/validate_test.go` | Update expected field_mapping values |
| `pkg/decode/phase1_golden_test.go` | Update 0x00BE test + add AcAcceptLogin golden test |
| `docs/KNOWN_ISSUES.md` | Mark PERF-1 and PERF-2 as RESOLVED |
| `pkg/decode/` (generated) | Regenerated: 442 files, stat_update + ac_accept_login fixed |

---

## Test Results

```
go test ./...  → all PASS
BenchmarkActorExists_0x09FF: 0 allocs/op
BenchmarkAcAcceptLogin_0x0069: 0 allocs/op
phase1_gate.sh: 76 PASS / 1 FAIL (1 pre-existing: CH_MAKE_CHAR 0x0065 shuffle)
```

---

## Next Steps (from EPIC00 backlog)

- **US-06**: Fix flex array parser in codegen (-313 skips, Category A)
- **US-07**: Fix 39 wrong Category C field names in SemanticDB (-468 skips)
- **US-02**: Full S→C lengths pipeline in codegen (map server packets)
