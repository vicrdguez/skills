# Decouple Setup and Proposal Publication Plan

## Approach
Apply ADR 0002's backend-independence correction through complete Setup and Proposal command paths. Use the merged implementation of #3 as the behavioral reference, not the partial code present when these artifacts were published. #8 and all its predecessors must be Merged before implementation begins.

## Implementation decisions
- The integration selects and binds one Backend before entering Workflow Mechanics. Bind repository coordinates once rather than threading GitHub owner/name through engine operations.
- Keep an existing semantic port, reshaping it as needed; add neither a generic provider registry nor a second production Backend. Avoid a giant interface collecting unrelated operations merely to share a factory.
- Opaque identity values have equality and transport semantics, not native rendering methods. Preserve existing external CLI inputs, output shapes, packet representations, and persisted records through integration-level conversion. No generated replacement IDs or record migration.
- Backend observations provide facts needed for ordering; the engine owns comparison and selection. Preserve the existing numeric tie-break behavior for GitHub identities without parsing opaque IDs in the engine.
- GitHub label names, HTTP details, native IDs, and provider presentation belong in the adapter/integration. Canonical workflow validation and publication recovery decisions belong in the engine. Transport-level observation and idempotent write reconciliation remain adapter responsibilities.
- The concrete Repository module supplies Git evidence. Reuse the merged remote-selection contract; passing the selected remote explicitly is an internal correction, not a new remote-selection policy or new CLI flag requirement.
- Shared signature changes may require mechanical migration of implementation/review callers now. Keep those paths working without speculative compatibility layers; their substantive ownership correction remains in later slices.
- This is a structural correction with preserved behavior. Keep existing regression coverage, use focused red-green checks where the current seam cannot express the contract, and avoid inventing new product behavior just to obtain a failing test.

### Module shapes & seams

#### [MODIFIED] CLI integration and Setup
**Interface:** existing Setup and Proposal commands, invocation choices, outcomes, and owned-file results. Provider construction yields a repository-bound Backend plus the selected local Git context.

**Test strategy:** existing command/Setup tests with temporary real repositories and the existing in-memory Backend. B1-B2 observe final results, not factory calls or concrete type names. Retain existing ambiguity and file-safety checks.

#### [MODIFIED] Workflow publication and Backend
**Interface:** semantic publication request/outcome using bound Backend operations and opaque record references; no provider coordinates required by Workflow callers.

**Test strategy:** B3-B6 use the existing CLI application seam and in-memory Backend with literal expected outcomes. Reuse publication interruption fixtures. The existing GitHub HTTP seam verifies native projection, identity conversion, and adopted records. This is not a new backend conformance suite.

#### [MODIFIED] Repository and Catalog consumers
**Interface:** retain concrete Git evidence operations and existing packet rendering. Adjust inputs only as required by repository binding and identity presentation.

**Test strategy:** retain existing Repository and Catalog tests; extend only where their inputs change. Architecture ownership is verified by Audit tracing production callers, not source-text assertions or mocks of private methods.

## Sequence
1. Reconcile the merged command paths and tests with this slice's scope.
2. Correct binding and identities through Setup and single-slice publication, preserving all affected callers.
3. Carry coordinated publication, recovery, and Git preflight through the same seam.
4. Audit ownership and compatibility, then update relevant capability documentation.
