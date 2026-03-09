# Known Issues and Design Limitations

Tracked issues that are pre-existing or by design. Not bugs requiring immediate fixes,
but they need awareness before extending the relevant codegen paths.

---

## CONCERN-2: PosDir field type mismatch in decode codegen ✅ RESOLVED (session 0013)

**Resolution applied**: Changed canonical param types in SemanticDB from `[]byte` to `[3]byte`
(for 3-byte packed position fields) and `[6]byte` (for 6-byte packed movement fields).
Updated all field_mapping expressions to use `[3]byte(packet.PosDir[:])` / `[6]byte(packet.MoveData[:])` forms.
Updated `internal/codegen/gen/decode.go` to emit `[3]byte(data[off:off+3])` / `[6]byte(data[off:off+6])`
— zero-alloc fixed-size array conversions (no `make()`, no heap escape).
Updated `internal/codegen/gen/events.go` to recognise `[3]byte` and `[6]byte` as passthrough types.
Regenerated all packages. `go build ./...` and `go test ./...` clean.

**Affected actions fixed**: actor_exists (PosDir), actor_connected (PosDir), entity_spawn (PosDir),
map_loaded (Coords), move_to (Coords), character_move (Coords), entity_move (Coords),
actor_moved (MoveData), character_moves (MoveData).

~~**File**: `pkg/decode/` (generated), `semantics/mappings.yaml`~~
~~**Identified**: session 0011~~

---

## CONCERN-3: Packet 0x0B09 is shared across inventory/storage/cart at high PACKETVER

**File**: `semantics/mappings.yaml`, `docs/PHANTOM_STRUCTS.md`
**Identified**: session 0011

At `PACKETVER_MAIN_NUM >= 20181002`, the following three packet type constants all
resolve to `0x0B09`:
- `inventorylistnormalType` (inventory)
- `storageListNormalType` (Kafra storage)
- `cartlistnormalType` (cart inventory)

The SemanticDB maps `0x0B09` to `packet_itemlist_normal` (inventory only). The storage
and cart variants use different semantics at earlier PACKETVER ranges where the IDs
don't overlap (storage has `name[NAME_LENGTH]` field for `20120925 <= PACKETVER < 20181002`).

**At PACKETVER >= 20181002**, all three variants converge to the same struct layout via
`packet_itemlist_normal`. This works correctly for that version range.

**Risk**: If someone adds storage or cart semantic actions pointing to `0x0B09`, the
SemanticDB cannot distinguish which semantic applies without the packetver range context.
The current single mapping is not wrong, but is fragile if the storage/cart codegen
paths are added later.

**Resolution needed**: Before implementing storage or cart item list decode actions,
explicitly document the shared ID and ensure packetver_range constraints correctly
partition the three semantic uses. A note should be added to the `0x0B09` entry in
`semantics/mappings.yaml`.

---

## CONCERN-4: SYNTH_CH_ENTER (0x0275) has unknown layout

**File**: `internal/codegen/stubs/synthetic_structs.hpp`
**Identified**: session 0011

`SYNTH_CH_ENTER` (0x0275, char server authentication, 37 bytes) has 35 bytes of
`_padding` because the field layout beyond `PacketType` is unknown. No rAthena source
defines this struct, and no map server code handles it (it is processed by the char
server, not map server).

**Risk**: If this struct is ever connected to the encode pipeline (send codegen), it
will silently write 35 zero bytes for all unknown fields.

**Mitigation applied**: The struct has a clear `WARNING: DO NOT USE IN ENCODE PIPELINE`
comment in `synthetic_structs.hpp`. The struct is currently only in the VersionTable
for length accounting, not wired to any SemanticDB send action.

**Resolution needed**: Research the actual 0x0275 layout from OpenKore or packet
captures before implementing any send action for this packet.

---

## CONCERN-6 (pre-existing): Shuffle table reassigns canonical IDs across PACKETVER

**File**: `internal/codegen/stubs/synthetic_structs.hpp`, `src/map/clif_packetdb.hpp`
**Identified**: session 0011

In `clif_packetdb.hpp`, the shuffle table reassigns canonical packet IDs to different
handlers at different PACKETVER ranges. For example:
- `0x009f` (normally `clif_parse_TakeItem`, 6 bytes) becomes `ChangeDir` at some versions
- `0x007e` (normally `clif_parse_TickSend`, 6 bytes) becomes `WantToConnection` at others
- `0x00f7` (normally `clif_parse_CloseKafra`, 2 bytes) becomes `TickSend` at others

The `SYNTH_*` structs are written for the **baseline** (non-shuffle) handlers only —
the handler at the lowest PACKETVER where that ID first appears. At shuffled versions,
`clif_shuffle.hpp` remaps the logical packet ID to a different wire ID, so the baseline
struct remains valid for the logical meaning.

**Status**: Known system-wide limitation. The shuffle map generator (`pkg/session/shuffle_map.go`)
handles the ID-to-ID remapping at runtime. The SemanticDB struct names correspond to
logical packet semantics, not wire IDs. This design is correct.

**No action needed** unless the shuffle-variant layouts differ structurally from the
baseline (which would require separate SYNTH_ structs per shuffle variant).

---

## PERF-1: ActorExists_0x09FF and related — 1 alloc/op due to string decode (US-05) ✅ RESOLVED

**Resolution applied**: Changed `nullTermString` in `pkg/decode/helpers.go` to use
`unsafe.String` + `unsafe.SliceData` (Go 1.20+) to construct a zero-copy string alias
over the input `[]byte`. The returned string shares the underlying byte array and requires
no heap allocation.

Benchmark after fix (session 0017):
```
BenchmarkActorExists_0x09FF  0 allocs/op (was 1)
BenchmarkAcAcceptLogin_0x0069  0 allocs/op
```

The string is valid for the lifetime of the session's read buffer. This is safe because
`pkg/decode` functions are called during event dispatch before the buffer is reused.

~~**File**: `pkg/decode/actor_exists.go`, `pkg/decode/helpers.go`~~
~~**Identified**: session 0016 (US-05 benchmarks)~~

---

## PERF-2: StatUpdate_0x00BE — leU32 reads 4 bytes on a 1-byte value field (US-05) ✅ RESOLVED

**Resolution applied**: Added width-mismatch detection in `fieldReadExpr`
(`internal/codegen/gen/decode.go`). When a canonical param type (e.g. `uint32`) is wider
than the actual rAthena field size (e.g. 1 byte), the codegen now emits a narrower read
with an explicit cast:
- `f.Size==1, goType=="uint32"` → `uint32(data[off])`
- `f.Size==2, goType=="uint32"` → `uint32(leU16(data, off))`
- etc.

Generated `StatUpdate_0x00BE` now reads `uint32(data[4])` (1 byte) instead of
`leU32(data, 4)` (4 bytes). The 5-byte golden test passes with an exact-size buffer.

~~**File**: `pkg/decode/stat_update.go`~~
~~**Identified**: session 0016 (US-05 golden tests)~~
