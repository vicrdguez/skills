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

### 6. Prepare and accept into the Workflow Ledger

1. Run `skl propose cleanup --repo <root>` before preparing new slices. It archives whole Proposals whose slices are all Merged or Superseded and unclaimed, removes only safe merged local source work, and reports preserved source work and repairs separately. Do not move ledger records or delete source work yourself, and do not skip a reported repair; archival never happens automatically on merge or review.
2. Commit any durable `CONTEXT.md` or ADR changes to the target branch. Durable project knowledge stays in the project repository; the ledger holds workflow records, not project documentation.
3. Prepare one intake directory for the proposal outside the source tree:
   - `proposal.md`: the durable approved proposal description.
   - `proposal.json`: `{"proposal": "<kebab-name>", "parent_title": "<multi-slice only>", "slices": [{"name": "<slug>", "title": "<issue title>", "branch": "<planned source branch>", "depends": ["<sibling slug or proposals/<proposal>/<slice>"]}]}`.
   - one directory per slice holding its frozen Contract files: `intent.md` and `behavior.md` are required; add `plan.md` and `tasks.md` only when warranted. Nothing else belongs in a slice directory.
4. Write the descriptive human-facing issue bodies to private temporary Markdown files — one per slice, plus a parent body for multi-slice work. Keep them self-contained descriptions; they are transport, not Contract content, and `skl` neither authors, persists, nor registers them for reuse.
5. Accept the proposal locally with `skl ledger accept --repo <root> --proposal-dir <dir> --issue <slice>=<body-file>` (repeat `--issue` per slice; add `--parent-body <file>` for multi-slice work). No source branch, worktree, or source artifact commit is prepared at this stage: the planned branch identity is recorded, not created.
6. Handle the outcome: `accepted` or `existing` records the work; `fix_required` names a concrete repair — fix it and repeat the same command. Issue publication is best-effort: a failed, uncertain, or missing-prose surface leaves the local acceptance authoritative and gates no later work. Publish the current issue view later with `skl ledger publish --repo <root> --proposal <proposal>` and freshly authored prose rather than repeating acceptance; the outcome names the private readback and the `issue-publication` guidance to author it from. Never publish with `gh` directly. A changed Contract is never replaced in place: direct a renewed proposal instead.
7. Read accepted content back through the public CLI with `skl ledger show --repo <root> --item <proposal>/<slice>`. Its output carries the exact accepted documents with full ledger commit and path references; cite those references when a document must be identified exactly.
8. A repository that has not adopted the Workflow Ledger keeps its established publication path until a human-directed administrative cutover with normal workers stopped; `skl` adopts existing work automatically in no case.

## Writing the change artifacts
These are the artifacts that each vertical slice will use for implementation:

Publishing them freezes them as the accepted contract. `skl ledger accept` records their exact bytes, and accepted files stay read-only from then on: progress and completion evidence live in phase reports, never in completion ticks or edits to these documents. This is the acceptance baseline: if it can be rewritten mid-flight to match whatever got built, or grown with things discovered during review, it stops being a contract and the change stops converging. Discoveries belong in PR findings or in a new proposal. There is no later addition and no exception.

So resolve the contradictions now, while you still can — between the artifacts themselves, and between them and the project's own rules. Afterwards nobody downstream can fix them; they can only stop and ask you.

Keep the artifact set compact and authoritative. Preserve approved decisions; do not turn incidental sketches, scenario count, test count, or a preferred construction order into new obligations. Within each artifact, use readable, uniquely referenceable descriptive headings without requiring IDs or a requirements database.

Label independently tracked commitments with descriptive local labels: `B<n>` for binding behavior rules, `A<n>` for architectural commitments, warranted `T<n>` for tasks, and `M<n>` for human-owned Manual Verification. Number independently tracked commitments only — never every paragraph or heading — do not duplicate an identity across documents, and expect no CLI numbering service: the labels are yours to assign and keep stable from acceptance on.

**Always**:
- `intent.md`: State the desired result, scope, exclusions, Definition of Done, and human-owned Manual Verification without duplicating detailed behavior. Follow the template from `skl skill --resource intent.md propose`.
- `behavior.md`: State scoped named rules where consequential ambiguity needs resolution, then add only binding scenarios that discriminate plausible interpretations. Rules govern their whole class of situations beyond the examples. Use `testing` and its public guidance (`skl skill --resource tests.md testing`) to choose observable seams and credible evidence without prescribing one test or task per scenario. Follow the template from `skl skill --resource behavior.md propose`.

**When warranted**:
- `plan.md`: Required when the approved design pins architecture. Preserve responsibility ownership, boundary assumptions, deliberately agreed interfaces, and verification strategy from the approved source; reference relevant existing ADRs and label incidental sketches as illustrative. Follow the template from `skl skill --resource plan.md propose`.
- `tasks.md`: Create it only when useful sequencing, dependencies, or coordination need an explicit ledger. Do not manufacture tasks from scenario or test counts. Follow the template from `skl skill --resource tasks.md propose`.

An unambiguously implied case may be implemented and tested without extending the frozen artifact set. Resolve every consequential behavioral or architectural gap before publication; silence does not delegate it, and no new convention relaxes an explicit obligation in an existing ledger.
