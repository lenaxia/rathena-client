# EPIC-01: Integration Tests — Live Server and Mock Server Replay

**Status**: Ready for implementation
**Created**: 2026-03-09
**Goal**: Prove the FSM and session layer work correctly against real rAthena wire bytes,
both by connecting to a live server and by replaying captured traffic deterministically
offline.

---

## Context

EPIC-00 explicitly deferred integration testing against a real server (see EPIC-00
"What This Epic Does NOT Cover"). All existing tests use `net.Pipe` stubs with
hand-crafted scripted responses. These stubs prove the FSM's control flow but cannot
catch:

- Incorrect packet framing on the real `packetver=20200401` wire format
- Wrong C→S byte layout (e.g. `CA_LOGIN` 0x0064 field ordering)
- Obfuscation key mismatches on the map server
- S→C packets the stub scripts don't cover (PIN code, ban list, extra char pages, etc.)
- `Feed()` faulting on an unknown packet ID received in the real session

A live rAthena server is already running at `127.0.0.1:6900` (Docker container
`rathena-renewal`, image `ghcr.io/lenaxia/docker-rathena:packetver20200401-renewal-20251223`,
ports 6900/6121/5121/8888 bound to host). Credentials: `botijo1` / `Melon.77`, char slot 0.

Real wire captures exist in `~/personal/goKore/docs/03_REFERENCE/dumps/` (17 files,
OpenKore hex-dump format, capturing full sessions at packetver 20200401).
DUMP1 captures against `192.168.5.105`; DUMP8_movement captures against `127.0.0.1`.
These provide ground-truth byte sequences for offline replay.

---

## Story Map

```
US-10  Fix S→C lengths pipeline   ──► eliminates all 34 SetLength calls from fsm.go
  │                                     (EPIC-00 story, prerequisite for US-09)
  │
  ├──► US-08  Live server test (Tier 1)  ──► runs against 127.0.0.1:6900
  │                                           requires Docker container up
  │                                           (already passes; US-10 makes it cleaner)
  │
  └──► US-09  Mock server replay (Tier 2) ──► offline, deterministic, no Docker needed
                                               derived from DUMP1 + DUMP8_movement
                                               requires US-10: replay test must not
                                               reproduce SetLength workarounds
```

**US-10 must complete before US-09.**  
US-08 is already implemented and passing; US-10 improves it by removing the 34
`SetLength` calls that the live test currently relies on via `fsm.go`.

---

## US-08 — Live Server Integration Test

### Problem

There is no test that runs `ConnectionFSM.Connect()` against a real rAthena server.
The `net.Pipe` stubs in `pkg/fsm/fsm_test.go` script exact responses and never send
bytes to a real TCP socket. This means framing bugs, wrong C→S packet layouts,
unexpected S→C packets, and obfuscation errors are invisible until goKore integration.

### What to test

The test must complete the full login → char select → map entry sequence using the
live server and then verify a minimal steady-state: that `MapSession.Feed()` can
accept at least one real S→C packet from the server without faulting.

Specifically:
1. `ConnectionFSM.Connect()` reaches `OnReady` without returning an error.
2. Inside `OnReady`, the test reads from the `net.Conn` and calls
   `mapSession.Feed()` in a tight loop for up to 5 seconds, collecting any events
   that fire via registered handlers.
3. After the first S→C packet with an unknown length, `MapSession.Feed()` returns
   `ErrUnknownPacket` exactly once. After that, the session is permanently faulted:
   all subsequent `Feed()` calls return `nil` and silently discard all bytes —
   no handlers ever fire again. This means any unknown-length S→C packet that
   arrives before `actor_exists` or `stat_update` will permanently prevent those
   events from firing. Use `mapSess.SetLength(id, n)` inside `OnReady` to
   pre-register lengths for any IDs seen in the dumps but absent from
   `lengths_map.go`. Document each call with packet name and verified size.
4. The test asserts that at least one of the following events fires during the 5-second
   window (the server sends these immediately on map entry):
   - `actor_exists` (0x09FF / 0x0078) — nearby actors on the map
   - `stat_update` (0x00B0 / 0x00B1) — character stats
5. The test asserts the decoded event fields are non-zero (i.e. the decode path
   produced real values, not all zeros from skipped fields).

### Credentials and server address

| Parameter | Value |
|---|---|
| Login server | `127.0.0.1:6900` |
| PACKETVER | `20200401` |
| Username | `botijo1` |
| Password | `Melon.77` |
| Char slot | `0` |

