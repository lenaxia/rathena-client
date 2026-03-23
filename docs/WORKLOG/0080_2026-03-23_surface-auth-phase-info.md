# 0080 — Surface all auth-phase info; FailInfo/NotifyInfo; SlotInfo; MapDomain; Users; JobLevel/Speed

**Date:** 2026-03-23

## Summary

Six targeted additions surfacing previously-discarded auth packet data through the FSM.
All verified against rAthena `src/common/packets.hpp` and OpenKore `src/Network/Receive.pm`.

## Changes

### 1. `CharacterInfoEntry` — `JobLevel` + `Speed`
- `JobLevel int32` — rAthena: `joblevel`; OpenKore: `lv_job`. Offset 16 (pv<20170830), 24 (pv>=20170830). Needed to compute skill point budget before map entry.
- `Speed int16` — rAthena: `speed`; OpenKore: `walkspeed`. Offset 54 / 62 / 82. Needed for movement timing. Both fields present in all 10 PACKETVER breakpoints, no width changes.

### 2. `IdentityInfo.MapDomain string`
- rAthena: `domain[128]` in `PACKET_HC_NOTIFY_ZONESVR` (0x0AC5, pv >= 20170315).
- OpenKore: XKoreProxy.pm `mapUrl` field — format `"hostname"` or `"hostname:port"`.
- When non-empty, callers should prefer this over `MapIP` for the map server address.
- Empty for 0x0081 (pv < 20170315) — no domain field in that struct.

### 3. `CharServerInfo.Users uint16`
- rAthena: `PACKET_AC_ACCEPT_LOGIN_sub.users` (uint16, offset 26 in sub-entry).
- OpenKore: `users` field in all `parse_account_server_info` variants.
- Present in both 32-byte (pv<20170315) and 160-byte (pv>=20170315) sub-entries.
- Enables intelligent server selection based on population.

### 4. `SlotInfo` + `OnSlotInfo` callback
- New struct: `SlotInfo{Normal, Premium, Billing, Producible, Total uint8}`
- rAthena: `PACKET_HC_ACCEPT_ENTER2` (0x082D, pv >= 20130000), `packets.hpp:508–517`.
- OpenKore: `$charSvrSet{normal_slot}`, `{billing_slot}`, etc. in `received_characters_slots_info`.
- Previously the 0x082D handler was a no-op `// Stay`. Now it parses and delivers slot quotas.
- Non-breaking: new optional callback, callers ignore if not needed.

### 5 & 6. `FailInfo` / `NotifyInfo` — breaking callback signature changes
- `OnFailed(func(FailInfo))` replaces `func(error)`.
  - `FailInfo{Phase AuthPhase, Err error}` — phase identifies login/char/map failure origin.
  - Internal `phaseError` wrapper in `connect()` carries phase through the error chain.
- `OnServerNotify(func(NotifyInfo))` replaces `func(uint8)`.
  - `NotifyInfo{Phase AuthPhase, Code uint8}` — all three SC_NOTIFY_BAN sites tagged.
- `AuthPhase` type with `PhaseLogin`, `PhaseChar`, `PhaseMap` constants + `String()`.

## Verification

All fields verified against:
- rAthena `src/common/packets.hpp` (GCC-preprocessed at pv=20180307 and pv=20030000)
- OpenKore `src/Network/Receive.pm` and `src/Network/XKoreProxy.pm`
- DUMP17 real capture for `joblevel` / `speed` golden values

## Tests Added

- `TestDecodeCharacterInfoEntry_JobLevelAndSpeed` — 5 breakpoints × sentinel values
- `TestDecodeCharacterInfoEntry_B8_RealCapture_JobLevelSpeed` — 4 real captured chars
- `TestConnect_OnIdentity_MapDomain` — domain_present + domain_empty subtests
- `TestConnect_OnSlotInfo` — slot counts round-trip via 0x082D
- `TestConnect_OnCharServerList` — extended with `Users` assertions
- `TestConnect_LoginRefused` — extended with `FailInfo.Phase == PhaseLogin`
- `TestConnect_CharServerRefused` — extended with `FailInfo.Phase == PhaseChar`
- `TestConnect_MapRefused` — extended with `FailInfo.Phase == PhaseMap`
- `TestConnect_ServerNotifyBan_Login` — extended with `NotifyInfo.Phase == PhaseLogin`

## Files Changed

- `pkg/events/character_info_entry.go` — +JobLevel, +Speed
- `pkg/decode/character_info.go` — decode JobLevel/Speed at correct offsets per breakpoint
- `pkg/decode/character_info_test.go` — 2 new tests
- `pkg/session/fsm.go` — IdentityInfo.MapDomain, CharServerInfo.Users, SlotInfo, AuthPhase, FailInfo, NotifyInfo, phaseError, onSlotInfo field, OnSlotInfo/OnFailed/OnServerNotify signatures, 3× onServerNotify call sites
- `pkg/session/fsm_parse.go` — read users from login accept sub-entry
- `pkg/session/fsm_test.go` — 9 new/extended tests, updated all callback signatures
- `pkg/session/fsm_replay_test.go` — updated OnFailed signature
