# Validate Marked Artifact Endpoints

## Why

The Workflow Engine currently discovers artifacts by replaying first-parent tree transitions and loading every intervening ledger. That makes inspection grow with implementation history and rejects temporary edits even when the accepted contract is restored. Workers also receive instructions that disagree with the approved candidate-first endpoint contract.

## What

Resolve the Artifact Baseline and Artifact Completion from slice-scoped commit-subject markers and validate their snapshots, not intermediate artifact contents. Deliver the complete path from proposal baseline preparation through inspection, current startup, implementation handoff, and independent Watchdog Review. Existing markerless evidence can be supplied explicitly without rewriting history or persisting Adoption metadata.

## Scope

- Use the exact subject prefixes `[baseline] <slice-slug>` and `[completion] <slice-slug>`; the slug is chosen before issue creation and identifies `.changes/<slice-slug>/`.
- Resolve unique relevant markers in the selected Git history, including reachable merge parents, without new repository-wide issue/PR searches or latest-pair guesses.
- Preserve proposal preflight and opaque body transport while requiring the marked, complete baseline at the pushed publication head.
- Compare endpoint path sets, regular non-executable blob modes, exact bytes except permitted completion ticks, and ancestry. Require completed automated checkboxes and unchecked Manual Verification at Completion.
- Keep artifacts in Completion and remove them in a later commit. Require absence at the review head without auditing intermediate trees or contents.
- Support baseline-only implementation, first-pass Audit before final ticks, and incomplete Needs Human preservation without pretending these phases are ready for review.
- Thread explicit `--artifact-baseline` and `--artifact-completion` full-SHA inputs through relevant inspection, startup, and handoff commands and generated continuation instructions.
- Update paired Propose, Implement, Audit, and Watchdog guidance, instruction rendering, and lifecycle documentation for this contract only.
- Prove behavior through the existing public CLI seam and measure representative-history CLI cost without a fragile timing gate.

## Out of Scope

- Slice #2: removing Target Snapshots, target containment, or automatic Synchronization Rework.
- Slice #3: the `.watchdog` Review Checkpoint, review counting, or repeat-review policy.
- Slice #4: removing handoff journals or redesigning transition reconciliation and cleanup ordering.
- Slice #5: candidate filtering, ownership discovery, claim refresh, or deferring Git/artifact work out of startup packets.
- Global slug-ownership audits, intermediate content audits, historical lifecycle reconstruction, target-branch scanning for marker discovery, or a new Git abstraction/test-only seam.
- Automatic markerless fallback, history rewriting, a new Adoption command, persisted endpoint override records, or overrides for new proposal publication.
- Changing agent judgment, Full Gate ownership, opaque Result Documents, the existing opaque Work Item/Submission identities, repository binding, or human-only merge authority.

## Definition of Done

- [ ] D1: A new slice publishes only with its complete, uniquely marked Artifact Baseline at the pushed head, without needing an issue identity first. Covered by B1.
- [ ] D2: Inspection scopes exact marker subjects to the slice and selected reachable history and reports missing or ambiguous required evidence without guessing or extra global backend searches. Covered by B2 and B3.
- [ ] D3: Endpoint validation enforces identical paths, mode `100644`, object type `blob`, exact content except permitted ticks, and phase-appropriate automated/manual checkbox rules. Covered by B4 and B5.
- [ ] D4: Review requires Baseline to be an ancestor of or equal to Completion, Completion before later retirement, and ledger absence at the review head; restored intermediate edits do not invalidate valid endpoints. Covered by B6 and B7.
- [ ] D5: Inspection and existing startup support baseline-only progress; implementation submission requires valid retired completion while Needs Human can preserve incomplete work. Covered by B8, B9, and B10.
- [ ] D6: Watchdog startup and verdict submission apply the same endpoint contract at fixed review/final heads and retain historical contract access and independent verification guidance. Covered by B11.
- [ ] D7: Explicit endpoint SHAs support markerless evidence only on relevant invocations, apply the same checks, reject ambiguous markers, and survive generated resume/inspection/handoff instructions without persisted Adoption state. Covered by B12 and B13.
- [ ] D8: Representative-history public CLI checks prove there are no content loads or Git processes per history commit; a runnable benchmark reports measurements rather than gating elapsed time. Covered by B14.
- [ ] D9: Paired skills, packet guidance, and capability documentation agree with these scenarios, explicitly supersede older ADR mandates only for this scope, and leave other corrections planned. Covered by the final documentation task and runnable CLI instruction/resource checks.

## Manual verification

None. All verification, including CLI fixtures, instruction checks, and the benchmark, is runnable by an Agent Worker.
