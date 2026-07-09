# Work Log 0087 — Catalog + fix the map-property packet family across all packetvers (issue #9)

**Date**: 2026-07-09
**Type**: Bug fix + packet catalog
**Scope**: `pkg/events/zc_notify_mapproperty2.go`, `pkg/decode/zc_notify_mapproperty2.go`,
          `pkg/decode/zc_notify_mapproperty2_test.go` (new),
          `pkg/session/receive_dispatch.go`, `pkg/session/lengths_map_overrides.go`,
          `pkg/session/mapproperty2_dispatch_test.go` (new)
**Severity**: BLOCKING (goKore) — at PACKETVER >= 20121010 the map-property packet
              (0x099B) arrived on the wire, was framed by the length table, but was
              never dispatched to any semantic action. goKore's `EventMapProperty`
              readiness signal never fired on modern packetvers, so the bot's first
              outbound action after map entry could be silently dropped by rAthena
              (the `sd->prev == nullptr` guard at clif.cpp:25784).
**Reference**: GitHub issue #9 — "zc_notify_mapproperty2: missing 0x099B dispatch
  at PACKETVER >= 20121010 (currently only 0x01D6 registered)"

**Maintainer directive**: "don't worry about code generation, we are not doing it
anymore, focus on cataloging the right packets in a way that works across relevant
packetvers, don't target just a single packetver." Accordingly the generated Go
files were hand-maintained (no codegen run) and the whole packet family was
cataloged across every relevant PACKETVER rather than patching a single version.

---

## Problem

`receiveDispatch[ActionZcNotifyMapproperty2]` listed only `0x01D6`. rAthena's
`clif_map_property` sends a DIFFERENT packet ID (`0x099B`, 8 bytes) for
PACKETVER >= 20121010, and `0x0199` (4 bytes) for older packetvers. Neither was
dispatched. The length table already framed `0x099B` (=8) for pv >= 20130320 and
`0x0199` (=4) unconditionally, so the packets were consumed but never decoded —
no semantic event fired.

## rAthena verification (source of truth — Rule 12)

Cloned rAthena to `/tmp/rathena` and read the authoritative source directly.

### clif_map_property — src/map/clif.cpp:6871-6903 (0x0199 / 0x099B)

The "property + flags" packet. Sent at map entry (clif.cpp:10836-10844), PvP/PK
zone changes (map.cpp:4604), and duel start/stop (duel.cpp:176,243,269). Built
with raw WBUFW/WBUFL macros — NO C struct:

```cpp
#if PACKETVER >= 20121010
    int16 cmd = 0x99b; unsigned char buf[8];
#else
    int16 cmd = 0x199; unsigned char buf[4];
#endif
    WBUFW(buf,0)=cmd;
    WBUFW(buf,2)=property;
#if PACKETVER >= 20121010
    WBUFL(buf,4) = ((...MF_PVP...)<<0)|((...gvg2...)<<1)|...<<10);  // bitfield
#endif
```

Layouts:
- pv <  20121010 → 0x0199, 4 bytes: cmd(W,0) + property(W,2)
- pv >= 20121010 → 0x099B, 8 bytes: cmd(W,0) + property(W,2) + flags(L,4)

`property` is `enum map_property` (clif.hpp:365-373): NOTHING=0, FREEPVPZONE=1,
EVENTPVPZONE=2, AGITZONE=3, PKSERVERZONE=4, PVPSERVERZONE=5, DENYSKILLZONE=6.

`flags` bitfield bits 0-10 (clif.cpp:6888-6898): PARTY, GUILD, SIEGE,
USE_SIMPLE_EFFECT, DISABLE_LOCKON, COUNT_PK, NO_PARTY_FORMATION, BATTLEFIELD,
DISABLE_COSTUME, USECART, SUNMOONSTAR_MIRACLE.

### clif_map_type — src/map/clif.cpp:6907-6914 (0x01D6)

A DISTINCT packet, not a packetver variant of the above. Sent for battlegrounds
(clif.cpp:11071). Uses C struct `PACKET_ZC_NOTIFY_MAPPROPERTY2`
(packets.hpp:966-969: `int16 packetType; int16 type;` = 4 bytes):

```cpp
PACKET_ZC_NOTIFY_MAPPROPERTY2 packet{};
packet.packetType = HEADER_ZC_NOTIFY_MAPPROPERTY2; // 0x1d6
packet.type = static_cast<decltype(packet.type)>(type);
```

`type` is `enum e_map_type` (clif.hpp:376-402): VILLAGE=0 ... UNUSED=29. No
PACKETVER guard — sent at all packetvers.

### packet_db registrations — src/map/clif_packetdb.hpp

- `packet(0x0199,4)` — line 185, UNCONDITIONAL (top of file).
- `packet(0x099b,8)` — line 1642, under `#if PACKETVER >= 20130320` (line 1600).

### Length-table guard discrepancy

rAthena sends 0x099B from `PACKETVER >= 20121010` (clif.cpp:6873) but only
registers its length under `PACKETVER >= 20130320` (clif_packetdb.hpp:1600). The
generated `lengths_map.go` copies that guard. For the window
`[20121010, 20130320)` the framer had `lengths[0x099B] = 0` → would treat 0x099B
as unknown. Fixed via `lengths_map_overrides.go`.

### Readiness-signal claim (issue motivation)

