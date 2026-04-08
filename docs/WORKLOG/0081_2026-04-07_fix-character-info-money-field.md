# 0081 — Fix: CharacterInfoEntry.Money never populated by decoder

**Date**: 2026-04-07
**Issue**: https://github.com/lenaxia/rathena-client/issues/1

## Summary

`CharacterInfoEntry.Money` existed as a field in `pkg/events/character_info_entry.go`
but was never written by `decodeCharacterInfoEntry` in `pkg/decode/character_info.go`.
The `money` field in `CHARACTER_INFO` was silently skipped in both PACKETVER branches,
leaving `Money` always 0 at char-list time.

## Root Cause

`decodeCharacterInfoEntry` jumped directly from `exp` to `jobexp` in both branches,
skipping the `money` field between them:

- `pv >= 20170830`: exp at offset 4 (int64, 8 bytes) → money at **offset 12 (int32, 4 bytes)** → jobexp at offset 16 (int64, 8 bytes)
- `pv < 20170830`: exp at offset 4 (int32, 4 bytes) → money at **offset 8 (int32, 4 bytes)** → jobexp at offset 12 (int32, 4 bytes)

The decoder read the correct absolute byte offsets for all other fields (which are
computed from absolute positions, not accumulated shifts), so all fields after `money`
were correct — only `money` itself was missing.

## rAthena Verification

Source: `src/common/packets.hpp:31–42`

```cpp
struct CHARACTER_INFO{
    uint32 GID;
#if PACKETVER >= 20170830
    int64 exp;
#else
    int32 exp;
#endif
    int32 money;      // ← always int32, always present at this position
#if PACKETVER >= 20170830
    int64 jobexp;
#else
    int32 jobexp;
#endif
    ...
```

No GCC preprocessor run needed — `money` is unconditional and its offset follows
directly from exp's width:

- `PACKETVER >= 20170830`: GID(4) + exp(8) = offset 12, money is int32 at offset 12
- `PACKETVER < 20170830`:  GID(4) + exp(4) = offset 8, money is int32 at offset 8

## Real Capture Validation

From `DUMP17_login_4chars` (pv=20200401, B8 layout):

| Character    | money at offset 12 (int32 LE) |
|--------------|-------------------------------|
| Almarc       | 1,000,000                     |
| Chrno Crusade| 0                             |
| Beyond Faith | 0                             |
| Eclair       | 0                             |

The real capture test (`TestDecodeCharacterInfoEntry_B8_RealCapture`) now asserts
these values directly.

## Files Changed

- `pkg/decode/character_info.go` — added `dst.Money = leI32(b, 12)` (pv >= 20170830)
  and `dst.Money = leI32(b, 8)` (pv < 20170830)
- `pkg/decode/character_info_test.go` — added `money` parameter to all builder
  functions (`buildCharInfoB2`, `buildCharInfoB5`, `buildCharInfoB7`, `buildCharInfoB8`,
  `buildCharInfoB9`); added `Money` assertions to all tests; added `money` field with
  real values to the `B8_RealCapture` table test

## Test Results

```
go build ./...         PASS
go test ./...          PASS (all packages)
go test -race ./...    PASS (all packages)
```

All 15 `TestDecodeCharacterInfo*` tests pass including the real-capture test with
`Almarc.Money = 1000000`.

## Scope Note

The goKore `connector.go` change described in Issue #1 (firing `EventStatsUpdated`
with the initial zeny value) is a goKore concern, not rathena-client. This fix
delivers the corrected `Money` field in `CharacterInfoEntry` so goKore can use it.
