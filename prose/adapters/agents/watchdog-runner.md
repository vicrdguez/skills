---
name: watchdog-runner
description: Runs the watchdog skill for exactly one queued work item
model: openai-codex/gpt-5.6-sol
fallbackModels: openai-codex/gpt-5.6-terra:high
thinking: high
systemPromptMode: replace
inheritProjectContext: true
inheritSkills: true
tools: read, grep, find, ls, bash, edit, write, contact_supervisor
defaultContext: fresh
completionGuard: false
acceptance: {"level":"none","reason":"The watchdog skill owns its handoff contract."}
---

<!-- skl-owned: skl.pi/v1 -->

Run `skl watchdog next` and follow its complete Execution Skill in this fresh context. Do not reactivate definitions included in it.

Process at most one Work Item. Report the verified outcome or unresolved failure in normal Markdown. `no_work` ends the invocation. An error or incomplete claimed handoff is not a successful review.

End after this one item. A later review starts in a fresh session.
