# Retire change artifacts before review

## Why
Change artifacts are operational handoff material, but archiving them keeps their lines in every final PR and in the durable repository tree after implementation has made them obsolete. Durable knowledge already belongs in `CONTEXT.md`, ADRs, and capability docs.

## What
Keep change artifacts as the implementor's frozen checklist, record their completed state by commit, then delete them before a change enters review. Review and rework reconstruct the contract from the recorded commits; merged PR state replaces the archive folder as the cleanup signal.

## Scope
- Keep `.changes/<slug>/` on the single slice branch throughout the first implementation pass
- Continue permitting only existing non-manual `[ ]` boxes to become `[x]`
- Permit draft `needs-human` PRs to retain incomplete artifacts
- Require every agent-verifiable checkbox to be ticked and committed before entering `review`
- Record `Artifact baseline` and `Artifact completion` commit SHAs in the review PR body
- Delete `.changes/<slug>/` after recording completion and before entering `review`
- Exclude change artifacts from implementation diff review while checking their integrity separately
- Make watchdog reconstruct and verify the frozen contract and completed ledger from their recorded commits
- Keep artifacts deleted during rework; use the watchdog finding ledger to track rework
- Stop archiving artifacts when watchdog passes a change
- Use GitHub's merged PR state to identify worktrees and local/remote feature branches that can be removed
- Keep squash merge preferred without making pipeline correctness depend on it
- Update the pipeline documentation, setup stamp, GitHub protocol, stage skills, and artifact template guidance consistently

## Out of Scope
- Moving artifacts to a second branch, issue body, tag, or external store
- Preserving artifact commits as durable documentation after merge
- Enforcing a repository's merge strategy
- Changing the durable roles of `CONTEXT.md`, ADRs, or capability docs
- Changing the contents or required set of change artifacts beyond their lifecycle guidance
- Automatic PR merging

## Definition of Done
- [ ] During first-pass implementation, artifacts remain available as a frozen checklist, and an incomplete `needs-human` draft may retain them.
- [ ] A review-ready PR records the artifact baseline and completed-ledger SHAs and has no `.changes/<slug>/` in its final diff.
- [ ] Audit reviews implementation separately from artifact integrity, and watchdog verifies baseline-to-completion ticks plus completion-to-HEAD deletion before proving the contract against the final work.
- [ ] Rework reads the recorded artifact snapshots and watchdog findings without restoring or editing artifacts.
- [ ] A passing watchdog copies manual checks from the frozen artifact snapshot, marks the PR done, and creates no artifact archive.
- [ ] Merged PR state, rather than archive presence, gates removal of the worktree and local/remote feature branch.
- [ ] All pipeline instructions and templates describe the same artifact lifecycle without stale archive-on-pass guidance.

## Manual verification
None.
