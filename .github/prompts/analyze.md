You are performing a deep analysis of the rathena-client codebase. This is a READ-ONLY task — do not make any code changes.

**Read README-LLM.md first** for full architectural context.

Rules:
1. Read README-LLM.md for the synchronous-library architecture (caller → session → generated decode/encode), the 13 critical rules, and the verification pipeline (GCC preprocessor + semantic DB).
2. Read `docs/DESIGN/HLD.md` and relevant `docs/BACKLOG/` items as needed.
3. Be specific — reference file paths, function names, type names, packet IDs, and data flows. Do NOT reference line numbers (they drift). For any packet-structure claim, cite the rAthena source or GCC preprocessor output (Rule 12).
4. If you find bugs or design flaws, describe them precisely with reproduction steps or code references.
5. Do not create branches, PRs, or make any file changes.
6. If the analysis reveals issues that should be fixed, suggest using `/fix` or `/implement` in your response.
7. For packet-structure analysis, clone rAthena and ground every claim in source: `git clone --depth 1 https://github.com/rathena/rathena.git /tmp/rathena`, then cite specific structs in `/tmp/rathena/src/map/packets_struct.hpp` (primary), `packets.hpp`, or `common/packets.hpp`. Never analyze packet behaviour from memory — read the actual rAthena source (Rule 12).

Output format:
## Analysis

### Topic
[What was analyzed]

### Findings
[Detailed findings with code references]

### Recommendations
[Suggested actions, if any — reference appropriate commands like `/fix` or `/implement`]
