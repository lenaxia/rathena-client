# 0033 — 2026-03-09 — Codegen Encode Dispatcher Fixes

## Summary

Three defects found and fixed in the codegen encode pipeline:

1. **`generateEncodeDispatcher` always returned `[]byte`** — even for fixed-size packets, causing heap allocation via `make([]byte, N)`. Fixed to detect when all C→S implementations share the same fixed size and return `[N]byte` (stack-allocated, 0 allocs/op).

2. **Dispatcher included S→C struct implementations** — `skill_use` action had `PACKET_ZC_NOTIFY_SKILL` implementations (S→C) mixed with `SYNTH_CZ_USE_SKILL_TOID` (C→S). The old code emitted cases for all implementations, causing mixed sizes (10 vs 33 bytes) and forcing `[]byte` return. Fixed by filtering to C→S structs only before size computation.

3. **`writeFile` overwrote hand-written files** — codegen regeneration overwrote `pkg/encode/move_to.go` (hand-written, `// Manually implemented` header). Fixed by checking if existing file starts with the codegen header before overwriting; non-generated files are now preserved.

4. **`isSendStruct` filter was inconsistent** — send.go and encode.go had separate `strings.HasPrefix` checks for `PACKET_CZ_/CH_/CA_` that omitted `SYNTH_CZ_/CH_/CA_`. Extracted `isSendStruct()` shared helper to `gen/doc.go` and updated both generators. This caused `send.CharacterMove` to not be generated.

## Files Changed

### Codegen:
- `internal/codegen/gen/doc.go` — added shared `isSendStruct()` helper
- `internal/codegen/gen/encode.go` — fixed `generateEncodeDispatcher` to compute common size, use `[N]byte` for fixed-size, filter to C→S only; updated `GenerateEncodeDirFiles` to use `isSendStruct`
- `internal/codegen/gen/send.go` — updated `isSend` filter to use `isSendStruct` (adds SYNTH_ prefix support)
- `internal/codegen/main.go` — `writeFile` now preserves hand-written files (non-generated header)

### Generated (regenerated after fixes):
- `pkg/encode/actor_action.go` — now returns `[7]byte`
- `pkg/encode/skill_use.go` — now returns `[10]byte`, only `0x0862` case (ZC stubs removed)
- `pkg/send/character_move.go` — newly generated (`CharacterMove{Coords [3]byte}`)
- All other `pkg/encode/`, `pkg/send/`, `pkg/events/`, `pkg/decode/` files regenerated

### Test files:
- `pkg/encode/actor_action_test.go` — removed `bytes` import; `p1 != p2` works for `[7]byte`
- `pkg/encode/skill_use_test.go` — removed `bytes` import; `p1 != p2` works for `[10]byte`

## Benchmark Results

```
BenchmarkEncodeActorAction-14    1000000000    0.62 ns/op    0 B/op    0 allocs/op
BenchmarkEncodeMoveTo-14         95105377      12.62 ns/op   0 B/op    0 allocs/op
BenchmarkEncodeSkillUse-14       1000000000    0.83 ns/op    0 B/op    0 allocs/op
```

Previously `BenchmarkEncodeSkillUse` showed `16 B/op, 1 allocs/op`. Now **0 allocs/op** confirmed.

## Full Test Results

```
go test -count=1 ./...   → all PASS
go test -race ./...      → all PASS
grep -r "^\s*go " pkg/   → empty (zero goroutines in pkg/)
go test -bench=. -benchmem ./pkg/...  → 0 allocs/op on all benchmarks
```
