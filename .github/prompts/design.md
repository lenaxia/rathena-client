You are iterating on a **design document** for the rathena-client repository — the step that comes *before* `/implement` or `/fix`. The goal is a reviewed, approved design, not code.

Output target: a design document under `docs/DESIGN/` (for cross-cutting/architectural work) or `docs/BACKLOG/EPIC-XX/` (for epic- or story-scoped work), following the repository's existing conventions. `docs/DESIGN/HLD.md` is the authoritative design document — update it in place if the design changes architecture, never silently duplicate.

Rules:
1. Read README-LLM.md first — especially the architecture section (the synchronous transformation layer, PACKETVER-aware framer, semantic-action API), the 13 critical rules, and the verification pipeline (GCC preprocessor + semantic DB). Read any existing doc that touches the same area before writing.
2. Decide where the design lives:
   - Cross-cutting / architectural → a new file in `docs/DESIGN/` named descriptively, or an in-place edit to `docs/DESIGN/HLD.md`.
   - Story- or epic-scoped → the relevant `docs/BACKLOG/EPIC-XX/` directory.
   - Updating an existing design → edit it in place; do not silently duplicate.
3. Scope the design to the request text from the collaborator. If the request is ambiguous, state the ambiguity explicitly and pick the narrowest reasonable scope (Rule 10).
4. A design doc must cover at minimum: problem statement, goals/non-goals, proposed design, alternatives considered, data-flow / component interactions, failure-mode analysis, and open questions. Trace every claim about a packet ID, field type, or struct layout to source (rAthena file:line or GCC preprocessor output) — do not describe behaviour from memory (Rules 6, 12).
5. State assumptions up front (with confidence levels) and validate each one against source/tests before relying on it.
6. Workflow — follow the Code Change Workflow: feature branch (`design/` or `docs/` prefix), open a PR, iterate through the automated review until it posts APPROVE.
7. **MERGE HOLD — this command never auto-merges.** After the automated review posts APPROVE, STOP. Do not merge. Post a comment on the PR summarising the design and stating it is approved and awaiting an explicit `/merge`.
8. Do not write production code in this step — only the design document and supporting diagrams/tables.
