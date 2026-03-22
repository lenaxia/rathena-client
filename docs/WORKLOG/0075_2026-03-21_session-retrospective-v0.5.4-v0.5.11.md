# 0075 — Session: v0.5.4–v0.5.11 Encoder Audit & Fixes — Retrospective & State

**Date**: 2026-03-21
**Status**: COMPLETE
**Scope**: `pkg/encode/`, `pkg/decode/`, `pkg/session/`, `internal/codegen/`, `semantics/`
**Versions shipped**: v0.5.4 → v0.5.11 (8 releases)

---

## Summary

Eight releases fixing the encoder/decoder layer that goKore depends on. This worklog
documents the complete state after the audit series so future sessions have a single
reference point.

---

## What Was Fixed (Chronological)

### v0.5.4 — `OnMapSessionCreated` (worklog 0068)

**Bug**: rAthena co-delivers `ZC_ACCEPT_ENTER` + inventory burst in one TCP segment.
`sessionCore.feed()` drained all frames before `feedUntil` returned and `OnReady` fired,
dropping every packet with no registered handler.

**Fix**: Added `ConnectionFSM.OnMapSessionCreated` — fires after FSM auth handlers are
registered, before `feedUntil`. Callers register semantic handlers here to capture
the inventory burst.

**Test**: `TestConnect_OnMapSessionCreated_HandlersFire` — co-delivers `ZC_AID` + `ZC_ACCEPT_ENTER`
in one write; asserts early handler fires (Part A), OnReady-only handler does not (Part B).

---

### v0.5.5 — `EncodeItemUse` wrong packet ID (worklog 0069)

**Bug**: `EncodeItemUse` hardcoded `0x00A7`. At `pv >= 20080910`, `0x00A7` → `clif_parse_SolveCharName`.
Every item use → disconnect.

**Fix**: Semantics DB split to two implementations via MCP: `0x00A7` for `pv < 20080910`,
`0x0439` for `pv >= 20080910`. Codegen regenerated.

---

### v0.5.6 — 16 missing receive-direction dispatch entries (worklog 0067 follow-up)

**Bug**: Semantics DB had 16 packet IDs added in v0.5.2 that were never committed to the DB —
every codegen run silently dropped them from `receive_dispatch.go`.

**Fix**: All 16 implementations added via semantics MCP:
- Actor middle-gen (pv 20091103–20131222): 0x07F7–0x090F for connected/exists/moved
- Dispatch-only: 0x0983, 0x099F, 0x09CA, 0x0295, 0x02D0, 0x084B, 0x0ADD

---

### v0.5.7 — `EncodePickupItem` wrong packet ID (worklog 0070/0071)

**Bug**: `EncodePickupItem` hardcoded `0x009F`. At `pv >= 20101124`, `0x009F` reassigned.
Every item pickup → disconnect.

**Fix**: Hand-written dispatcher following `move_to.go` pattern. 8 explicit pre-shuffle cases
+ `shuffledCtoSID(pv, 0x009F)`. Stable `0x0362` post-20180307. Cross-validated against
OpenKore: 57 direct matches, 0 mismatches.

---

### v0.5.8 — 5 shuffle-era encoder bugs + `EncodeEnterWorld` missing (worklog 0072/0073)

**Bugs** (all systematic, same root cause — generated `_ = packetver`):
- `drop_item`: `0x00A2` → correct `0x0363` post-20180307 via `shuffledCtoSID(pv, 0x00A2)`
- `look`: **triple bug** — wrong ID (`0x009B`), wrong size (`[4]byte` not `[5]byte`), `Dir` at
  `p[3]` instead of `p[4]`. All three fixed.
- `move_from_storage`: `0x00F5` → `0x0365`
- `move_to_storage`: `0x00F3` → `0x0364`
- `skill_use_location`: `0x0116` → `0x0366`
- `enter_world`: missing encoder entirely; goKore used raw `conn.Write`. `EncodeEnterWorld`
  added (hand-written, 2 bytes `[0x7D, 0x00]`, registered in `init()` via codegen).

---

### v0.5.9 — `EncodeFriendsAdd` shuffle-era bug + codegen lint rule (worklog 0074)

**Bug**: `EncodeFriendsAdd` hardcoded `0x0202`. `0x0202` IS in `clif_shuffle.hpp` and is
remapped per-week (e.g. `0x0962` at `pv=20130515`). Fixed with hand-written
`shuffledCtoSID(pv, 0x0202)` dispatcher.

