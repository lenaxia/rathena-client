# Work Log 0088 — Migrate gokore-semantics MCP server to rathena-client

**Date**: 2026-07-10
**Type**: Feature — developer tooling
**Scope**:
  - `README-LLM.md` (Rule 5 rewording)
  - `go.mod`, `go.sum` (new dep: `gopkg.in/yaml.v3`)
  - `internal/codegen/semantics/loader.go` (rewritten on yaml.v3)
  - `internal/semanticsdb/` (new — editor layer over mappings.yaml)
  - `cmd/semantics-tool/{main.go,mcp/,cli/}` (new — MCP server + CLI)
  - `semantics/mappings.yaml` (delete empty duplicate action
    `received_character_ID_and_Map` — pre-existing validation finding)
**Severity**: Unblocks every future mappings.yaml edit.

---

## Problem

The README Rule 9 says:

> **ALWAYS use the `gokore-semantics` MCP server to READ and WRITE the semantic
> DB. NEVER edit `semantics/mappings.yaml` directly.**

… but no MCP server source exists in this repo, and a search of `/workspace`
found none for the rathena-client `semantic_actions:` schema anywhere accessible.
The only "tooling" was the goKore repo's `cmd/tools/semantics/` CLI, which targets
a different schema (goKore's own flat `mappings:` list with `fields:`) and cannot
edit rathena-client's `semantic_actions:` block.

Worklog 0086 (the previous issue fix) explicitly states the consequence:

> *Per README rule 9, DB edits go through the `gokore-semantics` MCP server,
> **which is not available in this environment**.*

Worklog 0087 went further and **did** edit `mappings.yaml` directly by hand, then
ran codegen to verify:

> *This PR's edit was made by hand in this agentic session (no MCP server is
> running in this workspace), then verified by re-running codegen against the
> edited file.*

Every prior mappings.yaml change in git history (d76a1d9, 79a746a, ae1c7d8)
shows plain hand-edits despite the commit messages claiming "via MCP". The Rule 9
workflow was aspirational, not actually exercised.

This worklog migrates the goKore MCP server into rathena-client and adapts it to
the rathena-client schema, so future edits can follow Rule 9 as written.

## Decision: scope the zero-deps rule to `pkg/`

The original README Rule 5 ("No External Runtime Dependencies") was repo-wide:
"`go.mod` must have zero `require` entries". This was overzealous for two reasons:

1. The rule's stated rationale ("embeddable with no transitive dependency
   surprises") only really applies to `pkg/` — the part consumers import.
2. The hand-rolled YAML parser in `internal/codegen/semantics/loader.go`
   (327 lines of indent-counting `bufio.Scanner` logic) was fragile and
   format-coupled. Writing a matching **writer** by hand would have added
   another ~150 lines of the same fragility.

