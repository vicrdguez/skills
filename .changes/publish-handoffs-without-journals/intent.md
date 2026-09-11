# Publish Safe Handoffs Without Journals

## Why

Implementation and Watchdog exchange a Work Item through separate GitHub writes. Exposing a destination before evidence and source cleanup finish lets the other lane claim incomplete work. Recovery currently uses hidden implementation transitions and PR label history, and can release a later Claim at the same SHA. A local cleanup error can also misreport a published handoff as failed.

## What

Deliver approved slice 4 as a standalone safety producer before the candidate-first consumer in slice 5. Publish and verify evidence, finish source cleanup while the destination remains nonclaimable, then release its `wip` last. Retry forward only where visible state, supplied Result Documents, and readbacks prove the action; otherwise stop without changing Claims or inventing durable execution state.

Dependencies: None. Slices 1, 2, 3, and 5 and the separate pending claim-safety task are not blockers.

## Scope

- Implementation submit and Needs Human, including first Submission creation, existing Rework, issue-only decisions, and draft preservation.
- Watchdog pass, rework, and Needs Human publication, including source cleanup and the release point.
- Shared recovery protections across semantic retries, explicit resume, and status; no caller may bypass an ambiguous Claim refusal.
- Exact opaque evidence and structured-anchor readbacks, observed completed-unclaimed no-op success, changed-input refusals, and accepted-write/lost-response recovery.
- Removal of transition-journal production and consumption, including hidden `from`, `target`, `head`, `body_digest`, `decision_digest`, `directory`, and `completed` operation records. Ordinary Git heads remain valid evidence.
- Removal of `resume_state` dependence and PR timeline inference of transition direction. Fresh workers inspect the same Work Item's Git, artifacts, and visible feedback.
- Safe separation of opaque decision transport from retained trusted pin metadata while slices 2 and 3 remain unmerged.
- Post-publication local cleanup warnings that preserve successful outcomes without rollback or Claim reacquisition.
- Public CLI regression proof through the actual GitHubBackend HTTP adapter and real Git/files, with controlled other-lane interleavings.

## Out of Scope

- Slice 5's wholesale candidate discovery, queue filtering, ordering, ownership-link migration, or checkout-free startup. Only journal-dependent selection exclusions invalidated by this change are removed; producer safety must already hold for a label-only consumer.
- Slice 2's target policy, Target Snapshot removal, or conflict-to-Synchronization-Rework removal. Preserve the policy present when implementation begins.
- Slice 3's Review Checkpoint, review-count policy, review-pin removal, and timeline bounce-count removal. If merged, preserve evidence -> checkpoint -> release ordering and its cleanup contract; otherwise preserve existing review policy.
- Slice 1's artifact markers or ledger-validation replacement.
- Taking over the separate other-session claim-safety task. Reuse its shared observable protections if landed; implement only protections required by these handoffs and their recovery.
- Leases, locks, Claim Tokens, operation IDs, new journals, hidden content-hash records, private authoritative databases, automatic Claim expiry, and durable Agent Worker execution state.
- Prose interpretation, automatic recovery of every historical command, historical metadata deletion/migration, or claiming that all metadata disappears in this slice alone.
- Queue supervisors, dev-runner decomposition, harness execution, and human merge decisions.

## Definition of Done

- [ ] B1 Publish implementation review with release last: new and Rework Submissions become claimable only after verified evidence and complete source cleanup.
- [ ] B2 Publish implementation pauses with release last: issue-only and draft pauses preserve their evidence and work before releasing Claims, without `resume_state`.
- [ ] B3 Publish Watchdog verdicts with release last: all verdicts verify required evidence and source cleanup, and any already-landed checkpoint, before release.
- [ ] B4 Keep destinations nonclaimable when source cleanup fails: every affected producer retains protection and preserves published evidence at each failed cleanup step.
- [ ] B5 Retry a provable partial handoff forward: retained exact Result Documents and unambiguous source/evidence observations complete missing effects without duplicate publication.
- [ ] B6 Recognize verified completed unclaimed handoffs: exact evidence and final state produce no-op success without a journal or a second publication.
- [ ] B7 Refuse changed input during handoff recovery: mismatched bodies, decisions, summaries, or anchors never silently replace already-published evidence.
- [ ] B8 Preserve a new Claim after an uncertain release: accepted writes with lost responses and failed readbacks cannot cause retry, resume, or status to release a subsequent Claim.
- [ ] B9 Refuse stale commands at the same SHA: an old command whose tuple is indistinguishable from a later claimed round stops without changing state or adding identity storage.
- [ ] B10 Report cleanup warnings after verified publication: local cleanup failures leave the successful outcome intact without resubmission, rollback, or `wip` reacquisition.
- [ ] B11 Stop on unknown or contradictory recovery state: ambiguous direction, missing evidence, and drift cause actionable refusal, not status-driven Claim release.
- [ ] B12 Recover without transition journals or timeline direction: new commands neither write nor consume retired operation records, while independently owned pin and review policies remain intact.
- [ ] B13 Keep envelope-shaped decisions opaque: trusted-author decision prose cannot become transition or pin metadata after digest removal, and retained legitimate pins still work.
- [ ] B14 Resume only the selected Work Item from current evidence: packets direct fresh workers to inspect preserved Git, artifacts, and feedback without a persisted execution cursor or `resume_state`.

## Manual verification

None.
