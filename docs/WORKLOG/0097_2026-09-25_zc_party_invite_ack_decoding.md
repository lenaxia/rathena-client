# Work Log 0097 — ZC party invite-ack decoding (zc_party_join_req_ack)

**Date**: 2026-09-25
**Type**: Feature — new semantic action, event, and decoders
**Scope**:
  - `semantics/mappings.yaml` — new action `zc_party_join_req_ack` with two implementations (via `cmd/semantics-tool`)
  - `pkg/events/zc_party_join_req_ack.go` — new event struct
  - `pkg/decode/zc_party_join_req_ack.go` — new decoders (0x02C5 modern, 0x00FD legacy)
  - `pkg/session/actions.go` — `ActionZcPartyJoinReqAck SemanticAction = 464`, `String()` case, `maxSemanticAction` bump
  - `pkg/session/receive_dispatch.go` — `ActionZcPartyJoinReqAck` receive entries for both packet IDs
  - No `lengths_map.go` change: `t[0x00FD] = 27` (base table, clif_packetdb.hpp:101) and `t[0x02C5] = 30` (pv >= 20070227 block) already exist.

**Severity**: MODERATE — the party-invite handshake was half-decoded. The invited
client's side (`zc_party_join_req`) existed; the inviter's reply packet had no
event, so a consumer could never learn whether its invite was accepted,
rejected, or failed.

