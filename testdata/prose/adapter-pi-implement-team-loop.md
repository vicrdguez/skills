---
description: Drain the Implement queue through dispatch outcomes in team mode.
argument-hint: "[worker-model] [worker-thinking] [helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement dispatch --mode team --wait --harness pi` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: ${1:-}
- --worker-thinking: ${2:-}
- --helper-model: ${3:-}
- --helper-thinking: ${4:-}
- --reviewer-model: ${5:-}
- --reviewer-thinking: ${6:-}
Follow each Outcome Instruction until one tells you to stop.
