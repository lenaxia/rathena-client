# EPIC-07: Semantic Action API

**Status**: Ready for implementation  
**Created**: 2026-03-18  
**Goal**: Add a semantic action API to `pkg/session` so that callers (goKore) can register
receive handlers and send packets by semantic action name — expressed as a generated
`SemanticAction` enum — with zero knowledge of packet IDs, encode functions, or
packetver-conditional dispatch.

---

## Context

rathena-client currently exposes `RegisterHandler(id uint16, fn HandlerFunc)` and
requires callers to supply raw packet IDs. goKore therefore contains:

- `internal/network/packetver/action_selector.go` — 4969 lines of generated
  action→packet ID mappings that duplicate knowledge already in `semantics/mappings.yaml`
- `internal/network/adapters/` — ~440 generated files translating versioned packet
  structs to canonical params, because goKore receives raw bytes and must decode them
- `internal/network/params/` — ~465 generated canonical param structs that exist purely
  as an intermediate translation layer between raw bytes and handler logic
- Raw `types.PacketID(0x...)` literals sprinkled across every handler registration file

All of this exists because rathena-client hands goKore raw bytes, forcing goKore to
own protocol knowledge it has no business owning.

The fix is to add a semantic layer to `pkg/session` that:

1. Exposes a generated `SemanticAction` enum covering all receive-direction and
   send-direction actions
2. Generates a dispatch table mapping each `SemanticAction` to its set of packet IDs
   and the corresponding decode functions
3. Provides `RegisterSemanticHandler[E any]` — a generic free function that accepts a
   semantic action constant and a typed handler `func(E)` where `E` is an `events.*`
   struct, fans out to `RegisterHandler` for every matching packet ID, and wraps each
   with the correct decode function
4. Provides `Send` — a free function that accepts a semantic action constant, a
   `send.*` request struct (as `interface{}`), and an `io.Writer`, resolves the correct
   packet ID and encode function for the session's packetver, applies obfuscation, and
   writes the bytes. `Send` is non-generic because the registry cannot provide
   compile-time type safety; callers must pass the correct `send.*` type for the action.

After this epic, goKore's handler registration becomes:

```go
session.RegisterSemanticHandler(mapSess, session.ActionActorMoved, h.handleActorMoved)
session.Send(mapSess, conn, session.ActionMoveTo, send.MoveTo{X: 100, Y: 200})
```

goKore never sees a packet ID, an encode function, or an adapter.

---

## Story Map

```
US-19  Generate SemanticAction enum (actions.go)          ──────────────┐
US-20  Generate receive dispatch table                    ──────────────┤
US-21  Generate send encoder registration (register.go)  ──────────────┤
US-22  Implement RegisterSemanticHandler + Send           ──────────────┘
                                                                         │
                                                                         ▼
                                                          goKore integration unblocked
```

US-19 must complete before US-20 and US-21 (they depend on the enum values).
US-20 and US-21 are independent of each other.
US-22 depends on US-20 and US-21.

**Build dependency note**: `pkg/encode/register.go` (US-21 output) references
`session.ErrWrongSendType` and `session.RegisterSendEncoder`, which are defined in
`pkg/session/semantic.go` (US-22). Therefore the codebase only compiles once US-21
and US-22 are both merged together. Land US-19, US-20, US-21, US-22 in a single
PR or in two PRs: (US-19 + US-20) followed by (US-21 + US-22).

**Init ordering note**: Go initializes package-level `var` declarations before running
`init()` functions, in dependency order. `pkg/session` is a dependency of `pkg/encode`,
so `pkg/session`'s vars (including `sendRegistry`) are fully initialized before
`pkg/encode`'s `init()` runs. `RegisterSendEncoder` calls in `pkg/encode`'s `init()`
therefore safely populate an already-allocated `sendRegistry`. `Send` can only be
called from `main()` or tests — both of which start after all `init()` functions
complete — so the registry is always fully populated before first use.

**Codegen atomicity note**: US-19 (`actions.go`) and US-21 (`register.go`) must be
regenerated together in a single `go generate` invocation (or committed atomically).
`actions.go` defines `maxSemanticAction` which sizes the `sendRegistry` array; if
`register.go` references a `SemanticAction` constant that does not exist in an older
`actions.go`, the build will fail. The codegen `run()` function in `main.go` must
invoke `genActions` before `genRegister`, and both outputs must be committed in the
same change.

**`main.go` integration spec**: three new local wrapper functions must be added to
`internal/codegen/main.go` following the exact pattern of the existing `genDecode`,
`genEncode` wrappers (see `main.go:1217-1254`). The `run()` function must call them
as Steps 10, 11, 12 — **after** the existing Step 9 (`genEncode`) — in this order:

```go
// Step 10: Generate SemanticAction enum (pkg/session/actions.go)
log.Println("Generating SemanticAction enum...")
if err := genActions(db, outDir); err != nil {
    return fmt.Errorf("actions: %w", err)
}
log.Println("  → pkg/session/actions.go")

// Step 11: Generate receive dispatch table (pkg/session/receive_dispatch.go)
log.Println("Generating receive dispatch table...")
if err := genReceiveDispatch(db, outDir); err != nil {
    return fmt.Errorf("receive_dispatch: %w", err)
}
log.Println("  → pkg/session/receive_dispatch.go")

// Step 12: Generate send encoder registration (pkg/encode/register.go)
log.Println("Generating send encoder registration...")
if err := genRegister(db, vt, outDir); err != nil {
    return fmt.Errorf("register: %w", err)
}
log.Println("  → pkg/encode/register.go")
```

The three local wrapper functions follow the exact signature pattern of `genDecode`:

```go
func genActions(db *semantics.DB, outDir string) error {
    src, err := gen.GenerateActionsFile(db)
    if err != nil {
        return err
    }
    return writeFile(filepath.Join(outDir, "pkg", "session", "actions.go"), src)
}

func genReceiveDispatch(db *semantics.DB, outDir string) error {
    src, err := gen.GenerateReceiveDispatchFile(db)
    if err != nil {
        return err
    }
    return writeFile(filepath.Join(outDir, "pkg", "session", "receive_dispatch.go"), src)
}

func genRegister(db *semantics.DB, vt preprocess.VersionTable, outDir string) error {
    src, err := gen.GenerateRegisterFile(db, vt)
    if err != nil {
        return err
    }
    return writeFile(filepath.Join(outDir, "pkg", "encode", "register.go"), src)
}
```

`GenerateActionsFile`, `GenerateReceiveDispatchFile`, and `GenerateRegisterFile` are
the top-level exported functions in the new generator files (`gen/actions.go`,
`gen/receive_dispatch.go`, `gen/register.go`). They return `(string, error)` — just
generated source, no filename — because each generates exactly one file at a fixed
path. This differs from `GenerateDecodeDirFiles` (which returns a `map[string]string`
for a whole directory) but matches the single-file generators like `GenerateShuffleFile`
and `GenerateLengthsFile`.

