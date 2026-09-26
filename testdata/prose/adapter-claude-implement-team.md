---
name: implement-team
description: Implement a single claimed change with a team of implementer subagents, against its accepted behavioral and architectural contract.
disable-model-invocation: true
arguments: [helper_model, helper_thinking, reviewer_model, reviewer_thinking]
argument-hint: "[helper-model] [helper-thinking] [reviewer-model] [reviewer-thinking]"
---

<!-- skl-owned: skl.adapter/v1 -->

Run `skl implement next --mode team --helper-model '$helper_model' --helper-thinking '$helper_thinking' --reviewer-model '$reviewer_model' --reviewer-thinking '$reviewer_thinking'` and follow the Execution Skill it returns.
