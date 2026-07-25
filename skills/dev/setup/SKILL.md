---
name: dev-setup
description: One-time project setup for the dev pipeline -- stamps the pipeline map into AGENTS.md/CLAUDE.md and bootstraps the issue board. Idempotent, re-run to refresh.
disable-model-invocation: true
---

One-time setup that makes a repo ready for the dev pipeline (`/explore` → `/propose` → `/implement` → `/watchdog` → human merge). Every step is idempotent: re-running refreshes only what this skill owns and touches nothing else.

## 1. Stamp the pipeline map

Append the block below verbatim to `AGENTS.md` (create the file if missing). Never modify content outside the markers; if the markers already exist, replace only what is between them.

For `CLAUDE.md`: if it exists, add a `@AGENTS.md` import line if not already present; if it does not exist, create it containing just that line.

```md
<!-- dev-pipeline:start -->
## Dev pipeline

This repo is developed through a staged pipeline. Each stage is a skill; invoke the stage you are asked to perform.

| Stage | Skill | When |
|---|---|---|
| Explore | `/explore` | Interview to reach shared understanding; durable docs via `/domain` |
| Propose | `/propose` | Materialize the conversation into tracer-bullet slices on the board |
| Implement | `/implement` | Claim one slice, TDD it at the pinned seams, refactor via `/review`, submit |
| Watchdog | `/watchdog` | Adversarial double verification in a fresh context; lands or bounces |
| Merge | human | Accepts the change; the acceptance gate |

Reference skills: `/design` (deep modules & seams), `/domain` (glossary, ADRs, capabilities), `/tdd` (the red → green loop), `/review` (two-axis review engine).

- Board protocol: `docs/github.md`. Labels: `ready` → `wip` → `review` → `done`, with `rework` for bounces.
- Change artifacts: `.changes/<slug>/` on the slice branch; archived to `.changes/archive/<date>-<slug>/` on a watchdog pass.
- Worktrees: `.worktrees/<slug>`, one per slice, gitignored.
- Durable docs: `CONTEXT.md` (glossary), `docs/adr/`, `docs/capabilities/`.
<!-- dev-pipeline:end -->
```

## 2. Bootstrap the board

- Copy [github.md](./reference/github.md) to `docs/github.md`, overwriting any previous copy — this skill owns that file.
- Create the labels defined there (`gh label create ... --force` is safe to re-run).

## 3. Repo hygiene

- Ensure `.worktrees/` is in `.gitignore`; add the line if it is missing.
