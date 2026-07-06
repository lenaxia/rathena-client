You are implementing a feature or user story for the rathena-client repository.

**Read README-LLM.md first** — it contains the 13 critical rules (Rules 0–12).

Rules:
1. Read README-LLM.md before making any changes — it contains hard rules for TDD, type safety, architecture, the synchronous-library contract, and rAthena-as-source-of-truth.
2. Read the relevant design document — `docs/DESIGN/HLD.md` (the authoritative design), or the relevant `docs/BACKLOG/` epic/user-story — before starting.
3. Follow TDD (Rule 1): write tests FIRST — they must fail, then implement, then pass. Multiple happy-path + unhappy-path + edge cases + fuzz tests (for bit-packing) + benchmarks (0 allocs/op for decode/encode).
4. For any packet-structure work, clone rAthena and source field names, C types, order, and sizes from rAthena only: `git clone --depth 1 https://github.com/rathena/rathena.git /tmp/rathena`, then cross-reference `/tmp/rathena/src/map/packets_struct.hpp` (primary), `packets.hpp`, and `common/packets.hpp`, and trace `#if PACKETVER` conditionals (Rule 6). Query the semantic DB via the `gokore-semantics` MCP server only as a starting point — never edit `semantics/mappings.yaml` directly — and verify against rAthena source before implementing (Rules 9, 12). Cite every claim. If rathena-client/DB/intuition disagrees with rAthena, rAthena wins.
5. No `interface{}`, `any`, or reflection in any decode/encode path; keep the public API packet-ID agnostic — no `uint16` packet ID in any exported signature (Rule 8).
6. Never add a goroutine in `pkg/` (Rule 3) or an external dependency to `go.mod` (Rule 5). Never grep or edit `semantics/mappings.yaml` directly (Rule 9).
7. Never perform destructive git operations (Rule 2).
8. Run full-repo validation before pushing — zero failures required (full repo):
   ```bash
   go build ./...
   go test ./...
   go test -race ./...
   grep -r "^\s*go " pkg/                # must be empty
   go test -bench=. -benchmem ./pkg/...  # decode/encode alloc targets (Rule 4)
   ```
9. Create a work log in `docs/WORKLOG/` (Rule 0).
10. Leave the codebase in zero-error state — fix any pre-existing errors you encounter.
