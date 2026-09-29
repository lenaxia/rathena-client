# BUG-03 — RESOLVED: variable-length decoders trust embedded frame length (45 packet IDs)

Found 2026-09-29 by `FuzzFeedMapSessionWithDispatch` (corpus
`testdata/fuzz-known/061fce683bbdf652`), enumerated by the variable-length
branch of `TestReceiveDispatchLengthAudit`.

## Symptom

`AcAcceptLogin_0x0AC4` panics (index out of range at offset 46) on a frame
whose embedded length is 30: the framer accepts any embedded length >= 4 as
legal, but the decoder unconditionally reads the struct's fixed prefix.

## Root cause

Generated variable-length decoders slice `data[off:end]` for the fixed prefix
without comparing the frame's embedded length against the struct's minimum
prefix size. A truncated (hostile, buggy, or desynced) server response crashes
the client — this is remotely reachable: 0x0AC4 (login accept), 0x09FF/0x09FE
(modern spawn), 0x0B08–0x0B0A (inventory) are all in the affected set.

## Affected IDs (variable-length class)

0x0069, 0x006B, 0x008D, 0x00B4, 0x00B7, 0x0109, 0x0136, 0x0152, 0x0162,
0x0166, 0x01C3, 0x025A, 0x0442, 0x07F7, 0x07F8, 0x07F9, 0x0836, 0x0856,
0x0857, 0x0858, 0x08C0, 0x090F, 0x0914, 0x0915, 0x09D7, 0x09DA, 0x09DB,
0x09DC, 0x09DD, 0x09DE, 0x09EB, 0x09FD, 0x09FE, 0x09FF, 0x0A3B, 0x0A59,
0x0A6B, 0x0AA2, 0x0AC4, 0x0ADB, 0x0B03, 0x0B08, 0x0B09, 0x0B0A, 0x0B8D

## Fix direction (pick one, in preference order)

1. **Codegen bounds guard (preferred):** emit a prefix check at the top of
   every variable-length decoder — `if len(data) < N { return zeroEvent }`
   (or return a decode error once the API supports it) where N is the struct's
   fixed-prefix size known at generation time. Loop counts derived from
   `(len(data)-prefix)/elem` already behave correctly for short frames.
2. **Session-level minimum:** extend the generated length tables with a
   minimum-embedded-length column and have `feed()` treat shorter frames as
   stream corruption (same path as `ErrUnknownPacket`).

Option 1 keeps the session hot path untouched and is localized to the emitter.

## Containment (until fixed)

- The 45 IDs are frozen in `knownLengthOverread`
  (`pkg/session/dispatch_length_audit_test.go`); a NEW offender fails CI.
- `fuzzDispatchAll` skips these IDs so fuzz runs target NEW bugs.

## Risk

HIGH relative to BUG-02: reachable from any network peer. A malicious or
misbehaving server (or a MITM'd stream after desync) crashes the bot process.
Not exploitable beyond DoS (reads are in-buffer).

---
## Resolution (2026-09-29, PR #33)

Fixed in the codegen emitter + hand-written decoders; audit now reports **0
over-reads** and `knownLengthOverread` is empty.

- Both classes (BUG-02 and BUG-03) resolved by GUARDS, not by era-pinning:
  generateDecodeFunc's layout fallback deliberately keeps selecting the most
  recent layout — decoders are runtime-packetver-parameterized by design
  (golden tests decode legacy IDs such as 0x0078/0x0079 at modern packetvers
  with modern-sized frames).
- Every generated decode branch emits `if len(data) < N { return e }` where
  N is the layout's minimum extent (gen/decode.go layoutMinLen); a frame
  shorter than the declared extent — hostile, truncated, or a legacy table
  length — decodes to a zero event instead of panicking.
- Hand-written decoders (inventory lists, 0x006B/0x009E/0x011F) carry
  equivalent entry guards; their count loops were already length-derived.
- Quarantined corpus in testdata/fuzz-known/ now passes; re-verify any time:

      cp pkg/session/testdata/fuzz-known/* pkg/session/testdata/fuzz/FuzzFeedMapSessionWithDispatch/
      go test ./pkg/session -run 'FuzzFeedMapSessionWithDispatch/'
