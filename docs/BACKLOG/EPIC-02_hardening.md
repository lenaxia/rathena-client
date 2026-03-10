# EPIC-02: Library Hardening — Bug Fixes, Safety, and Code Quality

**Status**: Ready for implementation  
**Created**: 2026-03-09  
**Revalidated**: 2026-03-09 (all 17 bugs verified against actual source)  
**Goal**: Eliminate all confirmed bugs found during the post-EPIC-01 architectural review,
harden the session framing engine, fix unsafe memory aliasing, correct the FSM's char-page
model, and clean up codegen output quality so the library is safe for Phase 7 (goKore
integration).

---

## Context

An architectural review conducted after EPIC-01 completion identified confirmed issues
across the library. These range from a critical infinite loop in the session framing engine,
to a use-after-reuse hazard in the decode hot path, to codegen quality problems that will
confuse any consumer of the library.

None of these issues block the current test suite (which passes), but several are silent
correctness hazards that will manifest as production bugs during Phase 7 goKore integration.
They must be fixed before Phase 7 begins.

The issues are grouped into four stories by area of concern:

```
US-11  Session framing hardening       (critical safety — fix first)
US-12  FSM correctness fixes           (protocol correctness)
US-13  Codegen output quality          (consumer-visible API quality)
US-14  Confirmed decode gaps            (field decoding correctness)
```

All four stories are independent and can proceed in parallel.

---

## Story Map

```
US-11  Session framing hardening  ─────────────────────────────────────────────┐
US-12  FSM correctness fixes      ──────────────────────────────────────────────┤
US-13  Codegen output quality     ──────────────────────────────────────────────┤
US-14  Confirmed decode gaps       ──────────────────────────────────────────────┘
                                                                                 │
                                                                                 ▼
                                                              EPIC-02 exit gate ── Phase 7 ready
```

No dependency ordering between the four stories. All can run concurrently.

---

## US-11 — Session Framing Engine Hardening

### User Story

**As a** goKore operator running the bot against a live rAthena server,  
**I want** the session framing engine to fault immediately on malformed or zero-length
variable-length packets, never spin infinitely on bad input, and not silently corrupt
decoded strings across Feed() calls,  
**so that** malformed server output produces a clean error I can log, the process does
not hang, and stored event fields are not silently overwritten.

### Problem

Three confirmed bugs in `pkg/session/session.go` make the framing engine unsafe.

**Bug 11-A — Infinite loop on zero-length variable packet**  
`pkg/session/session.go` lines 81–106

After reading `frameLen` from bytes `[2:4]` of a variable-length packet, there is no guard
against `frameLen < 4`. If a server sends a variable-length packet with `frameLen == 0`
in the embedded length field, the framing loop hangs forever:

```go
// session.go:81-106 (current)
case frameLen == -1:
    if len(c.recvBuf) < 4 { goto done }
    frameLen = int(binary.LittleEndian.Uint16(c.recvBuf[2:4]))  // becomes 0
// No guard here. Falls through to:
if len(c.recvBuf) < frameLen {  // 0 <= len (always true) — break never taken
    break
}
// dispatch fires on recvBuf[:0] — no-op
consumed += frameLen   // consumed += 0
c.recvBuf = c.recvBuf[0:]  // recvBuf unchanged
// outer loop repeats forever: Feed() never returns, CPU at 100%
```

Values `frameLen = 1, 2, 3` also cause mis-framing: the session accepts a frame shorter
than a valid packet header, advancing by 1–3 bytes and producing unpredictable dispatch
on subsequent iterations.

Verified in source: `session.go:86` reads `frameLen` with no subsequent minimum check.

**Bug 11-B — Copy-to-front skipped when `consumed == 0`**  
`pkg/session/session.go` lines 108–113

```go
done:
    if consumed > 0 {        // session.go:110
        n := copy(c.buf, c.recvBuf)
        c.recvBuf = c.buf[:n]
    }
```

When every `Feed()` call delivers a partial frame (common for large map-server inventory
dumps arriving in small TCP segments), `consumed` is always 0 and the copy-to-front is
skipped. `c.recvBuf` advances via `append` into a new backing array and gradually detaches
from `c.buf`. The "zero steady-state allocations" invariant is violated for slow streams.

Verified in source: `session.go:110` is the only copy-to-front, gated on `consumed > 0`.

**Bug 11-C — `nullTermString` aliasing hazard**  
`pkg/decode/helpers.go` line 64

```go
return unsafe.String(unsafe.SliceData(b), n)  // helpers.go:64
```

