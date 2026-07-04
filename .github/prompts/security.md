You are performing a security-focused review of the rathena-client codebase.

**Read README-LLM.md first** for security-relevant coding standards.

Rules:
1. Check every one of these areas:
   - **Malicious server packets:** A server is an untrusted input source. Can any packet cause an out-of-bounds read, a panic, or index-out-of-range in a decode function? Is every byte read bounds-checked against the declared packet length?
   - **Input validation:** Are count fields, length fields, and variable-length payloads validated at the boundary before use? Could a crafted count field drive a huge allocation (resource exhaustion / DoS)?
   - **Packet handling:** Are there manual byte-construction paths bypassing the generated encoders? Generated encode functions enforce correct framing — raw byte paths bypass that.
   - **Hot-path type safety:** Is there any `interface{}`, `any`, or reflection in a decode/encode path? That is forbidden (Rule 8) and a panic/performance hazard.
   - **Decode allocations:** Does a change risk escaping an event struct to the heap, increasing GC pressure under load (1000 concurrent consumers)? (Rule 4)
   - **Concurrency:** Are there goroutines spawned inside `pkg/`? That is forbidden (Rule 3) and at 1000 consumers multiplies 1000×. The library must remain a pure synchronous transformation.
   - **Credentials:** Do login/char/map credentials flow through the library? Verify usernames/passwords are never logged unredacted.
   - **Semantic DB / rAthena:** Is any code grepping or editing `semantics/mappings.yaml` directly instead of using the `gokore-semantics` MCP server? That bypasses validation. (Rule 9)
   - **Secrets:** Are there hardcoded secrets, API keys, or credentials in the diff?
   - **External dependencies:** Has a `require` entry been added to `go.mod`? That is forbidden (Rule 5) and expands the attack/dependency surface.
2. If code changes are needed to fix security issues, create a branch, open a PR, and follow the code change workflow.
3. Never handle or create secrets.
4. For read-only security analysis, post findings as a comment.

Output format:
## Security Review

### Scope
[What was reviewed]

### Findings
| # | Severity | Description | Location | Remediation |
|---|----------|-------------|----------|-------------|
| 1 | Critical/High/Medium/Low | [description] | file:line | [fix] |

### Threat Surface Impact
[How this affects the overall threat surface]

### Verdict
[SAFE / CONCERNS FOUND] — [one sentence summary]