Note: `genReceiveDispatch` does not need `vt` — it only references `decode.*` function
names derived from `PacketID` strings in the semantic DB. The VersionTable is not
needed for receive dispatch generation.

---

## US-19 — Generate SemanticAction Enum

### User Story

**As a** goKore developer,  
**I want** a generated `SemanticAction` typed constant for every action in the semantic DB,  
**so that** I can write `session.ActionActorMoved` instead of `0x09FD` and get
compile-time safety on action names.

### Problem

There is no runtime representation of semantic action names in rathena-client. Action
names exist only in comments (e.g. `// ActorMoved is the event emitted for the actor_moved
action.`) and are not accessible to code that wants to register or dispatch by name.

### Design

Add a new codegen step that emits `pkg/session/actions.go`:

```go
// Code generated by internal/codegen. DO NOT EDIT.

package session

// SemanticAction identifies a protocol-level semantic action.
// Each constant corresponds to one entry in semantics/mappings.yaml.
type SemanticAction uint16

const (
    ActionUnknown        SemanticAction = 0
    ActionActorConnected SemanticAction = 1
    ActionActorDied      SemanticAction = 2
    ActionActorExists    SemanticAction = 3
    ActionActorMoved     SemanticAction = 4
    ActionChatMessage    SemanticAction = 5
    ActionItemExists     SemanticAction = 6
    ActionMoveTo         SemanticAction = 7
    ActionSkillUse       SemanticAction = 8
    // ... one constant per action in semantics/mappings.yaml, sorted alphabetically ...

    // maxSemanticAction is the highest assigned SemanticAction value.
    // Used to size the sendRegistry array in semantic.go.
    maxSemanticAction SemanticAction = ActionSkillUse // replaced by codegen with actual last value
)
```

Rules for the enum:
- Include ALL actions from `semantics/mappings.yaml` — both receive-direction and
  send-direction. Callers use the same constant set for both `RegisterSemanticHandler`
  and `Send`.
- Constant name: `Action` + PascalCase(action_name). e.g. `actor_moved` →
  `ActionActorMoved`.
- `ActionUnknown SemanticAction = 0` is the first entry; sequential values start
  from 1. The numeric values are opaque — callers must never compare them to literal
  integers.
- `maxSemanticAction` is an unexported constant equal to the **highest assigned
  SemanticAction value**. It is used by `semantic.go` to size the
  `sendRegistry` array (`[maxSemanticAction+1]`). The comment in the generated file
  must say "highest assigned SemanticAction value" not "count", to avoid confusion
  if the enum ever gains a gap. Do NOT use the phrase "total count" — it is only
  coincidentally equal to the highest value today (because values are sequential
  from 1) but the semantics are distinct.
- Sort constants alphabetically within the file for stable codegen output (no
  spurious diffs on regeneration).
- Generate a `String() string` method on `SemanticAction` that returns the constant
  name (e.g. `ActionActorMoved`) for known values and `SemanticAction(N)` for unknown
  ones. This is required so that panic messages from `RegisterSemanticHandler` and
  `Send` include the action name rather than an opaque integer. Pattern: a generated
  `switch` over all constants, default returns `fmt.Sprintf("SemanticAction(%d)", a)`.
  Add to the same `actions.go` file, below the const block.

### Implementation

Add `GenerateActionsFile(db *semantics.DB) (string, error)` in
`internal/codegen/gen/actions.go`. The `internal/codegen/gen/` directory already
exists with established generator files (`decode.go`, `encode.go`, `events.go`,
`send.go`, `lengths.go`, `shuffle.go`, `obfuscation.go`). Follow the same pattern:
a standalone `gen*` function, a `text/template` template, and a `writeFile` call.
Invoke `genActions` from `run()` in `internal/codegen/main.go` after the existing
codegen steps.

### Acceptance Criteria

- [ ] `pkg/session/actions.go` is generated and committed
- [ ] Every action in `semantics/mappings.yaml` has a corresponding `Action*` constant
- [ ] `ActionUnknown SemanticAction = 0` is the first entry
- [ ] `maxSemanticAction` unexported constant equals the highest assigned value;
  generated comment says "highest assigned SemanticAction value" not "count"
- [ ] Constants are sorted alphabetically
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] Codegen unit test added to the existing `internal/codegen/gen/gen_test.go` verifies: correct constant count,
  `maxSemanticAction` equals last constant value, `ActionUnknown = 0` present,
  no duplicate values, alphabetical ordering
- [ ] `SemanticAction.String() string` method generated in `actions.go` — returns
  constant name for known values (e.g. `"ActionActorMoved"`), `"SemanticAction(N)"`
  for unknown; verified by codegen unit test checking at least one known and one
  unknown value
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us19_semantic_action_enum.md` written

---

## US-20 — Generate Receive Dispatch Table

### User Story

**As a** caller of `RegisterSemanticHandler`,  
**I want** the session to know which packet IDs and decode functions correspond to
each receive-direction semantic action,  
**so that** registering a handler by action name automatically covers all packetver
variants without any protocol knowledge in the caller.

### Problem

The decode functions in `pkg/decode/` are public but unregistered — nothing connects
`session.ActionActorMoved` to `{decode.ActorMoved_0x007B, decode.ActorMoved_0x01DA,
decode.ActorMoved_0x022C, decode.ActorMoved_0x09DB, decode.ActorMoved_0x09FD}`.
This connection must be generated from the same semantic DB data that produced the
decode functions.

### Design

Add a new codegen step that emits `pkg/session/receive_dispatch.go`:

```go
// Code generated by internal/codegen. DO NOT EDIT.

package session

import (
    "github.com/lenaxia/rathena-client/pkg/decode"
    "github.com/lenaxia/rathena-client/pkg/events"
)

// receiveEntry holds one packet-ID → decode-function mapping for a semantic action.
type receiveEntry struct {
    id uint16
    fn func([]byte, uint32) interface{}
}

