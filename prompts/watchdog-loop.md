---
description: Drain the watchdog queue with one fresh subagent per work item
argument-hint: "[max-items] [model=provider/model:thinking]"
---

Act only as the scheduler for the watchdog queue. This Pi session is dedicated to the watchdog loop and must not run the implementation loop.

Process at most ${1:-10} work items. Arguments: $@

If an argument starts with `model=`, use its value as the per-run `model` override on every `watchdog-runner` launch. Otherwise use the agent's configured default. Ignore the first argument as a model override when it is only the item limit.

For each iteration, launch exactly one foreground subagent with `agent: "watchdog-runner"`, `context: "fresh"`, the current project as `cwd`, and this task:

> Load and follow the watchdog skill exactly. Process at most one eligible work item, complete its full handoff, report the result, and exit. Do not run the outer queue loop.

Wait for it to finish, then record its PR URL and board state.

Continue with another fresh runner when:
- the item reached verified `done`, `rework` or `needs-human`; or
- the runner definitively failed before claiming an item.

Stop when:
- no eligible `review` item remains;
- a claimed item did not reach verified `done`, `rework` or `needs-human`;
- the result does not establish whether an item was claimed;
- the item limit is reached.

Never resume or reuse a previous runner. Never pass one runner's conversation into the next. Do not review, audit, or reinterpret findings yourself. The skill owns the work contract; you own only sequential lifecycle control. Never launch a runner at a `needs-human` item: only a person requeues those.

At completion, summarize every attempted PR and its final board state.
