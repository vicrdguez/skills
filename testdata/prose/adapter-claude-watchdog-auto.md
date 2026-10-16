---
name: watchdog-auto
description: Drain the Watchdog queue through dispatch outcomes in Auto Mode.
disable-model-invocation: true
arguments: [worker_model, worker_thinking]
argument-hint: "[worker-model] [worker-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl watchdog dispatch --wait --auto --harness claude` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: $worker_model
- --worker-thinking: $worker_thinking
Follow each Outcome Instruction until one tells you to stop.