// receiveDispatch maps each receive-direction SemanticAction to its full set of
// packet IDs and decode functions across all packetver variants.
var receiveDispatch = map[SemanticAction][]receiveEntry{
    ActionActorMoved: {
        {id: 0x007B, fn: func(d []byte, pv uint32) interface{} { return decode.ActorMoved_0x007B(d, pv) }},
        {id: 0x01DA, fn: func(d []byte, pv uint32) interface{} { return decode.ActorMoved_0x01DA(d, pv) }},
        {id: 0x022C, fn: func(d []byte, pv uint32) interface{} { return decode.ActorMoved_0x022C(d, pv) }},
        {id: 0x09DB, fn: func(d []byte, pv uint32) interface{} { return decode.ActorMoved_0x09DB(d, pv) }},
        {id: 0x09FD, fn: func(d []byte, pv uint32) interface{} { return decode.ActorMoved_0x09FD(d, pv) }},
    },
    ActionChatMessage: {
        {id: 0x008D, fn: func(d []byte, pv uint32) interface{} { return decode.ChatMessage_0x008D(d, pv) }},
        {id: 0x008E, fn: func(d []byte, pv uint32) interface{} { return decode.ChatMessage_0x008E(d, pv) }},
    },
    // ... all receive-direction actions ...
}
```

Rules:
- Direction is **per-implementation, not per-action**. The `semantics.Action` struct
  has no `Direction` field. To determine whether an implementation is receive-direction,
  use the `isReceiveStruct(impl.StructName)` function that already exists in the `gen`
  package (`internal/codegen/gen/events.go`). **Do NOT call `inferDirection` from the
  `semantics` package** — that function is unexported (`loader.go:233`). `isReceiveStruct`
  covers exactly the same prefixes: `PACKET_ZC_`, `PACKET_HC_`, `PACKET_AC_`,
  `PACKET_SC_`, `PACKET_TC_`, `SYNTH_ZC_`, `SYNTH_HC_`, and lowercase `packet_*`.
  An action with mixed-direction implementations (e.g. a CZ_ and a ZC_ under the same
  action name) is valid YAML; only the receive-direction implementations go in
  `receiveDispatch`.
- Only include implementations that have a non-empty `PacketID` and `StructName`.
  Implementations with either field empty are silently skipped by the decode generator
  and will have no corresponding decode function; omit them here too. In the generated
  file, emit a comment above the map documenting any actions that were entirely omitted
  (all implementations either send-direction or incomplete). The term "SKIP stub" in
  earlier documentation refers to these absent decode functions — there is no explicit
  stub concept in the generated code, just absence.
- The `fn` wrapper uses `interface{}` return because Go generics cannot be used in map
  values with varying type parameters at compile time. The type assertion happens inside
  `RegisterSemanticHandler` (US-22). The concrete type returned by each `fn` wrapper
  is always the `events.*` struct value (not a pointer). If an older packetver decode
  branch is absent, the decode function still returns a zero-value `events.*` struct
  — the handler will fire with a zero-value event. This is correct behaviour; the
  caller is responsible for checking packetver-sensitive fields if needed. Callers of
  `RegisterSemanticHandler` must use `func(events.ActorMoved)` (value receiver), not
  `func(*events.ActorMoved)`. Passing a pointer-receiver handler will compile but
  panic at first dispatch. This constraint is documented in the
  `RegisterSemanticHandler` godoc (see US-22).
- `pkg/session` importing `pkg/decode` is safe — verify no import cycle exists before
  implementing. Confirm: `decode` imports only `pkg/events`; `events` imports nothing;
  `session` currently imports only stdlib. No cycle is introduced. Add an AC item to
  verify this (`go build ./...` passes and `go list -deps pkg/decode | grep session`
  produces no output).
- **Known inter-action packet ID collisions in `mappings.yaml`**: `0x008D`
  (`PACKET_ZC_NOTIFY_CHAT`) appears as a receive-direction implementation in both
  `chat_message` and `public_chat`. `0x008E` appears in both `chat_message`
  (`PACKET_ZC_NOTIFY_CHAT`) and `self_chat` (`PACKET_ZC_NOTIFY_PLAYERCHAT`). These
  are pre-existing data model issues in `mappings.yaml` — not introduced by this epic
  — and will be addressed in a future DB cleanup story. The consequence for callers:
  if `RegisterSemanticHandler` is called for both `ActionChatMessage` and
  `ActionPublicChat`, the second registration silently overwrites the first for the
  shared packet ID `0x008D`, because `RegisterHandler` always overwrites. Callers
  must not register both simultaneously until the collision is resolved. The
  `receiveDispatch` generator emits the collision faithfully (one entry per
  implementation, no deduplication); no special handling is needed in the codegen
  itself.

### Implementation

Add `genReceiveDispatch(db *semantics.DB, outDir string) error`
in `internal/codegen/gen/receive_dispatch.go`, following the same pattern as the
existing generator files in `internal/codegen/gen/`. The function iterates the
semantic DB's action→implementations list. For each implementation that is
receive-direction and has both `PacketID` and `StructName` non-empty, emit a
`receiveEntry` row. If an implementation is skipped (empty fields or send-direction),
emit a comment of the form `// ActionXxx: skipped (no decode function for 0xNNNN)`
above the map entry or at the top of the map if the action has no valid implementations.
The VersionTable is **not** needed — receive dispatch only references decode function
names which are derivable from `PacketID` alone (following the same naming convention
as `pkg/decode/`). Do not use the term "SKIP stub" anywhere in generated comments.

**Decode function name formula**: For each receive-direction implementation, the
referenced decode function name is constructed as:

```
decode.<PascalCase(action_name)>_<impl.PacketID>
```

where `PascalCase(action_name)` uses the same `actionNameToGoIdent` helper already
in the `gen` package (`events.go`), and `impl.PacketID` is used **exactly as stored
in the DB** — uppercase hex with `0x` prefix, e.g. `"0x007B"`. The `0x` and hex
digits are part of the Go function name, matching the naming used by `gen/decode.go`
(`packetIDtoFuncSuffix` at `decode.go:479` returns the ID unchanged). Example:
action `"actor_moved"` + packet ID `"0x007B"` → `decode.ActorMoved_0x007B`.

### Acceptance Criteria

- [ ] `pkg/session/receive_dispatch.go` is generated and committed
- [ ] Every receive-direction action with at least one valid implementation
  (non-empty `PacketID` and `StructName`, receive-direction struct) has an entry
  in `receiveDispatch`
- [ ] Actions whose every implementation is send-direction or has empty fields are
  absent from the map (with a generated comment above the map documenting each
  omitted action and the reason)
- [ ] Direction is determined per-implementation via `isReceiveStruct(impl.StructName)`
  (from `internal/codegen/gen/events.go`), not assumed at the action level
- [ ] No import cycle introduced (`go build ./...` passes); confirm with
  `go list -deps github.com/lenaxia/rathena-client/pkg/decode | grep session` → empty