These values must be overridable via environment variables
(`RATHENA_ADDR`, `RATHENA_PACKETVER`, `RATHENA_USER`, `RATHENA_PASS`,
`RATHENA_CHARSLOT`) so the test works in CI with a different server.

### Implementation

**File**: `pkg/fsm/live_integration_test.go`  
**Build tag**: `//go:build integration`  
**Run command**: `go test -tags integration -timeout 60s -v ./pkg/fsm/`

Structure:

```go
//go:build integration

package fsm_test

import (
    "context"
    "net"
    "os"
    "strconv"
    "testing"
    "time"

    "github.com/lenaxia/ragnarok-go-client/pkg/decode"
    "github.com/lenaxia/ragnarok-go-client/pkg/fsm"
    "github.com/lenaxia/ragnarok-go-client/pkg/session"
)

func TestLiveServer_FullAuthSequence(t *testing.T) {
    addr      := envOrDefault("RATHENA_ADDR",      "127.0.0.1:6900")
    pverStr   := envOrDefault("RATHENA_PACKETVER", "20200401")
    user      := envOrDefault("RATHENA_USER",      "botijo1")
    pass      := envOrDefault("RATHENA_PASS",      "Melon.77")
    slotStr   := envOrDefault("RATHENA_CHARSLOT",  "0")

    pver, _ := strconv.ParseUint(pverStr, 10, 32)
    slot, _ := strconv.ParseUint(slotStr, 10, 8)

    dialer := func(ctx context.Context, a string) (net.Conn, error) {
        d := &net.Dialer{Timeout: 10 * time.Second}
        return d.DialContext(ctx, "tcp", a)
    }

    server := fsm.ServerConfig{
        LoginAddr:   addr,
        Packetver:   uint32(pver),
        StepTimeout: 15 * time.Second,
    }
    creds := fsm.Credentials{
        Username: user,
        Password: pass,
        CharSlot: uint8(slot),
    }

    type result struct {
        mapSess *session.MapSession
        conn    net.Conn
    }
    ready := make(chan result, 1)

    f := fsm.New(server, creds, dialer).
        OnCharServerList(func(_ []fsm.CharServerInfo) int { return 0 }).
        OnCharList(func(_ []byte) uint8 { return uint8(slot) }).
        OnReady(func(s *session.MapSession, c net.Conn) {
            ready <- result{s, c}
        }).
        OnFailed(func(err error) {
            t.Errorf("OnFailed: %v", err)
        })

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Connect() blocks until OnReady fires or an error occurs.
    if err := f.Connect(ctx); err != nil {
        t.Fatalf("Connect: %v", err)
    }

    r := <-ready

    // Register handlers for core map-entry packets.
    var gotActorExists, gotStatUpdate bool
    r.mapSess.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
        e := decode.ActorExists_0x09FF(data, pv)
        if e.ID != 0 { gotActorExists = true }
    })
    r.mapSess.RegisterHandler(0x0078, func(data []byte, pv uint32) {
        e := decode.ActorExists_0x0078(data, pv)
        if e.ID != 0 { gotActorExists = true }
    })
    r.mapSess.RegisterHandler(0x00B0, func(data []byte, pv uint32) {
        e := decode.StatUpdate_0x00B0(data, pv)
        if e.StatType != 0 || e.Value != 0 { gotStatUpdate = true }
    })
    r.mapSess.RegisterHandler(0x00B1, func(data []byte, pv uint32) {
        _ = decode.StatUpdate_0x00B1(data, pv)
        gotStatUpdate = true
    })

    // Drain the connection for up to 5 seconds.
    buf := make([]byte, 4096)
    deadline := time.Now().Add(5 * time.Second)
    r.conn.SetDeadline(deadline)
    for time.Now().Before(deadline) {
        n, err := r.conn.Read(buf)
        if n > 0 {
            if feedErr := r.mapSess.Feed(buf[:n]); feedErr != nil {
                t.Errorf("Feed error: %v", feedErr)
                break
            }
        }
        if err != nil { break }
    }
    r.conn.Close()

    if !gotActorExists && !gotStatUpdate {
        t.Error("no actor_exists or stat_update event fired in 5-second window")
    }
}

func envOrDefault(key, def string) string {
    if v := os.Getenv(key); v != "" { return v }
    return def
}
```

