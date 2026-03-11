# EPIC-05: lengths_map.go Correctness — Packet Length Table Audit & Fix

**Status**: IN PROGRESS  
**Created**: 2026-03-11  
**Priority**: HIGH — incorrect lengths cause `session.Feed()` to fault or mis-frame packets at runtime  
**Discovered by**: Systematic comparison of `pkg/session/lengths_map.go` against OpenKore
`recvpackets.txt` at two spot-check packetvers (20180621, 20200401)

---

## Problem Statement

`pkg/session/lengths_map.go` is the generated packet length lookup table used by
`session.Feed()` to frame incoming TCP bytes. If an entry is wrong (wrong length, wrong
packetver threshold, or missing entirely), `Feed()` will mis-frame all subsequent packets
and either fault with `ErrUnknownPacket` or silently decode garbage.

A systematic spot-check against OpenKore's `recvpackets.txt` at two packetvers revealed
**four distinct root-cause classes** producing hundreds of incorrect entries:

### Spot-Check Results

| Metric | pv=20180621 | pv=20200401 |
|---|---|---|
| Our table size | 1,285 | 1,328 |
| OpenKore table size | 1,429 | 1,537 |
| **Missing from ours entirely** | **338** | **398** |
| Total mismatches | 203 | 228 |
| — OpenKore says `2` (C→S shuffle slot) | 64 | 64 |
| — Ours says `0` (unknown/disabled) | 52 | 61 |
| — **Both nonzero, truly different lengths** | **87** | **103** |

> **Note on OpenKore `recvpackets.txt`**: OpenKore's file contains *both* C→S and S→C
> packets (it is a complete packet length table, not purely S→C). The `ok=2` entries are
> C→S shuffle slot stubs that OpenKore enumerates as 2-byte header-only. These may or may
> not indicate bugs depending on whether the ID is genuinely C→S-only at that packetver.

---

## Investigation Status (2026-03-11)

All four root causes have been fully investigated. Results below.

---

## Root Cause Classes

### RC-1: Nested Struct Size Collapse (CONFIRMED BUG — ROOT CAUSE IDENTIFIED)

**Affected packets**: `0x0219`, `0x021A`, `0x0226`, `0x0238`, `0x097D`, and others  
**Symptom**: Our length is ~14-48 bytes; GCC / OpenKore says 282-288 bytes

The codegen fails to expand nested struct types when computing packet total size.
`PACKET_ZC_BLACKSMITH_RANK` contains a `RANKLIST` member:

```c
// packets.hpp
struct RANKLIST {
    char names[10][24];   // 240 bytes
    uint32 points[10];    // 40 bytes
};                        // total: 280 bytes

struct PACKET_ZC_BLACKSMITH_RANK {
    int16 packetType;     // 2 bytes
    RANKLIST list;        // 280 bytes — codegen collapses this!
};                        // correct total: 282 bytes; ours: 42
```

GCC-verified correct sizes:
| Packet | Ours | OpenKore | GCC truth |
|---|---|---|---|
| `0x0219` (ZC_BLACKSMITH_RANK) | **42** | 282 | **282** |
| `0x021A` (ZC_ALCHEMIST_RANK) | **42** | 282 | **282** |
| `0x0226` (ZC_TAEKWON_RANK) | **42** | 282 | **282** |
| `0x0238` (ZC_PK_RANK) | **42** | 282 | **282** |
| `0x097D` (top-10 variant) | **48** | 288 | TBD |

**Root cause (confirmed)**: `RANKLIST` contains `char names[10][(23+1)]` — a **2D array**.
The parser's `reArrayField` regex is `^(\w+)\s+(\w+)\[([^\]]+)\]$` which only matches
single-bracket arrays. `char names[10][(23+1)]` has two bracket pairs and does not match.
It falls through to the "unknown/unparsed" branch, contributing 0 bytes.

Simulation result:
- `RANKLIST.TotalSize` computed = **40** (only `uint32 points[10]` = 40 counted; `names` = 0)
- Correct = **280** (10×24 names + 10×4 points)
- `PACKET_ZC_BLACKSMITH_RANK` = 2 + 40 = **42** (wrong) vs 2 + 280 = **282** (correct)

