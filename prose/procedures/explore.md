Interview the user relentlessly until you share one understanding of the change. Map it as a **design tree**: every decision branches into the decisions that hang off it.

## Start

- When the user supplies the exact path to a `.thinking` artifact, read it as starting context and carry its settled concepts forward unless the project contradicts them. Read only the path the user gives.
- When the user names a superseded slice, read its Contract with `skl ledger show --item <proposal>/<slice>`, then its reports by running that command once with `--phase implement` and once with `--phase watchdog`. Keep what the failed attempt established, and reopen the decisions its supersession implicates.

Check: you have read everything the user named.

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
