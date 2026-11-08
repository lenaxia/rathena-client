# BUG-02 — RESOLVED: legacy-variant decoders read past length-table frame (13 packet IDs)

Found 2026-09-29 by `FuzzFeedMapSessionWithDispatch` (corpus
`testdata/fuzz-known/75869bcf4b578ee9`), enumerated by
`TestReceiveDispatchLengthAudit`.

## Symptom

Panics (index out of range) when the framer hands a legacy packet ID's frame to
its decoder. First hit: `ActorConnected_0x01D9` reads through offset 55 while
`lengths_map.go` assigns 0x01D9 = 53.

## Root cause

`semantics/mappings.yaml` scopes legacy packet IDs to packetver ranges (e.g.
`actor_connected` / `0x01D9` / [20030001, 20050410]), and the length table
correctly carries rAthena's `clif_packetdb.hpp` size for those IDs (53).
But the codegen evaluates the C struct guards (`#if PACKETVER >= ...`) at a
modern packetver when emitting the decoder for those IDs, producing the modern
layout (0x0079's decoder reads to offset 107, 0x01D9's to 55) against the
legacy length.

`receive_dispatch.go` registers all packetver variants regardless of session
packetver (documented as intentional), so the mismatched decoder is armed on
every session.

## Affected IDs (fixed-length class)

0x006A, 0x0079, 0x007B, 0x009E, 0x0114, 0x0119, 0x011F, 0x01D8, 0x01D9,
0x01DA, 0x022E, 0x0284, 0x02B9

## Fix direction

The emitter must evaluate struct guards at the packetver range from the
mapping (e.g. the range midpoint or each bound), not at the session/modern
packetver. Decoders whose range is [lo, hi] should be emitted against `hi`'s
struct layout if and only if the length table's size for that ID at `hi`
matches; otherwise the mapping or length source is inconsistent and codegen
must fail loudly.

## Containment (until fixed)

- The 13 IDs are frozen in `knownLengthOverread`
  (`pkg/session/dispatch_length_audit_test.go`); a NEW offender fails CI.
- `fuzzDispatchAll` skips these IDs so fuzz runs target NEW bugs.

## Risk

Low for gokore on modern servers: rAthena does not send these legacy IDs at
modern packetvers. On capture/replay of old-server traffic, decoders silently
read adjacent bytes (garbage trailing fields) and panic only at buffer
boundaries.

---
## Resolution (2026-09-29, PR #33)

Fixed in the codegen emitter + hand-written decoders; audit now reports **0
over-reads** and `knownLengthOverread` is empty.

- Both classes resolved by GUARDS, not by era-pinning layouts: decoders are
  runtime-packetver-parameterized by design (golden tests decode legacy IDs
  such as 0x0078/0x0079 at modern packetvers with modern-sized frames), so
  the layout fallback keeps selecting the most recent layout.
- Every generated decode branch emits `if len(data) < N { return e }` where
  N is the layout's minimum extent (gen/decode.go layoutMinLen); a frame
  shorter than the decoder's declared extent — hostile, truncated, or a
  legacy table length — decodes to a zero event instead of panicking.
- Hand-written decoders (inventory lists, 0x006B/0x009E/0x011F) carry
  equivalent entry guards; their count loops were already length-derived.
- Quarantined corpus in testdata/fuzz-known/ now passes; re-verify any time:

      cp pkg/session/testdata/fuzz-known/* pkg/session/testdata/fuzz/FuzzFeedMapSessionWithDispatch/
      go test ./pkg/session -run 'FuzzFeedMapSessionWithDispatch/'
