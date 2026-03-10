# Phase 7 Specification — goKore Integration

**Date**: 2026-03-09  
**Status**: Awaiting approval before implementation starts  
**Repository**: `~/personal/goKore` (the integration target)  
**Library**: `~/personal/rathena-client` (what we are integrating)

---

## Overview

Phase 7 replaces goKore's entire `internal/network/` layer with rathena-client.
This is a big-bang replacement: no thin adapters, no side-by-side period.
When done, goKore's network code is ~200 lines of wiring instead of ~6,000 lines
of generated structs, registries, adapters, and params.

**The goal is not feature parity with the old code.** The goal is correct behavior
against a real rAthena server, with goKore tests passing and the bot functional.

---

## What Gets Deleted from goKore

| Directory / File | Lines | Reason |
|---|---|---|
| `internal/network/packets/generated/` | ~14,000 | Replaced by `rathena-client/pkg/decode` + `pkg/events` |
| `internal/network/packetver/` | ~2,000 | Replaced by `rathena-client/pkg/session` length tables + `pkg/fsm` |
| `internal/network/adapters/` | ~6,000 | No replacement needed; decode fns output `events.*` directly |
| `internal/network/params/` | ~3,000 | Replaced by `rathena-client/pkg/events` types |
| `internal/network/receive/` | ~600 | Replaced by `session.Feed()` call in goKore's read loop |
| `internal/network/connection/fsm.go` | ~155 | Replaced by `rathena-client/pkg/fsm.ConnectionFSM` |
| `internal/network/connection/session.go` | ~155 | Replaced by FSM-internal token storage |
| `internal/network/tokenizer_*.go` | ~400 | Replaced by `session.Feed()` framing |
| `internal/network/manager.go` | ~600 | Replaced by `internal/network/connector.go` (~100 lines) |
| `internal/network/encryption/` | ~200 | Replaced by `rathena-client/pkg/session` obfuscation |

---

## What Gets Kept in goKore

| Directory / File | Why |
|---|---|
| `internal/hook/` | goKore's event bus — kept as-is |
| `internal/network/handlers/` | Domain logic — kept; handler signatures change to accept `events.*` |
| `internal/network/send/builders/` | C→S packet builders — kept; rewritten to call `rathena-client/pkg/encode` |
| `internal/network/tcp/connection.go` | Socket ownership — kept; provides `net.Conn` to FSM Dialer |
| `internal/bot/` | Bot lifecycle — largely unchanged |

---

## User Stories

### Story 0: Prerequisites (rathena-client repo, ~1 day)

**Before any goKore work begins**, rathena-client needs:

- `go.mod` must NOT have a replace directive pointing to a local path (or goKore must use `replace`)
- Verify `go test ./...` still passes after adding `require github.com/lenaxia/rathena-client` to goKore

**Acceptance criteria**:
- `go get github.com/lenaxia/rathena-client` works from goKore's directory
  OR a `replace` directive in goKore's `go.mod` points to the local checkout
- `go build ./...` passes in goKore after the dependency is added

---

### Story 1: Add rathena-client dependency + create connector skeleton (~0.5 day)

**Scope**: goKore only

**Deliverables**:
1. `go.mod` updated with `require github.com/lenaxia/rathena-client`
   (or `replace` pointing to local `~/personal/rathena-client`)
2. `internal/network/connector.go` — new file with:
   - `type Config struct { LoginAddr, Username, Password string; CharSlot uint8; Packetver uint32; DialTimeout time.Duration }`
   - `func Start(ctx context.Context, cfg Config, dispatcher *hook.Dispatcher) error` — stub that panics "not yet implemented"
3. `internal/network/connector_test.go` — test that `Start()` with a test Dialer fires `OnReady` (using net.Pipe)

**Nothing is deleted yet** in Story 1.

**Acceptance criteria**:
- `go build ./...` passes
- Connector test passes

---

### Story 2: Wire FSM into connector + implement goKore read loop (~1 day)

**Scope**: `internal/network/connector.go`

Implement `Start()`:

