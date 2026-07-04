You are fixing a bug in the rathena-client repository.

**Read README-LLM.md first** — it contains the 13 critical rules (Rules 0–12).

Rules:
1. Read README-LLM.md and `docs/DESIGN/HLD.md` (relevant section) before making any changes.
2. Identify the root cause — do not fix symptoms. For packet-structure bugs, verify ground truth against rAthena via the GCC preprocessor (`validation/`) before changing anything (Rules 6, 9, 12).
3. Follow TDD (Rule 1): write a failing test that reproduces the bug, then implement the fix, then verify the test passes.
4. Include regression tests that would catch the bug if it reappears.
5. No `interface{}`, `any`, or reflection in any decode/encode path; keep the public API packet-ID agnostic (Rule 8).
6. Never add a goroutine in `pkg/` (Rule 3) or an external dependency to `go.mod` (Rule 5). Never grep or edit `semantics/mappings.yaml` directly — use the `gokore-semantics` MCP server only (Rule 9).
7. Never perform destructive git operations (`git checkout .`, `git reset --hard`, `git clean -fd`) — Rule 2.
8. Run full-repo validation before pushing — zero failures required (full repo, not just your code):
   ```bash
   go build ./...
   go test ./...
   go test -race ./...
   grep -r "^\s*go " pkg/                # must be empty
   go test -bench=. -benchmem ./pkg/...  # decode/encode alloc targets (Rule 4)
   ```
9. Create a work log in `docs/WORKLOG/` (Rule 0).
10. If the fix touches multiple layers (decode/encode → session framer → dispatcher), ensure tests cover the full byte-in → typed-event-out path.
