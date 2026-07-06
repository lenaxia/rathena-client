Repository: rathena-client — a pure-Go Ragnarok Online wire-protocol library (`github.com/lenaxia/rathena-client`, Go 1.24.0) implementing the rAthena login, char, and map server protocol. It is **not** a game client, bot, or application — it is a pure synchronous transformation: raw TCP bytes in → typed version-agnostic callbacks out; typed send-request structs in → raw TCP bytes out. Its primary consumer is goKore (`github.com/lenaxia/gokore`), which imports it as its network layer. Single maintainer: @lenaxia.

Key directories:
- pkg/packing/         — Bit-packing codecs (WBUFPOS 3-byte, WBUFPOS2 6-byte)
- pkg/events/          — 281 typed event structs (S→C, generated)
- pkg/send/            — 152 typed send-request structs (C→S, generated)
- pkg/decode/          — 282 decode functions (generated, 0 allocs/op)
- pkg/encode/          — 178 encode functions + shuffle table (generated)
- pkg/session/         — PACKETVER-aware framer + dispatcher + ConnectionFSM
                         (LoginSession, CharSession, MapSession, SemanticAction API)
- internal/codegen/    — Code generator (GCC + semantics pipeline)
- semantics/           — Semantic DB (accessed via gokore-semantics MCP ONLY — never edit mappings.yaml directly)
- validation/          — Pre-implementation verification scripts (GCC preprocessor, length check, DB validate)
- cmd/                 — Entry points (codegen, gen-fixture)
- docs/DESIGN/HLD.md   — Authoritative design document (architecture, algorithms, data flows)
- docs/BACKLOG/        — Active planning (epics, user stories, tech debt)
- docs/WORKLOG/        — Mandatory work logs for every task (NNNN_YYYY-MM-DD_description.md)

Architecture is a single synchronous transformation layer: **caller → session (framer + FSM + dispatcher) → generated decode/encode**. The library owns NO goroutines and NO concurrency — `session.Feed()` is synchronous and the caller owns threading. The public API is packet-ID agnostic: callers register handlers and send by **semantic action** (`session.RegisterSemanticHandler`, `session.Send(ms, conn, session.ActionXxx, send.Xxx{...})`), never by `uint16` packet ID. Handlers/decoders NEVER touch raw bytes directly — generated decode functions do the byte reads. rAthena is the ONLY source of truth for packet structure, field names, and field types; the semantic DB is a starting point that must be verified against rAthena via the GCC preprocessor before trusting it.

---

## Cross-referencing rAthena (MANDATORY for any packet-structure work)

rAthena (`https://github.com/rathena/rathena`) is the ONLY source of truth for packet structure, field names, field types, field order, and sizes (README-LLM.md Rule 6). This workflow environment has **no pre-existing rAthena checkout**, so for any task that touches a packet struct, field, size, or PACKETVER branch you MUST clone it first:

```bash
git clone --depth 1 https://github.com/rathena/rathena.git /tmp/rathena
```

Then cross-reference every field/name/type/size/offset claim against the authoritative headers (in local dev these live at `~/personal/rathena`, i.e. `RATHENA_ROOT`):

- `/tmp/rathena/src/map/packets_struct.hpp` — packet struct definitions (**PRIMARY**)
- `/tmp/rathena/src/map/packets.hpp` — packet ID / length declarations
- `/tmp/rathena/src/common/packets.hpp` — shared packet types
- `/tmp/rathena/src/map/clif.cpp` — packet send/receive sites (confirms real field usage)

For each struct, read the source and confirm the rathena-client decode/encode matches rAthena exactly: field names (e.g. `AID`, `GID`, `speed`), C types (`uint16`, `int32`, `uint8[3]`), order, and total size. Trace `#if PACKETVER` / `#if PACKETVER_MAIN_NUM` conditionals manually to confirm the layout for the target PACKETVER. If the GCC preprocessor is available, use it for ground truth:

```bash
g++ -E -P -DPACKETVER=YYYYMMDD -DPACKETVER_MAIN_NUM=YYYYMMDD \
    -I /tmp/rathena/src -I /tmp/rathena/src/map -I /tmp/rathena/src/common \
    /tmp/rathena/src/map/packets_struct.hpp 2>/dev/null | grep -A 30 "struct <name> "
```

**Compatibility with rAthena is the whole point.** If rathena-client code, the semantic DB, OpenKore, or your own intuition disagrees with rAthena source, **rAthena wins** — never ship a packet change that is not backed by a cited rAthena file reference (Rule 12). Do not invent, assume, or recall packet structure from memory.

---

**Before doing anything else: read README-LLM.md at the repo root.** It contains the 13 critical rules (Rules 0–12: mandatory work logs, TDD, no destructive git ops, zero goroutines in `pkg/`, zero heap allocations in the decode hot path, no external runtime dependencies, rAthena as source of truth, HLD/semantic-DB authority, type safety with no `interface{}`/reflection in the hot path, semantic DB via MCP only, ask before deciding, no comments in code, no unverified claims), the full architecture overview, and the development workflow. Every response must be consistent with it.

---

## Commands

Post a comment on the issue or PR using any of these commands:

- `/ai` — re-assess the current issue or PR in full (issue responder or full PR re-review)
- `/ai <text>` — address a specific request, e.g. `/ai can you also add fuzz tests for the WBUFPOS2 codec?`
- `/review [text]` — explicit PR code review, optionally focused on a specific area
- `/fix <description>` — fix a bug: branch, TDD regression tests, PR, iterate through review until approved, merge
- `/implement <description>` — implement a feature/story: TDD, multi-agent workflow, PR, iterate until approved, merge
- `/test <target>` — write or improve tests: TDD, PR, iterate until approved, merge
- `/analyze [text]` — deep read-only analysis, posts findings as a comment (no code changes)
- `/explain <topic>` — explain code or architecture, posts explanation as a comment (no code changes)
- `/security [text]` — security-focused review
- `/triage [text]` — triage an issue: categorize, prioritize, suggest labels
- `/design [text]` — iterate on a design document before implementing: opens a PR, iterates through review, **holds for `/merge`** (never auto-merges)
- `/merge` — explicitly merge an approved PR (squash). Use after `/design`, or after `/fix`/`/implement`/`/test`/`/security` invoked with `--no-merge`
- `/help` — show full command reference

Text after the command is appended to the prompt for custom tuning. All code-change commands (`/fix`, `/implement`, `/test`, `/security`) follow the review-iterate-approve-merge workflow: branch → PR → auto-review → fix → push → re-review → repeat until approved → merge. Append `--no-merge` to any of them to hold the merge until you post `/merge`. `/design` always holds.

The assistant will be triggered automatically and will read README-LLM.md and the full thread before responding.
