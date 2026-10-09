---
description: Drain the Implement queue through dispatch outcomes in team mode.
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --mode team --dispatch --wait` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots. For --auto, pass the bare flag when its value is true; omit it when false or empty.
- --worker-model: $1
- --worker-thinking: $2
- --helper-model: $3
- --helper-thinking: $4
- --reviewer-model: $5
- --reviewer-thinking: $6
- --auto: $7
Follow each Outcome Instruction until one tells you to stop.
