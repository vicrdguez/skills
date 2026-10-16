---
description: Drain the Implement queue through dispatch outcomes in standard Auto Mode.
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement dispatch --wait --auto --harness opencode` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: $1
- --worker-thinking: $2
- --reviewer-model: $3
- --reviewer-thinking: $4
Follow each Outcome Instruction until one tells you to stop.
