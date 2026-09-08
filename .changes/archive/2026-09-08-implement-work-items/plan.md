# Implement Work Items Plan

## Approach
Extend the deep Workflow module with implementation start, resume, submit, and Needs Human operations. The CLI supplies typed intent and opaque body files; Workflow owns eligibility, Claim handling, fixed Git preconditions, and backend transitions. The Agent Worker receives one concrete Instruction Packet and continues to own TDD, Audit, code changes, project checks, commits, merges, and pushes.

Build Artifact lifecycle validation as the existing critical internal ledger-history module rather than spreading Git diff rules through command handlers. Extend the Backend port only with normalized queries and mutations exercised by implementation.

## Implementation decisions
- Expose stage-oriented `skl implement` operations for next/start, explicit resume, review submission, and Needs Human handoff. Do not add generic label, PR, Git, or state mutation commands.
- Expected `work_available`, `no_work`, `fix_required`, and `needs_human` outcomes are successful structured results; invalid invocation and unexpected operational failures are nonzero.
- Select oldest Rework first, then oldest Ready for Implementation, with stable Work Item identity as the tie-breaker. Eligibility requires all Dependencies to be semantically Merged, not merely closed or Ready for Merge.
- Preserve the V1 best-effort `wip` Claim and single-operator assumption. Add `wip`, read projections back, and stop on contradictory state; introduce no Claim Token.
- Ordinary next skips `wip`. Resume requires explicit Work Item identity or an unambiguous conventional worktree and never silently claims another item.
- Work Start observes a Target Snapshot and emits it in the packet and retry command. The Agent Worker merges it. Submission validates that exact snapshot is an ancestor of the proposed head. If interrupted before work changes, resume may safely observe a newer snapshot; ambiguous changed history needs repair rather than guessing.
- Normal finding-driven Rework retains its prior target relationship. Synchronization Rework is distinct and pins a newer target in the later lifecycle slice.
- Build the Instruction Packet through the Catalog so Implement, TDD, Audit, Design, and Domain render once and are declared in `included_skills`.
- Keep the existing TDD sequence, one logical commit per behavioral task, Audit fixed-point semantics, two independent axes, parallel-when-capable behavior, sequential fallback, Full Gate, and finding disposition rules.
- `skl` does not execute, configure, or accept attestations for project tests, typechecking, linting, formatting, parsing, or Audit.
- Extend ledger-history inspection from proposal Baseline to the full first-parent `absent* → present+ → absent*` lifecycle. The unique present-to-absent commit is deletion; its first parent is Artifact Completion.
- Allow multiple tick and code commits while present. From Baseline through Completion, file/path structure is fixed and only existing non-manual `[ ]` becomes `[x]`; all agent-verifiable boxes are checked at Completion and Manual Verification remains unchecked.
- A merge commit must leave the ledger tree equal to its first parent. Reject imported, split, repeated, recreated, missing, or ambiguous transitions and any presence after deletion.
- First-pass review submission requires local head equals pushed remote head, Target Snapshot ancestry, valid complete ledger retirement, and a stable head throughout the mutation. Re-read after ambiguous remote writes and resume forward.
- Rework submission requires the same PR and retired artifacts still absent; it never recreates the ledger.
- Body files are opaque. The agent chooses prose and dispositions; the semantic command chooses the state transition. Append only the exact machine-owned closing footer to the PR body.
- Successful body publication removes its private temporary directory. Fixable failure retains the directory, Claim, and state for the same or resumed worker.
- First-pass success creates/updates a PR, adds `review`, then removes `ready` and Claims only after the Submission is durable. Rework success swaps `rework + wip` to `review` on the same PR.
- Before code exists, Needs Human publishes on the issue. When work exists, push and preserve it in one draft PR. Retain the exact state to resume.
- Revise Implement and Audit definitions surgically: replace selection, GitHub, fixed-fact, and artifact-location mechanics while preserving their reasoning, order, gates, Audit axes, and TDD behavior.

### Module shapes & seams

#### [MODIFIED] Workflow
**Interface:** request the next implementation Work Item, resume one by identity, submit one for review, or pause one for a human; receive a typed outcome and Instruction Packet where applicable.

Workflow hides queue ordering, eligibility, Claim reconciliation, state transitions, fixed heads, and publication ordering. The CLI does not coordinate those steps.

Dependencies: normalized Backend port, concrete Repository, Instruction Catalog, and private temporary Result Document storage.

**Test strategy:** invoke all behavior scenarios through `skl implement` against an in-memory Backend and temporary real Git repositories. Assert returned outcomes and final observable Workflow Projections, not internal calls.

#### [MODIFIED] Implementation Ledger history
**Interface:** inspect a branch/ref and slug and return Baseline, optional Completion/deletion, lifecycle phase, integrity facts, and precise violations.

This module owns first-parent tree transitions and frozen checkbox-delta rules. It parses only the fixed Implementation Ledger contract needed for these established invariants; it never parses Result Documents, issue bodies, PR bodies, or commit messages.

**Test strategy:** construct literal Git graphs covering multiple tick commits, code-only commits, valid deletion, deletion before completion, unchecked Manual Verification, recreation, path-changing merges, and ambiguous cycles. Assert fixed commit identities and violations through the module interface.

#### [MODIFIED] Backend port and GitHub adapter
**Interface:** query normalized Ready/Rework candidates, Dependencies, Claims, source issues, Submissions, comments, labels, and remote refs; conditionally project Claim, Awaiting Review, and Needs Human transitions.

Keep GitHub issue/PR endpoints, label names, pagination, and IDs inside the adapter. The port exposes semantics needed by Workflow, not a generic forge client.

**Test strategy:** Workflow behavior uses the in-memory adapter. HTTP adapter tests cover pagination, label/event normalization, create/update reconciliation, and mutation timeouts followed by observation.

#### [MODIFIED] Repository
**Interface:** resolve conventional worktrees and refs, compare target ancestry and local/remote heads, and supply first-parent trees/diffs to ledger inspection.

The module remains concrete and read-mostly for active implementation. It does not merge, commit, push, or delete the ledger.

**Test strategy:** temporary real repositories exercised through Workflow and ledger-history tests.

#### [MODIFIED] Instruction Catalog
**Interface:** build a specialized Implement packet from typed Work Item, target, artifact snapshot, Submission, finding, and command facts.

Catalog owns templating and once-only skill composition, not queue or state decisions.

**Test strategy:** golden Markdown and decoded JSON packet assertions prove concrete values and exact bundled definitions without testing prose fragments individually.

## Sequence
1. Add normalized implementation candidates, Dependencies, Claims, and Submission records to Workflow and Backend.
2. Implement next/no-work/resume and Target Snapshot packet behavior.
3. Complete Git-only ledger lifecycle and integrity validation.
4. Implement first-pass review submission with opaque body publication and issue-closing footer.
5. Add Rework resume/resubmission and repairable refusal behavior.
6. Add issue-only and draft-Submission Needs Human handoffs.
7. Surgically migrate Implement and Audit definitions to engine-supplied facts and historical ledgers.
8. Update Work Item lifecycle documentation and run consistency searches for stale board/archive mechanics.
