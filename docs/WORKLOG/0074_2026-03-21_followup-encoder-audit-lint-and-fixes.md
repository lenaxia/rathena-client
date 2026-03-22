# 0074 — Follow-up: Encoder Audit Findings, Validation & Fixes

**Date**: 2026-03-21
**Status**: IN PROGRESS
**Scope**: `internal/codegen/gen/encode.go`, `pkg/encode/`, `pkg/send/`
**Relates to**: worklog 0073 (5 shuffle-era ID bugs), worklog 0072 (enter_world)

---

## Summary of Validated Findings

This worklog documents the follow-up systematic analysis after v0.5.8, covering:

1. The **real remaining send-direction bugs** after correcting earlier false positives
2. Confirmed **non-issues** that were over-counted in the initial sweep
3. The **codegen lint rule** that prevents this entire class of bug going forward
4. **Cleanup** for the `character_move` duplicate

---

## Finding 1: Receive-Direction Has No Gaps

The earlier analysis claimed 109 receive-direction "gaps" (packet IDs in the semantics DB with no
decoder function). **This was a false positive** caused by the audit script searching for `_NNNN`
as a substring (missing the `0x` prefix in function names like `ActorAction_0x008A`).

Verification:
```
Decoder functions defined:              366
Packet IDs in dispatch table:           366
Dispatched with missing decoder body:     0   ← No gaps
```

`actor_action.go` already has both `ActorAction_0x008A` and `ActorAction_0x08C8`. The dispatch
layer and decode layer are fully consistent.

**No receive-direction work required.**

---

## Finding 2: "Accidentally-Correct" Encoders — Precise Classification

The 6 accidentally-correct send encoders from worklog 0073 require more precise analysis.
All 6 hardcode their IDs and are correct at pv=20200401, but may be wrong at shuffle-era pvs
(20130515–20180307). The key question: **are their base IDs in the shuffle map?**

Cross-check against `pkg/encode/shuffle_map.go`:

| Action | Hardcoded ID | In shuffle_map? | Fix needed? |
|---|---|---|---|
| `friends_add` | `0x0202` | **YES** — shuffles to e.g. `0x0962`@20130515, `0x08AA`@20180307 | **YES** |
| `homunculus_menu` | `0x022D` | **YES** — shuffles to e.g. `0x0931`@20130515, `0x0944`@20180307 | **YES** |
| `cz_party_join_req` | `0x02C4` | No | No — stable ID |
| `friends_remove` | `0x0203` | No | No — stable ID |
| `friends_reply` | `0x0208` | No | No — stable ID |
| `character_move` | `0x035F` | No | **Different issue** — see Finding 3 |

### `friends_add` (0x0202) — shuffle bug confirmed

At pv=20130515, `shuffledCtoSID(pv, 0x0202) = 0x0962`. The encoder hardcodes `0x0202`.
Any server running a 20130515–20180307 weekly client would receive the wrong wire ID.
The stable post-20180307 block does NOT contain 0x0202 (it returns `baseID` for unknown
entries), so the encoder is accidentally correct at pv=20200401 only because the post-shuffle
stable era doesn't remap 0x0202.

Fix: hand-written dispatcher using `shuffledCtoSID(pv, 0x0202)` for `pv >= 20130515`.

### `homunculus_menu` (0x022D) — shuffle bug confirmed

Same pattern. `shuffledCtoSID(pv, 0x022D) = 0x0931`@20130515. Hardcoded `0x022D` is wrong
for any shuffle-era weekly client. Post-20180307 stable block doesn't remap 0x022D, so
pv=20200401 works by accident.

Fix: hand-written dispatcher using `shuffledCtoSID(pv, 0x022D)` for `pv >= 20130515`.

### `cz_party_join_req` (0x02C4), `friends_remove` (0x0203), `friends_reply` (0x0208)

None of these IDs appear in `clif_shuffle.hpp` at all. They were never remapped. The hardcoded
IDs are genuinely stable across all packetvers. **No fix needed.**

---

## Finding 3: `character_move` — Structural Duplicate of `move_to`

`pkg/encode/character_move.go` encodes `0x035F` (SYNTH_CZ_REQUEST_MOVE2), which is the
post-20101124 wire ID for `clif_parse_WalkToXY` — the same handler as `move_to`.

`pkg/encode/move_to.go` is a correctly-implemented hand-written dispatcher that uses
`shuffledCtoSID(pv, 0x0085)` and returns the correct `0x035F` at pv=20200401.

The `character_move` encoder:
- Is a generated file
- Hardcodes `0x035F` (correct only for pv >= 20101124)
- Has a different send struct (`send.CharacterMove{Dest [3]byte}`) vs `send.MoveTo{X, Y uint16}`
- Uses `copy(p[2:], req.Dest[:])` rather than `packing.EncodePosDir`