**There is only one 2D array field in all rAthena packets**: `RANKLIST.names[10][(23+1)]`
in `packets.hpp`. No other 2D arrays exist in `packets_struct.hpp` or `common/packets.hpp`
(verified by GCC grep at pv=20180621).

**Fix**: Add a 2D array regex to `internal/codegen/preprocess/parser.go`:
```go
var re2DArrayField = regexp.MustCompile(`^(\w+)\s+(\w+)\[([^\]]+)\]\[([^\]]+)\]$`)
```
When matched: `size = evalExpr(dim1) * evalExpr(dim2) * typeSizes[typ]`. Concrete steps in US-05-1 below.

---

### RC-2: Wrong Packetver Threshold for Actor Entity Packets (CONFIRMED BUG — ROOT CAUSE IDENTIFIED)

**Affected packets**: `0x0071`, `0x0092` (and possibly others)  
**Symptom**: Our table assigns **156** bytes at `pv >= 20170315`; rAthena GCC says **28** always

```go
// lengths_map.go (WRONG)
if pv >= 20170315 {
    t[0x0071] = 156    // WRONG — should be 28
    t[0x0092] = 156    // WRONG — should be 28
    ...
}
```

GCC verification at every version from 20150101 through 20200101:
```
g++ -E -P -DPACKETVER=20180621 ... clif_packetdb.hpp → packetdb_addpacket(0x0071,28,...)
g++ -E -P -DPACKETVER=20170315 ... clif_packetdb.hpp → packetdb_addpacket(0x0071,28,...)
g++ -E -P -DPACKETVER=20200101 ... clif_packetdb.hpp → packetdb_addpacket(0x0071,28,...)
```

**Root cause (confirmed)**: Two separate but related bugs in the SemanticDB:

**Bug A — Wrong packet_id in SemanticDB for `received_character_ID_and_Map` action**:
The action `received_character_ID_and_Map` has `packet_id=0x0071` with
`struct=PACKET_HC_NOTIFY_ZONESVR`. But `PACKET_HC_NOTIFY_ZONESVR` is a CHAR server packet
whose actual header IDs are:
- Before pv 20170315: `HEADER_HC_NOTIFY_ZONESVR = 0x0081`
- From pv >= 20170315: `HEADER_HC_NOTIFY_ZONESVR = 0x0AC5`

The struct is **never** at 0x0071. SemanticDB has a wrong packet_id. `PACKET_HC_NOTIFY_ZONESVR`
is 156 bytes (2 + 4 + 16 + 4 + 2 + 128). The Part 3 join pass emits 156 → 0x0071 starting
at the breakpoint where the struct first appears (pv=20170315). **This is pure SemanticDB error.**

**Bug B — Struct ID reassignment not reflected in SemanticDB**:
`PACKET_ZC_NPCACK_SERVERMOVE` had `HEADER = 0x0092` before pv=20170315, then changed to
`HEADER = 0x0AC7`. The SemanticDB still maps packet_id=0x0092 to this struct. After 20170315,
the Part 3 join pass emits the struct's 156-byte size to 0x0092 — but 0x0092 is now a
different 28-byte packet (`ZC_NPCACK_SERVERMOVE` moved away).

**Fix strategy**:
- For Bug A: Fix the SemanticDB action to use the correct packet_id (`0x0081` pre-20170315,
  `0x0AC5` post-20170315)
- For Bug B: The Part 3 join pass must respect the `HEADER_*` packetver ranges, or the
  SemanticDB must have versioned packet_id entries. Simpler fix: set packetver_max on the
  implementation to stop emitting size at the version where the header moved.
  Concrete steps in US-05-2 below.

Key confirmed wrong assignments (GCC-verified):
| Packet | Ours | OpenKore | rAthena GCC |
|---|---|---|---|
| `0x0071` | **156** at pv≥20170315 | 28 | **28** (always, per clif_packetdb) |
| `0x0092` | **156** at pv≥20170315 | 28 | 28 before 20170315; **0xAC7=156** after |

