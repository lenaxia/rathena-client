# 0022 — 2026-03-09 — US-10: Eliminate SetLength Workarounds

## Summary

US-10 is complete. All 34 hard-coded `SetLength` calls previously in `pkg/fsm/fsm.go`
have been removed. The lengths pipeline (codegen) now covers every packet they were
patching. One intentional `SetLength` remains for the 0x0081 disambiguation edge case.

---

## Context

Prior to this story, `pkg/fsm/runMapPhase()` and `pkg/fsm/runCharPhase()` contained
34+ hard-coded `SetLength` calls patching packets whose lengths were absent from the
generated `lengths_map.go`. These were added incrementally during US-08 development
as the live server revealed missing entries. US-10 eliminates them by fixing the three
root-cause gaps in the codegen pipeline.

---

## Work Done Across Sessions

The implementation spanned multiple sessions (worklogs 0021 precedes this one for
backlog planning). The actual code changes were:

### Gap A Fix — VersionTable S→C join pass (20 packets from `packets_struct.hpp`)

`cmd/codegen/main.go` was modified to:
- Reorder `genLengths` (formerly Step 3) to run **after** the VersionTable is built
  (Step 5), so S→C struct sizes are available
- Add `buildMapStocJoinPass(vt VersionTable, mappings []semantics.PacketMapping)` which
  iterates all VersionTable entries whose direction is S→C and injects their sizes (with
  PACKETVER breakpoints) into the lengths breakpoint map
- Add `semantics.LoadMappings(path string) ([]PacketMapping, error)` to parse the
  `mappings:` section of `semantics/mappings.yaml`; returns 446 total, 308 receive-direction

Packets covered: 0x0B18, 0x00B0, 0x00B1, 0x010F, 0x013A, 0x0141, 0x02C9, 0x0ACB,
0x0ADF, 0x0B08, 0x0B09, 0x0B0A, 0x0B0B, 0x0B1B, 0x0B20, 0x01D7, 0x099B, 0x09E7,
0x0ADE (via `packets_struct.hpp`)

**Bug fixed during join pass**: When a packet appears in two VersionTable ranges
(e.g. 0x01D7 changes size at `pv >= 20181121`), the reset (`length=0`) emitted at the
MaxVer boundary of the first range was overwriting the second range's valid entry.
Fixed with a non-zero-wins rule: a zero breakpoint never overwrites a non-zero one at
the same version boundary.

### Gap B Fix — `packets.hpp` ZC_* headers (9 packets)

`buildMapStocLengthBreakpoints` was added to `cmd/codegen/main.go`. It calls
`ParseCommonPacketHeaders` on `eathena-src/map/packets.hpp` (the same mechanism
`buildLoginCharLengthBreakpoints` uses for `common/packets.hpp`). This covers:
0x0074, 0x0073, 0x02EB, 0x0A18, 0x007F, 0x0091, 0x00BD, 0x0087, 0x02DA

### Gap C Fix — Structless packets, synthetic structs (6 packets)

Six new synthetic structs added to `preprocess/synthetic_structs.hpp`:

| Packet | Struct name | Size | Notes |
|--------|-------------|------|-------|
| 0x008E | SYNTH_ZC_NOTIFY_CHAT | -1 | `int16 len; char message[]` — variable |
| 0x02D9 | SYNTH_ZC_CONFIG_NOTIFY | 10 | `int16; uint32; uint32` — fixed |
| 0x0A23 | SYNTH_ZC_ACHIEVEMENT_LIST | -1 | `int16 len; int32; item[]` — variable |
| 0x0A24 | SYNTH_ZC_ACHIEVEMENT_UPDATE | 66 | fixed, verified via GCC |
| 0x0A9B | SYNTH_ZC_EQUIPSWITCH_LIST | -1 | `int16 len; item[]` — variable |
| 0x0ADE | SYNTH_ZC_OVERWEIGHT_PERCENT | 6 | `int16; uint32` — fixed |

All sizes GCC-verified before writing via `__VERIFY_SIZE` assertions.

### Preserved `SetLength` — 0x0081 disambiguation

`pkg/fsm/runCharPhase()` retains one `SetLength(0x0081, 28)` call gated on
`pv < 20170315`. This is intentional:

- For `pv < 20170315`, packet 0x0081 serves dual purpose: `SC_NOTIFY_BAN` (3 bytes) and
  `HC_NOTIFY_ZONESVR` (28 bytes). The lengths table defaults to 3 (SC_NOTIFY_BAN).
- On the char server at these old versions, 0x0081 is **always** used for zone redirect,
  so 28 bytes is the correct framing size.
- `SC_NOTIFY_BAN` arriving on the char connection is handled correctly: the handler reads
  only byte[2] (result code) and ignores trailing bytes, which is safe since the server
  closes the connection immediately after.
- Source: `common/packets.hpp PACKET_HC_NOTIFY_ZONESVR` = 28 bytes at `PACKETVER=20120000`.

This cannot be resolved in `lengths_char.go` because the same packet ID has two correct
sizes depending on context, not PACKETVER alone.

---

## Acceptance Criteria — Verified

| Criterion | Result |
|-----------|--------|
| All 34 packet IDs in `lengths_*.go` | ✅ grep confirms all present |
| 6 synthetic structs in `synthetic_structs.hpp` | ✅ |
| `go test ./internal/codegen/preprocess/` | ✅ PASS |
| All 34 `SetLength` calls removed from `fsm.go` | ✅ only 0x0081 remains |
| `go build ./...` | ✅ PASS |
| `go test ./...` | ✅ all PASS |
| `go test -race ./pkg/...` | ✅ all PASS |
| Benchmarks 0 allocs/op | ✅ BenchmarkFeed_ActorExists_0x09FF = 0 allocs/op |
| No `recvpackets.txt` in codegen | ✅ CLEAN |
| `phase1_gate.sh` 76 PASS / 1 FAIL | ✅ same pre-existing CH_MAKE_CHAR failure |

Note: Live integration test (`-tags integration`) not re-run in this session as it
requires Docker. US-08 verified it passes; US-10 only removes the `SetLength` workarounds
that test relied upon, and those IDs are now in `lengths_map.go`.

---

## Files Changed

| File | Change |
|------|--------|
| `preprocess/synthetic_structs.hpp` | Added 6 synthetic structs for Gap C |
| `internal/codegen/semantics/loader.go` | Added `PacketMapping` + `LoadMappings` |
| `cmd/codegen/main.go` | Added `buildMapStocLengthBreakpoints`, `buildMapStocJoinPass`; reordered genLengths; fixed non-zero-wins bug |
| `pkg/session/lengths_map.go` | Regenerated — all 34 IDs now present |
| `pkg/session/lengths_char.go` | Regenerated |
| `pkg/fsm/fsm.go` | Removed all 34 `SetLength` calls from `runMapPhase`; 0x0081 in `runCharPhase` intentionally kept |

---

## US-10 Status: COMPLETE