`nullTermString` returns a Go `string` that is a zero-copy alias into the session receive
buffer (`c.buf` in `sessionCore`). The aliased bytes are valid only during the handler
callback. After the handler returns, the copy-to-front at `session.go:111` calls
`copy(c.buf, c.recvBuf)`, overwriting the bytes that the `unsafe.String` points to.

Any handler that stores a string field from an event (e.g., `savedName = event.Name`)
will silently read overwritten memory on the next `Feed()` call. This is a use-after-reuse
hazard.

The existing `helpers.go:52` comment says "valid only as long as the underlying []byte is
not modified" but:
1. Nothing in the API enforces this.
2. The lifetime constraint is not documented on the `HandlerFunc` type (the contract point
   that consumers directly interact with).

### Implementation

**Fix 11-A**: Add a minimum frame length guard after reading the embedded length:

```go
case frameLen == -1:
    if len(c.recvBuf) < 4 {
        goto done
    }
    frameLen = int(binary.LittleEndian.Uint16(c.recvBuf[2:4]))
    if frameLen < 4 {
        c.faulted = true
        return ErrUnknownPacket{ID: packetID}
    }
```

The minimum valid value is 4 (2 bytes packet ID + 2 bytes length field). A server
sending a smaller embedded length is either malicious or has a framing bug; the session
should fault and force the caller to close the connection.

**Fix 11-B**: Always run the copy-to-front, not only when `consumed > 0`:

```go
done:
    n := copy(c.buf, c.recvBuf)
    c.recvBuf = c.buf[:n]
    return nil
```

This unconditionally reanchors `c.recvBuf` to `c.buf` on every `Feed()` call, preventing
unbounded backing array growth regardless of how many frames were consumed.

**Fix 11-C**: The default behavior (zero-alloc alias) is correct for handlers that do not
retain strings. The fix is documentation and a safe escape path:

1. Update the `HandlerFunc` godoc at `session.go:23` to state the lifetime constraint
   explicitly:
   ```go
   // HandlerFunc is a callback invoked synchronously by Feed() for each decoded frame.
   // ...
   // IMPORTANT: string fields in decoded events (e.g. event.Name) are zero-copy aliases
   // into the session receive buffer. They are valid only for the duration of this callback.
   // Do NOT store them past the return of HandlerFunc. To retain a string, copy it:
   //   name = decode.CopyString(event.Name)
   type HandlerFunc func(data []byte, packetver uint32)
   ```

2. Export a `CopyString` helper in `pkg/decode`:
   ```go
   // CopyString returns a heap-allocated copy of s that is safe to retain beyond
   // the HandlerFunc callback lifetime. Use this whenever any string field from a
   // decoded event must outlive the handler.
   func CopyString(s string) string { return string([]byte(s)) }
   ```

Do not change `nullTermString` to allocate by default — that would break the 0 allocs/op
benchmark requirement. The zero-alloc path remains the default.

### New tests required

Add to `pkg/session/session_test.go`:

- `TestFeed_VariableLength_ZeroEmbeddedLen_Faults`: send a variable-length packet with
  embedded length 0; verify `Feed()` returns `ErrUnknownPacket` without spinning.
- `TestFeed_VariableLength_TruncatedEmbeddedLen_Faults`: embedded length 1, 2, 3; each
  must fault.
- `TestFeed_CopyToFront_PartialFrames`: feed a 180-byte packet 1 byte at a time across
  180 `Feed()` calls; verify 0 allocations after the first few calls (use
  `testing.AllocsPerRun` on the steady-state calls) and that the backing array does not
  grow.
- `TestFeed_NullTermString_HandlerMayNotRetain`: verify that a handler storing
  `event.Name` directly sees corrupt data on the next cycle (documents the hazard), and
  that using `decode.CopyString(event.Name)` produces correct data.

### Acceptance Criteria

- [ ] `Bug 11-A`: feeding a variable-length packet with embedded length 0, 1, 2, or 3
  returns `ErrUnknownPacket` and `Feed()` returns without spinning
- [ ] `Bug 11-B`: `TestFeed_CopyToFront_PartialFrames` passes; `testing.AllocsPerRun`
  shows 0 allocations in steady state across 180 single-byte `Feed()` calls
- [ ] `Bug 11-C`: `pkg/decode/CopyString` exported helper exists with godoc; `HandlerFunc`
  godoc in `pkg/session/session.go` warns about string lifetime and references `CopyString`
- [ ] `pkg/session/session_test.go` has the four new tests; all pass
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `go test -bench=. -benchmem ./pkg/session/` still shows 0 allocs/op for
  `BenchmarkFeed_SmallFixedPacket` and `BenchmarkFeed_ActorExists_0x09FF`