---

### RC-3: Large Gap of Missing Packet IDs — MOSTLY NON-ISSUE (ROOT CAUSE IDENTIFIED)

**Symptom**: 338–398 packet IDs in OpenKore `recvpackets.txt` are not in our table

**Root cause (confirmed by exhaustive GCC check)**:
Of 338 missing IDs at pv=20180621:
- **335 are NOT in any rAthena header** — they are OpenKore-reverse-engineered packets
  (from kRO traffic captures, Ragexe disassembly, etc.) that rAthena has no struct definition
  for. We cannot derive these from GCC and they are out-of-scope for this epic.
- **3 are in rAthena headers**: `0x08A2`, `0x0A3D`, `0x0A49`
  - `0x08A2`: C→S in `clif_shuffle.hpp` (length=7/5/-1 depending on shuffle slot) — ok=2 in OpenKore (stub)
  - `0x0A3D`: not found in any header (likely an OpenKore-only entry despite the scan flagging it)
  - `0x0A49`: C→S in `clif_packetdb.hpp` with `sizeof(PACKET_CZ_PRIVATE_AIRSHIP_REQUEST)` — **dropped by our parser**

**Sub-root-cause RC-3b: sizeof() in clif_packetdb.hpp not parsed**:
Some entries in `clif_packetdb.hpp` use `sizeof(STRUCT_NAME)` instead of a literal integer
for the length parameter. After GCC `-E -P`, `sizeof()` is NOT evaluated — it stays as
`sizeof(struct PACKET_CZ_PRIVATE_AIRSHIP_REQUEST)`. Our `rePacketEntry` regex requires a
literal `-?\d+` and silently drops these entries.

At pv=20180621, there are **5 such sizeof entries** — all C→S packets:
- `0x08A2`, `0x0838`, `0x0365` → `sizeof(struct PACKET_CZ_SSILIST_ITEM_CLICK)`
- `0x0A49` → `sizeof(struct PACKET_CZ_PRIVATE_AIRSHIP_REQUEST)`

These are all C→S; the `session.Feed()` S→C framing table does not need them for receiving.
However for correctness of the C→S encode side, these should be resolved.

**Conclusion**: The "338 missing IDs" alarm is almost entirely expected and benign —
OpenKore's table contains reverse-engineered data our GCC pipeline cannot produce.
The `sizeof` gap is real but affects only 5 C→S entries. **RC-3 is LOW priority.**

**Fix** (optional): Extend `ParsePacketDB` to resolve `sizeof(STRUCT_NAME)` against a
provided `StructDB`. Concrete steps in US-05-3 below.

---

### RC-4: Variable vs. Fixed Mismatches — PARTIALLY EXPLAINED

**Symptom**: We and OpenKore disagree on fixed vs. variable for some packets

**Root cause (confirmed for actor packets)**:
Actor entity packets (`0x09FF`, `0x09FD`, `0x09FE`, `0x09DB`) are **fixed-length per
PACKETVER** — their size changes at each rAthena struct breakpoint but is fixed within
a version. OpenKore marks them as `-1` (variable) because it uses a dynamic lookup table
rather than compiled-in per-version sizes. **We are correct; OpenKore uses a conservative -1.**

For other mismatches (e.g. `0x0166` we=32 vs ok=-1), per-packet GCC verification is needed.
These will be addressed case-by-case in US-05-4.

---

## Story Map

```
US-05-1  Fix nested struct size collapse  ──────────────────────────────► US-05-5 Regression
US-05-2  Fix wrong packetver breakpoints  ──────────────────────────────► US-05-5 Regression
US-05-3  Fill missing packet IDs          ──────────────────────────────► US-05-5 Regression
US-05-4  Fix variable/fixed mismatches    ──────────────────────────────► US-05-5 Regression
                                                                                    │
                                                                                    ▼
                                                                         US-05-5 Validate + Commit
```

US-05-1 through US-05-4 are largely independent and can run in parallel.
US-05-5 (validation + regression harness) runs after all fixes are applied.

---

## US-05-1 — Fix 2D Array Parsing (RC-1)

