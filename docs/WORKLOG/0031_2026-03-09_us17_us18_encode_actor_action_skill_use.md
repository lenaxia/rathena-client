# 0031 — 2026-03-09 — US-17 EncodeActorAction + US-18 EncodeSkillUse

## Summary

Implemented `EncodeActorAction` (US-17) and `EncodeSkillUse` (US-18) from EPIC-03.
Both replace broken generated stubs with correct, GCC-verified wire encoders returning `[N]byte` for zero allocation.

## Pre-Implementation Gate

GCC-verified at PACKETVER=20200401 (provided by orchestrator):

**US-17 clif_parse_ActionRequest:**
```
packetdb_addpacket(0x085a, 7, clif_parse_ActionRequest, 2, 6, 0);
packetdb_addpacket(0x088e, 7, clif_parse_ActionRequest, 2, 6, 0);
```
Wire: byte[0-1]=0x085A LE, byte[2-5]=TargetGID uint32 LE (pos[0]=2), byte[6]=Action uint8 (pos[1]=6). Total: 7 bytes.

**US-18 clif_parse_UseSkillToId:**
```
packetdb_addpacket(0x0862, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
packetdb_addpacket(0x089b, 10, clif_parse_UseSkillToId, 2, 4, 6, 0);
```
Wire: byte[0-1]=0x0862 LE, byte[2-3]=Lv uint16 LE (pos[0]=2), byte[4-5]=SkillID uint16 LE (pos[1]=4), byte[6-9]=TargetID uint32 LE (pos[2]=6). Total: 10 bytes.

## Design Decisions

- Return type `[7]byte` / `[10]byte` (not `[]byte`) — matches README-LLM.md §8 and the existing `EncodeMoveTo` precedent (`[5]byte`). Prevents heap allocation on encode path.
- `packetver` parameter accepted but ignored — single packet ID is used for all versions at PACKETVER >= 20200401. Parameter kept for API consistency with all other encode functions.
- Used existing `leU32Put` / `leU16Put` helpers from `pkg/encode/helpers.go`.

## Files Changed

- `pkg/encode/actor_action.go` — replaced generated stub (empty switch) with `EncodeActorAction` returning `[7]byte`
- `pkg/encode/skill_use.go` — replaced generated stub (broken duplicate-condition switch, wrong 33-byte payload) with `EncodeSkillUse` returning `[10]byte`
- `pkg/encode/actor_action_test.go` — new; 7 tests + benchmark
- `pkg/encode/skill_use_test.go` — new; 7 tests + benchmark

## Test Results

```
go test ./pkg/encode/ -v -run "TestEncodeActorAction|TestEncodeSkillUse"

TestEncodeActorAction_PacketID     PASS
TestEncodeActorAction_Length       PASS
TestEncodeActorAction_TargetID     PASS
TestEncodeActorAction_TypeNormalAttack PASS
TestEncodeActorAction_TypeSitStand PASS
TestEncodeActorAction_TargetIDZero PASS
TestEncodeActorAction_PacketverIgnored PASS
TestEncodeSkillUse_PacketID        PASS
TestEncodeSkillUse_Length          PASS
TestEncodeSkillUse_Lv              PASS
TestEncodeSkillUse_SkillID         PASS
TestEncodeSkillUse_TargetID        PASS
TestEncodeSkillUse_AllZero         PASS
TestEncodeSkillUse_PacketverIgnored PASS
```

All 14 tests PASS.

## Benchmark Results

```
BenchmarkEncodeActorAction-14   1000000000   1.010 ns/op   0 B/op   0 allocs/op
BenchmarkEncodeSkillUse-14      1000000000   0.893 ns/op   0 B/op   0 allocs/op
```

**0 allocs/op** confirmed on both. Stack-allocated `[N]byte` return prevents any heap escape.

## Full Suite

```
go test ./...   — all PASS
go test -race ./pkg/encode/  — PASS
grep -r "^\s*go " pkg/  — zero production goroutines (test files only, expected)
```