- [ ] `grep -r "^\s*go " pkg/` produces empty output (zero goroutines invariant)
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us11_session_hardening.md` written

---

## US-12 — FSM Correctness Fixes

### User Story

**As a** developer integrating the ConnectionFSM into goKore (Phase 7),  
**I want** the FSM to handle multi-page char lists correctly, propagate write errors
immediately, contain no dead code that obscures intent, and close connections it owns
in all error paths,  
**so that** auth works correctly against servers that pack multiple characters per page,
network failures produce accurate error messages, and there are no resource leaks.

### Problem

Six confirmed issues in `pkg/fsm/fsm.go` were found during the architectural review.

**Bug 12-A — `pagesTotal` stores character count, not page count**  
`pkg/fsm/fsm.go` lines 449–479

`PACKET_HC_CHARLIST_NOTIFY` (0x09A0) carries the total number of **characters**, not pages.
The FSM assigns `res.pagesTotal = total` (line 455) and sends one `CH_CHARLIST_REQ`
(0x09A1) per character. The comment at line 455 acknowledges this: `// rAthena sends one
page per char in practice; we treat total == pages`.

The completion check at line 475 — `res.pagesRecv >= res.pagesTotal` — compares pages
received against characters expected. If rAthena packs more than one character per page
(e.g., 3 characters arrive in 1 `0x099D` response), `pagesRecv` will be 1 but `pagesTotal`
will be 3, so the char phase loop never terminates and blocks until `StepTimeout`.

**Important nuance**: fixing the termination model requires knowing how many characters are
in each `0x099D` response, which means parsing the `CHARACTER_INFO` struct — a
PACKETVER-dependent struct that the FSM currently treats as opaque bytes (`rawChars []byte`,
passed to `OnCharList` for goKore to parse). The conservative fix is:

- Rename `pagesTotal` → `charsExpected` (variable name fix for clarity).
- Change the termination condition to count by pages-received, not chars-expected, and
  stop when `pagesRecv > 0` and no more `0x099D` packets arrive within `StepTimeout`.
  Alternatively, trust the server: `0x09A0` includes a total and `0x099D` packets arrive
  in sequence until exhausted — use a `pagesExpected` computed from `ceil(total / pageSize)`
  once the first `0x099D` is received and page size can be inferred.

The simplest correct approach: after receiving `0x09A0`, count `0x099D` responses until
`feedStep` times out (indicating no more pages are coming), rather than tracking
`pagesTotal`. This matches rAthena's server-side behavior where the server sends all pages
without waiting for ACKs (source: `char_clif.cpp char_send_CharList`).

**Bug 12-B — Write errors discarded in 0x09A0 handler**  
`pkg/fsm/fsm.go` line 463

```go
_ = writeDeadline(conn, pkt, f.stepTimeout())  // line 463
```

The error from `writeDeadline` is unconditionally discarded. A connection failure during
multi-page charlist requests is swallowed. The FSM then blocks in `feedStep` waiting for
responses that will never arrive, eventually timing out with a "step timeout" error
instead of the true network error.

The `HandlerFunc` signature `func([]byte, uint32)` has no error return, so errors from
inside handlers cannot be returned directly. Fix via a closure-captured variable:

```go
var writeErr error
charSess.RegisterHandler(0x09A0, func(data []byte, pv uint32) {
    // ...
    for i := uint32(0); i < total; i++ {
        pkt := buildCharlistReq()
        if err := writeDeadline(conn, pkt, f.stepTimeout()); err != nil {
            writeErr = err
            return
        }
    }
})
// After feedStep returns, check writeErr before proceeding:
if writeErr != nil {
    return "", fmt.Errorf("fsm: send charlist req: %w", writeErr)
}
```

**Bug 12-C — `charDone` is dead code**  
`pkg/fsm/fsm.go` lines 483 and 499

```go
charDone := &res.done   // line 483 — assigned
// ...
_ = charDone            // line 499 — immediately suppressed
```

`charDone` is a pointer to `res.done` that is never dereferenced. The loop condition at
line 486 uses `!res.done` directly. This is a refactoring remnant.

Fix: remove both lines. The char phase loop can optionally be refactored to use
`feedUntil(conn, charSess, ...)`, matching the login phase (line 278) and map phase
(line 627). However, the char phase loop differs — it sends `CH_SELECT_CHAR` mid-loop
when `res.gotCharList && !slotSent`, which `feedUntil` does not accommodate. The manual
loop should be kept but cleaned of dead code.

**Bug 12-D — `stepTimeout()` fallback is unreachable dead code**  
`pkg/fsm/fsm.go` lines 89–91 and 166–171

`New()` at line 89 always sets `server.StepTimeout = 30 * time.Second` when zero. The
`stepTimeout()` method at line 167 still checks `if f.server.StepTimeout > 0` and has
a fallback `return 30 * time.Second` at line 170 that can never execute. Additionally,
the magic value `30 * time.Second` appears at two sites (lines 90 and 170).

Fix:
```go
const defaultStepTimeout = 30 * time.Second