Per maintainer sign-off, README Rule 5 was reworded to scope zero-deps to `pkg/`
only, and to allow `gopkg.in/yaml.v3` under `internal/` and `cmd/`. The new
wording preserves the "embeddable" property for `pkg/` (Go's module graph
excludes `internal/`/`cmd/` deps from the importer's closure when those packages
aren't imported).

## Implementation

### 1. Migrated the existing loader to yaml.v3

`internal/codegen/semantics/loader.go`:
- 327 lines → 200 lines
- All hand-parsing (`bufio.Scanner`, `countIndent`, `splitKV`, `parseIntInto`,
  `setImplField`, the indent-12/14/16 state machine) deleted
- Replaced with yaml.v3 decode into a `rawFile` struct + a `tolerantRange`
  custom unmarshaler that preserves null positions in `packetver_range:`
  sequences (yaml.v3 drops null items when decoding into `[]int` directly)
  and accepts both bare integers and quoted strings (mappings.yaml is
  inconsistent: worklog-0087 used `"20121009"` strings for the
  `zc_notify_mapproperty2` ranges)

All 6 existing tests in `internal/codegen/semantics/*_test.go` pass unchanged.

### 2. New editor package: `internal/semanticsdb/`

| File | Purpose | LOC |
|---|---|---|
| `db.go` | Types (`Action`, `Implementation`, `DB`), Load, ListActions, GetAction, GetImplementation, Statistics, search helpers | ~310 |
| `mutate.go` | CreateAction, DeleteAction, UpdateActionMetadata, AddImplementation, UpdateImplementation, DeleteImplementation, Save, SaveTo | ~520 |
| `validate.go` | Structural validation (action-name pattern, packet_id format, dup detection, packetver_range sanity, cross-action conflict detection) | ~180 |
| `search.go` | Substring search across name/openkore/description/struct/packet_id | ~50 |

Key design: load the YAML into a `yaml.Node` tree (which preserves source
formatting — comments, quote style, key order), then walk the tree to expose a
typed Go API. Mutations edit the node tree directly; `Save` serializes the whole
tree back out. The `RoundTripByteIdentical` test (see below) proves the round-trip
is byte-identical for an unmutated file.

Flow-style subtlety: yaml.v3 parses empty `[]` and `{}` as nodes with
`Style=FlowStyle`. Appending a child to such a node keeps the flow style
(`implementations: [{...}]` on one line), which doesn't match the rest of the
file. The fix is to reset `seq.Style = 0` in `getImplsSeq` and `appendImplToAction`
before appending. This was caught by the `TestCLI_AddImplementation_FlowStyleFix`
regression test.

### 3. New MCP server: `cmd/semantics-tool/mcp/`

JSON-RPC over stdio (single-goroutine blocking read loop — no goroutines spawned
by the library itself, matching the rathena-client invariant). 14 tools:

| Tool | Description |
|---|---|
| `list_actions` | List all action names alphabetically |
| `get_action` | Get full detail for one action |
| `list_implementations` | List impls of one action |
| `get_implementation` | Get one impl by packet_id |
| `search_actions` | Substring search across fields |
| `validate` | Run structural validation |
| `stats` | Aggregate counts |
| `export` | Dump whole DB as JSON |
| `create_action` | Create a new empty action |
| `update_action` | Update action description/openkore_name |
| `delete_action` | Delete an action |
| `add_implementation` | Add a packet_id impl to an action |
| `update_implementation` | Update impl struct_name or packetver range |
| `delete_implementation` | Delete one impl |

Mutating tools load → mutate → Save in one transaction (atomic per call),
matching the goKore `gokore-semantics` behaviour.

### 4. New CLI: `cmd/semantics-tool/cli/`

Built on stdlib `flag` (no cobra — that would widen the dep allowlist). 14
subcommands mirror the MCP tools 1:1. Flags come BEFORE positional args (stdlib
`flag` limitation: it stops parsing flags at the first non-flag). Documented in
`--help`.

### 5. main.go: serve → MCP, otherwise CLI

`serve` (or no args) runs the MCP server. Any other first arg runs the CLI.
Matches the goKore `cmd/tools/semantics/main.go` shape.

### 6. Pre-existing validate finding fixed

The new `validate` tool surfaced a pre-existing issue: an empty stub action
`received_character_ID_and_Map` (capital letters) existed alongside the
canonical `received_character_id_and_map` (lowercase). Both had
`implementations: []` so neither was wired to anything; the capital-case one was
a leftover from worklog 0009's case-collision fix. Deleted via the new MCP tool:

```bash
semantics-tool --file semantics/mappings.yaml delete-action received_character_ID_and_Map
```

Result: 6 lines removed, validation now clean.

## Validation

| Check | Result |
|---|---|
| `go build ./...` | ✓ pass |
| `go test -race ./...` | ✓ all packages pass, no races |
| Zero goroutines in `pkg/` production code | ✓ (`grep -rE "^\s*go " pkg/ --include="*.go" \| grep -v _test.go` is empty) |
| `pkg/` has no external imports | ✓ (`go list -deps github.com/lenaxia/rathena-client/pkg/...` produces only stdlib + self paths) |
| New mappings.yaml round-trips byte-identically with no mutation | ✓ (`TestProductionMappings_RoundTripByteIdentical`) |
| Existing codegen pipeline still works | ✓ (`go test ./internal/codegen/...` passes) |

### New test coverage

- `internal/semanticsdb/db_test.go` — 16 tests covering load, list, get,
  create/delete/update action, add/update/delete impl, validation (min>max,
  dup detection, bad action name), search, statistics, production round-trip
- `internal/semanticsdb/roundtrip_test.go` — `TestProductionMappings_RoundTripByteIdentical`
  (strongest guarantee: no-op Save produces zero bytes of change)
- `cmd/semantics-tool/cli/cli_test.go` — 11 tests covering all CLI subcommands
  end-to-end against a temp mappings.yaml, including the flow-style regression
  test
- `cmd/semantics-tool/mcp/server_test.go` — 7 tests exercising the MCP
  JSON-RPC protocol via subprocess (initialize, tools/list, stats,
  create_action, add_implementation, validate, error paths)

## Future work (not done here)

- **CI gate**: README Rule 5's new wording recommends a CI check that
  `go list -deps github.com/lenaxia/rathena-client/pkg/...` produces only stdlib
  + self paths. Adding this is left to a follow-up PR.
- **Update goKore** to consume this MCP server instead of its own (the goKore
  repo's `cmd/tools/semantics/` is unaffected by this change — it still operates
  on goKore's own `internal/network/semantics/mappings.yaml` for its own schema).
- **MCP `client` tool surface** — currently this server has no equivalent of
  goKore's `client.Database()` accessor pattern; everything goes through the
  `dispatchTool` switch. If more advanced queries are needed, expose them as new
  read-only tools rather than reaching into the DB package.

## Sources

- goKore MCP server source: `/workspace/gokore/cmd/tools/semantics/{main.go,mcp/server.go,mcp/tools.go,cli/}`
- rathena-client mappings.yaml schema: `semantics/mappings.yaml:1-6858`
- rathena-client codegen loader (pre-migration): `internal/codegen/semantics/loader.go` (327 lines, deleted)
- Worklog 0086 quote on MCP unavailability
- Worklog 0087 quote on hand-edit precedent
