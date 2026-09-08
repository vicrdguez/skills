# Decouple Implementation Operations

## Why
Implementation must use the repository-bound Backend seam without embedding GitHub representations or delegating workflow decisions to its adapter. This completes the implementation portion of the backend-independence correction after #3.

## What
Carry implementation selection, Claims, resume, inspection, and handoffs through the seam established by decouple-setup-and-publication while preserving existing behavior, Git evidence requirements, packets, and durable recovery records.

## Scope
- Depend on decouple-setup-and-publication being Merged; its #8 dependency gates the complete #3 Workflow.
- Correct every implementation operation delivered by #3, including new Work Start, finding-driven Rework, explicit/worktree resume, inspection, review submission, and Needs Human handoffs.
- Keep eligibility, canonical state validation, deterministic ordering, transition permission, and recovery policy exclusively in the Workflow Engine.
- Consume repository-bound Backend observations with opaque Work Item and Submission references; the engine neither resolves GitHub remotes nor interprets native numbers, labels, or comment metadata syntax.
- Keep Git evidence, Target Snapshot ancestry, fixed heads, and ledger history separate from backend observations.
- Move native closing-footer and record-reference rendering to integration code while preserving opaque prose and existing public representations.
- Preserve interrupted Claims, adopted records, persisted Target Snapshots, previous reviewed heads, and pending handoff recovery.

## Out of Scope
- New lifecycle behavior, different queue ordering, changed claims or review policy, and fixing unrelated pre-existing defects.
- A second Backend, a new Repository abstraction, native identifier migration, or new workflow configuration.
- Review/status/completion decision migration, except mechanical shared-call-site changes needed to keep those paths building.
- Running agent judgments or project gates inside the CLI, and a standalone portability test suite.

## Definition of Done
- [x] B1 Selection and no-work preserve engine-owned eligibility, ordering, and Claim effects through normalized Backend records.
- [x] B2 Explicit and worktree resume retain the same opaque Work Item identity and existing pinned obligations.
- [x] B3 First-pass and Rework packets preserve their distinct synchronization instructions and once-only skill composition.
- [x] B4 Inspection and invalid handoffs retain all existing Git/state invariants and repair outcomes independently of provider representation.
- [x] B5 First-pass and Rework review submission preserve the same Submission, opaque body, native closing reference, and lifecycle effects.
- [x] B6 Needs Human preserves issue-only versus draft handoffs, resume state, and Result Document lifetime.
- [x] B7 Interrupted handoffs resume forward with the same Claim, head obligations, and durable identity without duplicate publication.
- [x] Every implementation decision is engine-owned and every provider detail is integration-owned, as verified against ADR 0002 across production callers, Backend code, and packet rendering.
- [x] Existing implementation, adapter, Repository, and Catalog regression checks remain green; documentation reflects ownership without changing advertised behavior.

## Manual verification
None.
