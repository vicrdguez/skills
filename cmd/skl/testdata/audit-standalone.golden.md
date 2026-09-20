Protocol: skl.instructions/v1
Skill: audit
Included skills: none
Facts: {}
Resources: reference/acceptance.md, reference/smells.md

---
name: audit
description: Review the changes since a fixed point (commit, branch, tag or merge-base) along two axes - Standards (does the code follow this repo documented coding standards) and change Artifacts (does the code match what the originating change asked for?) Runs both reviews in parallel subagents and reports them side by side. Use when the user wants to review a branch, a PR, work-in-progress changes or asks to "review since X"
---

Two-axis review of the diff between `HEAD` and a fixed point the user supplies:

- **Standards** — does the code conform to this repo's documented coding standards? This includes any documented standard in the repo but also make use of project/repo specific skills to aid here. E.g. the repo has a specific skill to review a module or a layer present in the project
- **Artifacts** — does the code faithfully implement the originating intent, behaviors, plan and tasks?
  1. The Workflow Engine's endpoint inspection reports identical paths, regular non-executable blobs, and exact bytes except permitted lowercase completion ticks, with Manual Verification unchecked; normal submission also requires completed automated boxes and later ledger retirement
  2. Every `intent.md` item in "Definition of Done" is demonstrably met
  3. Every rule, scenario, and architectural obligation is accounted for with credible grouped many-to-many evidence, and the changed test set preserves required behavioral and failure-mode protection
  4. The full suite is green
  5. Read `plan.md` against the diff. If the implementation diverged, note it.

Item 1 is what stops the contract moving to meet the code. The rest are judged against the **complete final implementation**, even when the diff under review is only the latest increment. Reuse, strengthening, consolidation, or removal of tests is acceptable when protection remains; scrutinize removed or weakened assertions for lost coverage without demanding per-test bookkeeping.

Both axes run as **parallel sub-agents** when the harness supports them, so they don't pollute each other's context, then this skill aggregates their findings. The sequential fallback below preserves both axes when it does not.

Use the supplied Work Item and Submission facts for originating context. This skill performs no backend mutations or workflow transitions.


## Process

### 1. Pin the fixed point

The goal is to review the work done for the single claimed unit of work. Which point that is depends on the round:

- **First workflow review of a change** — the merge-base with `main`, or the parent of the implementor's first commit. Never that first commit itself: `git diff <it>...HEAD` would omit everything it introduced. An independent Audit still uses any fixed point its caller explicitly supplies.
- **Repeat review after a bounce** — the `Reviewed head` recorded in the previous reviewer's summary, so the round reads only what changed since.


Artifact integrity uses its own, unmoving Artifact Baseline and, when available, Artifact Completion from the engine's endpoint inspection. It never advances with review rounds. Refresh fixed Git facts with the packet's `inspect_command`, or `skl implement inspect --remote <name> --item <number>` when invoked independently; preserve any caller-supplied `--artifact-baseline <full-sha>` and `--artifact-completion <full-sha>`. The engine validates endpoint identity and snapshots, while you read and judge the historical contract.


If the user provides a fixed point — a commit SHA, branch name, tag, `main`, `HEAD~5`, etc. — use that instead. If no PR comparison or fixed point can be resolved, ask for one.


Capture the diff command once: `git diff <fixed-point>...HEAD` (three-dot, so the comparison is against the merge-base). Also note the list of commits via `git log <fixed-point>..HEAD --oneline`.

Before going further, confirm the fixed point resolves (`git rev-parse <fixed-point>`). A bad ref should fail here, not inside two parallel sub-agents. An empty diff is legal when human disposition alone resolved a review: run the deterministic checks, judge the final state, and report no new findings.

### 2. Identify the artifacts source

Look for the originating artifacts, in this order:

1. The supplied Work Item's exact Artifact Baseline and Completion; read with `git show <snapshot>:.changes/<slug>/<file>`.
2. A path the user passed as an argument.
3. Artifacts in `.changes/<slug>` for the in-flight unit of work matching the branch name or feature; after retirement, read them from the historical Artifact Baseline and Completion without recreating them
4. If nothing is found, ask the user where the artifacts are. If they say there isn't one, the **Artifacts** sub-agent will skip and report "no Artifacts available".


### 3. Identify the standards and acceptance sources

Anything in the repo that documents how code should be written, such as `CODING_STANDARDS.md` or `CONTRIBUTING.md`.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** from `skl skill --resource reference/smells.md audit` — a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing, plus the two rules that bind it.

Retrieve the shared contract criteria with `skl skill --resource reference/acceptance.md audit`. Give those criteria to both reviewers. They distinguish behavioral conformance, architectural conformance, and local implementation quality; sharing them with Watchdog does not execute Audit again.

### 4. Run the deterministic checks once

These two produce facts, not judgements — a diff read or an exit code. Run them here, before spawning anything, and hand the recorded results to both briefs. Two reviewers running them concurrently would contend over the same worktree, and a fact produced inside a reviewer's context is a fact the two axes can end up reporting differently.

