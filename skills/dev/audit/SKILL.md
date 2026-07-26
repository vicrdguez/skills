---
name: audit
description: Review the changes since a fixed point (commit, branch, tag or merge-base) along two axes - Standards (does the code follow this repo documented coding standards) and change Artifacts (does the code match what the originating change asked for?) Runs both reviews in parallel subagents and reports them side by side. Use when the user wants to review a branch, a PR, work-in-progress changes or asks to "review since X"
---

Two-axis review of the diff between `HEAD` and a fixed point the user supplies:

- **Standards** — does the code conform to this repo's documented coding standards? This includes any documented standard in the repo but also make use of project/repo specific skills to aid here. E.g. the repo has a specific skill to review a module or a layer present in the project
- **Artifacts** — does the code faithfully implement the originating intent, behaviors, plan and tasks?
  1. Every `intent.md` item in "Definition of Done" is demonstrably met
  2. Every `behavior.md` scenario has materialized as a test (or for prose changes, encoded)
  3. The full suite is green
  4. Read `plan.md` against the diff. If the implementation diverged, note it.

Both axes run as **parallel sub-agents** so they don't pollute each other's context, then this skill aggregates their findings.

Use `docs/github.md` only for fetching originating issues or PRs — this skill performs no board operations and no label transitions.


## Process

### 1. Pin the fixed point

Look for the first commit the implementor for this change did. The goal is to review the work done for the single claimed unit of work.


If the user provides the fixed point — a commit SHA, branch name, tag, `main`, `HEAD~5`, etc. — use that instead. If this skill was invoked independently of a claimed unit of work and no fixed point was given, ask for one.

Capture the diff command once: `git diff <fixed-point>...HEAD` (three-dot, so the comparison is against the merge-base). Also note the list of commits via `git log <fixed-point>..HEAD --oneline`.

Before going further, confirm the fixed point resolves (`git rev-parse <fixed-point>`) and the diff is non-empty. A bad ref or empty diff should fail here — not inside two parallel sub-agents.

### 2. Identify the artifacts source

Look for the originating artifacts, in this order:

1. Issue references in the commit messages (`#123`, `Closes #45`, etc.) — fetch via the workflow in `docs/github.md`.
2. A path the user passed as an argument.
3. Artifacts in `.changes/<slug>` for the in-flight unit of work matching the branch name or feature
4. If nothing is found, ask the user where the artifacts are. If they say there isn't one, the **Artifacts** sub-agent will skip and report "no Artifacts available".

### 3. Identify the standards sources

Anything in the repo that documents how code should be written, such as `CODING_STANDARDS.md` or `CONTRIBUTING.md`.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** in [smells.md](./reference/smells.md) — a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing, plus the two rules that bind it. Paste that file's full contents into the Standards sub-agent prompt — the sub-agent has no other access to it.

### 4. Spawn both sub-agents in parallel

Dispatch both axes as parallel sub-agents, each in a fresh context carrying its brief:

- **Claude Code**: a single message with two `Agent` tool calls, using the `general-purpose` subagent for both.
- **pi** ([pi-subagents](https://github.com/nicobailon/pi-subagents)): a single `subagent` tool call with a parallel `tasks` array — `{ tasks: [{ agent: "reviewer", task: "<standards brief>" }, { agent: "reviewer", task: "<artifacts brief>" }] }`.
- **No sub-agent mechanism available**: run the two axes sequentially, Standards first.

**Standards sub-agent prompt** — include:

- The full diff command and commit list.
- The list of standards-source files you found in step 3, **plus [smells.md](./reference/smells.md) pasted in full** — the sub-agent has no other access to it.
- The brief: "Report — per file/hunk where relevant — (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Distinguish hard violations from judgement calls — documented-standard breaches can be hard, but baseline smells are always judgement calls, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Report each finding as its own bullet, anchored to `file:line`. Under 500 words — compress findings rather than omit any."

**Artifacts sub-agent prompt** — include:

- The diff command and commit list.
- The path or fetched contents of the Artifacts.
- The brief: "Report: (a) requirements, intent and behaviors the artifacts asked for that are missing or partial; (b) behaviour in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where the implementation looks wrong; (d) run the full test suite and report if it is not green; (e) read `plan.md` against the diff and note any divergence. Quote the spec line for each finding. Report each finding as its own bullet, anchored to `file:line`. Under 500 words — compress findings rather than omit any."

If the Artifacts is missing, skip the Artifacts sub-agent and note this in the final report.

### 5. Aggregate

Present the two reports under `## Standards` and `## Artifacts` headings, verbatim or lightly cleaned. Do **not** merge or rerank findings — the two axes are deliberately separate (see _Why two axes_).

End with a one-line summary: total findings per axis, and the worst issue _within each axis_ (if any). Don't pick a single winner across axes — that's the reranking the separation exists to prevent.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing → **Standards pass, Artifacts fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Artifacts pass, Standards fail.**

Reporting them separately stops one axis from masking the other.
