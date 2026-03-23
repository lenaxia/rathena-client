# 0077 — Fix: ActionBattleChat, ActionPartyChat, ActionSetWhisperState missing

**Date**: 2026-03-23
**Status**: COMPLETE
**Scope**: `pkg/session/actions.go`, `pkg/encode/register.go`, `semantics/mappings.yaml`,
           `internal/codegen/stubs/synthetic_structs.hpp`
**Severity**: BLOCKING — three send encoders existed but had no SemanticAction constants;
              callers could not use `session.Send` for battle chat, party chat, or ignore-state packets

---

## Problem

Systematic sweep after worklog 0076 (whisper gap) found three more encoder functions
in `pkg/encode/` with no DB action entry, no `ActionXxx` constant, and no
`RegisterSendEncoder` call — the exact same gap class as `whisper`:

| Encoder | Packet | Status |
|---|---|---|
| `EncodeBattleChat` | `CZ_BATTLEFIELD_CHAT` `0x02DB` | No DB entry, no constant, not registered |
| `EncodePartyChat` | `CZ_PARTY_MESSAGE` `0x0108` | No DB entry, no constant, not registered |
| `EncodeSetWhisperState` | `CZ_SETTING_WHISPER_PC` `0x00CF` | No DB entry, no constant, not registered |

All three are hand-written encoders (variable-length or fixed-size structs not in rAthena
C headers). Without a DB entry, codegen never emits the `ActionXxx` constant in
`actions.go` or the `RegisterSendEncoder` call in `register.go`.

## Root Cause

Same as worklog 0076: `actions.go` and `register.go` are fully generated from the
semantics DB. The three actions had no DB entries at all.

### Notes on `set_whisper_state` (0x00CF vs 0x00D0)

There are two distinct whisper-ignore packets in rAthena:
- `0x00CF` — `parseable_packet(0x00cf, 27, clif_parse_PMIgnore, 2, 26)` — ignore a
  specific nick by name, 27 bytes, no named C struct. This is what `EncodeSetWhisperState`
  encodes.
- `0x00D0` — `PACKET_CZ_SETTING_WHISPER_STATE` (in `packets.hpp:2062`) — 3-byte bulk
  state setter. A different packet with a real C struct.

The encoder correctly uses `0x00CF`. The DB entry and stub use `SYNTH_CZ_SETTING_WHISPER_PC`
to distinguish from `PACKET_CZ_SETTING_WHISPER_STATE`.

## Fix

### 1. Added SYNTH stubs to `synthetic_structs.hpp`

- `SYNTH_CZ_BATTLEFIELD_CHAT` — 2-byte stub (variable-length, same pattern as `SYNTH_CZ_WISPER`)
- `SYNTH_CZ_PARTY_MESSAGE` — 2-byte stub (variable-length)
- `SYNTH_CZ_SETTING_WHISPER_PC` — 27-byte stub (`int16 + char[24] + int8_t`) matching the
  fixed-size wire format

### 2. Added 3 actions to semantics DB via MCP

```
semantics_create_action("battle_chat", ...)  + add_implementation("0x02DB", SYNTH_CZ_BATTLEFIELD_CHAT)
semantics_create_action("party_chat", ...)   + add_implementation("0x0108", SYNTH_CZ_PARTY_MESSAGE)
semantics_create_action("set_whisper_state", ...) + add_implementation("0x00CF", SYNTH_CZ_SETTING_WHISPER_PC)
```

### 3. Ran codegen

Emitted:
- `ActionBattleChat SemanticAction = 21` — variable-length `return EncodeBattleChat(r, pv), nil`
- `ActionPartyChat SemanticAction = 188` — variable-length
- `ActionSetWhisperState SemanticAction = 225` — **fixed-size** `b := EncodeSetWhisperState(r, pv); return b[:], nil`
  (codegen correctly detects `[27]byte` return via `fixedReturnEncoders` AST scan)

## Packet Sources (rAthena verified)

| Packet | File | Line |
|---|---|---|
| `CZ_BATTLEFIELD_CHAT` 0x02DB | `clif_packetdb.hpp` | 921 |
| `CZ_PARTY_MESSAGE` 0x0108 | `clif_packetdb.hpp` | 108 |
| `CZ_SETTING_WHISPER_PC` 0x00CF | `clif_packetdb.hpp` | 78 |

None are in `clif_shuffle.hpp` — all are stable IDs.

## Test Results

```
--- PASS: TestEncodeBattleChat_WireFormat
--- PASS: TestActionBattleChat_Registered
--- PASS: TestEncodePartyChat_WireFormat
--- PASS: TestActionPartyChat_Registered
--- PASS: TestEncodeSetWhisperState_WireFormat
--- PASS: TestActionSetWhisperState_Registered

BenchmarkEncodeBattleChat:      25 ns/op, 0 B/op, 0 allocs/op
BenchmarkEncodePartyChat:       33 ns/op, 0 B/op, 0 allocs/op
BenchmarkEncodeSetWhisperState:  0.2 ns/op, 0 B/op, 0 allocs/op ✓
```

`go test ./...` — all packages pass.
