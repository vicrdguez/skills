---
description: Drain the Implement queue through dispatch outcomes in team mode.
argument-hint: "[worker-model] [worker-thinking] [helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --mode team --dispatch --wait --worker-model '${1:-openai-codex/gpt-6-sol}' --worker-thinking '${2:-xhigh}' --helper-model '${3:-openai-codex/gpt-6-luna}' --helper-thinking '${4:-xhigh}' --reviewer-model '${5:-openai-codex/gpt-6-sol}' --reviewer-thinking '${6:-xhigh}'` and follow each Outcome Instruction until one tells you to stop.