### Goal
Correctly compute `TotalSize` for `RANKLIST` (and any future 2D array fields) so that
`PACKET_ZC_BLACKSMITH_RANK` and related ranking packets get the correct 282-byte length.

### Root Cause (Confirmed)
`internal/codegen/preprocess/parser.go`:`reArrayField` regex does not match 2D arrays.
`char names[10][(23+1)]` is silently dropped, giving `RANKLIST.TotalSize = 40` instead
of 280.

### Exact Fix

In `internal/codegen/preprocess/parser.go`, add a 2D array regex and handler before
the existing `reArrayField` check (order matters — match 2D before 1D):

```go
// re2DArrayField matches: TYPE NAME[DIM1][DIM2]
var re2DArrayField = regexp.MustCompile(`^(\w+)\s+(\w+)\[([^\]]+)\]\[([^\]]+)\]$`)
```

In `ParseStructBody`, in the field-parsing loop, add before the `reArrayField` check:
```go
if m := re2DArrayField.FindStringSubmatch(line); m != nil {
    typ, _, dim1Expr, dim2Expr := m[1], m[2], m[3], m[4]
    elemSize := typeSizes[typ]
    if elemSize == 0 {
        // unknown type in 2D array — skip
    } else {
        size := evalExpr(dim1Expr) * evalExpr(dim2Expr) * elemSize
        offset += size
        fields = append(fields, Field{...})
    }
    continue
}
```

### Acceptance Criteria
- [ ] `RANKLIST.TotalSize` = 280 after fix
- [ ] `PACKET_ZC_BLACKSMITH_RANK`, `PACKET_ZC_ALCHEMIST_RANK`, `PACKET_ZC_TAEKWON_RANK`,
  `PACKET_ZC_KILLER_RANK` all compute length = 282
- [ ] `0x0219`, `0x021A`, `0x0226`, `0x0238` in generated `lengths_map.go` = 282
- [ ] Existing parser tests still pass
- [ ] New test: `TestParseStructBody_2DArray` verifies `char names[10][(23+1)]`
- [ ] Worklog entry

---

## US-05-2 — Fix Wrong Packet ID Assignments in SemanticDB (RC-2)

### Goal
Fix the SemanticDB and/or the Part 3 join pass so that struct size changes are attributed
to the correct packet IDs at the correct PACKETVER ranges.

### Root Cause (Confirmed)
Two bugs:

**Bug A**: Action `received_character_ID_and_Map` has `packet_id=0x0071` but
`struct=PACKET_HC_NOTIFY_ZONESVR`. The struct's actual headers are 0x0081 (before 20170315)
and 0x0AC5 (from 20170315 onward). The Part 3 join pass emits size=156 to 0x0071 starting
at the first PACKETVER where the struct exists in the VersionTable (~20170315). This is wrong —
0x0071 is always 28 bytes and unrelated to `PACKET_HC_NOTIFY_ZONESVR`.

**Bug B**: Action `zc_npcack_servermove` has `packet_id=0x0092` but
`struct=PACKET_ZC_NPCACK_SERVERMOVE`. The struct's header was 0x0092 before 20170315 and
0x0AC7 from 20170315 onward. The implementation in the SemanticDB has no `packetver_max` set,
so the Part 3 join pass continues to emit size=156 to 0x0092 indefinitely, even after the
struct moved to 0x0AC7.

### Exact Fix

**Fix A (SemanticDB)**: Update the `received_character_ID_and_Map` / `received_character_id_and_map`
action implementations:
- Remove the `packet_id=0x0071` implementation (it maps to a char server HC_ packet, not a map server packet)
- If the action is needed on the char server side, use `packet_id=0x0081` (pre-20170315) and
  `packet_id=0x0AC5` (from 20170315) with appropriate `packetver_min`/`packetver_max`

**Fix B (SemanticDB)**: Update the `zc_npcack_servermove` action's `packet_id=0x0092`
implementation to add `packetver_max=20170315` so the Part 3 join stops emitting the
156-byte size to 0x0092 at that version.