**New**: `GenerateEncodeDirFilesWithShuffleCheck` lint rule — codegen now fails if any
generated encoder hardcodes a packet ID that appears in `clif_shuffle.hpp`. Would have
caught all 8 encoder ID bugs at generation time.

---

### v0.5.10 — Cleanup (worklog 0074 addendum)

- `character_move.go` promoted to hand-written (scope limitation comment survives codegen)
- `.gitignore` for root-level test binaries
- Confirmed `cz_party_join_req`/`friends_remove`/`friends_reply` are correct (no fix needed)

---

### v0.5.11 — Codegen lint allowlist + final DB corrections

- Codegen allowlist built from hand-written files before `cleanGeneratedDir` — eliminates
  false positives for `friends_add`, `homunculus_menu`, `master_login`
- `cz_party_join_req` DB bounded to `pv >= 20111102`
- `close_storage.go` promoted to hand-written with protocol-removal warning
- `friends_add` `0x0202` shuffle era cross-validated against OpenKore: 152 weekly blocks,
  0 mismatches

---

## Current State (post v0.5.11)

### Send encoders — complete

| Encoder | Status | Wire ID at pv=20200401 |
|---|---|---|
| `move_to` | ✅ hand-written | `0x035F` via `shuffledCtoSID(pv, 0x0085)` |
| `pickup_item` | ✅ hand-written | `0x0362` via `shuffledCtoSID(pv, 0x009F)` |
| `drop_item` | ✅ hand-written | `0x0363` via `shuffledCtoSID(pv, 0x00A2)` |
| `look` | ✅ hand-written | `0x0361` via `shuffledCtoSID(pv, 0x009B)` |
| `move_from_storage` | ✅ hand-written | `0x0365` via `shuffledCtoSID(pv, 0x00F5)` |
| `move_to_storage` | ✅ hand-written | `0x0364` via `shuffledCtoSID(pv, 0x00F3)` |
| `skill_use_location` | ✅ hand-written | `0x0366` via `shuffledCtoSID(pv, 0x0116)` |
| `item_use` | ✅ codegen (2 impls) | `0x0439` |
| `enter_world` | ✅ hand-written | `0x007D` (stable, never shuffled) |
| `friends_add` | ✅ hand-written | `0x0202` via `shuffledCtoSID(pv, 0x0202)` |
| `character_move` | ✅ hand-written | `0x035F` (pv >= 20101124 only; use `move_to` for full coverage) |
| `close_storage` | ⚠️ hand-written | `0x00F7` (pv <= 20050110 only; protocol removed) |
| `public_chat` | ⚠️ hand-written | `0x00F3` (pv >= 20040726) / `0x008C` (baseline) |
| `homunculus_*` | 🚫 out of scope | hardcoded, wrong for shuffle era |
| All other ~150 | ✅ generated | stable IDs, never shuffled |

### Receive dispatch — complete

- 366 packet IDs in dispatch table, 366 decoder functions defined. Zero gaps.
- All `actor_connected/exists/moved` middle-gen IDs (0x07F7–0x090F) dispatched.

### Codegen — hardened

- Shuffle overlap lint rule: codegen fails if a generated encoder hardcodes a shuffled ID
- Allowlist: hand-written files auto-detected; `homunculus_menu`/`master_login` explicit exceptions
- Semantics DB entries accurate for all actionable send-direction packets

---

## Known Remaining Gaps (low priority, no production impact at pv=20200401)

| Item | Status | Impact |
|---|---|---|
| `cz_party_join_req` encoder has no `pv < 20111102` guard | DB bounded but encoder ignores | No impact — no server runs pv < 20111102 |
| `public_chat` encoder doesn't respect `pv > 20080909` upper bound | DB bounded but encoder ignores | No impact — `public_chat` unused at modern pv |
| `homunculus_menu`/`homunculus_attack` wrong shuffle-era IDs | Out of scope | No impact — homunculus not supported |
| No integration smoke test | Gap | Would catch all encoder bugs mechanically |

---

## Recommended Next Step

**Live integration test against a real rAthena server.**

All the infrastructure bugs are fixed. The question is whether goKore actually works
end-to-end. A connection attempt will immediately surface any remaining packet-level issues
that the unit tests don't cover (timing, sequence, server-side validation of field values).

A minimal smoke test (`cmd/smoketest` or goKore integration test) that:
1. Connects to a test rAthena server
2. Logs in with test credentials
3. Selects a character
4. Enters the map
5. Asserts `OnReady` fires without errors
6. Sends `EnterWorld`, verifies inventory burst arrives in `OnMapSessionCreated`

...would be the permanent regression guard for the entire v0.5.x fix series.
