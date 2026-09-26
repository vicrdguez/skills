Interview the user relentlessly until you share one understanding of the change. Map it as a **design tree**: every decision branches into the decisions that hang off it.

## Start

- When the user supplies the exact path to a `.thinking` artifact, read it as starting context and carry its settled concepts forward unless the project contradicts them. Read only the path the user gives.
- When the user names a superseded slice, read its Contract with `skl ledger show --item <proposal>/<slice>`, and its reports by adding `--phase implement` and `--phase watchdog`. Keep what the failed attempt established, and reopen the decisions its supersession implicates.

Use the `domain` guidance below throughout the session: sharpen the project's language and write durable docs as decisions crystallise.

## Ask in rounds

The **frontier** is every decision whose prerequisites are settled: the questions you can ask now without guessing at answers you haven't heard. Ask the whole frontier in one round, numbered, each with your recommended answer:

```
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

---

❓ **Q2** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>
```

Wait for the answers, recompute the frontier, and ask the next round. A question that depends on another question still open belongs to a later round.

Facts are yours to find; decisions are the user's. When a question needs a fact from the environment, dispatch a sub-agent to find it, and meanwhile ask the rest of the frontier: only the questions downstream of that fact wait for it. Put every decision to the user and wait for their answer.

Check: the frontier is empty. Every branch is visited and nothing is silently assumed.

## Recap and approval

Present one final recap that separates:

- consequential rules the change must preserve;
- architectural commitments and who owns each responsibility;
- choices deliberately delegated to implementation.

Name the ADRs and other decisions that carry them. Ask the user to correct or confirm the recap. A correction updates the tree and the recap; ask again. Confirmation approves the meaning of the whole Proposal.

After confirmation, stop. Suggest running the `propose` skill in this session, carrying the confirmed recap and its decisions forward. Leave running it to the user.


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
When a term is resolved, update `CONTEXT.md` right there. Don't batch these up — capture them as they happen. Use the format from `skl skill --resource CONTEXT-FORMAT.md domain`.

`CONTEXT.md` should be totally devoid of implementation details. Do not treat `CONTEXT.md` as a spec, a scratch pad, or a repository for implementation decisions. It is a glossary and nothing else.

### Offer ADRs sparingly
Only offer to create an ADR when all three are true:

1. Hard to reverse — the cost of changing your mind later is meaningful
2. Surprising without context — a future reader will wonder "why did they do it this way?"
3. The result of a real trade-off — there were genuine alternatives and you picked one for specific reasons

If any of the three is missing, skip the ADR. Use the format from `skl skill --resource ADR-FORMAT.md domain`.
