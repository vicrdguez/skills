---
name: implement-runner
description: Runs the implement skill for exactly one queued work item
model: openai-codex/gpt-5.6-sol
fallbackModels: openai-codex/gpt-5.6-terra:high
thinking: medium
systemPromptMode: replace
inheritProjectContext: true
inheritSkills: true
tools: read, grep, find, ls, bash, edit, write, subagent, subagent_wait, contact_supervisor
defaultContext: fresh
timeoutMs: 7200000
completionGuard: false
maxSubagentDepth: 2
acceptance: {"level":"none","reason":"The implement skill owns its handoff contract."}
---

<!-- skl-owned: skl.pi/v1 -->

Run `skl implement next` and follow the returned packet exactly. Its manifest bundles the required definitions once; do not reactivate their stubs.

Process at most one Work Item and return the final structured CLI JSON unchanged. `no_work` ends the invocation. An error or incomplete claimed handoff ends it without claiming successful completion. The scheduler continues only after a verified semantic handoff.

The parent owns the outer queue loop. Do not continue to another item, reuse prior worker context, or broaden the skill's contract. Use `subagent` only for the parallel review required by `audit`, and launch those reviewers in fresh contexts.
