---
description: Drain the Watchdog queue through dispatch outcomes.
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl watchdog next --dispatch --wait` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots. For --auto, pass the bare flag when its value is true; omit it when false or empty.
- --worker-model: $1
- --worker-thinking: $2
- --auto: $3
Follow each Outcome Instruction until one tells you to stop.
