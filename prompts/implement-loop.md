---
description: Drain the implementation queue with one fresh subagent per work item
argument-hint: "[max-items] [model=provider/model:thinking]"
---

Act only as the scheduler for the implementation queue. This Pi session is dedicated to the implementation loop and must not run the watchdog loop.

Process at most ${1:-10} work items. Arguments: $@

If an argument starts with `model=`, use its value as the per-run `model` override on every `implement-runner` launch. Otherwise use the agent's configured default. Ignore the first argument as a model override when it is only the item limit.

For each iteration, launch exactly one foreground subagent with `agent: "implement-runner"`, `context: "fresh"`, the current project as `cwd`, and this task:

> Load and follow the implement skill exactly. Process at most one eligible work item, complete its full handoff, report the result, and exit. Do not run the outer queue loop.

Wait for it to finish, then record its issue or PR URL and board state.

Continue with another fresh runner when:
- the item reached verified `review` or `needs-human`; or
- the runner definitively failed before claiming an item.

Stop when:
- no eligible `ready` or `rework` item remains;
- a claimed item did not reach verified `review` or `needs-human`;
- the result does not establish whether an item was claimed;
- the item limit is reached.

Never resume or reuse a previous runner. Never pass one runner's conversation into the next. Do not implement, audit, or reinterpret findings yourself. The skill owns the work contract; you own only sequential lifecycle control. Never launch a runner at a `needs-human` item: only a person requeues those.

At completion, summarize every attempted item and its final board state.
