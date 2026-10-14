---
name: implement-loop
description: Drain the Implement queue through dispatch outcomes in standard mode.
disable-model-invocation: true
arguments: [worker_model, worker_thinking, reviewer_model, reviewer_thinking]
argument-hint: "[worker-model] [worker-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --dispatch --wait --harness claude` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: $worker_model
- --worker-thinking: $worker_thinking
- --reviewer-model: $reviewer_model
- --reviewer-thinking: $reviewer_thinking
Follow each Outcome Instruction until one tells you to stop.
