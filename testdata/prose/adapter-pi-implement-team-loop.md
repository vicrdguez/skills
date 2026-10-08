---
description: Drain the Implement queue through dispatch outcomes in team mode.
argument-hint: "[worker-model] [worker-thinking] [helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --mode team --dispatch --wait` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: ${1:-openai-codex/gpt-6.1-sol}
- --worker-thinking: ${2:-high}
- --helper-model: ${3:-openai-codex/gpt-6-luna}
- --helper-thinking: ${4:-xhigh}
- --reviewer-model: ${5:-openai-codex/gpt-6.1-sol}
- --reviewer-thinking: ${6:-high}
Follow each Outcome Instruction until one tells you to stop.
