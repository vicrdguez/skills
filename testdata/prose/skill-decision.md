Protocol: skl.instructions/v1
Skill: decision
Included skills: none
Facts: {}
Resources: reference/triage.md

---
name: decision
description: Resolve current Needs Human requests from the ledger-wide Decision Inbox with explicitly scoped human direction.
disable-model-invocation: true
---

# Decision Inbox

Resolve the current Needs Human requests in the configured Workflow Ledger. This retrieval carries no ledger facts, so it neither reads nor changes the inbox.

Run:

`skl decision inbox`

That command reads the ledger-wide inbox and returns the specialized instructions for the current requests. Add `--project <name>` to narrow it explicitly; a Project never comes from the current working directory. Retrieve the full triage rules for grouping related questions, presenting each request's commitments, conflicts, evidence, options, consequences, and recommendation, and distinguishing a human answer from discussion with:

`skl skill --resource reference/triage.md decision`

Reading or grouping requests records no decision. Only an explicitly scoped human answer submitted through the bound `skl decision apply` command records an answer and its continuation route.


