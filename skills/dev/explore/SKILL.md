---
name: explore
description: A relentless interview to sharpen a plan, design or idea which also creates durable docs (ADRs, capabilities and glossary) as we go.
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

The session is done when the frontier is empty: every branch of the design tree visited, nothing left silently assumed. Do not act on it until the user confirms you have reached a shared understanding.

After confirmation, stop. Suggest running the `propose` skill in the same session so it can materialize the shared understanding, but never run it yourself.

