---
name: propose
description: Materialize the current conversation into a change spec -- This is not about interviewing, just synthesis of what we've already discussed
disable-model-invocation: true
---

This skill takes the current conversation context and codebase understanding into a set of tickets: tracer-bullet vertical slices. Each will declare any blocking dependencies (if any) and include the artifacts that drive implementation. 

The thinking and decision making already happened, thus this stage is just precise materialization.


## Process

### 1. **Gather context**

Work from what is already in the conversation context.
- Locate Explore's user-confirmed final recap of consequential rules, architectural commitments, and delegated choices, together with every decision it references. This is the approved source for materialization; if it is absent or unconfirmed, stop and suggest running `explore`.
- If the user passes a reference (a spec path, an issue number or URL) as an argument, fetch it and read its full body and context.
- A superseded change enters through `explore`. Proceed only if this conversation contains the user's confirmation of renewed shared understanding; otherwise stop and suggest running `explore`.

### 2. Explore the repo to understand the current state of the codebase (optional)

If you haven't already, read the `CONTEXT.md`, using its vocabulary throughout the process. Respect existing ADRs that are relevant to the change.

### 3. Draft vertical slices

Break the work into **tracer-bullets** tickets.
- Each slice cuts a narrow but COMPLETE path through every layer (schema, API, UI, tests) - vertical, NOT a horizontal slice of one layer
- A completed slice is demoable and verifiable on its own
- Each slice is sized to fit in a single fresh context window
- Prefer separate Work Items for behaviors that deliver safe, useful results independently. Assess independence after declared Dependencies are Merged, without requiring later Work Items.
- Combine independently useful behaviors only for a concrete reduction in overall implementation or review burden. Shared files or a shared Workflow stage alone are insufficient.
- Assess review burden from the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together, using agreed requirements and focused repository inspection rather than line counts or exhaustive implementation planning. Different error cases alone do not require separate Work Items.
- Draft the artifacts for every slice, then fidelity-check and publish them as explained below.

Use the `design` skill to materialize the approved seams at which this change can be verified.
- Preserve any deliberately agreed seam.
- Where seam choice was delegated, prefer an existing seam and use the highest one that exposes the promised consequence.
- If a consequential seam is neither settled nor delegated, return it to the human before drafting rather than redesigning the accepted architecture.

### 4. Quiz the user

Present the proposed breakdown as a numbered list. For each ticket show:
- *Title*: Short and descriptive name
- *Blocked by* which other tickets (if any) must complete first
- *What it delivers*: the end-to-end behavior this ticket makes work
- *Review burden*: Briefly explain those concerns and any concrete reason for combining independently useful behaviors


Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it?
- Should any tickets be merged or split further?

Iterate until the user approves the breakdown. A single ticket is possible if the change is small.

If artifact elaboration materially changes proposed boundaries or Dependencies, return to this approval loop before publication: explain the discovery and propose the revised breakdown for approval before freezing the artifacts. Ordinary elaboration within an unchanged coherent delivery needs no renewed approval.

### 5. Materialize and review fidelity

Write every slice's artifacts using the guidance below. Before publication, run one bounded review in a fresh context across the complete proposed slice set.

Supply that reviewer explicitly with:

- the exact user-confirmed Explore recap;
- every referenced decision or ADR;
- all proposed slices and their artifact drafts.

The review checks only whether materialization omits, weakens, strengthens, contradicts, or invents obligations relative to those approved sources. It does not redesign the solution, reopen accepted choices, or fill a decision gap.

Correct demonstrable transcription errors against the approved source. An unsettled consequential choice, contradiction in the approved sources, or proposed semantic or architectural change returns to the human for resolution. If resolution changes slice boundaries or Dependencies, return to the approval loop in step 4. Faithful correction and ordinary elaboration require neither routine artifact-by-artifact rereading nor a second semantic approval ceremony.

### 6. Prepare and publish

