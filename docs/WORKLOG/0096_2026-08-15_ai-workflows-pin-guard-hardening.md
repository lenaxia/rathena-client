# 0096 — CI: harden ai-workflows pin guard to enabled:false

**Date:** 2026-08-15
**PR:** #26
**Branch:** `ci/exclude-ai-workflows-from-renovate`
**Type:** CI / config-only change (no Go code)

## Context

Work log [0095](0095_2026-08-15_renovate-analysis-onboarding.md) onboarded
this repo to Renovate with the org-standard config, which included a
`Never auto-merge lenaxia/ai-workflows updates` rule
(`automerge: false`). The AI-or-Not onboarding review
(lenaxia/AI-or-Not#45) subsequently established — against primary sources —
that `automerge: false` is insufficient for these pins:

1. Renovate's `github-actions` manager rewrites **only the `uses:` ref**;
   `with:` inputs are only ever updated for actions in Renovate's built-in
   community `uses-with` table (`lenaxia/ai-workflows` is not in it). Any
   Renovate PR for these pins — automerged or human-merged — bumps `uses:`
   while the sibling callers' `version:` input (the checkout ref for
   scripts/prompts) stays behind.
2. Once split, propagate.yml's repair sed
   (`s|version: OLD_TAG|version: NEW_TAG|g`, with `OLD_TAG` derived from the
   `uses:` line) can never match again — a permanent strand.

This is not theoretical: **talos-ops-prod main was stranded exactly this way
today** — Renovate's branch automerge landed `22e5abac` bumping `uses:` to
v0.2.10 while `version:` stayed v0.2.9 (fixed in talos-ops-prod#2286).

## Change

`renovate.json5` — the ai-workflows rule becomes:

```json5
{
  description: ['lenaxia/ai-workflows pins are propagate-only'],
  matchPackageNames: ['lenaxia/ai-workflows'],
  enabled: false,
}
```

`enabled: false` disables Renovate for these deps entirely: no automerge,
no PRs. Pins move only via propagate.yml lockstep syncs (org-wide tracking:
ai-workflows#39). The `anomalyco/opencode` guard is unchanged — that action
takes no `version:`-style input, so ordinary PR-based updates remain safe
there.

## Verification

- `renovate.json5` parses via a JSON5 validator; 3 packageRules, only the
  ai-workflows rule changed.
- `git diff` is a single rule swap (+4 comment lines documenting why).

## References

- PR [#26](https://github.com/lenaxia/rathena-client/pull/26) — this change
- ai-workflows#39 — org-wide exclusion + template hardening
- talos-ops-prod#2286 — the live strand that motivated this
- Prior: [0095](0095_2026-08-15_renovate-analysis-onboarding.md)
