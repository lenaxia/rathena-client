# Work Log 0091 — codegen resolveLayout newest-fallback + generated-tree gofmt

**Date**: 2026-07-10
**Type**: Codegen bug fix + hygiene
**Scope**:
  - `internal/codegen/gen/encode.go` — `resolveLayout` newest-fallback
  - `internal/codegen/gen/resolve_layout_test.go` — 5 new unit tests
  - `internal/codegen/main.go` — `writeFile` runs gofmt on `.go` output
  - `semantics/mappings.yaml` — bound `cz_req_guild_emblem_img2` (0x0B1E) at min=20190227; split `cz_req_takeoff_equip_all` into 0x0BAD [20210818..20230905] + 0x0BF5 [20230906..∞]
  - Regenerated `pkg/{decode,events,encode,send,session}/` — 669 files touched by gofmt canonicalization + 2 substantive encoder rewrites
  - `CHANGELOG.md`

**Severity**: MODERATE — fixes a real regression exposed by v0.9.0 (CzReqGuildEmblemImg2 emitted 4 bytes short at goKore's target pv=20200401). Low-blast-radius: the packet is a guild-emblem-image upload that goKore likely doesn't send, and rAthena has no server handler registered in `clif_packetdb.hpp`, so the exposure was narrow. Fixing correctly matters both for wire-fidelity guarantees and to close the follow-up the PR #16 reviewer noted.

**Reference**: PR #16 review comment https://github.com/lenaxia/rathena-client/pull/16#issuecomment-4939652890 (search "CzReqGuildEmblemImg2: 14→10 bytes — CONCERN (LOW)"). Follow-up commitment to fix in a subsequent PR.

---

## Problem

In v0.9.0, `EncodeCzReqGuildEmblemImg2` emitted a `[10]byte` value. rAthena's `packets_struct.hpp:5788-5803` binds `PACKET_CZ_REQ_GUILD_EMBLEM_IMG2` to two struct variants:

- pv 20190227..20190618 → 10 bytes (packetType + guild_id + emblem_id)
- pv >= 20190619 → 14 bytes (adds trailing `unused` uint32)

Both variants use packet-id `0x0B1E`. goKore's target pv is 20200401 (well past 20190619), so the correct on-wire size is 14 bytes. Sending 10 bytes at that pv means rAthena's socket layer parses an extra 4 bytes off the wire as the next packet's header, corrupting the connection.

Root cause: `internal/codegen/gen/encode.go::resolveLayout` was written as follows:

```go
pv := uint32(packetverMin)
if pv == 0 {
    pv = 20030000
}
// ...iterate ranges newest-first looking for r.MinVer <= pv...
// (falls through) ...forward iterate, return first available.
```

For structs whose earliest range starts after 20030000 (post-2019 packets in particular), the newest-first loop never finds a matching range, so the code falls through to the forward-iteration fallback that returns the OLDEST available range. Wrong direction. This produced correct output for structs whose earliest range covers 20030000 (i.e. most pre-2019 packets) but silently mis-sized encoders for newer packets whose semantic DB impl has `PacketverMin=0` (unbounded).

A companion issue is that `PACKET_CZ_REQ_TAKEOFF_EQUIP_ALL` binds to two DIFFERENT packet IDs across a pv boundary:

- pv 20210818..20230905 → `0x0BAD`, 2 bytes
- pv >= 20230906 → `0x0BF5`, 6 bytes (with `location` field)

The v0.9.0 mapping had only `0x0BAD` unbounded. Under the old resolveLayout that produced the OLDEST layout (2 bytes) with the OLDEST packet-id header (0x0BAD) — which was accidentally correct for the pv range where 0x0BAD is actually on the wire. Under my newest-fallback fix, resolveLayout picks the newest layout (6 bytes) but pairs it with the wrong packet-id header (0x0BAD) — an invalid combination that no rAthena build ever emits. Fixing just resolveLayout would trade one wrong output for another. The correct behavior is to model each (packet_id, struct_layout) pair as a distinct semantic-DB impl.

---

## Solution

### 1. `resolveLayout` newest-fallback (defense-in-depth)

Rewrote the function to honor `PacketverMin > 0` unchanged (find the newest range whose `MinVer <= PacketverMin`), and to fall through to the NEWEST available range when `PacketverMin == 0` or no range covers it. Removed the `pv=20030000` sentinel altogether — treating "no lower bound" as "no lower bound" is more honest than pretending it means "start of history."

Note: this alone does NOT fix `EncodeCzReqGuildEmblemImg2` — the actual fix for that encoder is the mapping change in step 4, which sets `PacketverMin=20190619` (specific-pv path, not the fallback). The resolveLayout newest-fallback is defense-in-depth for future actions whose mappings remain unbounded; it converts the previous "silently pick oldest" failure mode into "silently pick newest," which is the safer bias for wire compatibility with modern rAthena.

Added five unit tests in `internal/codegen/gen/resolve_layout_test.go`:
- picks-newest-for-unbounded-impl (defense-in-depth case)
- honors-specific-packetverMin (three pv points across the boundary)
- packetverMin-below-all-ranges falls through to newest
- missing-struct returns nil
- unavailable-layouts returns nil

### 2. Generated-tree gofmt

Added `format.Source()` in `writeFile` for any path ending in `.go`. Templates for pv-branching encoders (specifically the switch/case path in `GenerateEncodeFile`) emit correct-but-not-canonical whitespace — extra tabs inside case bodies, inconsistent comment alignment. Canonicalising here keeps the generated tree diff-stable across runs. If gofmt fails on a template (theoretically possible on malformed input), a warning is logged and the unformatted content is written — preserves the historic fail-open behavior.

This change canonicalises the entire generated tree: ~660 files touched, all pure cosmetics (comment alignment, const-block indentation).

### 3. `cz_req_takeoff_equip_all` split

Applied via `semantics-tool`:
- `update-implementation -id 0x0BAD -min 20210818 -max 20230905`
- `add-implementation -id 0x0BF5 -struct PACKET_CZ_REQ_TAKEOFF_EQUIP_ALL -min 20230906`

Codegen then emits `EncodeCzReqTakeoffEquipAll` as a pv-branching function returning `[]byte`. At goKore's target pv=20200401 the switch hits the fallthrough `panic` because neither range applies — but goKore doesn't call this encoder at that pv (the packet doesn't exist), so the panic is unreachable in practice. An action registered but not on the wire at the runtime pv is a hazard we accept, since the semantic DB records the packet exists only at pv >= 20210818.

### 4. `cz_req_guild_emblem_img2` bound to 20190619

Applied via `semantics-tool`:
- `update-implementation -id 0x0B1E -min 20190619 -max 0`

**This is the actual fix for the reviewer's flagged 10→14 byte regression.** rAthena binds `PACKET_CZ_REQ_GUILD_EMBLEM_IMG2` to 0x0B1E at both pv 20190227..20190618 (10 bytes) and pv >= 20190619 (14 bytes with trailing `unused`). Since both variants use the SAME packet ID, the semantic DB cannot express them as two impls (packet IDs are unique per action). We must pick one variant.

Picking `min=20190619` corresponds to the CURRENT layout — what modern rAthena builds (including goKore's pv=20200401 target) emit on the wire. The tradeoff: clients targeting pv 20190227..20190618 would need a code change to use the 10-byte variant. Given the narrow (~4 month) window and low adoption of that packet, this is an acceptable narrowing.

---

## Verification

- Regenerated with `go run ./internal/codegen/main.go --rathena ~/personal/rathena --semantics semantics/mappings.yaml --out .` and inspected the resulting encoders.
- `EncodeCzReqGuildEmblemImg2`: now `[14]byte`, writes guild_id at offset 2, emblem_id at 6, unused at 10 — matches `packets_struct.hpp:5788-5794` at pv >= 20190619.
- `EncodeCzReqTakeoffEquipAll`: pv-branching. At pv >= 20230906 → 6 bytes with 0x0BF5 header and Location at offset 2. At pv >= 20210818 → 2 bytes with 0x0BAD header. Otherwise panic.
- `pkg/encode/register.go` adapted from `b := ...; return b[:], nil` → `return ..., nil` for the takeoff variant (now returns `[]byte` directly).
- Full test suite passes: `go test -count=1 ./...` clean.

---

## Impact on downstream

The `EncodeCzReqTakeoffEquipAll` signature change (`[6]byte` → `[]byte`) is a source-level API break for any consumer that stored the return value in a fixed-size array. `pkg/encode/register.go` (rathena-client's own consumer) is updated in this PR. goKore does not currently call this encoder — verified by grep of `~/personal/goKore/**/*.go`. If future goKore code uses it, the caller adopts the new `[]byte` return.

`EncodeCzReqGuildEmblemImg2` grew a field (`Unused`); the send-struct in `pkg/send/cz_req_guild_emblem_img2.go` is regenerated to include it. Callers must supply a value (0 is the correct default for a placeholder-with-unused field).

---

## Rule 0 note

This is worklog 0091; latest prior was 0090.