1. **The documented gate** — the project's full suite, typecheck and lint, exactly once per invocation. A red gate is worth knowing before spending two reviewer contexts on it.
2. **Artifact integrity** — record the available endpoint evidence. Compare only the resolved Baseline and Completion, or Baseline and current provisional head before Completion; do not inspect intermediate artifact contents, infer Completion from deletion, or require monotonic intermediate ticks. Report missing integrity evidence explicitly.


### 5. Spawn both sub-agents in parallel

Dispatch both axes as parallel sub-agents, each in a fresh context carrying its brief:

- **Claude Code**: a single message with two `Agent` tool calls, using the `general-purpose` subagent for both.
- **pi** ([pi-subagents](https://github.com/nicobailon/pi-subagents)): a single asynchronous `subagent` call with a `workflowScript` using `runs.all` to launch both reviewers in fresh contexts. Set `timeoutMs: 3600000` on the workflow call and both reviewer items.
- **No sub-agent mechanism available**: run the two axes sequentially, Standards first.


**Standards sub-agent prompt** — include:

- The full diff command and commit list.
- The list of standards-source files you found in step 3, plus `skl skill --resource reference/smells.md audit`. Instruct the sub-agent to read those files and the command's output.
- The shared criteria from `skl skill --resource reference/acceptance.md audit`.
- The gate results from step 4.
- The precedence between sources, so the sub-agent knows what outranks what: frozen artifacts, then required tooling and CI, then the project's `AGENTS.md`, standards docs and quality skills, then language and framework correctness, security and accessibility rules, then the generic smell baseline. An explicit project or language `MUST`, `ALWAYS`, `NEVER` or equivalent can be a hard violation; a generic smell stays a judgement call unless a local rule or a concrete behavior or maintenance impact elevates it.
- The brief: "Report — per file/hunk where relevant — (a) every place the diff violates a documented standard: cite the standard (file + the rule); and (b) any baseline smell you spot: name it and quote the hunk. Tag each finding as `HARD` or `JUDGEMENT` as its first token; documented-standard breaches can be `HARD`, but baseline smells are always `JUDGEMENT`, and a documented repo standard overrides the baseline. Skip anything tooling enforces. Report each finding as its own bullet, anchored to `file:line`. Under 500 words. Compress findings rather than omit any."

**Artifacts sub-agent prompt** — include:

- The diff command and commit list.
- The paths or fetched contents of the Artifacts at the exact Baseline and, when available, Completion or provisional head.
- The shared criteria from `skl skill --resource reference/acceptance.md audit`.
- The gate and artifact-integrity results from step 4.
- The brief: "Report: (a) requirements, intent, rules, scenarios, or architectural obligations that are missing, partial, or contradicted; (b) behaviour in the diff that wasn't asked for (scope creep); (c) claimed evidence that does not expose the promised consequence or distinguish a plausible violation, including protection lost through removed or weakened assertions; (d) the gate result you were given, if it is not green — do not rerun it; (e) read `plan.md` against the diff and note any divergence; (f) any endpoint violation the integrity result reports, without adding intermediate-history or deletion-inference rules. Accept grouped many-to-many evidence and contract-preserving test reuse or consolidation; do not impose preferred test organization or implementation. Judge (a) to (c) against the complete final implementation even when the diff is only the latest increment. For an evidence gap, identify the obligation, plausible violation, and why existing evidence does not distinguish it. Quote the spec line for each finding. Tag each finding `HARD` or `JUDGEMENT` as its first token under the shared criteria: (a), (b), (c) and (f) are `HARD`; (d) is `HARD` when the gate is red; (e) is `JUDGEMENT` unless the divergence breaks a frozen requirement. Report each finding as its own bullet, anchored to `file:line`. Under 500 words — compress findings rather than omit any."


Nothing written after the artifacts were published is a requirement: not review comments, not rework notes. They can be evidence, never a spec line to hold the implementation against.

Assign every new Audit Finding an `F<n>` identity. Begin with `F1` when no `F<n>` exists; otherwise continue after the greatest existing `F<n>`. Preserve historical identifiers in other formats unchanged and never renumber earlier findings.

If the Artifacts is missing, skip the Artifacts sub-agent and note this in the final report.

### 6. Aggregate

Present the two reports under `## Standards` and `## Artifacts` headings, verbatim or lightly cleaned. Carry each finding's `HARD`/`JUDGEMENT` tag through verbatim: you did not see the evidence, the axis that found it did. Do **not** merge or rerank findings — the two axes are deliberately separate (see _Why two axes_).

End with a one-line summary: the `HARD` and `JUDGEMENT` counts per axis, and the worst issue _within each axis_ (if any). Don't pick a single winner across axes — that's the reranking the separation exists to prevent. State both step-4 results verbatim alongside it; they are the only part of the report that is not a judgement.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing → **Standards pass, Artifacts fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Artifacts pass, Standards fail.**

Reporting them separately stops one axis from masking the other.
