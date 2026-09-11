# Leave Integration Into main to Humans Plan

## Approach
Implement approved slice 2 independently; there are no blocking Dependencies. The inspected baseline is 2f68e43, including #32's opaque backend identities and late provider bindings. Follow CONTEXT.md's Ready for Merge and Merge Authority definitions and the candidate-first planned correction in docs/capabilities/work-item-lifecycle.md and docs/adr/0004-extract-workflow-mechanics-without-redesigning-agent-behavior.md. Those planned notes cover several slices; only target-pin removal and human-owned integration become current behavior here.

Delete the policy at its existing writers and consumers rather than introducing a replacement integration module, target configuration, or migration. Keep main as a literal Submission destination, not a new persisted per-item target. Target pins are not proof of the actual reviewed revision: removing them must not remove fixed-head publication checks.

## Implementation decisions

### Policy removal map
- workflow/implement.go: remove ImplementationItem.TargetSnapshot and TargetBranch, the snapshot argument through StartImplementation and prepareImplementationStart, target selection and head lookup for startup, local target-commit validation, and snapshot equality in Claim readback. Remove target facts and target-specific packet branches. Preserve branch, artifact, identity, Claim, and existing review-evidence checks not owned by this slice.
- workflow/handoff.go: remove the Target Snapshot presence/ancestry gate and item/default target resolution. New Submissions, including draft preservation, use main. Retain local/remote candidate-head guards, ledger checks, ownership, Result Document handling, and transition evidence.
- setup/implementation.go: remove target_snapshot, target_branch, and synchronization_target from implementationMetadata, ClaimImplementation writes, and ImplementationItems readers, including conflicting-target errors. Well-formed old JSON fields are ignored, not repaired or republished. Keep other metadata parsing, trust checks, review fields, resume state, and transition writers until their owning slices land. Remove ImplementationTarget from ImplementationBackend and its adapter implementation when its implementation/handoff consumers are gone; do not remove independent setup validation or proposal target behavior.
- cmd/skl/implement.go, setup/presentation.go, and catalog.go: remove --target-snapshot from every implementation subcommand, argument plumbing, target output fields, packet facts, and generated resume commands. Remove the forced git merge instruction. Retain selected remote propagation and provider-specific numeric CLI/packet presentation at the existing integration, not inside Workflow Mechanics.
- workflow/review.go: make a semantic pass choose ReadyForMerge independently of Mergeability. Remove unknown-mergeability refusal, requireMergeable guards, conflict target lookups, Synchronization Rework creation, and pass-retry compatibility that treats synchronization Rework as a successful pass. Retain exact retry evidence matching and all actual reviewed/final revision checks.
- workflow/status.go: remove done-to-Rework conflict rerouting and the mergeability requirement when reconciling an already evidenced pending pass. Continue deriving Merged from actual backend merge evidence and checking current head and other reconciliation validity.
- setup/watchdog.go: remove synchronization metadata publication and sync additions in CompleteReview. Remove conflict-specific done/rework/sync recovery recognition in ReviewSubmission; contradictory legacy overlaps require explicit inspection rather than fabricated approval or a revived target. Preserve ordinary partial-review reconciliation and the current timeline-based bounce-count logic, including historical sync exclusions, unless slice 3 has already replaced it. Do not remove the timeline merely because this file is touched.
- setup/preparation.go: stop provisioning sync as a supported automatic workflow label. Existing repository label definitions and stale labels are not globally removed.

### Stale sync policy
Tolerate stale sync without granting it a new workflow meaning. An existing rework projection remains Rework; sync alone grants no eligibility and resolves no contradictory labels. Startup and read-only observation perform no cleanup. Reuse AwaitImplementationReview's existing removal of sync on the selected Submission during its normal authorized handoff. Do not enumerate records for cleanup, delete old comments, reset count evidence, or write metadata to reconstruct a synchronization obligation.

Do not broaden this into removal of previous-reviewed-head requirements or all metadata writes. Retain existing review-evidence handling, including any narrowly necessary existing tolerance for a legacy sync item without a previous reviewed head; such tolerance must not produce a target merge instruction or a new pin. A normal Rework item that lacks evidence still follows the current explicit review-evidence repair path until slice 3 lands. Historical sync may remain an internal observation for that tolerance or current counting, not an active synchronization policy or new packet mode.

### Fixed main destination
The existing Submit implementation can reuse a PR's non-main base, and PublishImplementation can PATCH a differing base. Replace both behaviors: create with main; update only an already main-based PR. Refuse a known non-main existing PR before journaling or changing its body, draft state, lifecycle labels, or Claim. Recheck base in publication observations and review/status guards so a changed base cannot be silently repaired by a later PATCH or accepted on retry. A race discovered after an earlier valid effect is a repairable refusal, not rollback or a reason to overwrite the human's base.

