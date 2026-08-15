# 0095 — Renovate-Analysis Onboarding (Reusable Workflow)

**Date:** 2026-08-15  
**PR:** #25  
**Type:** CI / Chore  
**Status:** Pending review

## Problem

Renovate PRs (opened by `renovate[bot]`) were not AI-analyzed before merge.
The org now ships a reusable analysis workflow in `lenaxia/ai-workflows`
(PR #33, tag `v0.2.10`): it posts a `## Renovate PR Analysis` comment on every
open `renovate[bot]` PR and auto-merges only PRs it recommends.

The reusable workflow must be triggered by `schedule` + `workflow_dispatch`
only — never `pull_request` — because the opencode action's
`assertPermissions()` requires the event actor to be a repo collaborator with
write access, and `renovate[bot]` (an app bot account) can never be one. Every
`pull_request`-triggered run fails before the AI starts.

## Solution

Thin caller `.github/workflows/renovate-analysis.yml` delegating to the
reusable workflow, plus a forked repo-specific prompt
`.github/prompts/renovate-analysis.md` (the reusable job `cat`s
`context.md` + `core-rules.md` + `renovate-analysis.md` from the consumer
repo). Org-standard `renovate.json5` added with the GitHub-Actions
digest/patch/minor auto-merge rule and never-auto-merge guards.

## Upstream Verification

- Tag `v0.2.10` exists (`gh api repos/lenaxia/ai-workflows/git/refs/tags/v0.2.10`
  → annotated tag `126111e`, commit `5f900a2`).
- Reusable workflow at that tag declares exactly the inputs the caller passes:
  `project_name` (required, provided as `rathena-client`), `pr_number`
  (optional, default `""`), `runs_on` (optional).
- Caller triggers, `permissions:` block (id-token/contents/issues/
  pull-requests: write), `secrets: inherit`, and cron `0 */2 * * *` match the
  documented caller contract.
- `pr_number: ${{ inputs.pr_number }}` without `|| ''` is safe: on `schedule`
  events the `inputs` context is unpopulated and resolves to empty string.
- Prompt repo facts verified: `go.mod` requires only `gopkg.in/yaml.v3 v3.0.1`;
  the fork carries the correct zero-dependency-contract exclusions.

## Guardrails Carried

- `renovate.json5`: `anomalyco/opencode` and `lenaxia/ai-workflows` are never
  auto-merged (open versions change permission/workflow semantics — the same
  `assertPermissions()` failure class as above).
- Prompt retains the read-only guardrails: scratch files under `/tmp` only,
  "DO NOT post any comment" when no open PRs, no branch creation
  (`persist-credentials: false` upstream), post-and-verify loop, conservative
  "when in doubt → manual review" default.

## Post-Merge Verification Plan

1. Confirm the consumer-config change lands (`renovate-analysis.md` listed
   under `forked:` in `consumers/rathena-client.yaml`, ai-workflows#36) before
   the next propagate run — avoids overwriting the repo-specific fork.
2. Dispatch the workflow manually (`workflow_dispatch`, no `pr_number`) and
   confirm the analysis comment lands on any open Renovate PR.
3. Confirm schedule cadence (every 2h) picks up newly-opened Renovate PRs.

## Test Results

- `renovate.json5` validated as JSON5 (Renovate natively supports `.json5`).
- Diff vs `origin/main`: exactly the 3 claimed files, +145/-0 — no `.go`
  files touched, so `go build`/`go test ./...` outcomes cannot differ from
  `main`.