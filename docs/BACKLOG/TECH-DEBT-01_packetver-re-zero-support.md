# TECH-DEBT-01: PACKETVER_RE_NUM and PACKETVER_ZERO_NUM not supported by codegen

**Status**: Open  
**Identified**: 2026-03-11  
**Priority**: Medium (does not affect main kRO client; affects RE-window and Zero clients)

---

## Summary

The codegen preprocessor only defines `-DPACKETVER_MAIN_NUM=<N>`. It never defines
`-DPACKETVER_RE_NUM` or `-DPACKETVER_ZERO_NUM`. Any struct or packet header guarded
exclusively by these macros is invisible to the VersionTable and will never be
generated.

---

## Background

rAthena has three active client build flavors:

| Macro | Client | Active when |
|---|---|---|
| `PACKETVER_MAIN_NUM` | kRO main (Ragexe) | Default; set when `PACKETVER_RE` is NOT defined |
| `PACKETVER_RE_NUM` | kRO RE (RagexeRE) | `20151104 < PACKETVER < 20180704` OR `20200902 <= PACKETVER <= 20211118` |
| `PACKETVER_ZERO_NUM` | kRO Zero (Ragnarok Zero) | Separate server branch; never set by default |

These are defined in `src/config/packets.hpp`. `PACKETVER_MAIN_NUM` and
`PACKETVER_RE_NUM` are **mutually exclusive** — when one is set, the other is
undefined. `PACKETVER_ZERO_NUM` is always separately opt-in.

The vast majority of guards in `packets_struct.hpp` include both MAIN and RE
conditions on the same `#if` line, so they are picked up correctly when MAIN is set.

---

## Affected packets

### PACKETVER_RE_NUM only (no MAIN_NUM fallback) — 3 packets

These 3 packets have **different packet IDs and/or struct layouts** on the RE client
for the date range `20190807 <= RE_NUM`:

| Packet | RE ID | Main ID | Notes |
|---|---|---|---|
| `ZC_ADD_SKILL` | `0x0B31` | `0x0111` | `SKILLDATA` drops `name[24]`, adds `level2` |
| `ZC_SKILLINFO_LIST` | `0x0B32` | `0x010F` | Same `SKILLDATA` layout change |
| `ZC_SKILLINFO_UPDATE2` | `0x0B33` | `0x07E1` | Same `SKILLDATA` layout change |

The `SKILLDATA` nested struct also differs:
- **RE >= 20190807**: `{ id, inf, level, sp, range2, upFlag, level2 }` (no `name` field)
- **MAIN (all versions)**: `{ id, inf, level, sp, range2, name[24], upFlag }`

A bot running against a RE-flavor server in the affected date range will receive
skill packets with the new IDs and layout. The current codegen will not decode them
(unknown packet ID → `ErrUnknownPacket` fault).

### PACKETVER_ZERO_NUM only — 3 packets

These packets exist exclusively in the Ragnarok Zero client:

| Packet | ID | Struct |
|---|---|---|
| `ZC_QUEST_DIALOG` | `0x0BA6` | `PACKET_ZC_QUEST_DIALOG` |
| `ZC_QUEST_DIALOG_MENU_LIST` | `0x0BA7` | `PACKET_ZC_QUEST_DIALOG_MENU_LIST` |
| `ZC_MONOLOG_DIALOG` | `0x0BA9` | `PACKET_ZC_MONOLOG_DIALOG` |

These are Zero-server-only and irrelevant for main/RE goKore use. They generate
decode SKIP stubs (`// SKIP QuestDialog_0x0BA6: struct not found in VersionTable`).

---

## Current state

- `internal/codegen/preprocess/runner.go:39` — only sets `-DPACKETVER_MAIN_NUM`
- The 3 RE packets decode to the **old IDs and old SKILLDATA layout** (missing `level2`,
  has the `name[24]` field). On a RE server in the affected date range this is wrong.
- The 3 Zero packets generate empty SKIP stubs in `pkg/decode/`.

---

## Resolution plan

### Option A — RE support (medium effort)

Add a second preprocess pass that sets `-DPACKETVER_RE_NUM=<N>` (and unsets
`-DPACKETVER_MAIN_NUM`) at each RE-window breakpoint. Merge the resulting
VersionTable entries alongside the MAIN entries under `packetver_range` guards.

In `internal/codegen/preprocess/runner.go`, add a `BuildFlavor` enum
(`FlavorMain`, `FlavorRE`, `FlavorZero`) and thread it through `Preprocess`.
In `internal/codegen/main.go`, run `buildVersionTable` three times (MAIN, RE, Zero)
and merge via `BuildVersionTableMultiFlavor`.

The SemanticDB will need RE-specific implementations for the 3 affected actions
(`add_skill`, `skill_info_list`, `skill_info_update`) with `packetver_min: 20190807`
and the RE packet IDs.

### Option B — Zero support (low effort, narrow scope)

Add the 3 Zero-only structs to `synthetic_structs.hpp` manually (their layouts are
known from `packets_struct.hpp`). Update the SemanticDB implementations to reference
the SYNTH_ names. Zero-only decode stubs will be generated.

This does not require any codegen architecture change. Suitable for Zero support only.

### Option C — Do nothing (current state)

Acceptable if goKore only targets main kRO servers. The RE skill packets affect only
servers running in the RE-window date ranges (`20151104–20180704` or
`20200902–20211118`). The Zero packets are irrelevant outside the Zero server.

---

## Files to change

| File | Change |
|---|---|
| `internal/codegen/preprocess/runner.go` | Add `BuildFlavor` param; pass RE/ZERO defines |
| `internal/codegen/main.go` | Run three build-flavor passes; merge VersionTables |
| `internal/codegen/stubs/synthetic_structs.hpp` | Add Zero-only structs (Option B) |
| `semantics/mappings.yaml` | Add RE-specific implementations for 3 skill actions |
| `pkg/decode/quest_dialog.go` etc. | Regenerated (currently SKIP stubs) |

---

## References

- `src/config/packets.hpp` — canonical definition of all three `PACKETVER_*_NUM` macros
- `src/map/packets_struct.hpp:4239` — the four RE-only `#if` guards
- `src/map/packets_struct.hpp:5217` — the three Zero-only struct definitions
- `internal/codegen/preprocess/runner.go:39` — where MAIN_NUM is currently injected
