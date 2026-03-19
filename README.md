# rathena-client

**Module**: `github.com/lenaxia/rathena-client`
**Go**: 1.24.0
**Status**: All phases complete (Phases 0–7). Packet coverage complete for main kRO client. Ready for goKore integration.
**Version**: v0.3.3

---

## What this is

`rathena-client` is a pure Go library implementing the Ragnarok Online wire protocol as spoken by rAthena login, char, and map servers.

It is **not** a game client, bot, or application. It is a **pure protocol library** with a single contract:

- Receives raw TCP bytes → invokes typed, version-agnostic callbacks
- Accepts typed send-request structs → returns raw TCP bytes

Primary consumer: **goKore** (`github.com/lenaxia/gokore`) — the bot framework that imports this library as its network layer.

---

## Design invariants

| Invariant | Enforcement |
|---|---|
| Zero goroutines in `pkg/` | CI grep: `grep -r "^\s*go " pkg/` must be empty |
| Zero heap allocations in decode hot path | `go test -bench=. -benchmem ./pkg/...` → 0 allocs/op |
| No external runtime dependencies | `go.mod` has zero `require` entries |
| rAthena source is the only packet structure authority | GCC preprocessor verification gate |
| `Feed()` is synchronous — caller owns concurrency | Design invariant, not enforced by tests |
| Public API is packet-ID agnostic | No `uint16` packet ID in any exported goKore-facing signature |

---

## Package overview

```
github.com/lenaxia/rathena-client/
    pkg/packing/    Bit-packing codecs (WBUFPOS 3-byte, WBUFPOS2 6-byte)
    pkg/events/     281 typed event structs (S→C, generated)
    pkg/send/       152 typed send-request structs (C→S, generated)
    pkg/decode/     282 decode functions (generated, 0 allocs/op)
    pkg/encode/     178 encode functions + shuffle table (generated)
    pkg/session/    PACKETVER-aware framer + dispatcher + ConnectionFSM
                    (LoginSession, CharSession, MapSession, SemanticAction API)

    internal/codegen/   Code generator (GCC + semantics pipeline)
    semantics/          Semantic DB (edit via gokore-semantics MCP only)
    validation/         Pre-implementation verification scripts
```

`pkg/fsm` no longer exists as a separate package — `ConnectionFSM` was merged into `pkg/session` so that all low-level protocol primitives are inaccessible to external callers.

---

## Quick start

```go
import (
    "context"
    "net"

    "github.com/lenaxia/rathena-client/pkg/events"
    "github.com/lenaxia/rathena-client/pkg/send"
    "github.com/lenaxia/rathena-client/pkg/session"
    _ "github.com/lenaxia/rathena-client/pkg/encode" // register send encoders
)

f := session.New(
    session.ServerConfig{LoginAddr: "127.0.0.1:6900", Packetver: 20180307},
    session.Credentials{Username: "admin", Password: "admin", CharSlot: 0},
    func(ctx context.Context, addr string) (net.Conn, error) {
        return net.Dial("tcp", addr)
    },
)

f.OnReady(func(ms *session.MapSession, conn net.Conn, info session.ReadyInfo) {
    // Register handlers by semantic action — no packet IDs needed
    session.RegisterSemanticHandler(ms, session.ActionActorExists, func(e events.ActorExists) {
        // handles 0x0078, 0x01D8, 0x02EC, 0x09FF automatically across all packetvers
    })

    // Send by semantic action — no packet IDs, no shuffle, no obfuscation needed
    _ = session.Send(ms, conn, session.ActionMoveTo, send.MoveTo{X: 128, Y: 214})

    go readLoop(ms, conn)
})

if err := f.Connect(context.Background()); err != nil {
    log.Fatal(err)
}
```

See [`docs/USAGE.md`](docs/USAGE.md) for the complete integration guide.

---

## Build and test

```bash
go build ./...
go test ./...
go test -race ./...
go test -bench=. -benchmem ./pkg/...
grep -r "^\s*go " pkg/   # must produce no output
```

---

## Documentation

| Document | Purpose |
|---|---|
| [`README-LLM.md`](README-LLM.md) | Complete LLM starting point — rules, architecture, workflows |
| [`docs/USAGE.md`](docs/USAGE.md) | Integration guide for consumers (goKore and others) |
| [`docs/ADDING_PACKETS.md`](docs/ADDING_PACKETS.md) | How to add new packets, fix decode gaps, and add new PACKETVERs |
| [`docs/DESIGN/HLD.md`](docs/DESIGN/HLD.md) | High-level design authority |
| [`docs/BACKLOG/`](docs/BACKLOG/) | Open epics and tech debt |
| [`docs/WORKLOG/`](docs/WORKLOG/) | Session work logs |

---

## Implementation status

| Phase | Package | Status |
|---|---|---|
| 0 | `validation/` | Complete — GCC gate, length check, DB validate scripts |
| 1 | HLD + DB fixes | Complete |
| 2 | `pkg/packing` | Complete — 0 allocs/op, fuzz tests pass |
| 3 | `internal/codegen` | Complete — 770 structs in VersionTable |
| 4 | `pkg/events`, `pkg/send`, `pkg/decode`, `pkg/encode`, `pkg/session` (generated) | Complete — full main kRO coverage |
| 5 | `pkg/session` (hand-written framer) | Complete — 0 allocs/op |
| 6 | `ConnectionFSM` (merged into `pkg/session`) | Complete — 21 tests, net.Pipe stubs, zero goroutines |
| 7 | Semantic action API (`pkg/session`) | Complete — packet-ID-agnostic API, 277 receive actions, 178 send encoders |

**Gate**: 76 PASS / 1 FAIL (expected — CH_MAKE_CHAR shuffle, documented).

---

## Known limitations

| Item | Detail |
|---|---|
| **kRO main client only** | The codegen only processes `PACKETVER_MAIN_NUM`. RE-client skill packets (`ZC_ADD_SKILL` 0x0B31, `ZC_SKILLINFO_LIST` 0x0B32, `ZC_SKILLINFO_UPDATE2` 0x0B33) and Zero-client packets are not decoded. |
| **Ragnarok Zero not supported** | Three Zero-client-only packets (`ZC_QUEST_DIALOG` 0x0BA6, `ZC_QUEST_DIALOG_MENU_LIST` 0x0BA7, `ZC_MONOLOG_DIALOG` 0x0BA9) generate empty SKIP stubs. |
| **Homunculus / Mercenary** | Decode stubs exist for homunculus packets but have known field-type truncation bugs. Mercenary packets are absent entirely. |

See [`docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md`](docs/BACKLOG/TECH-DEBT-01_packetver-re-zero-support.md) for the full RE/Zero resolution plan.

---

## License

See [LICENSE](LICENSE).