**Known gap to address before this test can pass**: `MapSession` will permanently
fault on the first S→C packet whose length is absent from `lengths_map.go`.
After the fault, `Feed()` returns `nil` silently and no handlers ever fire —
making the `gotActorExists || gotStatUpdate` assertion impossible to satisfy.
Use `mapSess.SetLength(id, n)` inside `OnReady` to pre-register lengths for any
map-burst packet IDs absent from `lengths_map.go`. Document each call with
packet name and verified size. See the Shared pre-condition section for the
full design decision.

### Acceptance Criteria

- [ ] `pkg/fsm/live_integration_test.go` exists with `//go:build integration` tag
- [ ] `go test -tags integration -timeout 60s -v ./pkg/fsm/ -run TestLiveServer_FullAuthSequence`
  passes with the Docker container running
- [ ] Test prints which events fired and how many `Feed()` calls succeeded
- [ ] Test does not fault on any S→C packet during the 5-second window
      (all unknown lengths pre-registered via `SetLength` or permissive mode chosen —
      whichever approach is documented in the worklog)
- [ ] At least one `actor_exists` or `stat_update` event fires with a non-zero field
- [ ] All `SetLength` workarounds are commented with packet name and GCC-verified size
- [ ] Test is skipped (not failed) when the Docker container is not reachable:
  ```go
  conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
  if err != nil { t.Skipf("rAthena server not reachable at %s: %v", addr, err) }
  conn.Close()
  ```
- [ ] `go test ./pkg/fsm/` (without `-tags integration`) still passes — no regressions
- [ ] `go build ./...` passes
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us08_live_server.md` written

---

## US-09 — Mock Server Replay from Captured Dumps

### Problem

US-08 requires the Docker container to be up. In CI, on a new machine, or after a
server reset, the live test cannot run. There is no offline, deterministic, zero-flake
way to test the full auth + map-entry sequence using real rAthena bytes.

### What exists

`~/personal/goKore/docs/03_REFERENCE/dumps/DUMP1` and `DUMP8_movement` contain
real wire captures of complete sessions at packetver `20200401` against a real
rAthena server (`192.168.5.105`). Format is OpenKore hex-dump text:

```
>> Sent packet: XXXX  [Name] [N bytes]   YYYY.MM.DD HH:MM:SS
  0>  HH HH HH HH ...
<< Received packet:      XXXX - Name [N bytes]   YYYY.MM.DD HH:MM:SS
  0>  HH HH HH HH ...
```

DUMP1 covers: 0x0064 sent → 0x0AC4 recv → 0x0065 sent →
0x082D/0x006B/0x09A0 recv → 0x09A1 ×3 sent → 0x020D/0x08B9 recv →
0x099D ×3 recv (HC_ACK_CHARINFO_PER_PAGE, 159 bytes each) →
0x0066 sent (CH_SELECT_CHAR) → 0x0AC5 recv (HC_NOTIFY_ZONESVR, 156 bytes) →
0x0436 sent → map entry.

DUMP8_movement covers: same auth sequence + map entry + 118 total packets
(Sent + Received combined). After the map-entry point (0x0436 sent), there are
101 additional packets covering actor movement, stat updates, and actor
visibility.

### Design

**A fixture file format** (binary, `.fixture` extension) stores the captured S→C
bytes per phase, in order, with no parsing metadata — just the raw bytes as they
would arrive on the socket, phase-separated:

```
pkg/fsm/testdata/
    auth_20200401.fixture       # login + char + map entry S→C bytes
    movement_20200401.fixture   # same auth + actor/movement S→C bytes