`map_addblock(sd)` (sets `sd->prev`) at clif.cpp:10813; `clif_map_property(...)`
emitted at clif.cpp:10836-10844, AFTER it. The silent-drop rule
`if( sd && sd->prev == nullptr && packet_db[cmd].func != clif_parse_LoadEndAck )`
is at clif.cpp:25784-25785. So arrival of 0x099B reliably implies `sd->prev` is
set — confirmed.

## Semantic note (cataloging decision)

0x01D6 (clif_map_type, `e_map_type`) and 0x0199/0x099B (clif_map_property,
`map_property`) are distinct rAthena packets with different enum spaces whose
ranges overlap (map_property 0-6; e_map_type 0-29). They are grouped under the
single action `ActionZcNotifyMapproperty2` exactly as the issue requests (so
goKore's existing `EventMapProperty` wiring works unchanged). The new
`events.MapProperty`/`events.MapType` typed constants and the event doc comment
document which enum `Type` carries for each variant. `Flags` (nonzero only for
0x099B) lets consumers distinguish the modern map-property packet.

## Changes (hand-maintained; codegen deprecated per maintainer)

| File | Change |
|---|---|
| `pkg/events/zc_notify_mapproperty2.go` | Added `Flags uint32` (non-breaking — no positional struct literals exist). Cataloged the full packet family: `MapProperty` enum (clif.hpp:365-373), `MapType` enum (clif.hpp:376-402), `MapPropertyFlag*` bitfield bits (clif.cpp:6888-6898). Comprehensive doc comment. |
| `pkg/decode/zc_notify_mapproperty2.go` | Added `ZcNotifyMapproperty2_0x099B` (8-byte: property@2, flags@4) and `ZcNotifyMapproperty2_0x0199` (4-byte: property@2, Flags=0). Pure value reads, 0 allocs by construction. |
| `pkg/session/receive_dispatch.go` | Added `{0x099B, ...}` and `{0x0199, ...}` to `ActionZcNotifyMapproperty2`. `RegisterSemanticHandler` now registers all three IDs simultaneously. |
| `pkg/session/lengths_map_overrides.go` | Added `t[0x099B]=8` for window `[20121010, 20130320)` to close the rAthena packet_db/codegen guard gap. |

## Packetver coverage (catalog across all relevant versions)

| PACKETVER range | clif_map_property sends | clif_map_type sends | Dispatched via |
|---|---|---|---|
| < 20121010 | 0x0199 (4B) | 0x01D6 (4B) | 0x0199 → decoder, 0x01D6 → decoder |
| [20121010, 20130320) | 0x099B (8B) | 0x01D6 (4B) | 0x099B (length override), 0x01D6 |
| >= 20130320 | 0x099B (8B) | 0x01D6 (4B) | 0x099B (generated length table), 0x01D6 |

## Tests (TDD)

`pkg/decode/zc_notify_mapproperty2_test.go`:
- `TestZcNotifyMapproperty2_0x099B_PropertyAndFlags` — property + flag bits.
- `TestZcNotifyMapproperty2_0x099B_FullBitfield` — all 11 bits (0-10) set.
- `TestZcNotifyMapproperty2_0x099B_ZeroValues` — all-zero frame.
- `TestZcNotifyMapproperty2_0x099B_DecodesAcrossPacketvers` — pv 20121010, 20130320, 20180307, 20200401, 20210101.
- `TestZcNotifyMapproperty2_0x0199_PropertyOnly` — legacy property, Flags=0.
- `TestZcNotifyMapproperty2_0x0199_DecodesAcrossLegacyPacketvers` — pv 20000000..20121009.
- `TestZcNotifyMapproperty2_0x01D6_FlagsStaysZero` — regression: Flags stays 0.
- `BenchmarkZcNotifyMapproperty2_0x099B` / `_0x0199` — 0 allocs/op target.

`pkg/session/mapproperty2_dispatch_test.go`:
- `TestMapproperty2_Dispatch_HasAllVariants` — receiveDispatch has 0x01D6, 0x0199, 0x099B.
- `TestMapproperty2_0x099B_FiresAt_20200401` — the bug regression (modern).
- `TestMapproperty2_0x099B_FiresAt_Boundary_20121010` — rAthena send boundary.
- `TestMapproperty2_0x099B_FiresInGapWindow_20130319` — the [20121010,20130320) override window.
- `TestMapproperty2_0x0199_FiresAt_LegacyPacketver` — pv 20100700 legacy.
- `TestMapproperty2_0x01D6_StillFires` — regression: clif_map_type still works.

## Environment limitations

This Action's shell is restricted to git/gh/ls/cat/mkdir/jq — could not run
`go build`, `go test`, `go test -race`, or benchmarks locally. The decode fns are
pure value reads returning a struct of only `int16` + `uint32`, so 0 allocs/op is
guaranteed by construction. CI on the PR runs the full suite (`go test ./...`,
`-race`, `-bench=. -benchmem`, `grep -r "^\s*go " pkg/`).

## Out of scope (follow-ups)

- `semantics/mappings.yaml` was NOT edited (Rule 9 — MCP only, unavailable here).
  When MCP is available, add the 0x0199/0x099B implementations to the
  `zc_notify_mapproperty2` action so the hand-edits are reconciled with the DB.
- `fsm_live_integration_test.go` calls `s.SetLength(...)` (exported) which does not
  exist (only unexported `setLength`). It is `//go:build integration` so excluded
  from `go test ./...`; not fixed here. Should be renamed to `setLength` in a
  follow-up.
- Historical drift: rAthena's old clif_map_property variant is 0x0199, but the
  semantic DB historically paired "zc_notify_mapproperty2" with 0x01D6
  (clif_map_type). This catalog documents the correct pairing.
