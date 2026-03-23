# 0079 — CHARACTER_INFO decoder + OnCharList parsed API

**Date:** 2026-03-23

## Summary

Implemented hand-written `CHARACTER_INFO` decoder (`pkg/decode/character_info.go`) and
`CharacterInfoEntry` event struct (`pkg/events/character_info_entry.go`), replacing the
`[]byte` stubs that codegen emitted for the nested flex-array field in
`HC_ACCEPT_ENTER` (0x006B), `HC_ACK_CHARINFO_PER_PAGE` (0x099D/0x0B72), and
`HC_ACCEPT_ENTER2` (0x082D).

Changed `ConnectionFSM.OnCharList` signature from `func([]byte) uint8` to
`func([]events.CharacterInfoEntry) uint8` — breaking change, no backwards-compat layer.

Added `MapIP uint32` and `MapPort uint16` to `IdentityInfo` — previously parsed and
silently discarded inside the FSM.

## GCC Verification

```bash
for PV in 20030000 20100720 20100803 20110111 20110928 20111025 20141016 20141022 20170830 20220330; do
  g++ -E -P -DPACKETVER=$PV -DPACKETVER_MAIN_NUM=$PV -DPACKETVER_RE_NUM=0 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/common_hpp_stub.h \
    ~/personal/rathena/src/common/packets.hpp 2>/dev/null \
    | grep -A 50 "^struct CHARACTER_INFO"
done
```

Verified 10 PACKETVER snapshots. Wire sizes confirmed:

| Breakpoint | pv | Bytes |
|---|---|---|
| B0 | < 20100720 | 112 |
| B1 | >= 20100720 | 128 |
| B2 | >= 20100803 | 132 |
| B3 | >= 20110111 | 136 |
| B4 | >= 20110928 | 140 |
| B5 | >= 20111025 | 144 |
| B6 | >= 20141016 | 145 |
| B7 | >= 20141022 | 147 |
| B8 | >= 20170830 | 155 |
| B9 | >= 20220330 MAIN | 175 |

## Real Capture Cross-Check

Validated B8 (pv=20200401) against DUMP17_login_4chars (4 characters):
- Almarc (GID=150001, job=0x0fdf, level=124, slot=0, map=prt_fild08.gat) ✓
- Chrno Crusade (GID=587, job=0, level=1, slot=1, map=prontera.gat) ✓
- Beyond Faith (GID=150303, job=0x0fd9, level=200, slot=2, map=prt_sewb2.gat) ✓
- Eclair (GID=150304, job=0x0fdd, level=200, slot=3, map=prt_in.gat) ✓

## Files Changed

- `pkg/events/character_info_entry.go` — new: `CharacterInfoEntry` struct
- `pkg/decode/character_info.go` — new: `decodeCharacterInfoEntry`, `decodeCharacterInfoList`, `DecodeCharacterInfoList` (exported), `charInfoSize`
- `pkg/decode/character_info_test.go` — new: 13 tests covering B0/B2/B5/B7/B8/B8-real/B9 breakpoints, list decode, edge cases
- `pkg/events/received_characters.go` — hand-written: `Characters []byte` → `[]CharacterInfoEntry`
- `pkg/events/received_characters_page.go` — hand-written: same
- `pkg/decode/received_characters.go` — hand-written: calls `decodeCharacterInfoList`
- `pkg/decode/received_characters_page.go` — hand-written: same
- `pkg/session/fsm.go` — `OnCharList` signature, `IdentityInfo` +MapIP/MapPort, removed `CharacterInfo` stub, added `decode`+`events` imports
- `pkg/session/fsm_test.go` — updated `TestConnect_OnCharList` to use parsed entries
- `pkg/session/fsm_replay_test.go` — updated `OnCharList` callback signature

## Test Results

```
ok  github.com/lenaxia/rathena-client/pkg/decode    (13 new tests, all pass)
ok  github.com/lenaxia/rathena-client/pkg/session   (all pass including updated OnCharList test)
ok  github.com/lenaxia/rathena-client/...           (all packages pass)
```

## Known Limitations

- `PACKETVER_RE_NUM >= 20211103` hp/sp widening not implemented (goKore targets MAIN only).
  See `docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`.
- Codegen gap (nested flex-array of struct → `[]byte`) not fixed — tracked as tech debt.
  `NormalItemEntry` and `EquipItemEntry` have the same hand-written pattern.
- `CharacterInfo` stub in `fsm.go` left as a deprecated comment (no type body); callers
  use `events.CharacterInfoEntry` directly.