// In New():
if server.StepTimeout == 0 {
    server.StepTimeout = defaultStepTimeout
}

// stepTimeout() simplified:
func (f *ConnectionFSM) stepTimeout() time.Duration {
    return f.server.StepTimeout
}
```

**Bug 12-E — `conn` leak in `runMapPhase` when `onReady` is nil**  
`pkg/fsm/fsm.go` lines 525–643

`runLoginPhase` (line 212) and `runCharPhase` (line 319) both use `defer conn.Close()`
immediately after dialing. `runMapPhase` does not, because on the happy path the conn is
intentionally transferred to goKore via `OnReady`.

However, if `f.onReady == nil`, the conn is neither closed nor handed off (lines 638–641:
the `if f.onReady != nil` guard prevents transfer, and there is no else-branch to close).
The function returns `nil` (success) with a leaked file descriptor.

Current explicit closes: lines 545, 628, 632 (error paths only — the nil-onReady happy
path has no close).

Fix: flag-guarded defer:

```go
conn, err := f.dialer(ctx, mapAddr)
if err != nil {
    return fmt.Errorf("fsm: dial map %s: %w", mapAddr, err)
}
connTransferred := false
defer func() {
    if !connTransferred {
        conn.Close()
    }
}()
```

And at the hand-off:
```go
connTransferred = true
f.onReady(mapSess, conn)
```

This covers all error paths and the nil-onReady case. The three explicit `conn.Close()`
calls on lines 545, 628, and 632 can then be removed (the defer covers them), simplifying
control flow.

**Bug 12-F — `CharServerInfo.IP` byte-order not documented**  
`pkg/fsm/fsm.go` — `CharServerInfo` struct definition (line 50)

The `IP uint32` field in `CharServerInfo` stores the IPv4 address in **big-endian (network)
byte order** — confirmed by `binary.BigEndian.Uint32` at lines 374 and 407, and the
byte-shift formatting at lines 187 and 506–508. This is not documented on the struct.

A consumer who reads `charInfo.IP` and uses it directly as a native integer will get wrong
addresses. The FSM's own formatting code (`chosen.IP>>24, (chosen.IP>>16)&0xFF, ...` at
line 187) demonstrates the correct usage but is not exposed as a documented pattern.

Fix: add godoc to `CharServerInfo.IP` and add a formatting note:

```go
// IP is the char server's IPv4 address in big-endian (network) byte order,
// as written by rAthena's htonl() call.
// To format as a dotted-decimal string:
//   fmt.Sprintf("%d.%d.%d.%d", IP>>24, (IP>>16)&0xFF, (IP>>8)&0xFF, IP&0xFF)
IP uint32
```

### New tests required

Add to `pkg/fsm/fsm_test.go`:

- `TestConnect_WriteError_In09A0Handler`: stub sends `0x09A0` with `total=2`; on the
  first `0x09A1` write from the FSM, the stub closes the pipe; verify `Connect()` returns
  a network error, not a step timeout.
- `TestConnect_ContextCancelled`: pass an already-cancelled context to `Connect()`; verify
  it returns a context error before dialing.
- `TestConnect_OnReady_Nil_ConnClosed`: FSM with no `OnReady` registered; after map auth
  completes, verify the conn is closed (the scripted server's read end gets an EOF).

### Acceptance Criteria

- [ ] `Bug 12-A`: `pagesTotal` renamed and completion logic updated to use a correct
  termination model; inline comment explains the model; existing tests pass
- [ ] `Bug 12-B`: write error in `0x09A0` handler propagated correctly; new test
  `TestConnect_WriteError_In09A0Handler` passes
- [ ] `Bug 12-C`: `charDone` dead code (both lines 483 and 499) removed; no compilation
  errors
- [ ] `Bug 12-D`: `defaultStepTimeout` named constant introduced; `stepTimeout()` simplified
  to a direct field return; `30 * time.Second` appears exactly once in `fsm.go`
- [ ] `Bug 12-E`: flag-guarded defer in `runMapPhase`; three explicit `conn.Close()` error
  path calls removed; new test `TestConnect_OnReady_Nil_ConnClosed` passes
- [ ] `Bug 12-F`: `CharServerInfo.IP` godoc updated with byte-order explanation and
  formatting example
- [ ] All existing `pkg/fsm/` tests pass
- [ ] `go test -race ./pkg/fsm/` passes
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us12_fsm_fixes.md` written