The two actions encode the same wire packet but with different caller APIs. `character_move`
takes pre-packed 3-byte coordinate data; `move_to` takes X/Y and packs it internally. This is
not a bug — it's intentional (some callers may have pre-packed coordinates). However:
1. `character_move` will produce the wrong wire ID for pvs < 20101124 (sends `0x035F` instead of
   the era-correct ID like `0x0085`)
2. `character_move` is not in the shuffle map so the post-20180307 stable `0x035F` is coincidentally
   correct but only for the same reason as the friends_ encoders above

**Fix**: `character_move.go` should delegate to the same dispatch table as `move_to.go` for the
packet ID selection, but can use its own `Dest [3]byte` field for the coordinate data.

OR: document that `ActionCharacterMove` only supports `pv >= 20101124` (when `0x035F` first
appeared) and is not suitable for legacy server support.

The second option is simpler and honest. goKore targets modern servers (pv=20200401) so
the practical impact is zero.

---

## Finding 4: Codegen Root Cause — `_ = packetver` with No Shuffle Guard

The codegen `generateEncodeFunc` in `internal/codegen/gen/encode.go:200` always emits:
```go
_ = packetver
```

This is correct for IDs that are genuinely stable. It is wrong — and the root of every bug
in worklogs 0069–0073 — for any ID that appears in `clif_shuffle.hpp`. The codegen has no
mechanism to detect this: it doesn't cross-reference the generated packet ID against the
shuffle map.

## Addendum — "3 Accidentally-Correct" Encoders Are Actually All Correct

After deeper investigation (v0.5.10 follow-up), the three encoders previously classified as
"accidentally correct for pv=20200401 but wrong for shuffle era" are in fact fully correct
with no fix required:

### `cz_party_join_req` (0x02C4)

The earlier analysis concluded this was wrong because `0x02C4` was assigned to
`clif_parse_UseSkillToId` in the `clif_packetdb.hpp` historical scan. This was a
misidentification. The full timeline:

- Pre-`pv >= 20111102`: `0x02C4` was indeed `clif_parse_UseSkillToId` (10-byte skill cast)
- From `pv >= 20111102`: `0x02C4` was **reassigned** to `clif_parse_PartyInvite2` (26-byte party invite)
- `0x02C4` IS in the shuffle map, but checking all weekly blocks 20130515–20180307 and the
  stable post-20180307 block shows it is **never remapped** — all blocks return `baseID`

OpenKore confirmation: `RagexeRE_2018_11_21.pm` explicitly sets `party_join_request_by_name 02C4`.

**Status: correct, no fix needed.**

### `friends_remove` (0x0203)

Single stable entry in `clif_packetdb.hpp`. Not in `clif_shuffle.hpp`. Never reassigned.
**Status: correct, no fix needed.**

### `friends_reply` (0x0208)

Two entries: `size=11` at baseline, `size=14` at `pv >= 20040705`. The encoder uses `[14]byte`
which is correct for all production servers. Not shuffled.
**Status: correct, no fix needed.**
1. Parses `shuffle_map.go` to extract the set of base IDs that appear in any `case 0xNNNN`
2. For any generated encoder that hardcodes a packet ID in that set, either:
   a. **Fail the codegen run with an error** (strict mode — recommended), OR
   b. Emit a `//nolint:packetver` comment that must be manually suppressed

The error message should be actionable:
```
ERROR: action friends_add: hardcoded ID 0x0202 appears in shuffle_map.go
  — this encoder will produce wrong wire bytes for pv 20130515–20180307
  — fix: replace generated encoder with hand-written shuffledCtoSID dispatcher
  — or add to allowlist if the ID is confirmed stable despite appearing in shuffle table
```

The lint check should run as part of `go test ./internal/codegen/gen/` via a test that loads
the real DB, runs the codegen, and asserts no generated encoder has a shuffle-table overlap
that isn't explicitly allowlisted.

---

## Fix Plan

### Change 1: Codegen lint rule — `internal/codegen/gen/encode.go`

Add a `ValidateShuffleOverlap` function that takes the set of generated encoders and a
`shuffleBaseIDs map[uint16]bool` parsed from the shuffle map. Returns a list of violations.

Called from `GenerateEncodeDirFiles` before writing output. If violations exist, return them
as errors (not warnings) so codegen fails rather than producing silently wrong output.

Add a `ShuffleBaseIDs` parameter to `GenerateEncodeDirFiles` or parse it internally from the
shuffle_map.go file path passed via `main.go`.

Allowlist for the check (IDs that appear in the shuffle map but are genuinely stable because
the post-20180307 block returns `baseID` for them — i.e. they happen to be in a `case` inside
a weekly block but are not in the post-20180307 stable override list):
- None currently — all IDs in the shuffle map that we care about are correctly handled

