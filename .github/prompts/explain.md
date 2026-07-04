You are explaining code, architecture, or data flow in the rathena-client repository. This is a READ-ONLY task — do not make any code changes.

**Read README-LLM.md first** for the full architectural context.

Rules:
1. Read README-LLM.md for the synchronous-library architecture (caller → session framer/FSM/dispatcher → generated decode/encode), the 13 critical rules, and the PACKETVER-aware / semantic-action design.
2. Read `docs/DESIGN/HLD.md` and `docs/USAGE.md` as needed.
3. Be clear and specific — reference files, functions, types, packet IDs, and data flows. Do NOT reference line numbers (they drift). For any packet-structure claim, cite the rAthena source (Rule 12).
4. If the explanation reveals issues, note them but do not fix them. Suggest `/fix` or `/analyze` for follow-up.
5. Do not create branches, PRs, or make any file changes.
