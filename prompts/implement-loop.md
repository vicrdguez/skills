---
description: Drain the implementation queue with one fresh subagent per work item
argument-hint: "[max-items] [model=provider/model:thinking]"
---

<!-- skl-owned: skl.pi/v1 -->

Act only as the scheduler for the implementation queue. This Pi session is dedicated to the implementation loop and must not run the watchdog loop.

Arguments: $@

Parse them before launching:
- the first decimal argument is the item limit, defaulting to `10`;
- at most one `model=provider/model:thinking` argument is the per-run `model` override; and
- any other argument is invalid: stop and report it.

Process at most the parsed item limit. Without a model override, use the agent's configured default.

For each iteration, launch exactly one foreground subagent with `agent: "implement-runner"`, `context: "fresh"`, the current project as `cwd`, and this task:

> Run `skl implement next` and follow its concrete packet, loading no definitions already included in its manifest. Process at most one Work Item, complete its full handoff, return the final CLI JSON unchanged, and exit. Do not run the outer queue loop.

Wait for it to finish. Write its final structured CLI JSON to a private temporary file, then run `node ~/.pi/agent/prompts/queue-next.mjs implement <completed-count> <item-limit> <result-file>`.

Continue with another fresh runner only when the adapter reports `continue`: a verified `awaiting_review` or `needs_human` handoff with its Claim released.

Stop when:
- the structured outcome is `no_work`;
- a claimed item did not reach a verified terminal handoff;
- the result does not establish whether an item was claimed;
- the item limit is reached.

Never resume or reuse a previous runner. Never pass one runner's conversation into the next. Do not implement, audit, or reinterpret findings yourself. The skill owns the work contract; you own only sequential lifecycle control. Never launch a runner at a `needs-human` item: only a person requeues those.

At completion, summarize every attempted item and its canonical Workflow State. Stop on adapter errors or any result other than exact `continue`.
