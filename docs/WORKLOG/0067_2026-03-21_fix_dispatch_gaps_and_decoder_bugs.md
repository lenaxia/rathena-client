# 0067 — Fix 7 dispatch gaps, OOB read, and wrong guild info layout

**Date:** 2026-03-21
**Type:** Bug Fixes (multiple)
**Scope:** pkg/decode, pkg/events, pkg/session, docs

---

## Summary

Systematic audit of receive_dispatch.go vs lengths_map.go revealed 7 categories of bugs.
All 7 fixed in this session.

---

## Bugs fixed

### BUG-1: 0x0295 and 0x02D0 missing from ActionInventoryItemsEquip dispatch
Decoders existed but were not registered. Equip inventory silently dropped for
pv 20071002–20120924.
**Fix:** Two entries added to receive_dispatch.go.

### BUG-2: AreaSpell_0x08C7 OOB read + wrong field widths + missing 0x099F/0x09CA
- Generated decoder used synthetic struct with `leU16` for a 1-byte `RadiusRange`
  field, read `IsVisible` from `data[19]` on a 19-byte packet (OOB), length table
  hardcoded 20 instead of 19.
- 0x099F and 0x09CA (skill_entryType pv 20121212–present) had no decoders/dispatch.

**Fix:** Rewrote area_spell.go; added lengths_map_overrides.go entry (`t[0x08C7]=19`
at pv 20110718–20121211); added AreaSpell_0x099F (22 bytes) and AreaSpell_0x09CA
(23 bytes); updated dispatch.

GCC verified:
- pv=20110718: packet_skill_entry = 19 bytes (job=uint8, no level)
- pv=20121212: = 22 bytes (job=int32)
- pv=20130731: = 23 bytes (+level)
Source: packets_struct.hpp:1434–1454, lines 121–127.

### BUG-3: ZcGuildInfo_0x0A84 else branch wrong layout (pv 20161019–20170314)
Else branch read masterName at offset 70 and manageLand at 94. PACKET_ZC_GUILD_INFO
for 0x0A84 has no masterName — manageLand is at offset 70 for all pv >= 20161019.
**Fix:** Else branch replaced with same layout as if branch.
Source: packets_struct.hpp:4830–4848.

### BUG-NEW-1: ActionItemAppeared missing 0x084B and 0x0ADD (CRITICAL)
dropflooritemType has three IDs; only 0x009E was dispatched. All floor items
invisible on every modern server (pv > 20130000).
**Fix:** New decoders ItemAppeared_0x084B (19 bytes) and ItemAppeared_0x0ADD
(22/24 bytes via pv branch). Original 0x009E decoder simplified to pre-20130000
only (dead branches removed).

GCC verified sizes:
- pv=20130320: 2+4+2+2+1+2+2+1+1+2 = 19 bytes
- pv=20180418: +showdropeffect(1)+dropeffectmode(2) = 22 bytes
- pv=20181121: ITID uint32 (+2) = 24 bytes (covered by existing override)

### BUG-NEW-2: ActionActorStatusActive missing 0x0983 (CRITICAL)
status_changeType = 0x0983 (pv >= 20120618) never dispatched. Status effects
invisible on all clients from 2012 onwards.
**Fix:** New ActorStatusActive_0x0983 decoder (29 bytes, adds Total+Left+val1-3).
events.ActorStatusActive extended with five new fields (zero for 0x0196 path).

GCC verified: packet_status_change at pv=20120618:
PacketType(2)+index(2)+AID(4)+state(1)+Total(4)+Left(4)+val1(4)+val2(4)+val3(4) = 29 bytes.

### BUG-NEW-3: 9 missing actor middle-generation packet IDs (HIGH)
Three consecutive generations of actor IDs (pv 20091103–20131222) were entirely
absent from dispatch for all three actor actions.

Missing IDs:
- ActionActorExists:    0x07F9 (pv 20091103-20101123), 0x0857 (20101124-20120220), 0x0915 (20120221-20131222)
- ActionActorConnected: 0x07F8, 0x0858, 0x090F  (same ranges)
- ActionActorMoved:     0x07F7, 0x0856, 0x0914  (same ranges)

**Fix:** 9 new decoder functions + 9 dispatch entries.

GCC verified struct layouts at three breakpoints:
- pv=20091103: PacketLength+objecttype+GID (no AID), no robe
  idle=63, spawn=62, walking=69 bytes
- pv=20101124: +robe(2)
  idle=65, spawn=64, walking=71 bytes
- pv=20120221: +maxHP(4)+HP(4)+isBoss(1)
  idle=74, spawn=73, walking=80 bytes

### BUG-6: README-LLM wrong claims about encode panics (doc fix)
- Claimed "EncodeActorAction has trailing panic" — FALSE (uses default: branch)
- Claimed "case packetver >= 0 is always true" — EncodeSkillUse uses >= 20030000 not >= 0
**Fix:** README corrected.

---

## Files changed

- `pkg/decode/area_spell.go` — rewritten
- `pkg/decode/item_appeared.go` — rewritten (0x084B + 0x0ADD added, 0x009E simplified)
- `pkg/decode/actor_status_active.go` — rewritten (0x0983 added)
- `pkg/decode/actor_exists.go` — 3 new decoders appended (0x07F9, 0x0857, 0x0915)
- `pkg/decode/actor_connected.go` — 3 new decoders appended (0x07F8, 0x0858, 0x090F)
- `pkg/decode/actor_moved.go` — 3 new decoders appended (0x07F7, 0x0856, 0x0914)
- `pkg/decode/zc_guild_info.go` — else branch corrected
- `pkg/decode/bug_fixes_0067_test.go` — new golden tests
- `pkg/events/actor_status_active.go` — 5 new timer fields
- `pkg/session/receive_dispatch.go` — 16 new/corrected entries
- `pkg/session/lengths_map_overrides.go` — 0x08C7 length correction
- `pkg/session/dispatch_fixes_0067_test.go` — new dispatch integration tests
- `CHANGELOG.md`, `README-LLM.md` — updated
- `docs/WORKLOG/0067_*.md` — this file

---

## Test results

```
go test ./...
ok  github.com/lenaxia/rathena-client/pkg/decode   0.014s
ok  github.com/lenaxia/rathena-client/pkg/session  0.280s
ok  (all other packages pass)
```

All new tests pass. No regressions.
