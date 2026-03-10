# rathena-client

**Module**: `github.com/lenaxia/rathena-client`  
**Go**: 1.24.0  
**Status**: All phases complete (Phases 0–6). Ready for goKore integration (Phase 7).

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

---

## Package overview

```
github.com/lenaxia/rathena-client/
    pkg/packing/    Bit-packing codecs (WBUFPOS 3-byte, WBUFPOS2 6-byte)
    pkg/events/     417 typed event structs (S→C, generated)
    pkg/send/       163 typed send-request structs (C→S, generated)
    pkg/decode/     442 decode functions (generated, 0 allocs/op)
    pkg/encode/     80 encode functions (generated, returns fixed arrays)
    pkg/session/    PACKETVER-aware framer + dispatcher (Login/Char/Map sessions)
    pkg/fsm/        ConnectionFSM — login→char→map auth sequencer

    internal/codegen/   Code generator (GCC + semantics pipeline)
    semantics/          Semantic DB (edit via gokore-semantics MCP only)
    validation/         Pre-implementation verification scripts
```

---

## Quick start

```go
import (
    "context"
    "net"

    "github.com/lenaxia/rathena-client/pkg/decode"
    "github.com/lenaxia/rathena-client/pkg/events"
    "github.com/lenaxia/rathena-client/pkg/fsm"
    "github.com/lenaxia/rathena-client/pkg/session"
)

f := fsm.New(
    fsm.ServerConfig{LoginAddr: "127.0.0.1:6900", Packetver: 20180307},
    fsm.Credentials{Username: "admin", Password: "admin", CharSlot: 0},
    func(ctx context.Context, addr string) (net.Conn, error) {
        return net.Dial("tcp", addr)
    },
)

f.OnReady(func(ms *session.MapSession, conn net.Conn) {
    ms.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
        e := decode.ActorExists_0x09FF(data, pv)
        // use e.ID, e.PosDir, e.Type, etc.
    })
    // hand conn to your read loop
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
| [`docs/ADDING_PACKETS.md`](docs/ADDING_PACKETS.md) | How to fix decode gaps, implement stub encodes, and add new packets/PACKETVERs |
| [`docs/DESIGN/HLD.md`](docs/DESIGN/HLD.md) | High-level design authority |
| [`docs/WORKLOG/`](docs/WORKLOG/) | Session work logs (25 entries) |

---

## Implementation status

| Phase | Package | Status |
|---|---|---|
| 0 | `validation/` | Complete — GCC gate, length check, DB validate scripts |
| 1 | HLD + DB fixes | Complete |
| 2 | `pkg/packing` | Complete — 0 allocs/op, fuzz tests pass |
| 3 | `internal/codegen` | Complete — 482 structs in VersionTable |
| 4 | `pkg/events`, `pkg/send`, `pkg/decode`, `pkg/encode`, `pkg/session` (generated) | Complete |
| 5 | `pkg/session` (hand-written) | Complete — 12 tests, 0 allocs/op |
| 6 | `pkg/fsm` | Complete — 21 tests, net.Pipe stubs, zero goroutines |
| 7 | goKore integration | Next |

**Gate**: 76 PASS / 1 FAIL (expected — CH_MAKE_CHAR shuffle, documented).

---

## License

See [LICENSE](LICENSE).
