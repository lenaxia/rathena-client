# 0021 — 2026-03-09 — US-10 Backlog Story + EPIC-01 Story Map Update

## Summary

Wrote US-10 backlog story and corrected the EPIC-01 story map to reflect the
dependency of US-09 on US-10.

---

## Work Done

### 1. Audited the 34 SetLength calls in `pkg/fsm/fsm.go`

Mapped every `SetLength` call site to its rAthena struct source file:

| Source | Count | Packet IDs |
|--------|-------|------------|
| `packets_struct.hpp` | 20 | 0x0B18, 0x00B0, 0x00B1, 0x010F, 0x013A, 0x0141, 0x02C9, 0x0ACB, 0x0ADF, 0x0B08, 0x0B09, 0x0B0A, 0x0B0B, 0x0B1B, 0x0B20, 0x01D7, 0x099B, 0x09E7, 0x0B09, 0x0B0A |
| `packets.hpp` | 9 | 0x0074, 0x0073, 0x02EB, 0x0A18, 0x007F, 0x0091, 0x00BD, 0x0087, 0x02DA |
| `common/packets.hpp` | 1 | 0x0081 |
| no struct (clif_packetdb.hpp only) | 7 | 0x0283\*, 0x008E, 0x02D9, 0x0A24, 0x0ADE, 0x0A9B, 0x0A23 |

\* 0x0283 already has `SYNTH_ZC_AID` in `synthetic_structs.hpp`.

### 2. Identified three root-cause gaps

**Gap A** — VersionTable S→C sizes not joined into lengths generation  
`genLengths` (Step 3 in `main.go`) runs before the VersionTable is built (Step 5).
Struct sizes for `packets_struct.hpp` entries are already computed but never fed to
`GenerateLengthsFile`. Fix: reorder steps, add S→C join pass after VersionTable build.

**Gap B** — `packets.hpp` ZC_* structs not wired into map lengths  
`buildLoginCharLengthBreakpoints` already processes `common/packets.hpp` via
`SourceCommonPackets` + `ParseCommonPacketHeaders`. The identical mechanism exists for
`SourcePackets` (map `packets.hpp`) but is not called for map lengths generation.

**Gap C** — 7 structless packets (raw WFIFOW in clif.cpp, no rAthena struct)  
Pipeline already handles this class via `synthetic_structs.hpp` +
`InjectSyntheticStructs`. `SYNTH_ZC_AID` (0x0283) is the existing proof of concept.
The remaining 6 need synthetic struct entries added to that file.

### 3. Wrote `docs/BACKLOG/US-10_eliminate-setlength-workarounds.md`

No-fallback design: every length derived from GCC struct size or a hand-authored
synthetic struct verified against `clif_packetdb.hpp` + `clif.cpp`. No
`recvpackets.txt`. Key points:

- Step 1: Add 6 synthetic structs to `synthetic_structs.hpp` for the structless packets
- Step 2: Add `buildMapStocLengthBreakpoints` wiring `packets.hpp` into map lengths
- Step 3: Reorder codegen steps; add S→C join pass using VersionTable + SemanticDB
- Step 4: Delete all 34 `SetLength` calls from `pkg/fsm/fsm.go`

### 4. Updated `docs/BACKLOG/EPIC-01_integration_tests.md` story map

Changed the story map from "US-08 and US-09 are independent and can be implemented
in parallel" to:

```
US-10 → US-08 (already passing; US-10 removes the 34 SetLength calls it relies on)
US-10 → US-09 (hard dependency — replay test must not reproduce SetLength workarounds)
```

Rationale: US-09 processes the full DUMP8_movement map burst which contains the same
34 packet IDs as the FSM workarounds. Without US-10, the replay test would need its
own copy of those SetLength calls, which is exactly what US-10 eliminates.

---

## Files Changed

| File | Change |
|------|--------|
| `docs/BACKLOG/US-10_eliminate-setlength-workarounds.md` | Created |
| `docs/BACKLOG/EPIC-01_integration_tests.md` | Story map updated (dependency added) |

---

## No Code Changes

This worklog records planning and backlog work only. No production or test code was
modified. The next implementation step is US-10.
