# 0006 — Struct Layout Verification: Field-Level and Full-Redefinition PACKETVER Coverage

**Date**: 2026-03-07  
**Scope**: Design and implementation of `struct_layout.sh` + `parse_struct_fields.py`; integration into `phase1_gate.sh`; coverage of all three PACKETVER struct variation patterns.

---

## Goal

Extend `phase1_gate.sh` to verify struct **field layouts** (not just struct existence) across all PACKETVER breakpoints for Phase 1 target structs. The verifier must handle:

1. **Field-level changes** — individual fields added/removed/resized inside a single struct body
2. **Full struct redefinitions** — different struct bodies in `#if / #elif / #else` blocks with the same name
3. **UNAVAILABLE_STRUCT tombstones** — struct explicitly disabled at certain PACKETVERs
4. **`__attribute__((packed))` byte layout** — zero padding, offsets are cumulative sums only

---

## Key Discovery: All Three Patterns Are Identical to the Tool

From the preprocessor's perspective, all three PACKETVER variation patterns produce the same output format after `g++ -E -P`:

| Source pattern | Preprocessed result |
|---|---|
| Field-level `#if` inside struct | Plain struct body, no conditionals |
| Full redefinition `#if` / `#elif` / `#else` | Plain struct body (the active branch) |
| `UNAVAILABLE_STRUCT` in active branch | Struct body containing `UNAVAILABLE_STRUCT;` |

The tool does not need to know which pattern produced the output — it just reads the preprocessed file.

---

## Architecture

### New Files

**`validation/struct_layout.sh`** — Generic struct layout tool  
Three commands:
- `dump HEADER STRUCT PACKETVER [STUB=path]` — Print full field table (name, type, offset, size, note)
- `verify HEADER STRUCT PACKETVER EXPECTED [FIELDSPECS...] [STUB=path]` — Assert layout matches expectation
- `status HEADER STRUCT PACKETVER [STUB=path]` — Print `AVAILABLE | UNAVAILABLE | ABSENT`

`EXPECTED` values:
- Integer N → struct must be AVAILABLE with exactly N total bytes
- `UNAVAILABLE` → struct must contain `UNAVAILABLE_STRUCT` sentinel
- `ABSENT` → struct name must not appear in preprocessed output

**`validation/parse_struct_fields.py`** — Called by `struct_layout.sh` as a subprocess  
Reads a struct body from a tmpfile (not stdin — bash heredoc conflicts with stdin pipes).  
Handles:
- Scalar fields: `TYPE NAME;`
- Array fields: `TYPE NAME[EXPR];` — EXPR may be arithmetic like `(23 + 1)` (eval'd safely, GCC output only)
- `UNAVAILABLE_STRUCT` sentinel → exits with code 1
- `PosDir` / `MoveData` annotated with `packing=WBUFPOS` / `packing=WBUFPOS2`
- Zero-padding: offsets are purely cumulative field size sums

### Cache Layer

Preprocessed files are cached in `validation/output/layouts/` as  
`{header_safe}_{packetver}.h` (e.g. `map_packets_struct_20181121.h`).  
Subsequent calls for the same `(header, packetver)` pair skip g++ invocation.

---

## Structs Verified

### `map/packets_struct.hpp` — Field-level changes

| Struct | Breakpoints verified | Layouts |
|---|---|---|
| `packet_idle_unit` | 20080101, 20080102, 20091103, 20101124, 20120221, 20131223, 20150513, 20181121 | 8 |
| `packet_spawn_unit` | same set | 8 |
| `packet_unit_walking` | adds 20071106 | 9 |
| `packet_authok` | 20080101, 20080102, 20141022, 20160330 | 4 |

### `map/packets_struct.hpp` — UNAVAILABLE_STRUCT pattern

| Struct | Available range | Tombstoned at |
|---|---|---|
| `packet_idle_unit2` | PACKETVER < 20091103 | 20091103+ |
| `packet_spawn_unit2` | PACKETVER < 20091103 | 20091103+ |

### `map/packets.hpp` — Full struct redefinition

| Struct | Layouts | Sizes |
|---|---|---|
| `PACKET_ZC_ACCEPT_ENTER` | 3 redefinitions | 11B / 13B / 14B / 13B (non-monotonic at 20160330) |

Note: `PACKET_ZC_ACCEPT_ENTER` is **non-monotonic** — size goes 11→13→14→13 because the 20160330 breakpoint drops `sex` while keeping the same packet ID as the `< 20141022` body. This is the strongest argument for total-byte checking: it catches non-additive changes that field spot-checks alone might miss.

### `common/packets.hpp` — Field-level changes

| Struct | Breakpoints | Sizes |
|---|---|---|
| `PACKET_AC_ACCEPT_LOGIN_sub` | 20170314, 20170315 | 32B → 160B |
| `CHARACTER_INFO` | 20170315, 20180307 | 147B → 155B |
| `PACKET_HC_NOTIFY_ZONESVR` | 20170314, 20170315 | 28B → 156B |

---

## Important Finding: `name` Field Size

Earlier analysis (worklogs 0003–0005) computed `packet_idle_unit` sizes incorrectly because `char name[(23 + 1)]` was parsed as a single `char` (1 byte) rather than `char[24]` (24 bytes).

Correct sizes at key breakpoints for `packet_idle_unit`:

| PACKETVER | Correct total | Previous (wrong) |
|---|---|---|
| 20131223 | 102B | 79B |
| 20150513 | 104B | 81B |
| 20181121 | 108B | 85B |

The `parse_struct_fields.py` correctly evaluates array size expressions using `eval()` on GCC preprocessor output (safe: all macros already resolved, only arithmetic remains).

---

## Key Design Decision: stdin vs tmpfile

The initial implementation used `echo "$body" | parse_fields` where `parse_fields` contained a bash heredoc (`python3 << 'PYEOF'`). This silently discarded the piped input — bash heredoc redirects stdin, overriding the pipe.

**Fix**: Extract struct body to a tmpfile, pass the filename as `argv[1]` to the Python script. Clean up tmpfile in `cleanup_body()`.

---

## Gate Results

| Previous | This session |
|---|---|
| 36 PASS / 1 FAIL | **76 PASS / 1 FAIL** |

The 40 new assertions cover:
- 38 struct layout verifications (AVAILABLE with total + field spot-checks)
- 2 UNAVAILABLE_STRUCT tombstone verifications

The 1 remaining failure is the pre-existing `CH_MAKE_CHAR` shuffle table issue (documented since worklog 0004, deferred to Phase 3).

---

## Extending Coverage

To add a new struct to the gate:

1. Get its breakpoints: `awk '/^struct MYSTRUCT/,/^\}/' packets_struct.hpp | grep -oE 'PACKETVER.*[0-9]{8}'`
2. Dump layout at each breakpoint: `bash validation/struct_layout.sh dump map/packets_struct.hpp MYSTRUCT PV`
3. Add `check_layout` calls to `phase1_gate.sh` section 6

To add a new PACKETVER series (e.g. `PACKETVER_RE_NUM`, `PACKETVER_ZERO_NUM`): add separate preprocess calls with appropriate defines and new `check_layout` invocations.

---

## Files Modified

| File | Change |
|---|---|
| `validation/struct_layout.sh` | New — struct layout dump/verify/status tool |
| `validation/parse_struct_fields.py` | New — Python field parser |
| `validation/phase1_gate.sh` | Section 6 replaced: struct existence → full layout verification (40 assertions) |
| `docs/WORKLOG/0006_...` | This file |
