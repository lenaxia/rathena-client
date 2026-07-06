You are writing or improving tests for the rathena-client repository.

**Read README-LLM.md first** — TDD is mandatory (Rule 1).

Rules:
1. Follow the project's testing requirements exactly:
   - Multiple happy-path tests
   - Multiple unhappy-path tests (errors, invalid inputs, malformed/short bytes, boundary failures)
   - Edge case coverage
   - Integration tests that exercise the full byte-in → typed-event-out path (feed raw bytes through `session.Feed(...)` and assert the typed callback fires) — unit tests alone are not sufficient
2. Use table-driven tests following existing patterns in the codebase.
3. All tests must pass with `-race` flag: `go test -timeout 30s -race ./...`
4. For bit-packing functions, include **fuzz tests** (Rule 1).
5. For decode/encode functions, include **benchmarks verifying 0 allocs/op** (Rule 4) — run `go test -bench=. -benchmem ./pkg/...` and confirm no unexpected allocations.
6. For malformed or truncated server packets, the decoder must NEVER panic — add tests with short/garbage bytes.
7. Run full-repo validation before pushing — zero failures required (full repo):
   ```bash
   go build ./...
   go test ./...
   grep -r "^\s*go " pkg/   # must be empty
   ```
8. For new test files, follow the naming convention: `*_test.go` in the same package.
9. Check existing test files for patterns and utilities before writing new ones.
10. Create a work log in `docs/WORKLOG/` (Rule 0).
11. For decode/encode golden test bytes, derive the expected field layout from rAthena source, not intuition: `git clone --depth 1 https://github.com/rathena/rathena.git /tmp/rathena` and cross-reference the struct in `/tmp/rathena/src/map/packets_struct.hpp` (primary) so the test asserts the real rAthena field names/C types/order/sizes (Rule 12). A test that encodes the wrong layout and asserts it against itself proves nothing about compatibility.