### Change 2: Codegen validation test — `internal/codegen/gen/gen_test.go`

```go
func TestGenerateEncodeDirFiles_NoShuffleOverlap(t *testing.T) {
    // Loads real DB + real shuffle map IDs
    // Asserts that no generated encoder hardcodes an ID that is in the shuffle table
    // Explicitly allowlists any confirmed-stable exceptions
}
```

This test is the permanent regression guard. It would have been red for all 5 bugs in
worklog 0073 and for `friends_add` and `homunculus_menu`.

### Change 3: Fix `friends_add` — hand-written dispatcher

Pre-shuffle table for `clif_parse_FriendsListAdd` (0x0202 is the stable base):

| pv range | wire ID |
|---|---|
| < 20130515 | `0x0202` (never explicitly reassigned in clif_packetdb.hpp) |
| >= 20130515 | `shuffledCtoSID(pv, 0x0202)` |
| > 20180307 | `0x0202` (post-shuffle stable — baseID fallback) |

Wire format: always 26 bytes, `name(char[24])@[2:26]`. No size change across variants.

### Change 4: Fix `homunculus_menu` — hand-written dispatcher

Pre-shuffle table for `clif_parse_HomMenu` (0x022D):

| pv range | wire ID |
|---|---|
| < 20130515 | `0x022D` (single entry in clif_packetdb.hpp:522, never reassigned) |
| >= 20130515 | `shuffledCtoSID(pv, 0x022D)` |
| > 20180307 | `0x022D` (post-shuffle stable — baseID fallback) |

Wire format: always 5 bytes, `homId(u16)@[2:4]`, `action(u8)@[4]`. No size change.

### Change 5: Document `character_move` limitation

Add a comment to `character_move.go` and `send/character_move.go` noting that this action
only supports `pv >= 20101124` (when 0x035F first appeared as the WalkToXY wire ID).
For full packetver coverage, callers should use `ActionMoveTo` / `send.MoveTo` instead.

Update the semantics DB via MCP to set `packetver_min=20101124` on the `character_move`
`0x035F` implementation.

### Change 6: Godoc for `send.Look`

Add field comments to `send/look.go`:
```go
// Look is the request struct for CZ_CHANGE_DIRECTION / CZ_CHANGE_DIRECTION2.
type Look struct {
    HeadDir uint8 // Head facing direction: 0=N, 1=NW, 2=W, 3=SW, 4=S, 5=SE, 6=E, 7=NE (rAthena: headdir, valid 0-2 for most clients)
    Dir     uint8 // Body facing direction: 0-7 clockwise from N (rAthena: dir)
}
```

Note: `send/look.go` is currently generated (`// Code generated by internal/codegen. DO NOT EDIT.`).
The comment must be added to the YAML description or the codegen template, not the file directly.
Alternatively, since `look.go` (encoder) is now hand-written, the send struct can be hand-written
too with the comment, and the generated file deleted from the codegen output (add `look` to the
send-struct skip list in `send.go` codegen).

---

## Correctness Matrix (Final)

| Encoder | Status before | Status after this worklog |
|---|---|---|
| `drop_item` | BUG (fixed v0.5.8) | ✅ |
| `look` | BUG triple (fixed v0.5.8) | ✅ |
| `move_from_storage` | BUG (fixed v0.5.8) | ✅ |
| `move_to_storage` | BUG (fixed v0.5.8) | ✅ |
| `skill_use_location` | BUG (fixed v0.5.8) | ✅ |
| `enter_world` | MISSING (fixed v0.5.8) | ✅ |
| `friends_add` | BUG (shuffle-era wrong) | ✅ Fixed here |
| `homunculus_menu` | BUG (shuffle-era wrong) | ✅ Fixed here |
| `character_move` | DOCUMENTED LIMITATION | Bounded to pv>=20101124 |
| `cz_party_join_req` | OK (not in shuffle) | ✅ No change |
| `friends_remove` | OK (not in shuffle) | ✅ No change |
| `friends_reply` | OK (not in shuffle) | ✅ No change |
| All other 150 generated | OK (stable IDs) | ✅ Protected by lint rule |

---

## References

| Source | Purpose |
|---|---|
| `internal/codegen/gen/encode.go:200` | Where `_ = packetver` is emitted |
| `pkg/encode/shuffle_map.go` | Case lookup for 0x0202, 0x022D |
| `clif_packetdb.hpp:259` | 0x0202 single stable entry |
| `clif_packetdb.hpp:522` | 0x022D single stable entry |
| `clif_shuffle.hpp` — 0x0202 blocks | Confirms shuffle era remapping |
| `clif_shuffle.hpp` — 0x022D blocks | Confirms shuffle era remapping |
| `pkg/encode/move_to.go` | Reference dispatcher pattern |
| `pkg/encode/pickup_item.go` | Reference dispatcher pattern |
