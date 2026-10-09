---
description: Drain the Implement queue through dispatch outcomes in team mode.
argument-hint: "[worker-model] [worker-thinking] [helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking] [auto]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --mode team --dispatch --wait` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots. For --auto, pass the bare flag when its value is true; omit it when false or empty.
- --worker-model: ${1:-openai-codex/gpt-6.1-sol}
- --worker-thinking: ${2:-high}
- --helper-model: ${3:-openai-codex/gpt-6-luna}
- --helper-thinking: ${4:-xhigh}
- --reviewer-model: ${5:-openai-codex/gpt-6.1-sol}
- --reviewer-thinking: ${6:-high}
- --auto: ${7:-false}
Follow each Outcome Instruction until one tells you to stop.
