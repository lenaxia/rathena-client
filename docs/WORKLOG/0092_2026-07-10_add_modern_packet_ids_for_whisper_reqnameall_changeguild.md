# Work Log 0092 — semantics: add modern packet IDs for whisper/reqnameall/change_guild

**Date**: 2026-07-10
**Type**: Semantic-DB mapping corrections
**Scope**:
  - `semantics/mappings.yaml` — cap old packet IDs at their retirement pv, add modern packet IDs for three actions
  - Regenerated `pkg/decode/{private_message,zc_ack_reqnameall,zc_change_guild}.go`
  - Regenerated `pkg/session/{lengths_map,receive_dispatch}.go`

**Severity**: MODERATE — three actions were missing their modern packet IDs. The retired old IDs (0x0097, 0x0195, 0x01B4) are unreachable at modern packetvers per rAthena source, and the framer's length table correctly retires them — but without the modern replacement IDs (0x09DE, 0x0A30, 0x0B47) in the semantic DB, the framer has no way to route those packets to their semantic actions. Downstream: consumers registering handlers on `ActionPrivateMessage`, `ActionZcAckReqnameall`, or `ActionZcChangeGuild` never fired at modern pv.

**Reference**: uncovered while bumping goKore from rathena-client v0.8.0 → v0.9.1. Five goKore integration tests (`TestChat_Whisper_EventFires`, `TestAckReqnameall_Normal`, `TestAckReqnameall_NoPartyOrGuild`, `TestChangeGuild_Normal`, `TestChangeGuild_LeaveGuild`) were failing at pv=20200401 because the framer wasn't dispatching the packets.

---

## Problem

Three actions in `semantics/mappings.yaml` had only their OLDEST rAthena packet ID unbounded, with no corresponding modern-ID entries. When PR #16's codegen fidelity fixes started producing accurate length tables (retiring packet IDs at their real transition dates per `packets_struct.hpp`), these actions became unreachable at modern packetvers:

| Action              | Old ID  | Old-ID retires at   | Modern ID | Missing? |
|---------------------|---------|--------------------|-----------|----------|
| `private_message`   | 0x0097  | pv >= 20131204     | 0x09DE    | YES      |
| `zc_ack_reqnameall` | 0x0195  | pv >= 20150225     | 0x0A30    | YES      |
| `zc_change_guild`   | 0x01B4  | pv >= 20190703     | 0x0B1F, 0x0B47 | YES  |

At goKore's target `pv=20200401`, every one of these actions had `t[old_id] = 0` (retired) and no `t[modern_id]` registered — so wire packets for these actions were silently dropped by the framer as "unknown packet ID."

---

## Solution

Applied via `semantics-tool` (`update-implementation` + `add-implementation`) for each of the three actions. Each split follows the rAthena source pv guards exactly:

**`private_message`**:
- 0x0097 capped at `[20091104, 20131203]` (pre-senderGID variant, `packets_struct.hpp:5359-5367` and `:5368-5376`)
- 0x09DE added at `[20131204, ∞)` (senderGID + isAdmin int8 variant, `packets_struct.hpp:5348-5358`)

**`zc_ack_reqnameall`**:
- 0x0195 capped at `[0, 20150224]` (pre-title_id variant, `packets_struct.hpp:3575-3583`)
- 0x0A30 added at `[20150225, ∞)` (adds `title_id int32`, per `packets_struct.hpp:3564-3573`)

**`zc_change_guild`**:
- 0x01B4 capped at `[0, 20190702]` (pre-modern variant, `packets_struct.hpp:5824-5830`)
- 0x0B1F added at `[20190703, 20190806]` (14-byte modern layout: guild_id, emblem_id, AID, per `packets_struct.hpp:5816-5822`)
- 0x0B47 added at `[20190807, ∞)` (same 14-byte layout, packet ID change per RE 20190731 / MAIN 20190807, per `packets_struct.hpp:5806-5814`)

The three 0x0B1F/0x0B47 boundaries came from the source comment in `packets_struct.hpp`: "20190619 main exists in first versions, then removed" — indicating a narrow window where 0x0B1F was on the wire before rAthena settled on 0x0B47.

Verified via g++ preprocessor: at pv=20200401 the bindings resolve to 0x09DE (whisper), 0x0A30 (reqnameall), 0x0B47 (change_guild).

Regenerated `pkg/session/lengths_map.go` now correctly registers all three modern IDs at their transition pv. Regenerated `pkg/decode/*.go` produce single-branch decoders for each variant (since each impl now has a specific pv range).

---

## Downstream impact

goKore consumers registering handlers by ACTION continue to work — they just needed the framer to dispatch the modern wire IDs. The five failing goKore tests will be updated in a companion PR to send the modern wire IDs (0x09DE, 0x0A30, 0x0B47) matching what real rAthena servers emit at pv=20200401.

## Broader lengths_map.go changes

Beyond the three targeted actions, the codegen regeneration for this PR
also refreshed `pkg/session/lengths_map.go` — several dozen packet IDs
moved to slightly different pv boundaries. These changes stem from the
`buildMapStocJoinPass` fix in this same PR (respecting `PacketverMin`
when emitting length breakpoints) combined with the pre-existing
codegen fidelity fixes from PRs #16 and #17. Sample corrections:

- `0x022A/0x022B`: now registered at `pv >= 20050411` instead of
  unconditional (matches rAthena's explicit `#if PACKETVER >= 20050411`
  in `packets_struct.hpp`).
- `0x099B`: now at `pv >= 20121010` (matches rAthena's guard for
  ZC_NOTIFY_MAPPROPERTY2).
- `0x0906`: now at `pv >= 20111122`.
- `0x0B6F/0x0B72`: now at `pv >= 20201007`.

Every entry is a correctness improvement: the pre-fix codegen was
registering packet IDs at packetvers where rAthena does not send them.
Verified via g++ preprocessor cross-check on the top ~20 changed
entries. At goKore's target `pv=20200401`, none of these boundary
shifts change effective wire behavior (each shift either registers an
ID at an earlier pv, still active at 20200401, or leaves current-pv
behavior unchanged). The three targeted actions in this PR's summary
are the only ones with previously-unreachable modern IDs at
`pv=20200401`.

## Broader missing-modern-ID pattern

This is the same class of latent bug fixed for `skill_cast` (0x0B1A) and `add_exchange_item` (0x0B42) in PR #16, and for `cz_req_takeoff_equip_all` (0x0BF5) in PR #17. There are approximately 119 modern packet IDs bound at pv=20200401 per rAthena source; many of these actions in the semantic DB are likely missing their modern-ID entries (per the scan test that led to this fix). A systematic audit is deferred to a future PR — this one addresses only the three actions actively blocked by goKore's live-test suite.

## Rule 0 note

Worklog 0092; latest prior 0091.