---

## US-13 — Codegen Output Quality

### User Story

**As a** developer consuming `pkg/events` and `pkg/encode` in goKore (Phase 7),  
**I want** generated structs to follow Go naming conventions, have accurate field comments,
and have stubs that fail loudly rather than silently,  
**so that** I can write idiomatic Go against the API, am not misled by wrong documentation,
and get immediate feedback when I call an unimplemented function.

### Problem

Five confirmed quality issues in codegen output make the generated API misleading or
non-idiomatic. These affect every consumer of `pkg/events` and `pkg/encode`.

**Bug 13-A — `MoveData` comment says "direction"**  
`pkg/events/actor_moved.go` line 31 (and all `actor_moved_*.go` files)

```go
MoveData [6]byte // Packed movement coordinates (6 bytes: from_x, from_y, to_x, to_y, direction)
```

The 6-byte `WBUFPOS2` format does **not** encode direction. Byte 5 encodes `sx0` (high
nibble) and `sy0` (low nibble) — sub-cell interpolation offsets. This is explicitly
documented in `pkg/packing/packing.go:64-65`: "byte 5 is NOT a direction value. The 6-byte
format encodes no direction. Extracting (data[5] & 0xF0) >> 4 as direction is incorrect
(this is a known bug in goKore v1 that this library fixes)."

The generated comment directly re-introduces the goKore v1 bug in documentation.

Fix: update the codegen template for `MoveData [6]byte` fields to emit the correct comment:
```go
MoveData [6]byte // Packed movement data (6 bytes: from_x, from_y, to_x, to_y, sx0, sy0). Call packing.DecodeMoveData to unpack.
```

**Bug 13-B — Scalar fields documented as "may be nil"**  
`pkg/events/game_login.go` lines 8–12

```go
AccountID uint32 // Account ID from login server (may be nil for certain packet variants)
SessionID uint32 // Session ID 1 (login_id1 from 0x0AC4, may be nil)
// ... etc.
```

In Go, `uint32`, `uint16`, and `uint8` cannot be nil. `nil` is only meaningful for pointer,
slice, map, channel, interface, and function types. A consumer nil-checking these fields
will get a compilation error. The comment is copied from a C-nullable context and is
factually wrong.

Fix: update the codegen template to emit "zero if absent" instead of "may be nil" for
scalar (non-pointer) fields.

**Bug 13-C — Mixed `snake_case`/`PascalCase` in generated structs**  
Confirmed in: `pkg/events/actor_moved.go`, `pkg/events/actor_exists.go`,
`pkg/events/actor_connected.go`, `pkg/events/char_create.go`, `pkg/send/char_create.go`
(529 total occurrences across all generated files).

Examples from `pkg/events/actor_moved.go`:
```go
Clothes_color uint16  // violates Go naming conventions
Hair_style    uint16
Head_dir      uint16
Hair_color    uint16
Walk_speed    uint16
Object_type   uint8
```

All other fields in the same structs use `PascalCase` (`Lowhead`, `Tophead`, `EmblemID`,
etc.), making the API inconsistent. `golint`/`staticcheck` flag these with `ST1003`.

Fix: update the field name normalization in the codegen template to convert `under_score`
names to `PascalCase` for all generated exported fields.

The affected canonical names (non-exhaustive — verify full list from the grep output):

| Current (wrong) | Correct |
|---|---|
| `Clothes_color` | `ClothesColor` |
| `Hair_style` | `HairStyle` |
| `Head_dir` | `HeadDir` |
| `Hair_color` | `HairColor` |
| `Walk_speed` | `WalkSpeed` |
| `Object_type` | `ObjectType` |

This is a **breaking API change**. Since Phase 7 (goKore integration) has not yet started,
there are no external consumers. Make the fix now, before any downstream code exists.

After renaming, search the entire repository for references to the old field names and
update them. The SemanticDB (`mappings.yaml`) stores the OpenKore/canonical names
separately from Go names — no MCP changes are needed for this bug.

**Bug 13-D — `EncodeGameLogin` returns `nil` unconditionally**  
`pkg/encode/game_login.go` lines 9–13

