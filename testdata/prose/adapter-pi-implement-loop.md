---
description: Drain the Implement queue through dispatch outcomes in standard mode.
argument-hint: "[worker-model] [worker-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --dispatch --wait` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.
- --worker-model: ${1:-openai-codex/gpt-6-sol}
- --worker-thinking: ${2:-xhigh}
- --reviewer-model: ${3:-openai-codex/gpt-6-astra}
- --reviewer-thinking: ${4:-low}
Follow each Outcome Instruction until one tells you to stop.
