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
- If the user passes a reference (a spec path, an issue number or URL) as an argument, fetch it and read its full body and context. 
- Then read the relevant capabilities (`docs/capabilities/*`) as well to understand which ones are changing, potentially getting deprecated/removed or which new capabilities this change will bring
- A superseded change enters through `explore`. Proceed only if this conversation contains the user's confirmation of renewed shared understanding; otherwise stop and suggest running `explore`.

### 2. Explore the repo to understand the current state of the codebase (optional)

If you haven't already, read the `CONTEXT.md`, using its vocabulary throughout the process. Respect existing ADRs that are relevant to the change.

### 3. Draft vertical slices

Break the work into **tracer-bullets** tickets.
- Each slice cuts a narrow but COMPLETE path through every layer (schema, API, UI, tests) - vertical, NOT a horizontal slice of one layer
- A completed slice is demoable and verifiable on its own
- Each slice is sized to fit in a single fresh context window
- Write the artifacts, then publish them to the project's issue tracker as explained below. 

Use the `design` skill to sketch the seams at which this change will be tested.
- Always prefer existing seams rather than new ones
- Use the highest seam possible
- If new seams are needed, propose them at the highest point you can. The fewer seams across the codebase, the better.

Check with the user if the seams match their expectations. 

### 4. Quiz the user

Present the proposed breakdown as a numbered list. For each ticket show:
- *Title*: Short and descriptive name
- *Blocked by* which other tickets (if any) must complete first
- *What it delivers*: the end-to-end behavior this ticket makes work


Ask the user:

- Does the granularity feel right? (too coarse / too fine)
- Are the blocking edges correct — does each ticket only depend on tickets that genuinely gate it?
- Should any tickets be merged or split further?

Iterate until the user approves the breakdown. A single ticket is possible if the change is small.

### 5. Prepare and publish

1. Run `skl propose cleanup --repo <root>` before preparing new slices. It removes only safe local Git state for Work Items already observed Merged and reports everything it preserves.
2. Commit any durable `CONTEXT.md`, ADR, or capability changes to the target branch before creating slice branches.
3. For each slice, choose a short verb-led kebab-case slug, create `.worktrees/<slug>` and its branch from the target, write `.changes/<slug>/`, commit the complete ledger at once, and push that exact head.
4. Write the parent and child issue bodies to private temporary Markdown files. The files are opaque transport: `skl` neither authors nor interprets them.
5. Publish the prepared Proposal with `skl propose publish --repo <root> --target <branch> --slice <slug>=<body-file>`. Repeat `--slice` for every child and add `--depends <dependent>:<blocker>` for each Dependency. For a multi-slice Proposal also pass `--parent-title <title> --parent-body <body-file>`.

`skl propose publish` preflights the entire declaration before changing GitHub, creates children in blocker-first order, and applies Ready last. If it returns `fix_required`, make the stated Git repair and repeat the same command. If it returns `needs_human`, stop and present its reason; do not guess which existing record to reuse.

## Writing the change artifacts
These are the artifacts that each vertical slice will use for implementation:

Publishing them freezes them. From that commit on, the only edit anyone may make is ticking an existing `[ ]` to `[x]` — nothing added, removed, reordered or reworded. This is the acceptance baseline: if it can be rewritten mid-flight to match whatever got built, or grown with things discovered during review, it stops being a contract and the change stops converging. Discoveries belong in PR findings or in a new proposal. There is no later addition and no exception.

So resolve the contradictions now, while you still can — between the artifacts themselves, and between them and the project's own rules. Afterwards nobody downstream can fix them; they can only stop and ask you.

**Always**:
- `intent.md`: Why / What / Scope / Out of scope / Definition of Done. Follow the [intent.md](./reference/intent.md) template
- `behavior.md`: The exact required behavior(s) to implement, in *Gherkin notation* that map to `intent.md` *Definition of Done* section. Since seams are where we test at, use `tdd` to define good tests and avoid anti-patterns. The final list of behaviours will translate directly to what should be implemented and tested. Follow the [behavior.md](./reference/behavior.md) template

**When warranted**:
- `plan.md`: The approach, the module shapes and seams chosen for implementation and any pinned decision the implementer MUST NOT make on its own. Follow the [plan.md](./reference/plan.md) template
- `tasks.md`: Follow the [tasks.md](./reference/tasks.md) template — it states when it is warranted

