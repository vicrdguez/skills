---
name: propose
description: Materialize the current conversation into a change spec -- This is not about interviewing, just synthesis of what we've already discussed
disable-model-invocation: true
---

This skills takes the current conversation context and codebase understanding into a set of tickets: tracer-bullet vertical slices. Each will declare any blocking dependencies (if any) and include the artifacts that drive implementation. 

The thinking and decision making already happened, thus this stage is just precise materialization.


## Process

### 1. **Gather context**

Work from what is already in the conversation context. 
- If the user passes a reference (a spec path, an issue number or URL) as an argument, fetch it and read its full body and context. 
- Then read the relevant capabilities (`docs/capabilities/*`) as well to understand which ones are changing, potentially getting deprecated/removed or which new capabilities this change will bring

### 2. Explore the repo to understand the current state of the codebase (optional)

If you haven't already read the `CONTEXT.md` using its vocabulary throughout the process. Respect existing ADRs that are relevant to the change.

### 3. Draft vertical slices

Break the work into **tracer-bullets** tickets.
- Each slice cuts a narrow but COMPLETE path through every layer (schema, API, UI, tests) - vertical, NOT a horizontal slice of one layer
- A completed slice is demoable and verifiable on its own
- Each slice is siced to fit in a single fresh context window
- Write the artifacts, then publish it to the project's issue tracker as explained below. 

Use `/design` to sketch the seams at which this this changes will be tested. 
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

### 5. Publishing and required Setup

Use `docs/github.md` as a reference of how to handle the issue board. If the file does not exist then copy it from [github.md](./reference/github.md) there.

If the change is split in multiple slices, create one parent issue holding the overall decisions so far and any notes relevant to all slices. *Each slice then will become a child issue*, tied to the parent. Publish all tickets in dependency order (blockers first). Then

1. **Pick a change ID**: A short, verb-led kebab-case slug like `add-order-cancellation`
2. **Ensure `.worktrees/` is gitignored**: add the line if it is missing
3. **Garbage-collect merge worktrees**: Remove any `.worktrees/<other>` whose changes have been already merged (its `.changes/archive/<date>-<other>/` is present on `main`), then delete its correspondent git branch. Run it from main, you are never in the worktree you remove.
4. **Commit durable docs**: Any `CONTEXT.md` or ADR eddits/additions left in the working tree from the explore session are durable, commit them to `main` now so they stay independently
5. **Create the worktree for the parent issue:** `git worktree add .worktrees/<slug> -b <slug>` off up-to-date `main`. Do the rest of propose **inside that worktree**.
6. Create `./changes/<slug>/` and all its artifacts directly inside the worktree

## Writing the change artifacts
These are the artifacts that each vertical slice will use for implementation:

**Always**:
- `intent.md`: Why / What / Scope / Out of scope / Definition of Done. Follow the [intent.md](./reference/intent.md) template
- `behavior.md`: The exact required behavior(s) to implement, in *Gherkin notation* that map to `intent.md` *Definition of Done* section. Since seams is where we test at, use `/tdd` to define good tests and avoid anti-patterns. The final list of behaviours will translate directly to what should be implemented and tested. Follow the [behavior.md](./reference/behavior.md) template 

**When warranted**:
- `plan.md`: The approach, the module shapes and seams chosen for implementation and any pinned decision the implementer MUST NOT make on its own. Follow the [plan.md](./reference/plan.md) template
- `tasks.md`: *Use when there is more than a couple non-behavioral chores*. Follow the [tasks.md](./reference.tasks.md) template