- [ ] `go test ./...` passes
- [ ] Codegen unit test added to the existing `internal/codegen/gen/gen_test.go` verifies `GenerateReceiveDispatchFile`
  output as a **generated source string** (not compiled), using inline mock
  `semantics.DB` data — the same approach as `TestGenerateEventsFile_Basic` in
  `gen_test.go`. Three cases:
  - an all-send-direction action (e.g. one `PACKET_CZ_` struct only) produces no entry
    in the generated map;
  - a receive-direction action (one `PACKET_ZC_` struct, non-empty `PacketID`) produces
    a correctly formatted `receiveEntry` line containing both the packet ID literal and
    the `decode.XxxName_0xNNNN` function reference;
  - a mixed-direction action produces only the receive-direction implementations.
  Tests check `strings.Contains(generatedSrc, expectedSnippet)` — they do NOT compile
  the generated output (which would require `decode.*` symbols to exist). This is the
  same pattern as all other codegen unit tests in `gen_test.go`.
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us20_receive_dispatch.md` written

---

## US-21 — Generate Send Dispatch Table

### User Story

**As a** caller of `Send`,  
**I want** the session to resolve the correct encode function for a send-direction
semantic action at the current packetver,  
**so that** I can call `session.Send(mapSess, conn, session.ActionMoveTo, req)` without
knowing which packet ID or encode function applies.

### Problem

The encode functions in `pkg/encode/` are public but unregistered. Nothing connects
`session.ActionMoveTo` to `encode.EncodeMoveTo` and the set of packet IDs it covers
across packetvers.

`pkg/encode` imports `pkg/session` (for `ShuffledCtoSID`). Therefore `pkg/session`
**cannot** import `pkg/encode` — it would create a fatal import cycle:

```
pkg/session → pkg/encode → pkg/session
```

The send dispatch table must therefore be populated at **init time by the encode
package itself**, using a `RegisterSendEncoder` hook in `pkg/session`.

### Design

**Step 1 — `pkg/session` defines the registry (hand-written, US-22).**

`pkg/session/semantic.go` declares:

```go
// SendEncoderFunc is the type stored in the send dispatch registry.
// It accepts the request as interface{} and the current packetver, and returns
// the fully-encoded, ready-to-send byte slice (packet ID bytes already written
// by the encode function; XOR obfuscation applied by Send after this call).
// Returns ErrWrongSendType if req is not the expected send.* type.
type SendEncoderFunc func(req interface{}, pv uint32) ([]byte, error)

// sendRegistry holds the registered encoder for each send-direction SemanticAction.
// Populated during package init by pkg/encode/register.go.
var sendRegistry [maxSemanticAction + 1]SendEncoderFunc

// RegisterSendEncoder registers fn as the encoder for action.
// Must be called from init() only; not goroutine-safe after init.
// Panics if called twice for the same action — indicates a codegen bug producing
// duplicate init() registrations.
func RegisterSendEncoder(action SemanticAction, fn SendEncoderFunc) {
    if sendRegistry[action] != nil {
        panic(fmt.Sprintf("session: RegisterSendEncoder called twice for action %v", action))
    }
    sendRegistry[action] = fn
}
```

`maxSemanticAction` is the **highest assigned SemanticAction value** (not a count),
declared as a generated constant in `pkg/session/actions.go` (US-19).

**Step 2 — `pkg/encode` registers itself (generated file).**

Add a new codegen step that emits `pkg/encode/register.go`:

```go
// Code generated by internal/codegen. DO NOT EDIT.

package encode

import (
    "github.com/lenaxia/rathena-client/pkg/send"
    "github.com/lenaxia/rathena-client/pkg/session"
)

