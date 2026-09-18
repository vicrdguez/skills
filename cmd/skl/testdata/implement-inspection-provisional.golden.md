---
name: implement
description: Implement a single claimed change following TDD, driven by its accepted Implementation Ledger.
disable-model-invocation: true
---

# Implement Inspection Continuation

Repository: acme/widgets on the selected remote `origin`
Work Item: #7
Branch: `widget`
Worktree: `<worktree>`
Artifact Baseline: `<baseline>`

This is a read-only continuation of a single Work Item. It selects no other work and acquires no Claim: keep the Claim exactly as it is. Nothing below authorizes successful completion by itself.

## Observed progress

The ledger is still present with partial completion ticks and preserved work after the Baseline. Inspect the remaining work and continue it without restarting the tasks already marked done; Artifact Completion belongs to the end of the whole implementation, not to this increment.

Read the historical accepted artifacts from the resolved endpoints rather than from the working tree:

- `git -C '<worktree>' show '<baseline>:.changes/widget/intent.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/behavior.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/plan.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/tasks.md'`


Refresh integrity with `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` before editing, before Audit, and before handoff, wherever the current procedure needs current evidence.

