# Quarantined fuzz corpus

Crashers found by `FuzzFeedMapSessionWithDispatch` (2026-09-29). They live here —
not in `../fuzz/` — so `go test ./pkg/session` stays green while the underlying
codegen bugs are fixed; the auto-run corpus directory must only contain inputs the
targets survive.

- `75869bcf4b578ee9` — index-out-of-range in `ActorConnected_0x01D9` (reads to
  offset 55 against the 53-byte legacy length-table entry). Bug class 1:
  fixed-length legacy-variant decoders.
- `061fce683bbdf652` — index-out-of-range in `AcAcceptLogin_0x0AC4` (reads to
  offset 46 against a legal 30-byte variable-length frame). Bug class 2:
  variable-length decoders trust the embedded length without bounds checks.

Full blast radius (58 packet IDs), root cause, and fix direction:
`pkg/session/dispatch_length_audit_test.go` and
`docs/BACKLOG/BUG-02_legacy-variant-length-overread.md` /
`docs/BACKLOG/BUG-03_variable-length-decoder-bounds.md`.

Both crashers PASS since the length-guard fix and have been PROMOTED into
`../fuzz/FuzzFeedMapSessionWithDispatch/` — `go test ./pkg/session` now
replays them as ordinary seeds on every run, keeping a permanent regression
guard on the truncated-frame axis. This directory is kept for history; new
crashers land here first, quarantined until triaged.

