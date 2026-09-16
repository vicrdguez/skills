# Integrate Before Audit

## Why

Review should judge the change together with a recently observed integration target, without chasing target movement indefinitely or shifting final merge authority away from the human.

## What

Require the implementor to merge one current-target snapshot immediately before each submission's Audit, including finding-driven Rework. Resolve conflicts before normal review and carry integration and final-state verification evidence into the existing Submission.

## Scope

- Implement, Audit, and Watchdog guidance for pre-Audit integration, integration-aware review, evidence freshness, and safe recovery.
- Public first, resume, and Rework Execution Skills, applicable inspection continuations, and the deferred Submission resource.
- Implementation after #48, #49, and #50 are Merged, independent of `verify-contract-conformance`; reuse the already-Merged #37/#46 human-integration behavior.

## Out of Scope

- Post-approval conflict-resolution modes, conflict-driven automatic Rework, human merge automation, or queue changes.
- Engine-owned target snapshots, synchronization receipts, new evidence schemas, or parsing agent-authored Verification prose.
- Testing-skill replacement, delegation, artifact-authoring redesign, or agent behavior evaluations.

## Definition of Done

- [ ] First implementation and finding-driven Rework direct worker-owned observation and integration at the late pre-Audit step, with conflicts resolved before normal review.
- [ ] Deferred Submission guidance records the actual integrated SHA in Verification while preserving distinct review and artifact references and opaque Result Documents.
- [ ] Audit and Watchdog account for integration effects without reopening accepted preferences or labeling unrelated upstream additions as scope creep; evidence covers the final functional state.
- [ ] Target movement alone does not restart the round, and observation, merge, or consequential conflict failures preserve progress without false completion claims or changed human merge authority.
- [ ] Focused verification at the public seams in `plan.md` demonstrates coherent instruction and resource delivery while retaining the prerequisite foundation's fixed-head, recovery, and lifecycle behavior.

## Manual verification

None.
