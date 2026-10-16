---
name: implement-team-loop
description: Drain the Implement queue through dispatch outcomes in team mode.
disable-model-invocation: true
arguments: [worker_model, worker_thinking, helper_model, helper_thinking, reviewer_model, reviewer_thinking]
argument-hint: "[worker-model] [worker-thinking] [helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement dispatch --mode team --wait --harness claude` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: $worker_model
- --worker-thinking: $worker_thinking
- --helper-model: $helper_model
- --helper-thinking: $helper_thinking
- --reviewer-model: $reviewer_model
- --reviewer-thinking: $reviewer_thinking
Follow each Outcome Instruction until one tells you to stop.
