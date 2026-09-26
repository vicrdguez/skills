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


# Codebase Design

Design **deep modules**: a lot of behaviour behind a small interface, placed at a clean seam, testable through that interface. Use this language and these principles wherever code is being designed or restructured. The aim is leverage for callers, locality for maintainers, and testability for everyone.

## Glossary

Use these terms exactly — don't substitute "component," "service," "API," or "boundary." Consistent language is the whole point.

**Module** — anything with an interface and an implementation. Deliberately scale-agnostic: a function, class, package, or tier-spanning slice. _Avoid_: unit, component, service.

**Interface** — everything a caller must know to use the module correctly: the type signature, but also invariants, ordering constraints, error modes, required configuration, and performance characteristics. _Avoid_: API, signature (too narrow — they refer only to the type-level surface).

**Implementation** — what's inside a module, its body of code. Distinct from **Adapter**: a thing can be a small adapter with a large implementation (a Postgres repo) or a large adapter with a small implementation (an in-memory fake). Reach for "adapter" when the seam is the topic; "implementation" otherwise.

**Depth** — leverage at the interface: the amount of behaviour a caller (or test) can exercise per unit of interface they have to learn. A module is **deep** when a large amount of behaviour sits behind a small interface, **shallow** when the interface is nearly as complex as the implementation.

**Seam** _(Michael Feathers)_ — a place where you can alter behaviour without editing in that place; the *location* at which a module's interface lives. Where to put the seam is its own design decision, distinct from what goes behind it. _Avoid_: boundary (overloaded with DDD's bounded context).

**Adapter** — a concrete thing that satisfies an interface at a seam. Describes *role* (what slot it fills), not substance (what's inside).

**Leverage** — what callers get from depth: more capability per unit of interface they learn. One implementation pays back across N call sites and M tests.

**Locality** — what maintainers get from depth: change, bugs, knowledge, and verification concentrate in one place rather than spreading across callers. Fix once, fixed everywhere.

## Deep vs shallow

**Deep module** = small interface + lots of implementation:

```
┌─────────────────────┐
│   Small Interface   │  ← Few methods, simple params
├─────────────────────┤
│                     │
│  Deep Implementation│  ← Complex logic hidden
│                     │
└─────────────────────┘
```

**Shallow module** = large interface + little implementation (avoid):

```
┌─────────────────────────────────┐
│       Large Interface           │  ← Many methods, complex params
├─────────────────────────────────┤
│  Thin Implementation            │  ← Just passes through
└─────────────────────────────────┘
```

When designing an interface, ask:

- Can I reduce the number of methods?
- Can I simplify the parameters?
- Can I hide more complexity inside?

## Principles

- **Depth is a property of the interface, not the implementation.** A deep module can be internally composed of small, mockable, swappable parts — they just aren't part of the interface. A module can have **internal seams** (private to its implementation, used by its own tests) as well as the **external seam** at its interface.
- **The deletion test.** Imagine deleting the module. If complexity vanishes, it was a pass-through. If complexity reappears across N callers, it was earning its keep.
- **The interface is the test surface.** Callers and tests cross the same seam. If you want to test *past* the interface, the module is probably the wrong shape.
- **One adapter means a hypothetical seam. Two adapters means a real one.** Don't introduce a seam unless something actually varies across it.

## Designing for testability

Good interfaces make testing natural:

1. **Accept dependencies, don't create them.**

   ```typescript
   // Testable
   function processOrder(order, paymentGateway) {}

   // Hard to test
   function processOrder(order) {
     const gateway = new StripeGateway();
   }
   ```

2. **Return results, don't produce side effects.**

   ```typescript
   // Testable
   function calculateDiscount(cart): Discount {}

   // Hard to test
   function applyDiscount(cart): void {
     cart.total -= discount;
   }
   ```

3. **Small surface area.** Fewer methods = fewer tests needed. Fewer params = simpler test setup.

## Relationships

- A **Module** has exactly one **Interface** (the surface it presents to callers and tests).
- **Depth** is a property of a **Module**, measured against its **Interface**.
- A **Seam** is where a **Module**'s **Interface** lives.
- An **Adapter** sits at a **Seam** and satisfies the **Interface**.
- **Depth** produces **Leverage** for callers and **Locality** for maintainers.

## Rejected framings

- **Depth as ratio of implementation-lines to interface-lines** (Ousterhout): rewards padding the implementation. We use depth-as-leverage instead.
- **"Interface" as the TypeScript `interface` keyword or a class's public methods**: too narrow — interface here includes every fact a caller must know.
- **"Boundary"**: overloaded with DDD's bounded context. Say **seam** or **interface**.

## Going deeper

- **Deepening a cluster given its dependencies** — see `skl skill --resource DEEPENING.md design`: dependency categories, seam discipline, and replace-don't-layer testing.
- **Exploring alternative interfaces** — see `skl skill --resource DESIGN-IT-TWICE.md design`: spin up parallel sub-agents to design the interface several radically different ways, then compare on depth, locality, and seam placement.


# Testing

Tests show that the change keeps its Contract. This is the reference that makes those tests worth keeping: what a good test is, where tests go, the anti-patterns, and what counts as evidence.

When exploring the codebase, read `CONTEXT.md` (if it exists) so test names and interface vocabulary match the project's domain language, and respect ADRs in the area you're touching.

## What a good test is

Tests verify behavior through public interfaces, not implementation details. Code can change entirely; tests shouldn't. A good test reads like a specification — "user can checkout with valid cart" tells you exactly what capability exists — and survives refactors because it doesn't care about internal structure.

Write tests before or after the code, whichever fits the change, and refactor whenever it helps.

When writing or judging a test, see `skl skill --resource tests.md testing` for examples. Before substituting a dependency, see `skl skill --resource mocking.md testing` for mocking guidelines.

## Seams — where tests go

A **seam** is where a module's interface lives: the place you observe behavior without reaching inside. Tests live at seams.

Test at the seams the Contract pins. Where the Contract leaves the seam to you, prefer an existing one, and use the highest seam that exposes the promised consequence.

## Anti-patterns

- **Implementation-coupled** — mocks internal collaborators, tests private methods, or verifies through a side channel (querying the database instead of using the interface). The tell: the test breaks when you refactor but behavior hasn't changed.
- **Tautological** — the assertion recomputes the expected value the way the code does (`expect(add(a, b)).toBe(a + b)`, a snapshot derived by hand the same way, a constant asserted equal to itself). The tell: it passes by construction and can never disagree with the code. Take expected values from an independent source: an accepted rule, a trusted example, or a justified property.

## Red on the bug

A bug fix's check goes **red on the bug**: it fails on the reported wrong behavior and passes with the fix, in either writing order. A failure from setup, an import or compilation is not red on the bug.

When the original failure can't be reproduced reliably or safely, a faithful isolated reproduction, a captured-trace replay or controlled fault injection can stand in, as long as it keeps the trigger and the observable failure. State what stays unverified; a material gap without credible evidence goes to a human decision.

## Evidence

Map obligations to checks many-to-many: one check can cover several obligations, and one obligation can need several checks. Judge the changed tests as a set. Reuse, strengthen, consolidate or remove checks while every required behavior and failure mode stays protected.