**Reference**: goKore bug report in goKore
`docs/07_WORK_LOG/1248_2026-09-25_bot_party_formation.md` §R4 (issue #385,
PR #421) — the R4 sub-item was blocked on this fix.

---

## Pre-implementation gate

### rAthena source verification (GCC ground truth)

rAthena master @ e985006 (2026-08-21), cloned to `/tmp/opencode/rathena`.

```
g++ -E -P -DPACKETVER=20200401 -DPACKETVER_MAIN_NUM=20200401 \
    -I src -I src/map -I src/common src/map/packets_struct.hpp
```

At pv=20200401 and pv=20070820 the struct is IDENTICAL — only the header
constant changes (packets_struct.hpp:5093-5102):

```c
struct PACKET_ZC_PARTY_JOIN_REQ_ACK {
    int16 PacketType;
    char characterName[(23 + 1)];   // NAME_LENGTH = 24
    int result;
} __attribute__((packed));
DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ_ACK, 0x02c5);  // pv >= 20070821
DEFINE_PACKET_HEADER(ZC_PARTY_JOIN_REQ_ACK, 0x00fd);  // pv <  20070821
```

Sender: `clif_party_invite_reply` (src/map/clif.cpp:7964-7978; the goKore bug
report's name `clif_party_inviteack` is the pre-rename name). Result enum
`e_party_invite_reply` (src/map/clif.hpp:160-172): 0=already in party,
1=rejected, 2=accepted, 3=full, 4=same-account, 5=blocked, 6=unknown,
7=offline, 8=invalid map property, 9=map cannot join, 10=memorial dungeon.
The clif.cpp doc comment additionally documents client result=11 (level
restriction, since 20170412); rAthena's enum does not define it and never
sends it — decoded as a plain int32 regardless.

`./validation/preprocess_check.sh` run with `RATHENA_ROOT=/tmp/opencode/rathena`
at PACKETVER=20200401 (434+686+131 structs OK) and 20070820 — all pass.

### Legacy 0x00FD wire size — 30 bytes (decision reversed in review round 1)

The initial commit decoded 0x00FD as a 27-byte frame with a uint8 result,
citing the wire-format doc comment `00fd <nick>.24S <result>.B`
(clif.cpp:7946) and `packet(0x00fd,27)` (clif_packetdb.hpp:101). Review
round 1 correctly rejected that layout under Rule 6 (rAthena wins):

- `clif_party_invite_reply` sends `sizeof(PACKET_ZC_PARTY_JOIN_REQ_ACK)` ==
  30 unconditionally — the struct is NOT packetver-gated, only the header
  constant is (clif.cpp:7968-7974, packets_struct.hpp:5093-5101). A current
  rAthena server at pv < 20070821 puts a 30-byte frame on the wire.
- The clif.cpp doc comment describes the 2007-era client format, not what
  the code below it sends.
- `clif_packetdb.hpp` registers inbound (C→S) handler packets; its length
  literals never govern server→client framing — `t[0x00FD]=27` was a stale
  Part-1 literal, exactly the class `lengths_map_overrides.go` exists to
  correct (its header comment says so).
- The repo's own sibling handling: 0x00FE (same family, same 20070821 gate,
  also a 30-byte unconditional struct) is framed at 30 and decoded as the
  struct.
- Regen-stability: the DB maps both IDs to PACKET_ZC_PARTY_JOIN_REQ_ACK, so
  a future codegen run emits a 30-byte/int32 decoder for 0x00FD.

Resolution (reviewer's Option 1): 0x00FD decodes identically to 0x02C5, and
`lengths_map_overrides.go` gains an unconditional `t[0x00FD] = 30` citing
packets_struct.hpp:5093-5101 + clif.cpp:7968-7974, GCC-verified at
pv=20070820 and pv=20200401. Without the override the framer would consume
27 of 30 bytes on a legacy-header frame and desync the stream by 3 bytes.
The goKore bug report's "0x00FD: <nick>.24S <result>.B" assumption and its
"Result uint8 per rathena's own versioning table" request were based on the
same two non-governing citations; goKore itself runs pv=20200401 (0x02C5
only), so the consumer-side impact is nil.

### Semantic DB

Queried via `cmd/semantics-tool` (CLI mode; MCP server unavailable in this
environment):

- `search -packet 0x02C5` → null; `search -packet 0x00FD` → null (both IDs unrouted)
- `search -name invite` → only `party_invite` (0x00FC C→S) and `reply_party_invite` (0x00FF C→S)
- Created: `create-action zc_party_join_req_ack` + `add-implementation` for
  `0x02C5 [20070821, ∞)` and `0x00FD [0, 20070820]` (the rAthena gate), plus
  `update-action -description` (note: the tool's `create-action`/
  `add-implementation` subcommands only parse flags placed BEFORE the
  positional argument — `-description X name`, not `name -description X`)
- `validate` → OK: no validation errors
- `create-action` appends at document end (internal/semanticsdb/mutate.go:27,
  pinned by internal/semanticsdb/db_test.go:233), so the new action takes the
  next constant value 464 without renumbering.

### Why codegen was not run

The bug report's requested change follows the manual-regression pattern this
repo already used for the sibling packet (`pkg/decode/zc_party_join_req_test.go`
is "Manually implemented — regression test for goKore bug report 0807"). A full
`internal/codegen` regeneration would need the rAthena revision the committed
artifacts were generated from; regenerating against current master (2026-08-21)
risks unrelated churn across every generated file. The DB entry above makes the
change regen-stable: a future codegen run picks up `zc_party_join_req_ack`
from the DB. Hand-written files match the generated patterns exactly (event
struct, per-ID decode functions with rAthena field citations, appended action
constant, sorted receive_dispatch entry).

---

## Implementation

Event (`pkg/events/zc_party_join_req_ack.go`):

```go
type ZcPartyJoinReqAck struct {
    CharacterName string
    Result        int32
}
```

Decoders (`pkg/decode/zc_party_join_req_ack.go`): `ZcPartyJoinReqAck_0x02C5`
and `ZcPartyJoinReqAck_0x00FD` share the identical 30-byte layout
(characterName@2 size 24 via `nullTermString`, result int32@26 via `leI32`) —
only the packet ID differs. Zero heap allocations — `nullTermString` is the
unsafe zero-copy helper. File headers use the ADDING_PACKETS.md §5 sanctioned
"Hand-written" variant (review round 1 minor).

Session wiring: `ActionZcPartyJoinReqAck SemanticAction = 464` + `String()`
case + `maxSemanticAction` bump (sizes `sendRegistry`); receive entries
`{0x02C5 → ZcPartyJoinReqAck_0x02C5, 0x00FD → ZcPartyJoinReqAck_0x00FD}`
under `ActionZcPartyJoinReqAck`; `lengths_map_overrides.go` sets
`t[0x00FD] = 30` (see decision record above).

## Tests (TDD — written first, confirmed red on the undefined symbols)

`pkg/decode/zc_party_join_req_ack_test.go` (golden frames synthesized from the
GCC layout above):
- `TestZcPartyJoinReqAck_0x02C5_Decode` — result enum happy paths incl. 11
- `TestZcPartyJoinReqAck_0x00FD_Decode` — legacy-header form incl. result 258
  (0x0102, above uint8 range — pins the int32 result)
- `TestZcPartyJoinReqAck_NulPaddedName` — NUL strip + all-NUL name
- `TestZcPartyJoinReqAck_FullWidthName` — 24-byte name without NUL
- `TestActionZcPartyJoinReqAck_Exists` — constant + `String()`
- `BenchmarkZcPartyJoinReqAck_0x02C5` / `_0x00FD`

`pkg/session/party_invite_ack_dispatch_test.go` (Feed-level, pattern of
worklog 0092's dispatch tests):
- `TestZcPartyJoinReqAck_0x02C5_FiresAtPv20200401` — framer length
  prerequisite (30) + handler fires with decoded event
- `TestZcPartyJoinReqAck_0x00FD_FiresAtLegacyPv` — same at pv=20070820,
  override prerequisite (30) + int32 result 258
- `TestZcPartyJoinReqAck_CrossPacketverLegacyFiresAtModernPv` — pins the
  registered-at-all-packetvers dispatch contract: 0x00FD frame under a
  modern-pv session fires the action through the legacy-ID decoder with the
  override keeping framing in sync (review round 1 request)

## Validation results

- `go build ./...` — OK
- `go test -count=1 ./...` — ALL PASS (one unrelated timing flake in
  `fsm_map_load_delay_test.go` on first run; passes on rerun and with -race)
- `go test -race -count=1 ./...` — ALL PASS
- `go test -bench=. -benchmem ./pkg/...` —
  `ZcPartyJoinReqAck_0x02C5` 2.77 ns/op 0 allocs/op;
  `ZcPartyJoinReqAck_0x00FD` 2.51 ns/op 0 allocs/op; CI's disallowed-alloc
  grep (applied locally) finds nothing
- `grep -r "^\s*go " pkg/ --include="*.go" | grep -v _test.go` — empty
- `./validation/preprocess_check.sh` at 20200401 and 20070820 — OK
- `semantics-tool validate` — OK
- `gofmt -l pkg/` on changed files — clean
  (`pkg/encode/repair_item*.go` and `pkg/session/mappropr2_dispatch_test.go`
  gofmt drift pre-exists on main; untouched)

## Review round 1 (PR #32) — 0x00FD layout corrected

Verdict: REQUEST CHANGES — 0x02C5 half "rAthena-verified and merge-ready";
the 0x00FD 27-byte/uint8 layout contradicted the current-rAthena send path,
the sibling 0x00FE handling, and was regen-unstable. Applied the reviewer's
Option 1 (repo-consistent): identical 30-byte decoder + unconditional
`t[0x00FD] = 30` override in `lengths_map_overrides.go` with the required
citations; added the cross-packetver dispatch pin and a >255 result case;
switched file headers to the sanctioned "Hand-written" variant. All tests
and benchmarks re-run green (0 allocs/op both decoders).
