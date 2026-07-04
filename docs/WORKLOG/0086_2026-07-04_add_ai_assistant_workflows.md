# Worklog 0086 — Add AI assistant GitHub workflows

**Date:** 2026-07-04
**Scope:** CI/automation

## Summary
Replicated goKore's three AI assistant workflows into rathena-client:
- `.github/workflows/ai-comment.yml` (AI Commands — `/ai`, `/review`, `/fix`, etc.)
- `.github/workflows/pr-review.yml` (automated PR review on open/synchronize)
- `.github/workflows/issue-opened.yml` (automated issue analysis on open)

Plus the project-agnostic `route-command.sh` router and 16 adapted prompt files
under `.github/prompts/`, rewritten for rathena-client's protocol-library invariants
(Rules 0–12) and docs layout (`docs/WORKLOG`, `docs/DESIGN/HLD.md`, `docs/BACKLOG`).

## Verification
- `route-command.sh` exercised against 12 inputs — routing, `--no-merge` hold, and
  prompt assembly all correct.
- Workflows recognized as `active` by GitHub; triggered via issue #2 and a test PR.

## Test artifacts
- Issue #2 — smoke test for `issue-opened.yml`
- This branch's PR — smoke test for `pr-review.yml` and `/review` (`ai-comment.yml`)