func init() {
    session.RegisterSendEncoder(session.ActionMoveTo,
        func(req interface{}, pv uint32) ([]byte, error) {
            r, ok := req.(send.MoveTo)
            if !ok {
                return nil, session.ErrWrongSendType
            }
            b := EncodeMoveTo(r, pv)
            return b[:], nil
        },
    )
    session.RegisterSendEncoder(session.ActionActorAction,
        func(req interface{}, pv uint32) ([]byte, error) {
            r, ok := req.(send.ActorAction)
            if !ok {
                return nil, session.ErrWrongSendType
            }
            b := EncodeActorAction(r, pv)
            return b[:], nil
        },
    )
    // ... one RegisterSendEncoder call per send-direction action with a
    // corresponding non-stub encode function ...
}
```

This file lives in `pkg/encode`, which already imports `pkg/session`, so no new
import edges are introduced.

The import graph after this change:

```
pkg/session  →  (stdlib only)         ← unchanged
pkg/encode   →  pkg/session           ← already true today
pkg/encode   →  pkg/send              ← already true today
```

No cycle. `Send` in `pkg/session` reads from `sendRegistry` — no import of `encode`
needed.

**Encode function contract — packet ID bytes.**

All encode functions write the packet ID into bytes 0–1 of the returned buffer.
Some encode functions (currently `EncodeMoveTo`, `EncodeActorAction`) call
`ShuffledCtoSID` internally and write the already-shuffled wire ID. Other encode
functions write the base (canonical) packet ID.

Because `Send` in `pkg/session` only applies XOR obfuscation (via `s.Encode(&id)`)
and never calls `ShuffledCtoSID`, this non-uniformity is transparent to the `Send`
implementation: `Send` reads the first two bytes back out, applies XOR, and writes
them back. The bytes 0–1 are always the correct wire ID (after any internal shuffle)
before `Send` touches them, and are the XOR-obfuscated wire ID after `Send` touches
them. No double-shuffle occurs.

**Variable-length encode functions.**

Five encode functions return `[]byte` directly (`EncodeBattleChat`, `EncodeGuildChat`,
`EncodePartyChat`, `EncodePublicChat`, `EncodeWhisper`). Of these, only `public_chat`
currently has a send-direction entry in `semantics/mappings.yaml`; the others
(`battle_chat`, `guild_chat`, `party_chat`, `whisper`) have encode functions but no
semantic action entries and therefore produce no `RegisterSendEncoder` call (known gap,
to be addressed in a future story by adding those actions to `mappings.yaml`).

For actions whose encode function does return `[]byte`, the wrapper in `register.go`
does not append `[:]` — the slice is returned directly:

```go
session.RegisterSendEncoder(session.ActionPublicChat,
    func(req interface{}, pv uint32) ([]byte, error) {
        r, ok := req.(send.PublicChat)
        if !ok {
            return nil, session.ErrWrongSendType
        }
        return EncodePublicChat(r, pv), nil   // already []byte — no [:]
    },
)
```

The codegen for `register.go` must detect the return type of each encode function
(fixed `[N]byte` vs `[]byte`) and emit the appropriate wrapper.

**`ErrWrongSendType` visibility.**

The sentinel error must be exported so `pkg/encode/register.go` can reference it.
Rename from `errWrongSendType` to `ErrWrongSendType`. Callers (goKore) can use
`errors.Is(err, session.ErrWrongSendType)` for precise error handling.

### Implementation

Add `genRegister(db *semantics.DB, vt preprocess.VersionTable, outDir string) error` in
`internal/codegen/gen/register.go`, following the same pattern as the existing
generator files (compare `GenerateDecodeDirFiles(db *semantics.DB, vt preprocess.VersionTable)`).
The `*semantics.DB` pointer matches every other generator; `vt preprocess.VersionTable`
is required to replicate the `layout.TotalSize <= 0` check from `encode.go:119-122`
which determines whether a given action's encode function returns `[N]byte` (fixed) or
`[]byte` (variable).

**Return-type inference for multi-implementation send actions**: Some send actions have
more than one send-direction implementation (e.g. `move_to` has `SYNTH_CZ_REQUEST_MOVE`
and `SYNTH_CZ_REQUEST_MOVE2`). The generated `EncodeMoveTo` uses a dispatcher that
returns a single `[N]byte` if all implementations agree on the same fixed size, or
`[]byte` if any is variable. `genRegister` must apply the same aggregation: call
`resolveLayout` for **every** send-direction implementation of the action, then:
- If all resolvable layouts have the same positive `TotalSize` → emit `b[:]` (fixed-array path).
- If any layout has `TotalSize <= 0`, or layouts disagree on size, or `resolveLayout`
  returns `nil` for any impl → emit no `[:]` (slice path).

**`resolveLayout` returning nil means variable-length, not absent**: if `resolveLayout`
returns nil for an implementation (e.g. `PACKET_CZ_REQUEST_CHAT` has no C struct
definition in rAthena and is absent from the VersionTable), treat its `TotalSize` as 0
for the purposes of `commonSize` aggregation. Do **NOT** skip the action from
`register.go` — a nil layout triggers the slice path (`return EncodeXxx(r, pv), nil`
with no `[:]`), which is correct since the hand-written encode function returns `[]byte`.
The `public_chat` action (`PACKET_CZ_REQUEST_CHAT`) is the current example of this case.

This mirrors `generateEncodeDispatcher`'s `commonSize` logic at `gen/encode.go:181-198`.
For single-implementation actions the aggregation trivially reduces to checking that
one layout's `TotalSize`. Output path: `pkg/encode/register.go`.

### Acceptance Criteria

- [ ] `pkg/encode/register.go` is generated and committed
- [ ] Every send-direction action with a non-stub encode function has a
  `RegisterSendEncoder` call in the `init()` function
- [ ] Fixed-array return types (`[N]byte`) are wrapped with `[:]`; variable-return
  types (`[]byte`) are used directly — confirmed by inspecting generated output
- [ ] Actions without an encode function are absent from `register.go` (with
  generated comment)
- [ ] `ErrWrongSendType` exported in `pkg/session/semantic.go` (US-22)
- [ ] `RegisterSendEncoder` panics on duplicate registration (codegen bug guard)
- [ ] No import cycle introduced — verified by `go build ./...` after US-22 also lands
  (US-21 alone references `session.ErrWrongSendType` and `session.RegisterSendEncoder`
  which are defined in US-22; these two stories must be merged together)
- [ ] `go test ./...` passes (with US-22 also merged)
- [ ] Codegen unit test added to the existing `internal/codegen/gen/gen_test.go` verifies the `TotalSize`-based
  return-type inference used by `GenerateRegisterFile`:
  - a mock action whose `resolveLayout(...)` returns a layout with `TotalSize > 0`
    (e.g. `TotalSize = 5`, matching `EncodeMoveTo`) emits `b[:]` in the wrapper;
  - a mock action whose `resolveLayout(...)` returns a layout with `TotalSize <= 0`
    (variable-length, matching `EncodePublicChat`) emits no `[:]` in the wrapper.
  The test uses inline `semantics.Action` + `preprocess.VersionTable` mock data —
  it does NOT inspect actual `pkg/encode/*.go` source files at test time; the
  `[N]byte` vs `[]byte` distinction is inferred solely from `VersionTable.TotalSize`
  (the same logic as `encode.go:119-122`). `public_chat` is currently the only
  send-direction action in `semantics/mappings.yaml` whose encode function returns
  `[]byte`; this is validated by the existing real-DB test rather than a new test.
  (`EncodeBattleChat`, `EncodeGuildChat`, `EncodePartyChat`, `EncodeWhisper` also
  return `[]byte` but their actions are absent from the DB — see US-21 known gap.)
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us21_send_dispatch.md` written

---

## US-22 — Implement RegisterSemanticHandler and Send

### User Story

**As a** goKore developer,  
**I want** `session.RegisterSemanticHandler` and `session.Send` free functions on
`MapSession`,  
**so that** I can wire handlers and send packets using only semantic action constants
and typed event/request structs — never packet IDs.

### Problem

The dispatch tables from US-20 and US-21 exist as generated data, but there is no
hand-written API that exposes them to callers. This story adds the two public free
functions and the supporting hand-written plumbing.

### Design

Add `pkg/session/semantic.go` (hand-written, never generated):

```go
package session

import (
    "errors"
    "fmt"
    "io"
)

// ErrWrongSendType is returned by Send when req is not the expected send.* type
// for the given action. Exported so callers can use errors.Is for precise handling.
var ErrWrongSendType = errors.New("session: Send called with wrong request type for action")

// SendEncoderFunc is the type stored in the send dispatch registry.
// It accepts the request as interface{} (the concrete send.* struct) and the
// current packetver, and returns the fully-encoded byte slice ready to send.
// The encode function has already written the correct wire packet ID (including
// any packetver-dependent shuffle) into bytes 0–1; Send applies only XOR
// obfuscation on top. Returns ErrWrongSendType if req is not the expected type.
type SendEncoderFunc func(req interface{}, pv uint32) ([]byte, error)

// sendRegistry holds the registered encoder for each send-direction SemanticAction.
// Populated during package init by pkg/encode/register.go.
// Array-indexed by SemanticAction value for O(1) lookup with zero allocation.
var sendRegistry [maxSemanticAction + 1]SendEncoderFunc

// RegisterSendEncoder registers fn as the encoder for action.
// Must be called from init() only. Not goroutine-safe after program init.
// Panics if called twice for the same action — indicates a codegen bug producing
// duplicate init() registrations.
func RegisterSendEncoder(action SemanticAction, fn SendEncoderFunc) {
    if sendRegistry[action] != nil {
        panic(fmt.Sprintf("session: RegisterSendEncoder called twice for action %v", action))
    }
    sendRegistry[action] = fn
}

// RegisterSemanticHandler registers fn to be called whenever the session receives
// any packet that maps to action. fn must be a func(E) where E is the events.*
// struct corresponding to action.
//
// E must be the concrete struct value type (e.g. events.ActorMoved), NOT a
// pointer (e.g. *events.ActorMoved). All decode functions return struct values,
// not pointers. Passing a pointer-receiver func will compile but panic at the
// first packet dispatch with a type mismatch message.
//
// IMPORTANT: string/[]byte fields in the decoded event (e.g. event.Name) are
// zero-copy aliases into the session receive buffer. They are valid only for the
// duration of the handler callback. Do NOT store them past the handler's return.
// To retain a string, copy it first: name = decode.CopyString(event.Name).
//
// All packetver variants of the action are registered simultaneously. If the
// session's packetver means only one variant will ever appear on the wire, the
// others will simply never fire — this is harmless.
//
// If action has no receive-direction dispatch entries (unknown or send-only action),
// RegisterSemanticHandler panics immediately.
//
// If a packet arrives and the decoded event cannot be type-asserted to E, the
// handler panics at dispatch time. This makes misconfiguration fail at the first
// received packet rather than silently dropping events.
//
// This is a free function rather than a method on MapSession because Go does not
// support generic methods.
func RegisterSemanticHandler[E any](s *MapSession, action SemanticAction, fn func(E)) {
    entries, ok := receiveDispatch[action]
    if !ok {
        panic(fmt.Sprintf("session: RegisterSemanticHandler: unknown or send-only action %v", action))
    }
    for _, e := range entries {
        s.RegisterHandler(e.id, func(data []byte, pv uint32) {
            raw := e.fn(data, pv)
            typed, ok := raw.(E)
            if !ok {
                panic(fmt.Sprintf("session: handler type mismatch for action %v packet 0x%04X: "+
                    "got %T, handler expects %T", action, e.id, raw, *new(E)))
            }
            fn(typed)
        })
    }
}

// Send encodes req using the registered encode function for action, applies XOR
// packet ID obfuscation if enabled, and writes the result to w.
//
// req must be the send.* struct value corresponding to action. If the concrete
// type does not match what was registered, Send returns ErrWrongSendType.
// Send accepts req as interface{} because the type check is performed inside
// the registered SendEncoderFunc closure at runtime — Go generics cannot provide
// compile-time safety here since the registry maps SemanticAction to interface{}.
// Callers should treat this as a typed call: always pass the exact send.* struct
// type documented for the action.
//
// The encode function (registered by pkg/encode/register.go at init time) is
// responsible for writing the correct wire packet ID — including any
// packetver-dependent shuffle — into bytes 0–1 of the buffer. Send reads those
// bytes back, applies s.Encode (rolling-key XOR obfuscation, a no-op when
// obfuscation is not active), and writes the final buffer to w.
//
// Send does NOT call ShuffledCtoSID — shuffle is the encode function's
// responsibility. Send only applies XOR obfuscation.
func Send(s *MapSession, w io.Writer, action SemanticAction, req interface{}) error {
    if int(action) >= len(sendRegistry) || sendRegistry[action] == nil {
        return fmt.Errorf("session: Send: unknown or receive-only action %v", action)
    }
    fn := sendRegistry[action]
    data, err := fn(req, s.core.packetver)
    if err != nil {
        return err
    }
    // Apply XOR obfuscation to the packet ID (bytes 0–1, little-endian).
    // The encode function has already written the correct (possibly shuffled) wire ID.
    // s.Encode applies the rolling-key XOR; it is a no-op when obfuscation is not
    // active. No ShuffledCtoSID call here — that would double-shuffle.
    //
    // Invariant: every encode function returns at least 2 bytes (the packet ID
    // header). The len(data) >= 2 guard is purely defensive and should never
    // be false for a well-formed encode function.
    if len(data) >= 2 {
        id := uint16(data[0]) | uint16(data[1])<<8
        s.Encode(&id)
        data[0] = byte(id)
        data[1] = byte(id >> 8)
    }
    _, err = w.Write(data)
    return err
}
```

Design notes:
- Both functions are free functions (not methods on `MapSession`) because Go
  does not support generic methods on `RegisterSemanticHandler`'s side. `Send`
  is non-generic: it accepts `req interface{}` and relies on the registered
  `SendEncoderFunc` closure to perform the runtime type assertion. Callers write
  `session.RegisterSemanticHandler(mapSess, session.ActionActorMoved, fn)` and
  `session.Send(mapSess, conn, session.ActionMoveTo, req)`.
- **No false generic safety**: `Send` does not use a type parameter because the
  registry is `[maxSemanticAction+1]SendEncoderFunc` keyed by `SemanticAction`
  value — there is no compile-time link between a `SemanticAction` constant and
  a specific `send.*` type. Using `Send[E any]` would give callers the impression
  of compile-time safety that does not exist; the wrong type still panics (via
  `ErrWrongSendType`) at runtime regardless.
- `Send` accesses `s.core.packetver` — `core` is an unexported field of `MapSession`.
  Since `semantic.go` is in the same package, this is fine. No API surface change to
  `sessionCore` is needed.
- **Shuffle responsibility**: `Send` never calls `ShuffledCtoSID`. The encode functions
  that need shuffle (currently `EncodeMoveTo`, `EncodeActorAction`) already call it
  internally. Other encode functions embed the base (canonical) packet ID. Either way,
  bytes 0–1 in the returned buffer are the correct wire ID before XOR obfuscation.
  Calling `ShuffledCtoSID` again in `Send` would double-shuffle and corrupt the ID.
- **ErrWrongSendType is exported** (`ErrWrongSendType`, not `errWrongSendType`) so that
  `pkg/encode/register.go` (in a different package) can reference it in the registered
  closure, and so goKore can use `errors.Is` for precise error handling.
- **sendRegistry is array-indexed**, not map-keyed, for O(1) lookup with zero heap
  allocation per `Send` call. `maxSemanticAction` is the highest assigned constant
  value, declared alongside the constants in `pkg/session/actions.go` (US-19).
- **receiveDispatch is map-keyed** (`map[SemanticAction][]receiveEntry`) because it is
  only accessed at registration time (cold path, called once at startup), not on the
  hot dispatch path. The inconsistency with `sendRegistry` (array) is intentional and
  documented here: `Send` is called per-packet (hot); `RegisterSemanticHandler` is
  called once (cold). Map lookup at registration time has negligible cost.
- **Send bounds check**: `int(action) >= len(sendRegistry)` is the correct check.
  `SemanticAction` is uint16; if `action` equals or exceeds `len(sendRegistry)`, the
  array access would panic before the old `action > len-1` check could fire — same
  semantics but the `>=` form makes the invariant unambiguous.
- **RegisterSemanticHandler panic timing**: the panic for an unknown/send-only action
  fires immediately at registration (start-up time). The panic for a type mismatch
  fires at the first packet dispatch, not at registration — Go type erasure means the
  concrete type `E` is not checkable at registration time without receiving a value.
  Both panics are still fast-fail: misconfiguration is caught before or at the first
  use, not silently.
- **E must be a struct value, not a pointer**: all decode functions return `events.*`
  struct values. A caller who writes `RegisterSemanticHandler[*events.ActorMoved](...)`
  will compile but get a type mismatch panic at first dispatch. The godoc on
  `RegisterSemanticHandler` must explicitly warn against pointer types.

### Testing

Add `pkg/session/semantic_test.go`:

- `TestRegisterSemanticHandler_ActorMoved`: create a `MapSession` with
  `packetver = 20140101` (in the range where `0x09FD` has fixed length 108 in the
  generated lengths table — no `SetLength` call needed), call
  `RegisterSemanticHandler` for `ActionActorMoved`, feed a synthetic raw `0x09FD`
  frame of exactly 108 bytes through `s.Feed()` (not by calling the handler directly),
  assert the handler is called with a correctly typed `events.ActorMoved` with expected
  field values.
- `TestRegisterSemanticHandler_AllVariants`: register a handler for `ActionActorMoved`,
  call `s.SetLength(id, length)` for all 5 packet IDs with their correct lengths from
  the actual lengths table so the framing engine can dispatch them. Use
  `NewMapSession(packetver >= 20071106)` (e.g. `20181121`) so the baseline lengths
  are pre-populated at modern values:
  - `0x007B`: fixed 60 bytes → `SetLength(0x007B, 60)`
  - `0x01DA`: fixed 60 bytes → `SetLength(0x01DA, 60)`
  - `0x022C`: fixed 65 bytes → `SetLength(0x022C, 65)` (65 at pv >= 20071106, not 64)
  - `0x09DB`: variable-length → `SetLength(0x09DB, -1)`
  - `0x09FD`: variable-length → `SetLength(0x09FD, -1)` (variable at pv >= 20141022)
  Feed one valid raw frame per variant (fixed frames use exact byte slice length;
  variable frames embed their length in bytes 2–3), assert the handler is called
  exactly 5 times, once per variant.
- `TestRegisterSemanticHandler_Overwrite`: call `RegisterSemanticHandler` twice for
  `ActionActorMoved` with two different handler functions. Assert the second handler
  fires (and the first does not) after the second registration. This matches the
  underlying `RegisterHandler` contract (`map.go:34`: "Overwrites any existing handler
  for that ID"). The godoc on `RegisterSemanticHandler` must document this overwrite
  behaviour explicitly.
- `TestRegisterSemanticHandler_PanicOnUnknownAction`: assert panic immediately when
  passing an out-of-range `SemanticAction` to `RegisterSemanticHandler`.
- `TestRegisterSemanticHandler_PanicOnTypeMismatch`: register a handler with the wrong
  event type (e.g. `func(events.ChatMessage)` for `ActionActorMoved`), feed a matching
  packet, and assert the panic fires at first dispatch (not at registration).
- `TestRegisterSemanticHandler_PanicOnPointerType`: register a handler with a pointer
  type (e.g. `func(*events.ActorMoved)`), feed a matching packet, assert panic fires
  at dispatch with a message indicating the type mismatch.
- `TestSend_MoveTo`: call `Send` with `ActionMoveTo` and a `send.MoveTo{X: 100, Y: 200}`
  struct; assert the bytes written to a `bytes.Buffer` have the correct wire packet ID
  and packed coordinates. Two sub-cases with exact expected values:
  - `packetver = 20180308` (post-shuffle era, `ShuffledCtoSID(20180308, 0x0085)` →
    `0x035F`): assert output bytes `[0x5F, 0x03, ...]` (little-endian `0x035F`).
  - `packetver = 20030000` (pre-shuffle, no entry in `ShuffledCtoSID`, returns
    `0x0085` unchanged): assert output bytes `[0x85, 0x00, ...]`.
  Both sub-cases assert bytes 2–4 decode to the correct packed `packing.EncodePosDir`
  value for X=100, Y=200, dir=0 (compute expected value from `packing.EncodePosDir`
  directly in the test — do not hardcode the 3-byte constant).
- `TestSend_VariableLengthAction`: call `Send` with `ActionPublicChat` (the only
  variable-length send-direction action currently in `semantics/mappings.yaml`) and a
  matching `send.PublicChat{Name: "Alice", Message: "hi"}` struct at
  `packetver = 20030000` (< 20040726, so wire ID is `0x008C`).
  Assert: (a) no error returned; (b) the first two bytes of output are
  `[0x8C, 0x00]` (little-endian `0x008C`); (c) bytes 2–3 hold the correct total frame
  length as little-endian uint16 (`4 + len("Alice : hi\x00")`); (d) bytes 4 onward
  equal `"Alice : hi\x00"`. This exercises the `[]byte`-returning (no `[:]`) path
  through `Send` and must pass.
  Note: `EncodeBattleChat`, `EncodeGuildChat`, `EncodePartyChat`, and `EncodeWhisper`
  also return `[]byte`, but their corresponding send-direction semantic actions
  (`battle_chat`, `guild_chat`, `party_chat`, `whisper`) do not yet exist in
  `semantics/mappings.yaml` and therefore produce no `RegisterSendEncoder` call.
  Add those actions to the YAML to enable their registration in a future story.
- `TestSend_ObfuscationApplied`: call `Send` with `EnableObfuscation` active, using
  known `key0/key1/key2` values; assert the first two bytes in the output match the
  expected XOR-obfuscated wire ID, not the raw packet ID.
- `TestSend_UnknownAction`: assert error (not panic) returned for an out-of-range
  action. Also test `action == ActionUnknown (0)` — `sendRegistry[0]` is nil and must
  return an error, not a nil pointer dereference.
- `TestSend_WrongType`: assert `ErrWrongSendType` returned when req has wrong type.
- `TestRegisterSendEncoder_DoublePanic`: call `RegisterSendEncoder` twice for the
  same action and assert it panics.
- `BenchmarkRegisterAndFeed_SemanticHandler`: register a handler, feed 1000 packets,
  report allocs/op. Note: the dispatch path unconditionally boxes the decoded event
  struct into `interface{}` inside the `receiveDispatch` lambda (the `fn` wrapper),
  then un-boxes it via type assertion in `RegisterSemanticHandler`. Because the boxing
  lambda is a closure over `entry.fn`, the boxed `interface{}` value cannot be
  stack-allocated — it will always escape to heap. Therefore exactly 1 alloc/op is
  the correct steady-state expectation for the dispatch path itself; 0 allocs/op is
  **not** achievable with this design and must not be stated as a goal. Record the
  baseline; do not gate the build on any specific alloc count. Fields that are `string`
  values will allocate if retained; the handler in this benchmark must not retain them.

### Acceptance Criteria

- [ ] `pkg/session/semantic.go` written with `RegisterSemanticHandler[E any]` and
  `Send` (non-generic, `req interface{}`) as documented above
- [ ] `ErrWrongSendType` (exported) and `SendEncoderFunc` and `RegisterSendEncoder`
  defined in `semantic.go`
- [ ] `RegisterSendEncoder` panics on double-registration (codegen-bug guard)
- [ ] `Send` bounds check uses `int(action) >= len(sendRegistry)` (not `> len-1`)
- [ ] `Send` returns an error (not panic) for `ActionUnknown` (0) — confirmed by
  `TestSend_UnknownAction` which explicitly tests `action == 0`
- [ ] `RegisterSemanticHandler` godoc explicitly warns that `E` must be a concrete
  struct value type, not a pointer or interface; documents zero-copy buffer aliasing
  warning from `HandlerFunc`; and documents overwrite behaviour (second registration
  for the same action silently overwrites the first — matches `RegisterHandler` contract)
- [ ] `RegisterSemanticHandler` second registration silently overwrites first —
  confirmed by `TestRegisterSemanticHandler_Overwrite`
- [ ] `RegisterSemanticHandler` panics immediately for unknown/send-only action —
  confirmed by `TestRegisterSemanticHandler_PanicOnUnknownAction`
- [ ] `RegisterSemanticHandler` type mismatch panics at first dispatch, not at
  registration — confirmed by `TestRegisterSemanticHandler_PanicOnTypeMismatch`
- [ ] `RegisterSemanticHandler` with pointer type panics at first dispatch —
  confirmed by `TestRegisterSemanticHandler_PanicOnPointerType`
- [ ] All 5 packetver variants of `ActionActorMoved` fire the handler —
  confirmed by `TestRegisterSemanticHandler_AllVariants` (with `SetLength` called
  for all 5 IDs using correct lengths: fixed for `0x007B`/`0x01DA`/`0x022C`,
  variable `-1` for `0x09DB`/`0x09FD`; `0x022C` length is **65** at pv >= 20071106)
- [ ] `Send` applies **only XOR obfuscation** via `s.Encode` — no `ShuffledCtoSID`
  call inside `Send`
- [ ] `Send` with obfuscation active emits correct XOR-obfuscated packet ID bytes —
  confirmed by `TestSend_ObfuscationApplied`
- [ ] `Send` with variable-length action (`ActionPublicChat`) returns correct frame —
  confirmed by `TestSend_VariableLengthAction`
- [ ] `Send` returns an error (not panic) for unknown action or wrong type
- [ ] All tests in `semantic_test.go` pass
- [ ] `BenchmarkRegisterAndFeed_SemanticHandler` reports allocs/op (baseline recorded;
  exactly 1 alloc/op expected on steady-state dispatch path — no build gate)
- [ ] `go build ./...` passes, `go test ./...` passes
- [ ] `grep -r --include="*.go" "^\s*go " pkg/ | grep -v "_test.go"` produces empty output (zero goroutines in production code)
- [ ] Worklog `docs/WORKLOG/NNNN_YYYY-MM-DD_us22_semantic_api.md` written

---

## Exit Criteria for EPIC-07

1. `go build ./...` — **PASS**
2. `go vet ./...` — **PASS**
3. `go test ./...` — **PASS**
4. `go test -race ./pkg/...` — **PASS**
5. `go test -bench=. -benchmem ./pkg/session/` — benchmark passes; record allocs/op as baseline (exactly 1 alloc/op on steady-state dispatch path due to unconditional `interface{}` boxing in the `receiveDispatch` lambda; 0 allocs/op is not achievable with this design)
6. `grep -r --include="*.go" "^\s*go " pkg/ | grep -v "_test.go"` — **empty** (zero goroutines in production code; test files in `pkg/fsm/` legitimately spawn goroutines and are excluded)
7. `pkg/session/actions.go` generated — every action in semantics DB has a constant; `maxSemanticAction` constant present with "highest assigned SemanticAction value" comment (not "count"); `SemanticAction.String()` method generated
8. `pkg/session/receive_dispatch.go` generated — all receive-direction implementations covered; actions with no receive-direction implementations absent with comment explaining omission
9. `pkg/encode/register.go` generated — all send-direction actions with encode functions registered via `init()`; `[N]byte` vs `[]byte` return types handled correctly; `actions.go`, `receive_dispatch.go`, and `register.go` must all be regenerated atomically in one `go generate` invocation (see Story Map for ordering: Step 10 before Step 12)
10. `pkg/session/semantic.go` hand-written — `RegisterSemanticHandler`, `Send`, `RegisterSendEncoder`, `ErrWrongSendType` implemented and tested
11. All codegen unit tests (US-19, US-20, US-21) pass in `internal/codegen/gen/`
12. Integration test `TestRegisterSemanticHandler_AllVariants` passes — all 5 packetver variants of `ActionActorMoved` dispatch correctly through `s.Feed()` with `SetLength` called for each ID using correct lengths (fixed for `0x007B`/`0x01DA`/`0x022C`, variable `-1` for `0x09DB`/`0x09FD`; `0x022C` = **65** at pv >= 20071106 — test must use packetver >= 20071106)
13. Send-side validation test: `TestSend_MoveTo` in `pkg/session/semantic_test.go`
    calls `Send` with `ActionMoveTo` and `send.MoveTo{X: 100, Y: 200}`, captures bytes
    written to a `bytes.Buffer`, and asserts for two packetvers:
    (a) `packetver=20180308`: first two bytes are `[0x5F, 0x03]` (little-endian `0x035F`,
    the post-shuffle wire ID from `ShuffledCtoSID(20180308, 0x0085)`);
    (b) `packetver=20030000`: first two bytes are `[0x85, 0x00]` (base ID `0x0085`,
    no shuffle entry);
    (c) bytes 2–4 in both cases decode to `packing.EncodePosDir(100, 200, 0)`.
    A full CZ→ZC loopback is out of scope — `ActionMoveTo` is client→server and has no
    ZC_ counterpart; feeding CZ_ bytes into the receive path tests session misuse.

---

## What This Epic Does NOT Cover

- Updating goKore to use the new API — that is follow-on integration work in goKore,
  not part of this epic
- LoginSession and CharSession semantic APIs — only `MapSession` is in scope; the auth
  phase is owned by the FSM and does not need semantic dispatch
- Compile-time enforcement that the right event type is paired with the right action
  constant — this would require one typed handle per action (280 methods) which is out
  of scope; the runtime panic at registration is sufficient
- Exposing `SemanticAction` constants for actions that have no receive-direction
  implementations — the constants are generated for all actions regardless, but
  `receiveDispatch` entries are only generated for actions that have at least one
  receive-direction implementation with a non-empty `PacketID` and `StructName`;
  the gap will be resolved as more decode functions are implemented in later epics
