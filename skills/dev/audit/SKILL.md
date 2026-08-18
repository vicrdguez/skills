---
name: audit
description: Review the changes since a fixed point (commit, branch, tag or merge-base) along two axes - Standards (does the code follow this repo documented coding standards) and change Artifacts (does the code match what the originating change asked for?) Runs both reviews in parallel subagents and reports them side by side. Use when the user wants to review a branch, a PR, work-in-progress changes or asks to "review since X"
---

Two-axis review of the diff between `HEAD` and a fixed point the user supplies:

- **Standards** — does the code conform to this repo's documented coding standards? This includes any documented standard in the repo but also make use of project/repo specific skills to aid here. E.g. the repo has a specific skill to review a module or a layer present in the project
- **Artifacts** — does the code faithfully implement the originating intent, behaviors, plan and tasks?
  1. The artifacts themselves are unchanged since they were published, apart from existing `[ ]` boxes ticked to `[x]`
  2. Every `intent.md` item in "Definition of Done" is demonstrably met
  3. Every `behavior.md` scenario has materialized as a test (or for prose changes, encoded)
  4. The full suite is green
  5. Read `plan.md` against the diff. If the implementation diverged, note it.

Item 1 is what stops the contract moving to meet the code. The rest are judged against the **complete final implementation**, even when the diff under review is only the latest increment.

Both axes run as **parallel sub-agents** so they don't pollute each other's context, then this skill aggregates their findings.

Use `docs/github.md` only for fetching originating issues or PRs — this skill performs no board operations and no label transitions.


## Process

### 1. Pin the fixed point

The goal is to review the work done for the single claimed unit of work. Which point that is depends on the round:

- **First review of a change** — the PR base merge-base, or the parent of the implementor's first commit. Never that first commit itself: `git diff <it>...HEAD` would omit everything it introduced.
- **Repeat review after a bounce** — the `Reviewed head` recorded in the previous reviewer's summary, so the round reads only what changed since.

Artifact integrity uses its own, unmoving baseline: the `Artifact baseline` SHA recorded in the originating issue, or for legacy changes the first commit containing `.changes/<slug>/`. It never advances with the review rounds — that is the point of it.

If the user provides the fixed point — a commit SHA, branch name, tag, `main`, `HEAD~5`, etc. — use that instead. If this skill was invoked independently of a claimed unit of work and no fixed point was given, ask for one.

Capture the diff command once: `git diff <fixed-point>...HEAD` (three-dot, so the comparison is against the merge-base). Also note the list of commits via `git log <fixed-point>..HEAD --oneline`.

Before going further, confirm the fixed point resolves (`git rev-parse <fixed-point>`) and the diff is non-empty. A bad ref or empty diff should fail here, not inside two parallel sub-agents. An empty diff is legal in one case: a repeat review the caller says resolved by human disposition alone. Nothing was supposed to change so run the deterministic checks, judge the final state, report no new findings.

### 2. Identify the artifacts source

Look for the originating artifacts, in this order:

1. Issue references in the commit messages (`#123`, `Closes #45`, etc.) — fetch via the workflow in `docs/github.md`.
2. A path the user passed as an argument.
3. Artifacts in `.changes/<slug>` for the in-flight unit of work matching the branch name or feature
4. If nothing is found, ask the user where the artifacts are. If they say there isn't one, the **Artifacts** sub-agent will skip and report "no Artifacts available".

### 3. Identify the standards sources

Anything in the repo that documents how code should be written, such as `CODING_STANDARDS.md` or `CONTRIBUTING.md`.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** in [smells.md](./reference/smells.md) — a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing, plus the two rules that bind it. Paste that file's full contents into the Standards sub-agent prompt — the sub-agent has no other access to it.

#### Optional Ponytail review

Check the available skills for `ponytail-review`. When present, include it only in the Standards axis as an additional over-engineering lens. For pi, pass `skill: "ponytail-review"` on the Standards task; do not pass it to the Artifacts task. If the skill is absent, omit the override and continue normally.

Ponytail findings are judgement calls, not documented-standard violations. Keep them separate from the ordinary Standards findings, and do not let them replace the documented standards or smell baseline.

### 4. Run the deterministic checks once

These two produce facts, not judgements — a diff read or an exit code. Run them here, before spawning anything, and hand the recorded results to both briefs. Two reviewers running them concurrently would contend over the same worktree, and a fact produced inside a reviewer's context is a fact the two axes can end up reporting differently.

