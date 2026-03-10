# 0027 — 2026-03-09 — US-15: injectMapPacketStructs (character_moves, sync)

## Summary

Implemented `injectMapPacketStructs` in `internal/codegen/main.go` to inject struct layouts
from `src/map/packets.hpp` into the VersionTable. This unblocks two previously-SKIPped decode
functions: `CharacterMoves_0x0087` and `Sync_0x007F`.

## Pre-Implementation Gate

GCC output verified (provided by orchestrator, PACKETVER=20181121):

```
struct PACKET_ZC_NOTIFY_PLAYERMOVE {
    int16  packetType;      // offset 0, size 2
    uint32 moveStartTime;   // offset 2, size 4
    uint8  moveData[6];     // offset 6, size 6
} __attribute__((packed));
// total: 12 bytes, fixed

struct PACKET_ZC_NOTIFY_TIME {
    int16  packetType;      // offset 0, size 2
    uint32 time;            // offset 2, size 4
} __attribute__((packed));
// total: 6 bytes, fixed
```

SemanticDB (queried via MCP, already corrected by orchestrator):
- `character_moves` / 0x0087: `MoveData = "[6]byte(packet.moveData[:])"`, `Time = "packet.moveStartTime"` ✓
- `sync` / 0x007F: `Time = "packet.time"` ✓ (was `packet.Time`, fixed before this task)

## Changes

### `internal/codegen/main.go`

1. Added `mapStructsToInject` slice listing `PACKET_ZC_NOTIFY_PLAYERMOVE` and `PACKET_ZC_NOTIFY_TIME`.

2. Added `injectMapPacketStructs` function — exact copy of `injectCommonPacketStructs` with:
   - Uses `SourcePackets` (not `SourceCommonPackets`)
   - Reads breakpoints from `src/map/packets.hpp` (not `src/common/packets.hpp`)
   - Uses `mapStructsToInject` (not `commonStructsToInject`)
   - Log prefix: "map/packets.hpp"
   - Error message mentions "map packet structs"
   - Uses `PacketsHPPStub` (already in Config) since packets.hpp needs the ryml stub

3. Added call in `run()` as Step 4d, immediately after `injectCommonPacketStructs`.

### Generated files

`pkg/decode/character_moves.go`:
```go
func CharacterMoves_0x0087(data []byte, packetver uint32) events.CharacterMoves {
    var e events.CharacterMoves
    _ = packetver
    e.MoveData = [6]byte(data[6:12])  // rAthena: moveData (offset 6, size 6)
    e.Time = leU32(data, 2)           // rAthena: moveStartTime (offset 2, size 4)
    return e
}
```

`pkg/decode/sync.go`:
```go
func Sync_0x007F(data []byte, packetver uint32) events.Sync {
    var e events.Sync
    _ = packetver
    e.Time = leU32(data, 2)  // rAthena: time (offset 2, size 4)
    return e
}
```

### Events verification

- `pkg/events/character_moves.go`: `MoveData [6]byte`, `Time uint32` ✓
  - Comment says "Packed coordinates: from_x, from_y, to_x, to_y" — does NOT say "direction" (Bug 13-A not present)
- `pkg/events/sync.go`: `Time uint32` ✓

### New test files

- `pkg/decode/character_moves_test.go` — 3 test cases + benchmark
- `pkg/decode/sync_test.go` — 3 test cases + benchmark

## VersionTable after codegen

```
VersionTable has 459 structs (from packets_struct.hpp)
→ 487 structs (after synthetic injection)
→ 488 structs (after common injection: PACKET_AC_ACCEPT_LOGIN)
→ 490 structs (after map injection: PACKET_ZC_NOTIFY_PLAYERMOVE, PACKET_ZC_NOTIFY_TIME)
```

## Test Results

```
go build ./...         exit 0
go test ./...          ALL PASS (pkg/decode: 0.003s)
go test -race ./...    ALL PASS
```

## Benchmark Results

```
BenchmarkCharacterMoves_0x0087-14    1000000000    0.2382 ns/op    0 B/op    0 allocs/op
BenchmarkSync_0x007F-14              1000000000    0.1889 ns/op    0 B/op    0 allocs/op
```

Both meet the 0 allocs/op requirement.

## Zero Goroutines Check

```
grep -r "^\s*go " pkg/ --include="*.go" | grep -v "_test.go"
# (empty — invariant satisfied)
```

## Known Issues

None. Post-validation fix applied by orchestrator:
- `pkg/events/character_moves.go` MoveData comment updated to: "Packed movement data (6 bytes: from_x, from_y, to_x, to_y, sx0, sy0). Call packing.DecodeMoveData to unpack. Byte 5 is NOT direction."
- `pkg/fsm` Bug 12-E (`connTransferred` defer) already applied in working tree; test `TestConnect_OnReady_Nil_ConnClosed` now passes.