```go
func EncodeGameLogin(req send.GameLogin, packetver uint32) []byte {
    switch {
    }
    return nil
}
```

The `switch {}` body is completely empty. The function always returns `nil`, silently
producing no bytes. This is a codegen stub that was never populated. A consumer calling
`EncodeGameLogin` receives `nil` and sends nothing — no error, no indication of failure.

The full implementation requires encoding `CA_LOGIN` (the C→S login packet), which is
out of scope for this epic (deferred to Phase 7). As a minimum fix, make the stub loud:

```go
func EncodeGameLogin(req send.GameLogin, packetver uint32) []byte {
    panic("EncodeGameLogin: not implemented — see docs/BACKLOG/EPIC-02_hardening.md US-13")
}
```

**Bug 13-E — No `//go:generate` directives**

There are no `//go:generate` directives anywhere in the repository (confirmed: `grep
-rn "go:generate" . --include="*.go"` produces no output). All generated files in
`pkg/events/`, `pkg/decode/`, `pkg/encode/`, `pkg/session/lengths_*.go`,
`pkg/session/shuffle_map.go`, and `pkg/session/obfuscation_keys.go` have no `go generate`
integration.

A developer who checks out the repository has no way to know which files are generated,
how to regenerate them, or what command to run. This is a Phase 7 onboarding blocker.

Fix: add a `//go:generate` directive, e.g. in `internal/codegen/doc.go` (create if it
does not exist) or in a `tools.go` at the repository root:

```go
//go:generate go run ./internal/codegen/main.go --rathena ~/personal/rathena --out .
```

