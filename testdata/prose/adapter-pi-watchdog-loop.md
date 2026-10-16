---
description: Drain the Watchdog queue through dispatch outcomes.
argument-hint: "[worker-model] [worker-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl watchdog dispatch --wait --harness pi` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: ${1:-}
- --worker-thinking: ${2:-}
Follow each Outcome Instruction until one tells you to stop.
