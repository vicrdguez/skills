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

The ledger was already retired during finding-driven Rework. Keep it absent and resolve the supplied findings against the current PR comparison; read the historical accepted artifacts at the resolved endpoints instead of recreating the ledger.

Read the historical accepted artifacts from the resolved endpoints rather than from the working tree:

- `git -C '<worktree>' show '<baseline>:.changes/widget/intent.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/behavior.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/plan.md'`
- `git -C '<worktree>' show '<baseline>:.changes/widget/tasks.md'`
Read the completed task ledger from Artifact Completion while preserving the Baseline contract above:

- `git -C '<worktree>' show '<completion>:.changes/widget/tasks.md'`

## Optional bounded delegation

The invocation did not establish whether a supported helper mechanism is available. A harness name alone proves nothing. Only if delegation would help, make one small bounded runtime check for the supported Claude `Agent` or Pi `subagent` mechanism; use a recipe only after support is established, otherwise perform the subwork serially. Do not install tools, create a capability registry or scheduler, or keep probing.

Do not require delegation, a separate test writer, a fixed helper count, or a worktree per helper. The selected Work Item and its Claim remain with this owner. Give each fresh helper a self-contained brief containing the bounded assignment, selected Work Item and working location, authoritative contract references or contents, relevant architectural commitments and standards, required observations, and explicit write responsibilities. Distinguish those obligations from implementation-detail preferences: helper choices remain subordinate to the accepted contract.

Prevent conflicting concurrent writers and Git operations by dividing responsibilities or serializing overlapping work. Helpers must not select queue work, acquire another Claim, change Workflow State, make the final Submission, or decide consequential unresolved requirements; return such decisions through the owner to the existing human-decision path. Helpers return contributions, checks, evidence, and limitations. The owner inspects and integrates every contribution, finishes missing work, and verifies the resulting functional state. Helper reports or isolated passing checks never replace affected integrated checks, the final Full Gate and Audit, artifact integrity, independent Watchdog Review, owner-controlled submission, or human-only merge.


Refresh integrity with `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` before editing, before Audit, and before handoff, wherever the current procedure needs current evidence.