Also verify that each generated file's package header already contains a `// Code generated
by internal/codegen. DO NOT EDIT.` marker (confirmed present in existing generated files).

### Implementation notes

For Bugs 13-A, 13-B, 13-C, 13-E: fix the template in `internal/codegen/gen/`, re-run
codegen, verify output, run `go build ./...` and `go test ./...`.

For Bug 13-C (field rename): after the template fix and codegen re-run, also search for
all existing references to old field names and update them. Run `staticcheck ./pkg/events/...`
to confirm no ST1003 warnings remain.

For Bug 13-D: one-line change in the generated file is acceptable since the function is
already broken; also fix the template so regeneration doesn't revert it.

### Acceptance Criteria

- [ ] `Bug 13-A`: every `MoveData [6]byte` field in `pkg/events/` has the corrected comment;
  re-running codegen produces the correct comment (no "direction")
- [ ] `Bug 13-B`: no generated event struct field comment contains "may be nil" for a
  scalar type; scalars use "zero if absent" language
- [ ] `Bug 13-C`: `Clothes_color`, `Hair_style`, `Head_dir`, `Hair_color`, `Walk_speed`,
  `Object_type` (and any other `under_score` names) renamed to `PascalCase` in all generated
  files and all callers; `grep -rn "Clothes_color\|Hair_style\|Head_dir\|Hair_color\|Walk_speed\|Object_type" .`
  produces empty output
- [ ] `Bug 13-D`: `EncodeGameLogin` panics with a clear "not implemented" message; `go vet`
  still passes
- [ ] `Bug 13-E`: `//go:generate go run ./internal/codegen/main.go ...` directive exists in
  the repository; `go generate ./...` runs the codegen without error
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] `staticcheck ./pkg/events/...` produces no `ST1003` warnings
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us13_codegen_quality.md` written

---

## US-14 — Confirmed Decode Gaps

### User Story

**As a** goKore bot operator,  
**I want** actor names, HP, and guild position data to be correctly decoded from received
packets, and encoded movement data to be bit-correct,  
**so that** mob/NPC names are visible in game state, HP tracking works, guild UI functions,
and movement packets sent to the server are not silently corrupted by a missing mask.

### Problem

Four confirmed bugs cause specific fields to always decode as zero/empty regardless of
the actual wire bytes, or encode incorrectly.

**Bug 14-A — `ActorExists_0x0078`: `Name` and `HP` always empty/zero**  
`pkg/decode/gaps_test.go` lines 57–84; `pkg/decode/actor_exists.go`

The SemanticDB maps `Name: string("")` for all `0x0078` implementations regardless of
PACKETVER. The generated decode function never reads the name field from the wire bytes.
Actor names are always `""` when using the legacy `0x0078` packet.

HP is also always `0` — the SemanticDB maps `HP: int32(0)`.

GCC-verified: `packet_idle_unit` at PACKETVER >= 20181121 has `name char[24]` at offset
84 and `HP int32` at offset 77. These fields exist in the wire format; the decode
function simply never reads them.

Root cause: the SemanticDB `field_mapping` for `actor_exists` → `0x0078` has static
literal values (`string("")`, `int32(0)`) instead of packet field expressions.

`TestGap_0x0078_Name_AlwaysEmpty` in `gaps_test.go` proves this bug but uses `t.Logf`
(not `t.Fatal`) so the test passes even with the bug present.

**Bug 14-B — `ActorMoved_0x09DB`: `Name` always empty**  
`pkg/decode/gaps_test.go` lines 121–148; `pkg/decode/actor_moved.go`

The generated decode function for `ActorMoved_0x09DB` skips `Name` because the SemanticDB
entry is marked with a complex expression: `strings.TrimRight(string(packet.name), "\x00")`.
The codegen skips complex expressions and emits a comment; `Name` is always `""`.

GCC-verified: `packet_unit_walking` at PACKETVER >= 20181121 has `name char[24]` at
offset 90.

`TestGap_0x09DB_Name_AlwaysEmpty` uses `t.Logf` (not `t.Fatal`).

**Bug 14-C — `ZcPositionIdNameInfo_0x0166`: `PosInfo` always nil**  
`pkg/decode/gaps_test.go` lines 210–236; `pkg/decode/zc_position_id_name_info.go`

The SemanticDB maps `PosInfo: "data[4:]"` — a slice expression. The codegen skips it and
emits a comment. `PosInfo` (guild position name data at offset 4) is silently discarded.

`TestGap_ZcPositionIdNameInfo_PosInfo_IsNil` uses `t.Logf("CONFIRMED GAP: ...")`, not
`t.Fatal`.

**Bug 14-D — `EncodeMoveData` missing `sx0 & 0x0f` mask**  
`pkg/packing/packing.go` line 88

```go
p[5] = uint8((sx0 << 4) | (sy0 & 0x0f))  // packing.go:88
```

`sy0` is correctly masked with `& 0x0f` before use. `sx0` has no mask before shifting.
If `sx0 > 15` (i.e., bit 4 or higher is set), the `uint8` truncation of `(sx0 << 4)`
produces wrong results. Example: `sx0 = 0x10` → `uint8(0x10 << 4)` = `uint8(256)` =
`0x00` — the field is silently zeroed.

The docstring at `packing.go:80` says "sx0 and sy0 must be 4-bit values (0–15); upper
bits are masked off" — the second clause is **false**: upper bits of `sx0` are NOT masked
off in the implementation. The docstring makes a promise the code does not keep.

This is a latent bug: rAthena's `sx0`/`sy0` are interpolation sub-cell values (0–15 in
normal usage), so the bug rarely triggers in practice. But it remains a correctness error
when inputs are out of range.

Fix:
```go
p[5] = uint8(((sx0 & 0x0f) << 4) | (sy0 & 0x0f))
```

Also update the docstring to state the actual behavior: "if sx0 or sy0 exceed 15, the
upper bits are masked off before encoding."

### Implementation

**For Bugs 14-A, 14-B**: follow the documented workflow:

1. Query SemanticDB via MCP (`semantics_get`, `semantics_list_fields`) for the packet and
   verify the current (wrong) field mapping.
2. Run GCC preprocessor to verify the correct rAthena field name and offset.
3. Update the SemanticDB `field_mapping` via MCP (`semantics_update_field_mapping`).
4. Re-run codegen.
5. Verify the generated decode function reads the field correctly.
6. Convert the `TestGap_*` tests to regression assertions: replace `t.Logf("CONFIRMED BUG: ...")`
   with `t.Fatal(...)` when the field is empty. Remove the "CONFIRMED BUG" / "CONFIRMED"
   log lines that document the gap.

**For Bug 14-B specifically**: the complex expression `strings.TrimRight(...)` should be
simplified to a direct field read (`string(packet.name[:n])` where `n` is the null-term
index) or to `nullTermString(data[offset:offset+24])`. Update the SemanticDB to use the
simpler expression if the codegen can handle it, or implement the decode manually if not.

**For Bug 14-C**: implement the `ZcPositionIdNameInfo_0x0166` decode function manually:

1. GCC-verify the struct layout for `PACKET_ZC_POSITION_ID_NAME_INFO` at relevant PACKETVERs.
2. Implement a hand-written decode function that reads `PosInfo` from `data[4:]`.
3. Convert `TestGap_ZcPositionIdNameInfo_PosInfo_IsNil` to a regression assertion.

**For Bug 14-D**: one-line fix in `packing.go:88`. Add a fuzz test covering `sx0` values
in range 16–31 to verify round-trip correctness.

### gaps_test.go disposition

The EPIC-02 exit criterion uses `grep -n "CONFIRMED BUG"` — but only Bug 14-A uses that
exact string. Bugs 14-B and 14-C use `"CONFIRMED:"` and `"CONFIRMED GAP:"` respectively.
The exit criterion must be broadened:

```bash
grep -n "CONFIRMED BUG\|CONFIRMED GAP\|CONFIRMED:" pkg/decode/gaps_test.go
```

After fixing all three, replace the gap-documenting `t.Logf` calls with regression
assertions:

```go
// Bug 14-A: was t.Logf("CONFIRMED BUG: ..."). Now:
if e.Name != "Poring" {
    t.Fatalf("ActorExists_0x0078 Name: got %q want %q — decode regression", e.Name, "Poring")
}
// Remove the if e.Name != "" block that documented the bug.
```

The `TestGap_0x09FF_Name_IsDecoded` and `TestGap_AddExchangeItem_Grade_20200401_IsZero`
tests document correct behavior, not bugs — rename to drop the "Gap" prefix and keep as-is.

### Acceptance Criteria

- [ ] `Bug 14-A`: `ActorExists_0x0078` decodes `Name` and `HP` correctly; `gaps_test.go`
  test is converted to a regression assertion (`t.Fatal` if `Name == ""` or `HP == 0`)
- [ ] `Bug 14-B`: `ActorMoved_0x09DB` decodes `Name` correctly; `gaps_test.go` regression
  test updated
- [ ] `Bug 14-C`: `ZcPositionIdNameInfo_0x0166` decodes `PosInfo` correctly; `gaps_test.go`
  regression test added
- [ ] `Bug 14-D`: `EncodeMoveData` line 88 uses `(sx0 & 0x0f) << 4`; docstring updated;
  fuzz test `FuzzEncodeMoveData_Sx0OutOfRange` added; round-trip for `sx0 = 16..31` is correct
- [ ] `grep -n "CONFIRMED BUG\|CONFIRMED GAP\|CONFIRMED:" pkg/decode/gaps_test.go` produces
  empty output (all confirmed gaps resolved and test language updated)
- [ ] `go test ./pkg/decode/` passes
- [ ] `go test ./pkg/packing/` passes
- [ ] `go test -fuzz=FuzzEncodeMoveData_Sx0OutOfRange -fuzztime=30s ./pkg/packing/` exits 0
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] GCC commands used and relevant struct output documented in worklog
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us14_decode_gaps.md` written

