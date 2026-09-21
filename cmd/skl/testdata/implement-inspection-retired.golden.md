---
name: implement
description: Implement a single claimed change against its accepted behavioral and architectural contract.
disable-model-invocation: true
---

# Implement Inspection Continuation

Repository: acme/widgets on the selected remote `origin`
Work Item: #7
Branch: `widget`
Worktree: `<worktree>`
Artifact Baseline: `<baseline>`
Artifact Completion: `<completion>`

This is a read-only continuation of a single Work Item. It selects no other work and acquires no Claim: keep the Claim exactly as it is. Nothing below authorizes successful completion by itself.

## Observed progress

The completed ledger is already retired. Keep it absent, reuse the resolved endpoints, and finish the remaining verification and handoff without recreating it.

Read the historical accepted artifacts from the resolved endpoints rather than from the working tree:

- `git -C '<worktree>' show '<baseline>:.changes/widget/intent.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/behavior.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/plan.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/tasks.md'`
Read the completed task ledger from Artifact Completion while preserving the Baseline contract above:

- `git -C '<worktree>' show '<completion>:.changes/widget/tasks.md'`


Refresh integrity with `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` before editing, before Audit, and before handoff, wherever the current procedure needs current evidence.

Inspection does not observe or integrate the current target and a preparation-time fetch does not satisfy that obligation. After the remaining implementation or finding work and focused checks, immediately before this submission round's Audit, return to the main Implement procedure: fetch `main` from the selected remote `origin`, record `git -C '<worktree>' rev-parse FETCH_HEAD`, and merge that exact observed SHA before resolving conflicts and reviewing the integrated state. Preserve evidence of a successful late integration across resume; resuming or later target movement alone does not require another merge. If observation, merge, or a consequential conflict cannot be completed, preserve progress and use ordinary repair, resume, or Needs Human guidance without claiming successful integration or a completed Audit.