```go
func Start(ctx context.Context, cfg Config, dispatcher *hook.Dispatcher) error {
    dialer := func(ctx context.Context, addr string) (net.Conn, error) {
        return net.DialTimeout("tcp", addr, cfg.DialTimeout)
    }

    serverCfg := fsm.ServerConfig{
        LoginAddr:   cfg.LoginAddr,
        Packetver:   cfg.Packetver,
        StepTimeout: 30 * time.Second,
    }

    creds := fsm.Credentials{
        Username: cfg.Username,
        Password: cfg.Password,
        CharSlot: cfg.CharSlot,
    }

    mapDone := make(chan struct{}, 1)

    f := fsm.New(serverCfg, creds, dialer).
        OnCharServerList(func(servers []events.CharServerInfo) int { return 0 }).
        OnCharList(func(chars []events.CharacterInfo) uint8 { return cfg.CharSlot }).
        OnReady(func(s *session.MapSession, conn net.Conn) {
            registerMapHandlers(s, dispatcher)
            go runMapLoop(ctx, s, conn, dispatcher, mapDone)
        }).
        OnFailed(func(err error) {
            dispatcher.Trigger(ctx, hook.EventConnectionFailed, err)
        })

    if err := f.Connect(ctx); err != nil {
        return err
    }
    <-mapDone
    return nil
}
```

**`registerMapHandlers`** is a stub that registers NO handlers yet (just logs "ready").
**`runMapLoop`** reads from conn and calls `s.Feed(buf[:n])`.

**Acceptance criteria**:
- Full auth sequence completes against real rAthena Docker (`go test -tags integration ./...`)
- `OnReady` fires
- Feed loop runs without panics
- `go test -race ./internal/network/` passes

---

### Story 3: Delete old network layers (~1 day)

**Scope**: goKore only — mass deletion

Delete in this order (each must leave `go build ./...` passing):