---

## Exit Criteria for EPIC-02

EPIC-02 is complete when all of the following are true:

1. `go build ./...` — clean
2. `go test ./...` — all pass, including the new tests from US-11, US-12, US-13, US-14
3. `go test -race ./pkg/...` — no data races
4. `go test -bench=. -benchmem ./pkg/...` — 0 allocs/op for all Phase 1 decode and
   Feed benchmarks (US-11 must not introduce any)
5. `grep -r "^\s*go " pkg/` — empty output (zero goroutines invariant)
6. `grep -n "CONFIRMED BUG\|CONFIRMED GAP\|CONFIRMED:" pkg/decode/gaps_test.go` — empty
   output (all confirmed decode gaps resolved and test assertions updated)
7. `grep -rn "Clothes_color\|Hair_style\|Head_dir\|Hair_color\|Walk_speed\|Object_type" .` —
   empty output (all snake_case field names renamed — verify full list from grep before
   starting work)
8. `grep -rn "may be nil" pkg/events/` — empty output (scalar nil comments removed)
9. `staticcheck ./pkg/events/...` — no `ST1003` naming warnings
10. `go generate ./...` — runs codegen without error
11. Worklogs written for all four stories (US-11 through US-14)

---

## What This Epic Does NOT Cover

These are explicitly deferred:

- Full `EncodeGameLogin` (CA_LOGIN) implementation — deferred to Phase 7 scope
- SemanticDB validation errors (306 known) — tracked separately via `semantics_validate`;
  only the decode-gap bugs (14-A, 14-B, 14-C) require SemanticDB changes in this epic
- `SetLength` public API concern — `SetLength` is used by the FSM itself; moving it to a
  test-only API requires restructuring and is deferred to Phase 7
- Homunculus and mercenary packet type truncation bugs — explicitly out of scope per
  README-LLM.md
- Phase 7 goKore integration — begins after EPIC-02 exit criteria pass
- Bug 12-A full fix requiring CHARACTER_INFO struct parsing to count characters per page —
  the conservative termination model (timeout-based) is sufficient for this epic