1. Run `skl propose cleanup --repo <root>` before preparing new slices. It removes only safe local Git state for Work Items already observed Merged and reports everything it preserves.
2. Commit any durable `CONTEXT.md` or ADR changes to the target branch before creating slice branches.
3. For each slice, choose a short verb-led kebab-case slug, create `.worktrees/<slug>` and its branch from the target, write the fidelity-reviewed drafts to `.changes/<slug>/`, and commit the complete ledger with subject `[baseline] <slug>` (optional explanatory text may follow after a space). Push that exact commit as the publication head.
4. Write the parent and child issue bodies to private temporary Markdown files. For every child, use the thin-pointer template below. Replace every placeholder with the slice summary, branch slug, and full commit SHA from `git rev-parse HEAD` in that slice worktree; that pushed, marked head is its Artifact Baseline. Keep artifact prose in Git, not in the issue. The files are opaque transport: `skl` neither authors nor interprets them.
5. Publish the prepared Proposal with `skl propose publish --repo <root> --target <branch> --slice <slug>=<body-file>`. Repeat `--slice` for every child and add `--depends <dependent>:<blocker>` for each Dependency. For a multi-slice Proposal also pass `--parent-title <title> --parent-body <body-file>`.

`skl propose publish` preflights the entire declaration before changing GitHub, creates children in blocker-first order, and applies Ready last. If it returns `fix_required`, make the stated Git repair and repeat the same command. If it returns `needs_human`, stop and present its reason; do not guess which existing record to reuse.

Child thin-pointer template:

```markdown
<summary>

Branch: `<slug>`
Artifact Baseline: `<baseline-sha>`
Implementation Ledger: `.changes/<slug>/` at the Artifact Baseline.
```

## Writing the change artifacts
These are the artifacts that each vertical slice will use for implementation:

Publishing them freezes them as the endpoint contract. Artifact Completion may differ from the marked Baseline only by ticking an existing `[ ]` to lowercase `[x]` outside Manual Verification — nothing added, removed, reordered or reworded, and Manual Verification remains unchecked. This is the acceptance baseline: if it can be rewritten mid-flight to match whatever got built, or grown with things discovered during review, it stops being a contract and the change stops converging. Discoveries belong in PR findings or in a new proposal. There is no later addition and no exception.

So resolve the contradictions now, while you still can — between the artifacts themselves, and between them and the project's own rules. Afterwards nobody downstream can fix them; they can only stop and ask you.

Keep the artifact set compact and authoritative. Preserve approved decisions; do not turn incidental sketches, scenario count, test count, or a preferred construction order into new obligations. Within each artifact, use readable, uniquely referenceable descriptive headings without requiring IDs or a requirements database.

**Always**:
- `intent.md`: State the desired result, scope, exclusions, Definition of Done, and human-owned Manual Verification without duplicating detailed behavior. Follow the template from `skl skill --resource reference/intent.md propose`.
- `behavior.md`: State scoped named rules where consequential ambiguity needs resolution, then add only binding scenarios that discriminate plausible interpretations. Rules govern their whole class of situations beyond the examples. Use `testing` and its public guidance (`skl skill --resource reference/tests.md testing`) to choose observable seams and credible evidence without prescribing one test or task per scenario. Follow the template from `skl skill --resource reference/behavior.md propose`.

**When warranted**:
- `plan.md`: Required when the approved design pins architecture. Preserve responsibility ownership, boundary assumptions, deliberately agreed interfaces, and verification strategy from the approved source; reference relevant existing ADRs and label incidental sketches as illustrative. Follow the template from `skl skill --resource reference/plan.md propose`.
- `tasks.md`: Create it only when useful sequencing, dependencies, or coordination need an explicit ledger. Do not manufacture tasks from scenario or test counts. Follow the template from `skl skill --resource reference/tasks.md propose`.

An unambiguously implied case may be implemented and tested without extending the frozen artifact set. Resolve every consequential behavioral or architectural gap before publication; silence does not delegate it, and no new convention relaxes an explicit obligation in an existing ledger.