1. **The documented gate** — the project's full suite, typecheck and lint, exactly once per invocation. A red gate is worth knowing before spending two reviewer contexts on it.
2. **Artifact integrity** — `git diff <artifact-baseline>...HEAD -- .changes/<slug>/`. The only permitted change is a paired replacement where a line's `[ ]` became `[x]`. Anything added, removed, reordered, reworded or unticked is a hard Artifacts finding.

### 5. Spawn both sub-agents in parallel

Dispatch both axes as parallel sub-agents, each in a fresh context carrying its brief:

- **Claude Code**: a single message with two `Agent` tool calls, using the `general-purpose` subagent for both.
- **pi** ([pi-subagents](https://github.com/nicobailon/pi-subagents)): a single `subagent` tool call with a parallel `tasks` array and `context: "fresh"`. When `ponytail-review` is available, inject it into the Standards task only — `{ tasks: [{ agent: "reviewer", task: "<standards brief>", skill: "ponytail-review" }, { agent: "reviewer", task: "<artifacts brief>" }], context: "fresh" }`. Omit `skill` when unavailable.
- **No sub-agent mechanism available**: run the two axes sequentially, Standards first.

**Standards sub-agent prompt** — include:

- The full diff command and commit list.
- The list of standards-source files you found in step 3, **plus [smells.md](./reference/smells.md) pasted in full** — the sub-agent has no other access to it.
- The gate results from step 4.
- The precedence between sources, so the sub-agent knows what outranks what: frozen artifacts, then required tooling and CI, then the project's `AGENTS.md`, standards docs and quality skills, then language and framework correctness, security and accessibility rules, then the generic smell baseline. An explicit project or language `MUST`, `ALWAYS`, `NEVER` or equivalent can be a hard violation; a generic smell stays a judgement call unless a local rule or a concrete behavior or maintenance impact elevates it.
- When `ponytail-review` was provided, instruct the sub-agent to load and apply it as an additional lens. Put its output under `### Ponytail review`, preserve its concise finding format and final net-lines estimate, and treat every Ponytail finding as a judgement call. It must still complete the documented-standards and smell-baseline review.
- The brief: "Report — per file/hunk where relevant — (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Tag each finding as `HARD` or `JUDGEMENT` as its first token; documented-standard breaches can be `HARD`, but baseline smells (and ponytail findings if available) are always `JUDGEMENT`, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Report each finding as its own bullet, anchored to `file:line`. Under 500 words, or 750 when `ponytail-review` is included. Compress findings rather than omit any; Ponytail findings must retain their one-line format."

**Artifacts sub-agent prompt** — include:

- The diff command and commit list.
- The path or fetched contents of the Artifacts, read at the artifact baseline.
- The gate and artifact-integrity results from step 4.
- The brief: "Report: (a) requirements, intent and behaviors the artifacts asked for that are missing or partial; (b) behaviour in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where the implementation looks wrong; (d) the gate result you were given, if it is not green — do not rerun it; (e) read `plan.md` against the diff and note any divergence; (f) any forbidden artifact mutation the integrity result reports. Judge (a) to (c) against the complete final implementation even when the diff is only the latest increment. Quote the spec line for each finding. Tag each finding `HARD` or `JUDGEMENT` as its first token: (a), (b), (c) and (f) are `HARD`; (d) is `HARD` when the gate is red; (e) is `JUDGEMENT` unless the divergence breaks a frozen requirement. Report each finding as its own bullet, anchored to `file:line`. Under 500 words — compress findings rather than omit any."

Nothing written after the artifacts were published is a requirement: not review comments, not rework notes. They can be evidence, never a spec line to hold the implementation against.

If the Artifacts is missing, skip the Artifacts sub-agent and note this in the final report.

### 6. Aggregate

Present the two reports under `## Standards` and `## Artifacts` headings, verbatim or lightly cleaned. Keep any `### Ponytail review` subsection inside `## Standards`. Carry each finding's `HARD`/`JUDGEMENT` tag through verbatim: you did not see the evidence, the axis that found it did. Do **not** merge or rerank findings — the two axes are deliberately separate (see _Why two axes_).

End with a one-line summary: the `HARD` and `JUDGEMENT` counts per axis, counting Ponytail findings in Standards, and the worst issue _within each axis_ (if any). Don't pick a single winner across axes — that's the reranking the separation exists to prevent. State both step-4 results verbatim alongside it; they are the only part of the report that is not a judgement.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing → **Standards pass, Artifacts fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Artifacts pass, Standards fail.**

Reporting them separately stops one axis from masking the other.
