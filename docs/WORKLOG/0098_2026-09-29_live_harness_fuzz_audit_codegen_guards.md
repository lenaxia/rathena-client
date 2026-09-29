# 2026-09-29 — Live harness, fuzz audit, codegen guards (BUG-02/03 resolution)

Session covering PR #33 commits `0638ce1..` through the guard landing.
Context: stress-test of the korangar/nostalro takeaways (live harness, fuzz,
drift gate), executed in a git worktree against origin/main.

## What was built

1. `hack/testserver/` — dockerized rAthena at a pinned ref
   (`RATHENA_REF` = e985006, PACKETVER=20200401), seeded test account,
   pktmirror capture overlay with shifted ports. Five infra bugs found and
   fixed en route by the CI loop itself: libmariadb-dev-compat (mysql_config),
   `user_pass` column rename, logs.sql import, ipban_db_* config group,
   port + settle-window readiness for char↔login registration.
2. `cmd/pktmirror` — passive trio mirror; emits RATF v1 fixtures
   byte-compatible with `loadFixture` (validated against a fake trio).
3. Fuzz targets: Feed framing ×3 session types with full receiveDispatch
   armed; packing round-trips. Nightly CI with corpus caching.
4. `dispatch_length_audit_test.go` — enumerates decoders that read past
   their length-table frame; now a tripwire (empty freeze list, new
   offenders fail CI).
5. Codegen drift gate — regen at the pin must be byte-identical.

## Bugs found (fuzz: 2 crashers in <6s each; audit: 58 IDs total)

- BUG-02 (13 fixed-length IDs): legacy-variant decoders over-read their
  length-table entry (e.g. ActorConnected_0x01D9 reads to 55 vs table 53).
- BUG-03 (45 variable-length IDs): decoders trust the embedded frame length
  (e.g. AcAcceptLogin_0x0AC4) — remotely reachable DoS from a hostile server.

## Codegen drift root-cause (resolved this session)

The committed tree was NOT regenerable: (a) v0.9.3 (c167ca7) hand-bounded 123
slice reads without touching the emitter; (b) length tables predated the pin.
Emitter now bounds fixed-extent []byte reads (parser gained anonymous-inline-
struct tracking so posInfo[MAX_GUILDPOSITION]-style members stay open —
worklog 0093's false positive, now structural). `} name[EXPR];` lines are no
longer stripped by ExtractStructs. Tree regenerated at the pin; drift gate
green. One hand-pass bug corrected: zc_change_item_option Slot 16→8 bytes.

## BUG-02/03 resolution

Single mechanism: per-layout length guards. `layoutMinLen()` (max fixed-field
extent; flex fields contribute offset; floor 2) → `if len(data) < N { return e }`
in every generated branch; equivalent entry guards in hand-written decoders
(inventory lists ×12, 0x006B=27, 0x011F=23, 0x009E per-branch 19/17 + min 6,
0x084B=19, 0x0ADD per-branch 24/22 + min 6). Layout selection intentionally
UNCHANGED (most-recent fallback): golden tests decode legacy IDs at modern
packetvers with modern-sized frames, so guards absorb legacy-table mismatches
rather than era-pinning. Era-pinning was tried and reverted — it broke
TestActorExists_0x0078_Golden_20181121 et al.

`registerMapBurstLengths` retired from the live test: every ID it registered
already exists in lengths_map.go with identical values (0x01D7's apparent
mismatch is the table's own pv conditional). Verified by the docker-backed
live run at head.

## Verification

- `go test ./...` green; race clean; both quarantined fuzz crashers now PASS.
- CI at head: Test, Regenerate and diff (byte-identical), Live rAthena
  (docker, 2 feeds / 0 errors / stat events decoded), Nightly fuzz 11.5 min,
  AI review.

## Reviewer findings addressed (7th-flag carriers + new)

- BUG-03 resolution doc falsely claimed era-pinning → rewritten to guard-only.
- item_appeared: ITAID hoisted under min-6 guard; 0x084B/0x0ADD guarded.
- WORKLOG entry (this file) added per convention.
- `captures/` added to .gitignore.
- integration.yml readiness gate can now actually fail.
- pktmirror: SIGTERM flush, logFile closed, promised `_c2s.bin`/`_s2c.bin`
  outputs actually written.
- audit decodePanics: goroutine+channel replaced with direct recover.