1. `internal/network/receive/` — delete all files; remove all imports
2. `internal/network/packets/generated/` — delete all 10 version dirs
3. `internal/network/adapters/` — delete all files
4. `internal/network/params/` — delete all files
5. `internal/network/packetver/` — delete `action_selector.go`, `registry.go`, all generated files
6. `internal/network/connection/fsm.go` + `session.go`
7. `internal/network/tokenizer_*.go`
8. `internal/network/manager.go` + `network_manager.go`
9. `internal/network/encryption/` — (rathena-client's session handles C→S obfuscation)

For each deleted package, find all import sites and either delete or update them.

**After each deletion, run `go build ./...` and fix compile errors.**

**Acceptance criteria**:
- `go build ./...` passes
- `grep -r "internal/network/packets" . | grep -v "_test.go"` returns empty
- `grep -r "internal/network/packetver" . | grep -v "_test.go"` returns empty
- `grep -r "internal/network/adapters" . | grep -v "_test.go"` returns empty
- `grep -r "internal/network/params" . | grep -v "_test.go"` returns empty
- `go test -tags integration ./internal/network/` still passes (connector tests)

**Note**: Many test files that depend on the old layers must also be deleted or rewritten.
These are the ~250 test files catalogued in the network layer analysis.
During this story, deleting old tests is acceptable — new tests come in Stories 4-8.

---

### Story 4: Authentication handlers (~0.5 day)

**Scope**: `internal/network/handlers/authentication/`

The FSM handles auth internally. goKore no longer needs packet handlers for:
- `0x0069` / `0x0AC4` (login accepted)
- `0x006A` / `0x083E` (login refused)
- `0x0081` (ban / zone server)
- `0x0065` (char connect)
- `0x006B` / `0x099D` (char list)
- `0x0071` / `0x0AC5` (map server info)
- `0x0283` (account ID echo)
- `0x0073` / `0x02EB` (map entry)

These are all handled by `pkg/fsm` internally. The goKore auth callbacks are:

```go
fsm.New(...).
    OnCharServerList(func(servers []events.CharServerInfo) int { ... }).
    OnCharList(func(chars []events.CharacterInfo) uint8 { ... }).
    OnReady(func(s *session.MapSession, conn net.Conn) { ... }).
    OnFailed(func(err error) { ... })
```

**Deliverables**:
- `internal/network/handlers/authentication/` rewritten to wire the FSM callbacks to `hook.Dispatcher` events:
  - `OnCharList` fires `hook.CharCharacterListEvent`
  - `OnReady` fires `hook.MapLoadedEvent`
  - `OnFailed` fires `hook.AuthLoginFailureEvent`

**Acceptance criteria**:
- `go test -tags integration ./internal/network/` passes (auth sequence verified)
- `hook.AuthLoginSuccessEvent`, `hook.CharCharacterListEvent`, `hook.MapLoadedEvent` all fire during integration test

---

### Story 5: Actor / movement handlers (~1 day)

**Scope**: `internal/network/handlers/actors/`, `internal/network/handlers/movement/`

Register in `registerMapHandlers`:

| Packet ID | rathena-client decode fn | goKore event |
|---|---|---|
| `0x09FF` / `0x0078` / `0x01D8` | `decode.ActorExists_0x09FF` etc. | `hook.ActorSpawnedEvent` |
| `0x09FE` / `0x01D9` | `decode.ActorConnected_0x09FE` etc. | `hook.ActorSpawnedEvent` |
| `0x09DB` / `0x007B` / `0x022C` | `decode.ActorMoved_0x09DB` etc. | `hook.ActorSpawnedEvent` (moving) |
| `0x0080` | `decode.ActorDiedOrDisappeared_0x0080` | `hook.EntityVanishedEvent` |
| `0x0087` / `0x035F` | `decode.CharacterMoves_0x0087` etc. | `hook.PlayerMovementConfirmedEvent` |

Handler signature pattern:
```go
s.RegisterHandler(0x09FF, func(data []byte, pv uint32) {
    e := decode.ActorExists_0x09FF(data, pv)
    dispatcher.Trigger(ctx, hook.EventActorSpawned, hook.ActorSpawnedEvent{
        ID:         e.ID,
        ActorType:  uint8(e.ObjectType),
        Speed:      uint16(e.Speed),
        X:          e.X,
        Y:          e.Y,
        Dir:        e.Dir,
    })
})
```

**Acceptance criteria**:
- `go test ./internal/network/handlers/actors/` passes
- `go test ./internal/network/handlers/movement/` passes
- Integration test with real server: `ActorSpawnedEvent` fires with non-zero ID

---

### Story 6: Stats handlers (~0.5 day)

**Scope**: `internal/network/handlers/stats/`

| Packet ID | rathena-client decode fn | goKore event |
|---|---|---|
| `0x00B0` | `decode.StatUpdate_0x00B0` | `hook.StatsUpdatedEvent` (may synthesize `LevelUpEvent`, `ZenyChangedEvent`) |
| `0x00B1` | `decode.StatUpdate_0x00B1` | `hook.StatsUpdatedEvent` |
| `0x00BD` | `decode.ZcStatus_0x00BD` | `hook.StatsFullDumpEvent` |
| `0x0141` | `decode.CoupleStatus_0x0141` | `hook.StatsFullDumpEvent` (supplement) |
| `0x0ACB` | `decode.LonglongparChange_0x0ACB` | `hook.LongLongParamChangeEvent` |

**Derived events from StatUpdate** (handler synthesizes based on `StatType`):
- `SP_BASELEVEL` / `SP_JOBLEVEL` changes → fire `hook.LevelUpEvent`
- `SP_ZENY` changes → fire `hook.ZenyChangedEvent`
- `SP_HP` / `SP_MAXHP` / `SP_SP` / `SP_MAXSP` → update internal HP/SP cache

**Acceptance criteria**:
- `go test ./internal/network/handlers/stats/` passes
- Integration test: `StatsFullDumpEvent` fires on map entry

---

### Story 7: Combat / skills handlers (~1 day)

**Scope**: `internal/network/handlers/combat/`, `internal/network/handlers/skills/`, `internal/network/handlers/status/`

| Packet ID | rathena-client decode fn | goKore event |
|---|---|---|
| `0x008A` / `0x0977` | `decode.ActorAction_*` | `hook.CombatAttackActionEvent` |
| `0x0196` | `decode.ActorStatusActive_0x0196` | `hook.StatusEffectChangedEvent` |
| `0x043F` | `decode.ActorStatusEffectExtended_0x043F` | `hook.StatusEffectChangedEvent` |
| `0x119` / `0x07FB` | `decode.SkillCast_*` | `hook.SkillCastStartedEvent` |
| `0x01DE` / `0x04D7` | `decode.SkillUse_*` | `hook.SkillDamageEvent` |
| `0x011A` | `decode.SkillUsedNoDamage_0x011A` | `hook.SkillUsedEvent` |
| `0x0114` | `decode.SkillUseFailed_0x0114` | `hook.SkillFailedEvent` |
| `0x043D` | `decode.SkillPostDelay_0x043D` | `hook.SkillCooldownEvent` |
| `0x02C4` | `decode.AreaSpell_0x02C4` | `hook.GroundSkillPlacedEvent` |
| `0x0120` | (decode fn) | `hook.GroundSkillRemovedEvent` |
| `0x01D7` | (decode fn) | `hook.VisualEffectEvent` (sprite change) |

**Acceptance criteria**:
- `go test ./internal/network/handlers/combat/` passes
- `go test ./internal/network/handlers/skills/` passes
- `go test ./internal/network/handlers/status/` passes

---

### Story 8: Inventory / items handlers (~1 day)

**Scope**: `internal/network/handlers/items/`

This is the most complex handler group because:
- Cards field is `[]byte` in rathena-client, `[4]uint16` in goKore
- Equip/unequip packets don't include ItemID — must look up from inventory state
- Stackable and non-stackable item list packets require parsing sub-structs

**Inventory state** (local to handler): maintain `map[uint16]uint16` (slot → ItemID) updated on each `InventoryItemAdded`, used for equip/unequip/arrow lookups.

**Acceptance criteria**:
- `go test ./internal/network/handlers/items/` passes
- Integration test: `InventoryItemAddedEvent` fires on map entry

---

### Story 9: NPC / chat / social handlers (~0.5 day)

**Scope**: `internal/network/handlers/npc/`, `internal/network/handlers/chat/`

NPC handlers are straightforward FULL MATCH cases (see analysis above).

Chat handlers need name lookup for `ChatMessage` (actor name not in packet).
Existing goKore logic for name lookup stays in the handler.

**Acceptance criteria**:
- `go test ./internal/network/handlers/npc/` passes
- `go test ./internal/network/handlers/chat/` passes

---

### Story 10: Send builders rewrite (~1 day)

**Scope**: `internal/network/send/builders/`

Replace goKore's send builders with calls to `rathena-client/pkg/encode`:

| goKore builder | rathena-client encode fn |
|---|---|
| `map_builder.go` RequestMove | `encode.RequestMove_0x035F(req, pv)` |
| `combat_builder.go` Attack | `encode.ActorAction_*` |
| `skill_builder.go` UseSkill | `encode.SkillUse_*` |
| `npc_builder.go` ContactNPC | `encode.NpcContact_*` |
| `chat_builder.go` SendChat | `encode.PublicChat_*` / `encode.SendChat_*` |
| `item_builder.go` PickupItem | `encode.PickupItem_*` (if present) |

For C→S packets not in rathena-client encode, the builder writes raw bytes directly.
The `MapSession.Encode()` method applies C→S packet ID obfuscation.

**Acceptance criteria**:
- `go test ./internal/network/send/` passes
- Integration test: bot sends RequestMove, receives position ACK

---

### Story 11: End-to-end integration test (~0.5 day)

Final validation:

1. `go test -tags integration -timeout 120s ./...` passes
2. Bot connects to rAthena Docker, authenticates, enters map
3. Asserts: `AuthLoginSuccessEvent`, `CharCharacterListEvent`, `MapLoadedEvent`, `ActorSpawnedEvent`, `StatsFullDumpEvent` all fire
4. Bot sends `RequestMove`, receives `PlayerMovementConfirmedEvent`
5. `go test -race ./...` passes (no races)
6. `go build ./...` passes clean

---

## Migration Risk Assessment

| Risk | Likelihood | Mitigation |
|---|---|---|
| rathena-client missing packets (events not in `pkg/events/`) | Medium | Gap list identified above; add to rathena-client in Story 0+ |
| goKore test count drops significantly | High | ~250 test files deleted; ~80 new tests written; net test count drops — acceptable |
| Handler behavior regression | Medium | Integration tests against real rAthena Docker verify correctness |
| C→S send builders missing in rathena-client `pkg/encode/` | Low | 80 encode functions cover most; gaps write raw bytes |

---

## Known Gaps in rathena-client (packets needed by goKore that don't exist yet)

These need to be added to rathena-client **before** their goKore handlers can be written:

1. **Inventory body items list** — `ZC_INVENTORY_ITEM_LIST_EQUIP` / `ZC_INVENTORY_ITEM_LIST_STACKABLE` (stackable + non-stackable item lists) — complex structs with variable-length sub-entries
2. **Character list parsing** — `ReceivedCharacters.CharInfo []byte` needs helper in rathena-client to decode `CHARACTER_INFO` structs
3. **Trade item from other player** — which packet ID covers `TradeItemAddedByOtherEvent`?
4. **Storage item list** — `ZC_STORE_NORMAL_ITEMLIST` variants
5. **Party invite received** (S→C) — find the correct packet ID

These gaps can be addressed as part of their respective stories (Stories 8-9) or added to rathena-client's Phase 2 backlog if not needed for the initial integration.

---

## Story Ordering and Estimated Duration

| Story | Description | Days |
|---|---|---|
| 0 | rathena-client dep / go.mod | 0.5 |
| 1 | Connector skeleton | 0.5 |
| 2 | FSM wired + read loop | 1.0 |
| 3 | Delete old layers | 1.0 |
| 4 | Auth handlers | 0.5 |
| 5 | Actor / movement | 1.0 |
| 6 | Stats | 0.5 |
| 7 | Combat / skills | 1.0 |
| 8 | Inventory / items | 1.0 |
| 9 | NPC / chat / social | 0.5 |
| 10 | Send builders | 1.0 |
| 11 | E2E integration test | 0.5 |
| **Total** | | **~9 days** |

---

## Acceptance Criteria for Phase 7 Complete

1. `go build ./...` passes in goKore (zero compile errors)
2. `go test ./...` passes in goKore (excluding integration-tagged tests)
3. `go test -tags integration -timeout 120s ./...` passes against rAthena Docker
4. `go test -race ./...` passes (zero races)
5. Zero references to deleted packages (`packets/generated`, `adapters`, `params`, `packetver`) in non-test code
6. `internal/network/connector.go` ≤ 200 lines
7. Work log created (`docs/WORKLOG/0025_...`) in rathena-client
8. Work log created (`docs/07_WORK_LOG/...`) in goKore

---

## Open Questions for Discussion

Before implementation starts, confirm:

1. **go.mod approach**: Use `replace` directive pointing to `~/personal/rathena-client` (simplest for local dev), or require a git tag? Recommendation: `replace` for now, tag later.

2. **Which goKore tests to keep vs. delete**: The ~250 test files in `internal/network/` test the old layer. Delete all of them in Story 3, then write new tests for the new connector + handlers in Stories 4-11?

3. **Scope creep guard**: If a goKore handler uses a field that is not in any rathena-client event struct, is it acceptable to zero/skip that field (mark as TODO) or must it be resolved before the story is marked complete?

4. **goKore hook events**: The ~120 goKore `hook.EventXxx` constants stay unchanged. Only the structs flowing through them change (from `params.*` to `events.*`). Confirm this is the intended approach.
