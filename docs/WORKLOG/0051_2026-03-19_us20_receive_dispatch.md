# 0051 — 2026-03-19 — US-20: Generate Receive Dispatch Table

## Task

US-20 from EPIC-07: Generate `pkg/session/receive_dispatch.go` — a `receiveDispatch` map
from `SemanticAction` to `[]receiveEntry`, connecting each receive-direction semantic action
to its set of packet IDs and corresponding decode functions.

## What Was Done

### 1. New generator: `internal/codegen/gen/receive_dispatch.go`

Added `GenerateReceiveDispatchFile(db *semantics.DB, vt preprocess.VersionTable) (string, error)`.

Key design decisions:
- Direction determined per-implementation via `isReceiveStruct(impl.StructName)` (from `events.go`)
- Implementations with empty `PacketID` or `StructName` skipped silently
- VersionTable parameter (nilable) used to skip implementations whose struct is absent from the VT — these have no corresponding generated decode function (e.g. Zero-client-only packets `PACKET_ZC_QUEST_DIALOG`, `PACKET_ZC_QUEST_DIALOG_MENU_LIST`, `PACKET_ZC_MONOLOG_DIALOG` at 0x0BA6/0x0BA7/0x0BA9, documented as out-of-scope in README-LLM.md)
- Unit tests pass `nil` VT — includes all receive-direction implementations regardless of VersionTable presence
- Actions with no receive-direction entries appear in a comment block above the map (not in the map)
- Decode function name formula: `decode.<PascalCase(action_name)>_<PacketID>` using existing `actionNameToGoIdent` helper

### 2. Updated `internal/codegen/main.go`

- Added `genReceiveDispatch(db *semantics.DB, vt preprocess.VersionTable, outDir string) error` wrapper
- Added Step 11 call in `run()` after Step 10 (SemanticAction enum)

### 3. Added unit tests to `internal/codegen/gen/gen_test.go`

Three test cases:
- `TestGenerateReceiveDispatchFile_AllSendDirection`: PACKET_CZ_ only → no entry in map, action appears in skip comment
- `TestGenerateReceiveDispatchFile_ReceiveDirection`: PACKET_ZC_ with non-empty PacketID → correctly formatted receiveEntry
- `TestGenerateReceiveDispatchFile_MixedDirection`: CZ_ + ZC_ → only ZC_ implementation in map

### 4. Generated output: `pkg/session/receive_dispatch.go`

- 277 receive-direction actions in `receiveDispatch` map
- 183 send-only/empty actions listed in skipped comment block
- Zero-client-only packets (0x0BA6, 0x0BA7, 0x0BA9) correctly excluded via VersionTable filter

## Import Cycle Verification

```
$ go list -deps github.com/lenaxia/rathena-client/pkg/decode | grep session
(empty output — no cycle)
```

`pkg/session` → `pkg/decode` introduces no cycle:
- `pkg/events` imports nothing
- `pkg/decode` imports only `pkg/events` and stdlib
- `pkg/session` imports stdlib + `pkg/decode` (new, via receive_dispatch.go)

## Test Results

```
ok  github.com/lenaxia/rathena-client/internal/codegen/gen   0.021s
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics  0.068s
ok  github.com/lenaxia/rathena-client/pkg/decode  (cached)
ok  github.com/lenaxia/rathena-client/pkg/encode  0.006s
ok  github.com/lenaxia/rathena-client/pkg/fsm     0.141s
ok  github.com/lenaxia/rathena-client/pkg/packing (cached)
ok  github.com/lenaxia/rathena-client/pkg/session 0.023s
```

`go build ./...` — clean, no errors.
`go test ./...` — all pass.

## Acceptance Criteria Status

- [x] `pkg/session/receive_dispatch.go` exists (277 receive-direction actions)
- [x] Every receive-direction action with valid implementation has an entry
- [x] Actions with no receive-direction implementations absent from map (with comment)
- [x] Direction determined per-implementation via `isReceiveStruct`
- [x] No import cycle (`go list -deps pkg/decode | grep session` → empty)
- [x] `go build ./...` passes
- [x] `go test ./...` passes
- [x] Codegen unit tests pass (3 new tests in gen_test.go)

## Notes

The spec stated `genReceiveDispatch` does not need the VersionTable. This was true for the naive
implementation, but Zero-client-only packets (0x0BA6/0x0BA7/0x0BA9) have struct entries in the
SemanticDB with non-empty PacketID and StructName, yet no generated decode functions (the structs
are absent from the VersionTable because they're Zero-client-only). The VersionTable parameter
was added as an optional filter (nil = include all) to handle this case without breaking unit tests.