### Systematic Audit
Beyond these two known bugs, audit ALL SemanticDB actions whose struct has a `HEADER_*`
that changed value between packetvers. For each such action, verify the Part 3 join
correctly associates struct sizes to packet IDs only within the valid PACKETVER range.

Cross-reference: query the SemanticDB for all actions with `direction=receive`, then
for each, GCC-preprocess at the action's struct breakpoints and confirm
`HEADER_<STRUCT> == packet_id` in that range.

### Acceptance Criteria
- [ ] `0x0071` = 28 at ALL packetvers (Part 1 always wins via clif_packetdb)
- [ ] `0x0092` = 28 (or the correct pre-20170315 value) and NOT 156
- [ ] `PACKET_HC_NOTIFY_ZONESVR` size (156) appears at `0x0081` (pre-20170315) and
  `0x0AC5` (post-20170315), not at 0x0071
- [ ] Audit complete: list of all other struct-header-change cases checked
- [ ] Worklog entry

---

## US-05-3 — Fill Missing Packet IDs

### Goal
Add the ~340–398 packet IDs that are completely absent from our table at modern packetvers.

### Investigation Needed (before implementing)
1. Extract the full set of missing IDs from the comparison script
2. Classify each missing ID:
   - **Class A**: Present in `packets.hpp` via `DEFINE_PACKET_HEADER` but missing from our
     Part 2 parser coverage
   - **Class B**: Only in `clif_packetdb.hpp` (C→S) — should be in our table but missing
     from Part 1 coverage
   - **Class C**: Not in rAthena headers at all (deprecated, removed, or OpenKore-specific)
3. For Class A + B: fix the codegen to pick them up
4. For Class C: create NOOP entries in the SemanticDB with `reason=deprecated_packet`

### Priority sub-groups
- **Critical** (0x07DC–0x07F4, 0x080C–0x0825): Mid-range IDs that likely include active
  gameplay packets. Missing lengths here will cause framing faults in modern sessions.
- **Important** (0x0300–0x035B): Large gap block — likely legacy packets, lower risk but
  should be filled for completeness.
- **Low** (scattered modern IDs): Individual new packets — add as encountered.

### Acceptance Criteria
- [ ] All IDs in 0x07DC–0x07F4 range that exist in rAthena accounted for
- [ ] All IDs in 0x080C–0x0825 range that exist in rAthena accounted for
- [ ] 0x0300–0x035B gap analyzed; present-in-rAthena entries added, removed ones
  documented as NOOP/deprecated
- [ ] Net coverage improvement: missing count < 100 at pv=20180621
- [ ] Worklog entry

---

## US-05-4 — Fix Variable/Fixed Mismatches

### Goal
Resolve all cases where our table and OpenKore disagree on whether a packet is
fixed-length vs. variable-length, with GCC as the tie-breaker.

### Investigation Needed (before implementing)
1. Extract the full both-nonzero mismatch list for pv=20180621 and pv=20200401
2. For each mismatch: run GCC to get the struct definition and determine:
   - Does the struct contain a flex array member? → variable (-1)
   - Is it a fixed-size struct? → fixed (TotalSize)
3. Where OpenKore says fixed and we say variable: verify GCC agrees with OpenKore before
   changing ours
4. Where we say fixed and OpenKore says variable: for actor packets (0x09FF, 0x09FD etc.)
   this is expected — our per-PACKETVER fixed size IS correct. Document these explicitly.