```

Each `.fixture` file is a concatenated sequence of phase blocks:

```
[4 bytes raw: 0x52 0x41 0x54 0x46  // "RATF" in ASCII order on disk]
[4 bytes: uint32 LE version = 1]
[4 bytes: uint32 LE packetver = 20200401]
[repeated phase blocks:]
  [1 byte: phase tag  0x01=login 0x02=char 0x03=map]
  [4 bytes: uint32 LE byte count N]
  [N bytes: raw S→C bytes for this phase]
[4 bytes raw: 0x45 0x4E 0x44 0x20  // "END " in ASCII order on disk]
```

C→S bytes (what the FSM sends) are **not stored** — the fixture only contains
server responses. The fixture server reads and validates C→S bytes from the FSM
but tolerates minor differences (e.g. different session tokens) by matching on
packet ID only, not full content.

**A fixture generator** (`cmd/gen-fixture/main.go`) parses the OpenKore dump
format and writes a `.fixture` file. Run once; output is committed to the repo.

**A `ScriptedServer`** (in `pkg/fsm/internal/testserver/` or
`pkg/fsm/scriptedserver_test.go`) that:
1. Holds a `.fixture` file pre-loaded into memory as three byte slices (login, char, map).
2. For each phase: listens on a `net.Pipe`, reads C→S bytes from the FSM (validates
   packet ID byte 0:2, discards content), then writes the S→C bytes from the fixture
   in 1–4096 byte chunks (randomly sized, seeded from `testing.T` name to be
   deterministic).
3. After all S→C bytes for the phase are written, closes the phase connection and
   advances to the next.

The `ScriptedServer` replaces `scriptedDialer` in `fsm_test.go` for these specific
tests. Existing `fsm_test.go` tests are unchanged.

**The test** (`pkg/fsm/replay_test.go`, no build tag — runs in
normal `go test`) drives `ConnectionFSM.Connect()` against the `ScriptedServer`
using the `auth_20200401.fixture` file and asserts:
1. `OnReady` fires (full auth sequence completes).
2. `Feed()` does not fault during the S→C bytes of the map-entry phase.
3. At least one `actor_exists` or `stat_update` event fires from map-entry packets
   in the fixture.

A second test uses `movement_20200401.fixture` and asserts actor movement events fire.

### Fixture extraction — what to parse from dumps

From `DUMP1` (auth sequence, all three phases):

**Login phase S→C** (bytes the server sends after receiving 0x0064):
- `0x0AC4` — 224 bytes (account info + char server list)

**Char phase S→C** (bytes the server sends after receiving 0x0065 + 0x09A1×3):
- `0x082D` — 29 bytes (char slot info)
- `0x006B` — 182 bytes (char list)
- `0x09A0` — 6 bytes (char pages notify)
- `0x020D` — 4 bytes (ban list, empty)
- `0x08B9` — 12 bytes (pin code request)
- (remaining packets until char select accepted and map redirect received)

**Map phase S→C** (bytes the server sends after receiving 0x0436):
- `0x0283` — ZC_AID (account ID echo), 6 bytes
- `0x0B18` — ZC_INVENTORY_EXPANSION_INFO, 4 bytes (arrives before ZC_ACCEPT_ENTER)
- `0x02EB` — ZC_ACCEPT_ENTER2 (for packetver 20200401), 13 bytes
- Initial burst of S→C packets (stats, actor visibility, etc.)

From `DUMP8_movement` (same auth + movement):
- Same auth sequence bytes
- Additional map-phase bytes: actor exists/moved/vanished packets, stat updates

The fixture generator must:
1. Parse the hex dump line by line.
2. For each `<< Received packet` block: decode the hex bytes and append to the
   current phase's byte buffer.
3. Phase transitions happen on C→S boundary packets (these are NOT stored in the
   fixture — the fixture contains S→C bytes only):
   - `>> Sent 0x0065` (CH_ENTER) → transition login→char phase
   - `>> Sent 0x0436` (CZ_ENTER) → transition char→map phase
   `0x0066` (CH_SELECT_CHAR) is **not** a phase boundary; it is sent mid-char-phase
   and triggers `0x0AC5` (HC_NOTIFY_ZONESVR) which is captured in the char phase
   S→C buffer. Key char phase S→C packets include `0x099D` ×3
   (HC_ACK_CHARINFO_PER_PAGE) and `0x0AC5` (HC_NOTIFY_ZONESVR).
4. Write the `.fixture` file.

### Implementation plan

1. `cmd/gen-fixture/main.go` — dump parser + fixture writer (pure I/O, no network)
2. Run it against DUMP1 and DUMP8_movement; commit the generated `.fixture` files
3. `pkg/fsm/testdata/` — committed fixture files
4. `pkg/fsm/scriptedserver_test.go` — `ScriptedServer` using `net.Pipe`
5. `pkg/fsm/replay_test.go` — tests using `ScriptedServer` + fixture files (no build tag)

### Acceptance Criteria

- [ ] `cmd/gen-fixture/main.go` parses DUMP1 correctly; extracted S→C byte sequences
  verified by hand for at least `0x0AC4`, `0x006B`, `0x0283`
- [ ] `pkg/fsm/testdata/auth_20200401.fixture` and `movement_20200401.fixture`
  committed with the fixture file magic header intact
- [ ] `ScriptedServer` correctly replays each phase in order, with random-chunk
  splitting seeded deterministically from the test name
- [ ] `TestReplay_FullAuth_20200401` passes with no build tags (runs in normal
  `go test ./pkg/fsm/`)
- [ ] `TestReplay_Movement_20200401` passes; at least one `actor_exists` or
  `actor_moved` event fires from the movement fixture
- [ ] `Feed()` does not fault during replay (no `ErrUnknownPacket`); any S→C packet
  IDs that require `SetLength` workarounds are documented as in US-08
- [ ] Tests are deterministic: run 10 times in a row, always pass
  (`go test -count=10 ./pkg/fsm/` passes)
- [ ] `go test -race ./pkg/fsm/` passes
- [ ] `go build ./...` passes
- [ ] `go test ./pkg/fsm/` (without `-tags integration`) includes the replay tests
  and passes — no Docker container required
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us09_mock_server_replay.md` written

---

## Shared pre-condition for both stories

### The `faulted` session problem

`MapSession.Feed()` returns `ErrUnknownPacket` the **first** time it encounters a
packet ID whose length is absent from `lengths_map.go`. After that the session is
permanently faulted: all subsequent `Feed()` calls return `nil` and silently drop
all bytes. Registered handlers never fire again.

This is a **blocking** issue, not a test-hygiene issue. Two problems exist:

**Problem 1 — Inside `runMapPhase()` (blocking for both US-08 and US-09):**
`0x0B18` (ZC_INVENTORY_EXPANSION_INFO, 4 bytes) arrives between `0x0283` and
`0x02EB` in both DUMP1 and DUMP8_movement. It was absent from `lengths_map.go`
and absent from the FSM's `SetLength` block.

**Status: FIXED.** `pkg/fsm/fsm.go:runMapPhase()` now calls
`mapSess.SetLength(0x0B18, 4)` with a source comment (added in worklog 0019).
`Connect()` can now reach `OnReady` against the live server.

**Problem 2 — Post-`OnReady` map burst (blocking for the test assertions):**
After `OnReady` fires, many S→C packets arrive in the initial map burst before
`actor_exists` and `stat_update`. Several are absent from `lengths_map.go` and
will permanently fault the session before any events fire. Confirmed absent
(spot-checked against DUMP1/DUMP8_movement):

| Packet | Purpose | Length |
|--------|---------|--------|
| 0x0091 | ZC_CHANGE_MAPSERVER (map redirect) | 22 bytes |
| 0x007F | ZC_SYNC_DATA (received sync) | 6 bytes |
| 0x0B1B | (unknown map burst) | variable |
| 0x0ACB | (unknown map burst) | variable |
| 0x0B20 | (unknown map burst) | variable |
| 0x0ADF | (unknown map burst) | variable |

The first unknown-length packet encountered faults the session permanently.

**Before implementing either story**, resolve Problem 2 by choosing one option:

**Option A — Fix the lengths**: run codegen with S→C lengths fully populated
(completing US-02 if not already done) so that `Feed()` can frame all packets
the server sends during a normal session.

**Option B — Permissive feed mode**: add a `SetUnknownPacketPolicy(policy)` to
`sessionCore` where `policy = Skip` logs and skips unknown-length packets by
reading until the next known packet boundary, and `policy = Fault` (the current
default) preserves existing behavior. This allows the integration tests to run
without fully completing US-02 first.

Alternatively, discover all absent packet IDs from the dumps and add manual
`SetLength` calls in the test's `OnReady` callback, documenting each with packet
name and GCC-verified size. This is viable if the count of missing entries is small.

Document the chosen option in the worklog. Option A is preferred if US-02 has
already been completed.

---

## Exit Criteria for EPIC-01

EPIC-01 is complete when all of the following are true:

1. `go test -tags integration -timeout 60s ./pkg/fsm/ -run TestLiveServer` passes
   with `rathena-renewal` Docker container running
2. `go test -count=10 ./pkg/fsm/` passes (replay tests, no Docker, deterministic)
3. `go test -race ./pkg/fsm/` passes
4. `go build ./...` passes
5. At least one real decoded event (non-zero fields) is asserted in both the live
   and replay tests
6. All S→C length workarounds (`SetLength` calls or permissive-mode decisions)
   are documented with packet name and GCC-verified size
7. Worklog written for US-08 and US-09
