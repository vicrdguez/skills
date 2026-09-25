Protocol: skl.instructions/v1
Skill: explore
Included skills: domain
Facts: {}
Resources: none

---
name: explore
description: A relentless interview to sharpen a plan, design or idea which also creates durable docs (ADRs and glossary) as we go.
disable-model-invocation: true
---

Interview the user relentlessly until you reach a shared understanding. Map this as a **design tree**: every decision branches into the decisions that hang off it.

When the user supplies the exact path to a `.thinking` artifact, read it as starting context and carry its settled concepts forward unless project evidence contradicts them. Never enumerate `.thinking` or select an artifact on the user's behalf.

If the user names a superseded slug or its closed issue/PR, first read that issue, the PR's audit ledger and review findings, and the abandoned artifacts. Use the failed attempt as evidence when mapping the design tree: keep what it established and reopen the decisions implicated in its supersession.

Use the `domain` skill throughout the session to sharpen the project's language and write durable docs as decisions crystallise.

Work the tree in **rounds**. The **frontier** is every decision whose prerequisites are already settled: the questions you can ask _now_ without guessing at answers you haven't heard yet. Ask the whole frontier in one round: number each question and give your recommended answer. Then wait for the user's answers before the next round.

Format a round like so:

```
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

---

❓ **Q2** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>
```

Each round the user answers reshapes the tree: settled decisions push the frontier outward and unblock questions that depended on them. Recompute the frontier and ask the next round. A question whose answer depends on another question still open in this round belongs to a _later_ round, not this one.

Finding _facts_ is your job, never the user's. When a frontier question needs a fact from the environment (filesystem, tools, etc.), dispatch a sub-agent to find it; don't ask the user for anything you could look up yourself. Don't block on it: a running exploration is an unsettled prerequisite, so only the questions downstream of it wait for the sub-agent to report; ask the rest of the frontier now. The _decisions_ are the user's: put each to them and wait.

The session is done when the frontier is empty: every branch of the design tree visited, nothing left silently assumed. Present one final approval recap that separates:

- consequential rules the change must preserve;
- architectural commitments and responsibility ownership;
- choices deliberately delegated to implementation.

Name the ADRs or other decisions that carry relevant context. Ask the user to correct or confirm this recap; confirmation is the semantic approval for the whole Proposal. If they correct it, update the design tree and recap before asking again.

After confirmation, stop. Suggest running the `propose` skill in the same session so it can materialize the shared understanding, carrying the exact approved recap and referenced decisions forward. Faithful artifacts do not require the user to reread or separately reapprove each one. Never run `propose` yourself.



## Included Skill: domain

---
name: domain
description: Actively build and sharpen a project's domain model. Use when the user wants to pin down domain terminology or a ubiquitous language, record an architectural decision, or when another skill needs to maintain the domain model
---

Actively build and sharpen the project's domain model as you design. This is the *active discipline* — challenging terms, inventing edge-case scenarios, and writing the glossary and decisions down **the moment they crystallise**. (Merely reading CONTEXT.md for vocabulary is not this skill — that's a one-line habit any skill can do. This skill is for when you're changing the model, not just consuming it.)


## File structure

Most repos have a single context:

```
/
├── CONTEXT.md
├── docs/
│   └── adr/
│       ├── 0001-event-sourced-orders.md
│       └── 0002-postgres-for-write-model.md
└── src/
```

If a `CONTEXT-MAP.md` exists at the root, the repo has multiple contexts; the map points to where
each one lives (per-context `CONTEXT.md` and `docs/adr/`). Single context is the default.

```
/
├── CONTEXT-MAP.md
├── docs/
│   └── adr/                          ← system-wide decisions
├── src/
│   ├── ordering/
│   │   ├── CONTEXT.md
│   │   └── docs/adr/                 ← context-specific decisions
│   └── billing/
│       ├── CONTEXT.md
│       └── docs/adr/
```

**Create files lazily** — only when you have something to write. If no `CONTEXT.md` exists, create one
when the first term is resolved. If no `docs/adr/` exists, create it when the first ADR is needed.

## During the session
### Challenge against the glossary
When the user uses a term that conflicts with the existing language in `CONTEXT.md`, call it out immediately. "Your glossary defines 'cancellation' as X, but you seem to mean Y — which is it?"

### Sharpen fuzzy language
When the user uses vague or overloaded terms, propose a precise canonical term. "You're saying 'account' — do you mean the Customer or the User? Those are different things."

### Discuss concrete scenarios
When domain relationships are being discussed, stress-test them with specific scenarios. Invent scenarios that probe edge cases and force the user to be precise about the boundaries between concepts.

### Cross-reference with code
When the user states how something works, check whether the code agrees. If you find a contradiction, surface it: "Your code cancels entire Orders, but you just said partial cancellation is possible — which is right?"

### Update `CONTEXT.md` inline
When a term is resolved, update `CONTEXT.md` right there. Don't batch these up — capture them as they happen. Use the format from `skl skill --resource reference/CONTEXT-FORMAT.md domain`.

`CONTEXT.md` should be totally devoid of implementation details. Do not treat `CONTEXT.md` as a spec, a scratch pad, or a repository for implementation decisions. It is a glossary and nothing else.

### Offer ADRs sparingly
Only offer to create an ADR when all three are true:

1. Hard to reverse — the cost of changing your mind later is meaningful
2. Surprising without context — a future reader will wonder "why did they do it this way?"
3. The result of a real trade-off — there were genuine alternatives and you picked one for specific reasons

If any of the three is missing, skip the ADR. Use the format from `skl skill --resource reference/ADR-FORMAT.md domain`.