### Acceptance Criteria
- [ ] All "we say fixed, GCC says variable" cases fixed to -1
- [ ] All "we say variable, GCC says fixed" cases (excluding actor packets) fixed
- [ ] Actor entity packets (0x09FF, 0x09FD, 0x09FE, 0x09DB, 0x0078, 0x0079, 0x007B)
  documented as intentionally fixed-per-PACKETVER (different from OpenKore's -1)
- [ ] Worklog entry

---

## US-05-5 — Regression Harness and Final Validation

### Goal
Add a persistent regression test that catches future length regressions before they
reach production.

### Deliverables
1. `validation/length_regression_test.go` (or `pkg/session/lengths_regression_test.go`):
   - At pv=20180621: compare our simulated table against pinned OpenKore 2018_06_21a
     recvpackets.txt — assert mismatch count < threshold
   - At pv=20200401: same for 2020_04_01b
   - Document expected differences (actor packets, deprecated IDs, C→S stubs)
2. Updated `validation/db_validate.sh` to run the length regression test
3. Final comparison run showing improvement:
   - Both-nonzero mismatches: target < 10 per packetver
   - Missing IDs: target < 50 per packetver

### Acceptance Criteria
- [ ] Regression test added and passing
- [ ] Both-nonzero mismatches < 10 at pv=20180621
- [ ] Both-nonzero mismatches < 10 at pv=20200401
- [ ] Missing count < 50 at pv=20180621
- [ ] `go build ./...` clean
- [ ] `go test ./...` clean
- [ ] Worklog entry `docs/WORKLOG/NNNN_2026-03-11_epic05_lengths_audit.md`

---

## Investigation Artifacts

### Comparison Script

The following Python script reproduces the full comparison. Run it from the repo root:

```bash
python3 docs/BACKLOG/EPIC-05/compare_lengths.py \
    --go pkg/session/lengths_map.go \
    --openkore ~/personal/openkore/tables/kRO/RagexeRE_2018_06_21a/recvpackets.txt \
    --pv 20180621
```

(Script to be added as `docs/BACKLOG/EPIC-05/compare_lengths.py`)

### Key GCC Commands

```bash
# Verify 0x0071 at any packetver:
g++ -E -P -DPACKETVER=20180621 -DPACKETVER_MAIN_NUM=20180621 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/clif_packetdb.hpp 2>/dev/null | grep '0x0071'
# Expected: packetdb_addpacket(0x0071,28,nullptr,0)

# Verify PACKET_ZC_BLACKSMITH_RANK size:
g++ -E -P -DPACKETVER=20180621 -DPACKETVER_MAIN_NUM=20180621 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/packets_hpp_stub.h \
    ~/personal/rathena/src/map/packets.hpp 2>/dev/null \
    | grep -A 10 'struct PACKET_ZC_BLACKSMITH_RANK'
# Expected: struct contains RANKLIST (280 bytes) + int16 = 282 total

# Check packet_idle_unit at 20170315 (the 156-byte breakpoint):
g++ -E -P -DPACKETVER=20170315 -DPACKETVER_MAIN_NUM=20170315 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    ~/personal/rathena/src/map/packets_struct.hpp 2>/dev/null \
    | grep -c 'packet_idle_unit' 
```

### Relevant Files

| File | Relevance |
|---|---|
| `pkg/session/lengths_map.go` | Generated length table — the artifact being fixed |
| `internal/codegen/main.go` | Codegen entrypoint — where `mapBreakpoints` is assembled |
| `internal/codegen/preprocess/parser.go` | Struct parser — where `TotalSize` is computed |
| `~/personal/rathena/src/map/clif_packetdb.hpp` | C→S packet registrations (Part 1 source) |
| `~/personal/rathena/src/map/packets.hpp` | Modern S→C structs (Part 2 source) |
| `~/personal/rathena/src/map/packets_struct.hpp` | Legacy S→C structs (Part 3/4 source) |
| `~/personal/openkore/tables/kRO/RagexeRE_2018_06_21a/recvpackets.txt` | Reference at pv=20180621 |
| `~/personal/openkore/tables/kRO/RagexeRE_2020_04_01b/recvpackets.txt` | Reference at pv=20200401 |

---

## Exit Criteria for EPIC-05

1. Both-nonzero mismatches < 10 at pv=20180621 (was 87)
2. Both-nonzero mismatches < 10 at pv=20200401 (was 103)
3. Missing packet IDs < 50 at pv=20180621 (was 338)
4. `0x0071` = 28, `0x0092` = 28 at all packetvers (GCC-verified)
5. `0x0219`, `0x021A`, `0x0226`, `0x0238` = 282 (GCC-verified)
6. Regression test passing and checking in CI
7. `go build ./...` clean
8. `go test ./...` clean
9. Worklog written