Apply the guard to implementation submit, draft preservation, Watchdog final-body publication and pass retries, and status handoff reconciliation. Report a non-main selected item for explicit human inspection and base repair; do not block unrelated items by adding a repository-wide base audit. Status must not quietly certify a pending non-main pass. No new automatic retarget command, configuration, default-branch selection request, or main commit lookup is needed. The existing proposal --target contract and preflight are not redesigned here.

### Review validity and integration ownership
Mergeability is observational only: mergeable, conflicting, unknown, and changes between those observations do not change a valid pass. Keep actual candidate branch-head lookups through ImplementationHead; remove only lookups of the integration target. Preserve fixed reviewed revision matching, pushed local/remote/PR heads, post-marker descendant validation, draft/merged safeguards, artifact retirement, Claim validity, exact review anchors and retry evidence, and existing operational error handling.

No CLI path merges or closes the source Work Item as a side effect of pass. Preserve the closing reference for GitHub's human-merge closure. Conflict details may be mentioned in the worker session summary without a new durable field, finding type, state, journal, or obligation. Do not promise conflict-free integration when reporting done.

### Module shapes & seams

#### [MODIFIED] Workflow command module
The approved highest test seam is newApp(...).Run in cmd/skl. Exercise implement next/resume/submit/needs-human, watchdog submit, status, command help, and skill rendering there. Use existing real temporary Git repository helpers and valid artifact history for this baseline. Compare public outcomes, packet JSON/Markdown, unchanged Git state, and published effects. Do not substitute direct prepareImplementationStart, metadata decoder, mergeability helper, or rendering-helper unit tests for scenario coverage.

Dependencies are ordinary Git through real repositories and the existing backend interface through its current binding. Reuse existing CLI fixtures in cmd/skl/implement_test.go, watchdog_test.go, and status_test.go for engine paths. Do not grow a new seam or move provider parsing into Workflow Mechanics. Preserve opaque WorkItemID and SubmissionID values, ordering facts, ClosingReference, and #32's late provider bindings; no numeric assumptions in shared code.

#### [MODIFIED] GitHub workflow adapter
For metadata, stale labels, PR bases, publication retries, and partial review recovery, inject setup.NewGitHubBackend with an httptest server through the same public CLI factory. Drive the complete command across the actual adapter; do not replace ReviewSubmission with precomputed Bounces or bypass PublishImplementation. Controlled HTTP should record public requests and reject unexpected target-selection or target-head reads while allowing actual candidate-head safety reads. Inspect accepted PR bodies/bases, labels, comments, and source issue state through this external exchange.

Use the smallest extension of existing fixtures. Group Scenario Outline cases into one table-driven test per behavior. Cover partial handoff and completed-retry cases without a general fault-injection framework. Preserve existing failure-count regression coverage rather than implementing slice 3's new policy under these tests.

### Guidance and documentation
Update skills/dev/implement/SKILL.md, skills/dev/watchdog/SKILL.md, skills/dev/audit/SKILL.md, and packet generation surgically. First workflow review/Audit compares from main's merge-base, pinned by the worker using ordinary Git; it is not an implementation target obligation or a CLI-computed startup fact. Keep Artifact Baseline separate and preserve the existing repeat-review fixed-point policy and independent Audit's explicit-user-fixed-point behavior.

Remove promises of target synchronization, conflict-triggered rework, and old-flag resume. Explain that existing progress stays in place and integration is human-owned. Old workers must finish using the old binary or stop for controlled cutover and fresh instructions. Do not accept an obsolete --target-snapshot silently or restore its metadata.

The final documentation task updates README.md, docs/capabilities/work-item-lifecycle.md, and the relevant consequences/planned notes in ADR 0004. Move only this slice's implemented target/integration statements into current behavior. Keep slice 1 marker changes, slice 3 review checkpoints/counting, slice 4 journal removal, and slice 5 queue/deferred-startup changes planned unless independently landed. Do not claim the whole candidate-first correction is complete. CONTEXT.md already gives the intended human-integration meanings; no unrelated glossary rewrite is needed.

## Sequence
1. Materialize B1-B2 through the public CLI and remove target startup/handoff writers and consumers together, retaining existing artifact semantics.
2. Materialize B3-B5 across fresh verdicts, retries, and status; delete all conflict-triggered synchronization creation and mergeability gates, not just the first-pass branch.
3. Materialize B6-B8 with the actual GitHubBackend to pin stale-label cleanup and main-only create/update/refusal behavior.
4. Materialize B9-B10, retaining unrelated safety/count regressions and aligning runtime guidance and removed-flag handling.
5. Complete the final scoped documentation task, run go test ./... and git diff --check, and verify the one-to-one behavior/task mapping. No new dependencies, migrations, or broader sibling refactors.
