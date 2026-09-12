# Decouple Review and Completion Operations

## Why
Backend independence is incomplete while review, status, completion, or cleanup still infer GitHub details or leave canonical workflow decisions in its adapter. This final slice completes the correction across all Workflow Mechanics delivered under #3.

## What
Carry review and terminal lifecycle operations through the repository-bound Backend and opaque semantic records established by the preceding slices, preserving all existing observable behavior and durable recovery state.

## Scope
- Depend on decouple-implementation-operations being Merged, transitively after decouple-setup-and-publication and every Work Item under #3.
- Cover Watchdog selection/resume, verdict publication, Needs Human/requeue, status, Synchronization Rework, human-merge observation, Dependency release, Coordination Item completion, supersession observation, and safe cleanup.
- Keep canonical state, ordering, bounce allowance, transition permission, completion eligibility, and recovery decisions exclusively in the Workflow Engine.
- Keep GitHub observations, labels, comments, native finding anchors/references, issue-closing syntax, and authentication in integration code.
- Preserve raw human comments and agent-authored findings without introducing engine interpretation of prose or human directives.
- Keep fixed reviewed heads, retired-ledger inspection, merge-conflict evidence, and local cleanup safety in concrete Git operations separate from the bound Backend.
- Finish affected packet/native presentation adjustments and trace every Workflow command for residual provider coupling, including operations added by #3 before this correction began.

## Out of Scope
- New review judgments, changed bounce limits, changed merge authority, automated merges, or different cleanup policy.
- A new Backend, provider framework, identity migration, workflow configuration, or Repository abstraction.
- Product changes unrelated to the corrected seam, agent gate execution inside the CLI, and a standalone portability test suite.

## Definition of Done
- [x] B1 Review selection and resume preserve engine-owned order, Claim semantics, and fixed review evidence.
- [x] B2 Pass, first failure, and subsequent failure preserve their established canonical outcomes and native publication effects.
- [x] B3 Needs Human observation and explicit requeue preserve resume semantics without interpreting opaque findings or directives.
- [x] B4 Synchronization Rework retains its fresh Target Snapshot and remains separate from the finding-driven bounce allowance.
- [x] B5 Status observes actual human merge, Dependency satisfaction, Coordination Item completion, and supersession with existing semantics.
- [x] B6 Cleanup preserves the established local-state safety policy using normalized merge observations and independent Git evidence.
- [x] B7 Interrupted review/completion operations reconcile forward from existing records without duplicate findings or premature terminal state.
- [x] No Workflow Mechanics delivered under #3 depend on a provider's repository syntax, labels, native identifiers, presentation, or adapter-owned workflow decisions; Audit traces all command paths and shared callers.
- [x] Existing lifecycle, GitHub adapter, Repository, and Catalog regression checks remain green, and capability documentation accurately describes ownership without changing advertised behavior.

## Manual verification
None.
