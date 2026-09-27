---
description: Drain the Watchdog queue through dispatch outcomes.
argument-hint: "[worker-model] [worker-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl watchdog next --dispatch --wait --worker-model '${1:-openai-codex/gpt-6-astra}' --worker-thinking '${2:-high}'` and follow each Outcome Instruction until one tells you to stop.
