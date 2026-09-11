# Continue Verified Dispatches

## Why
Independent implementation and Watchdog supervisors need a CLI-owned answer to whether their specific worker handed off successfully. Worker prose, exit status, current labels, and an older completion of the same Work Item cannot answer that safely.

## What
Slice 2 of the five approved slices, blocked by `wait-for-claimable-work`. Each CLI selection claims one Work Item and returns a worker startup command using existing explicit resume plus a supervisor continuation command. `skl <lane> next --after <opaque-reference>` verifies that dispatched round's durable handoff before selecting or waiting in the same lane. Continuation is mutating and non-idempotent; callers stop on lost or ambiguous responses rather than automatically replaying it.

## Scope
- Complete implementation and Watchdog CLI paths: selection, round identity, explicit resume, packet commands, handoff publication, backend rereads, continuation, and structured outcomes.
- Local packet Result Directory lifecycle across `next`, startup, and handoff: reuse or safely remove superseded marker-only directories, preserving result/repair documents; no global cleanup or migration.
- Round-scoped evidence on existing Work Item backend projections, surviving process restart, temporary-file cleanup, later rounds, and the other lane's advancement.
- Stage-specific completion: implementation reaches `awaiting_review` or `needs_human`; Watchdog reaches `ready_for_merge`, `rework`, or `needs_human`, after its own Claim is released.
- Reuse slice 1 waiting, duration validation, cancellation, selection ordering, and error behavior; preserve repository and selected remote in returned commands.
- Explicit recovery guidance for uncertain selection results; single-use calling convention without consumption markers or replay guarantees.
- CLI tests with the existing fake Backend and real temporary Git; concrete GitHub adapter HTTP tests for durable persistence, history, and uncertain writes.

## Out of Scope
- Harness integration, worker spawning, loop prompts, model configuration, installation, and live harness smoke tests; later approved slices consume returned commands.
- New registry, private authoritative database, leases, atomic Claims, competing same-lane supervisors, automatic worker replacement, automatic handoff repair, or a waiting-loop retry policy.
- Parsing Result Document prose, redesigning Audit or Watchdog judgment, changing bounce rules, ordinary Git mutations, or automatic merge.
- Retrofitting unverifiable legacy Claims with invented completion evidence; explicit single-item recovery remains available.

## Definition of Done
- [x] DOD1: Selection returns executable, repository/remote-bound commands for one read-back Claim; root-bound startup works before the conventional implementation worktree exists and with an existing worktree. Resume preserves the original Target Snapshot/reviewed head or refuses a stale changed obligation without repinning; handoff commands remain worktree-bound. Neither lane leaves a discarded next packet's marker-only Result Directory orphaned on startup. (B1)
- [x] DOD2: Both lanes complete next -> returned startup -> packet handoff without orphaned superseded marker-only directories, preserving any superseded directory containing result/repair documents. CLI-only verification accepts every permitted completed stage handoff, including legitimate Needs Human without a Submission, independently of worker success/error or prose. Successful continuation reports the verified previous Work Item and outcome separately from any newly dispatched item for progress reporting. (B2-B3)
- [x] DOD3: Incomplete, wrong-stage, contradictory, or unproven handoffs halt with explicit recovery and no next selection, waiting, replacement, or Claim release. Older rounds cannot authorize later unfinished rounds, including equal-head rounds. (B4-B5)
- [x] DOD4: Proven completion remains usable after the other lane advances or claims the item; evidence is historical, round-specific, and independent of current aggregate Claim/state. (B6)
- [x] DOD5: Continuation obeys slice 1's immediate `no_work` versus waiting `idle_timeout` contract and both `--poll <duration>`/`--poll=<duration>` forms, starts a fresh idle window per call, preserves options, and leaves forge failures operational. Reusing a completed reference is ordinary non-idempotent selection, not response replay. (B7-B8)
- [x] DOD6: Invalid continuation references fail safely; ambiguous selection retains work and instructions require stopping for explicit recovery rather than automatically replaying `next` or `next --after`. (B9-B10)
- [x] DOD7: Real GitHub reads reconstruct round completion across adapter instances, pagination, later rounds, and unrelated metadata; existing uncertain-write readback is preserved and untrusted/contradictory evidence cannot authorize continuation. (B11-B12)
- [x] DOD8: Existing Full Gate regressions preserve PR #19 W1-W4 protections, single-item operation semantics, human merge authority, and Dependency eligibility only after Merged. (C2)

## Manual verification
None. Acceptance is observable at the approved CLI and HTTP seams; harness execution and live human merge verification belong outside this slice.
