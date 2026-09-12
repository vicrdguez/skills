# Leave Integration Into main to Humans

## Why
Implementation-start target pins and conflict-triggered Synchronization Rework make agents perform integration that belongs to the Merge Authority. A reviewed candidate can be correct without containing a historical target commit or being conflict-free against today's main. Retrying a passing verdict or observing status must not silently turn approval into more implementation work.

## What
Deliver approved candidate-first slice 2 as a standalone change with no blocking Dependencies. Remove the Target Snapshot and automatic Synchronization Rework policies from implementation startup, submission, Instruction Packets, Watchdog verdict publication and recovery, and status reconciliation. Use main as the only Submission destination. A valid passing Watchdog Review reaches Ready for Merge regardless of mergeability; integration and merge remain human-owned.

## Scope
- Remove target_snapshot, target_branch, and synchronization_target metadata fields, their writers and consumers, and implementation-start target branch/head lookups.
- Remove Target Snapshot facts, --target-snapshot flags, resume obligations, local target-commit availability checks, and submitted-head ancestry checks against an implementation target pin.
- Remove generated instructions that require merging a pinned target or synchronizing main before coding or review.
- Create Submissions against main, including draft preservation through implement needs-human; update an existing main-based Submission without choosing a configurable or repository-default destination.
- Refuse the affected command with explicit human repair guidance when its existing Submission has a base other than main. Preserve the human's PR base rather than silently retargeting it.
- Publish a valid pass as done for mergeable, conflicting, and unknown mergeability, including mid-publication changes, retries, and unambiguous status reconciliation.
- Stop conflict-triggered target lookups, sync metadata writes, sync label additions, and automatic Rework creation. Conflicts are informational session context, not an integration gate.
- Tolerate stale sync on existing Rework without a target obligation. Leave it alone during observation/startup and remove it only through the existing authorized implementation-to-review handoff. Do not create a migration or sweep labels.
- Preserve revision/head publication safety, artifact integrity, Claims, review evidence, existing failure-count policy, human-only merge, and source-issue closure only after human merge.
- Update Implement, Watchdog, and Audit guidance plus current lifecycle documentation for this slice alone.

## Out of Scope
- Slice 1 artifact marker/endpoint validation changes or changes to proposal publication Git preflight.
- Slice 3 removal of hidden review pins, previous-reviewed-head requirements, or replacement of bounce counting with the private Review Checkpoint and completed Review Count policy.
- Slice 4 transition-journal removal, handoff ordering redesign, or generalized recovery/cleanup-warning changes.
- Slice 5 candidate queue filtering, selected-item loading, or deferred startup that works without local project objects.
- New CLI-owned main merge-base calculation, fetching, worktree preparation, commits, pushes, integration, conflict resolution, or merging. Ordinary Git remains Agent Worker work except human-owned integration.
- New target configuration, configurable PR destinations, repository-wide metadata/label cleanup, migrations, or backward compatibility for --target-snapshot. Existing workers finish with the old CLI or use a controlled cutover; no legacy pin is restored.
- Redesigning independent review, Audit judgment, the Full Gate, Debt Markers, Manual Verification, queue adapters, or backend identity/binding contracts already landed in #32.

## Definition of Done
- [x] D1 Implementation next and resume succeed after main advances without resolving, persisting, restoring, or requiring an implementation target pin, while preserving existing work and unrelated startup checks.
- [x] D2 A pushed candidate with valid retired artifacts can be submitted without the current or formerly pinned target commit being locally available or an ancestor of its head.
- [x] D3 A valid fresh Watchdog pass publishes Ready for Merge for mergeable, conflicting, or unknown mergeability, without synchronization side effects or closing the source issue.
- [x] D4 Retrying the same valid pass completes or confirms Ready for Merge despite changed mergeability, retaining existing review evidence and head safeguards and never rerouting to synchronization.
- [x] D5 Status preserves done and reconciles an unambiguous partial pass forward despite conflicting or unknown mergeability, without target lookups or automatic Rework.
- [x] D6 Existing sync-labeled Rework remains governed by its ordinary Rework state, Claim, and retained review-evidence rules, ignores obsolete target metadata, and only loses stale sync at its normal authorized review handoff.
- [x] D7 New and updated Submissions use main as their sole destination, including draft preservation, without a target-selection lookup or new target configuration.
- [x] D8 An existing non-main PR causes an actionable command-scoped refusal before a workflow handoff or PR edit; its base, body, labels, and existing Claim remain intact for explicit human repair.
- [x] D9 Removing target pins and mergeability gates preserves fixed reviewed revision, pushed-head, post-marker ancestry, artifact, Claim, and publication-validity checks and the current failure-count policy.
- [x] D10 Public CLI help and rendered Implement, Watchdog, and Audit guidance contain no --target-snapshot or forced integration obligation; they identify main-based first-review scope and human integration ownership without moving ordinary Git work into the CLI.
- [x] D11 README, lifecycle capability, and ADR notes accurately describe only the delivered integration-policy correction and leave unimplemented sibling decisions planned.

## Manual verification
None
