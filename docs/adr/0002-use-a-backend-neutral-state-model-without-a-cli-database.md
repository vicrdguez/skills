# Use a backend-neutral state model without a CLI database

The Workflow Engine owns canonical states, eligibility, ordering, invariants, and transitions rather than treating GitHub labels as the domain model. One Work Item keeps the same identity from publication through merge while backend issues and pull requests are projections or attachments. A backend exposes normalized records and conditional mutation primitives, while its labels and fields are projections of engine state. Durable truth is reconstructed from the active backend together with Git branches and active change artifacts; the CLI keeps no private authoritative database. This preserves one set of mechanics for the initial GitHub backend and a later complete local backend without duplicating workflow policy.

## Backend independence correction

Correct provider assumptions across all existing Workflow operations as a separate, behavior-preserving change, rather than limiting the correction to implementation operations. Its scope includes operations introduced by intervening slices before the correction lands. Workflow Mechanics must use a backend-independent seam; this correction does not add another Backend or require a complete Local Backend to prove that seam. The aim is to make later extension local to the Backend integration without changing existing observable behavior.

The correction follows merge of every Work Item under Coordination Item #3, so it covers the complete set of Workflow Mechanics delivered by that proposal rather than an intermediate subset.

The Workflow Engine receives a repository-bound Backend selected outside the engine. Provider discovery, credentials, endpoints, and native representations belong to the Backend integration. Work Item and Submission identities cross the seam as small opaque values, not interfaces with native rendering behavior; existing external identifiers remain unchanged. The engine does not interpret those values as issue numbers or provider-specific references.

The Backend translates its projections into semantic records and materializes requested mutations; it does not own eligibility or other workflow decisions. Canonical state meanings, ordering, invariants, and permitted transitions belong exclusively to the Workflow Engine. For example, Awaiting Review is canonical state, while GitHub's `review` label is only its Workflow Projection. Provider-specific presentation and linking syntax remain outside Workflow Mechanics.

Git evidence remains a separate concern supplied by the concrete Repository module, not hidden behind the Backend. The Workflow Engine combines commit, tree, and ancestry evidence with Backend observations to enforce its mechanics. Provider discovery stays outside the engine, and Git remote selection is supplied explicitly rather than assumed to be `origin`; this correction does not require another Repository abstraction.

## Consequences

- Backend and Git state can diverge during multi-system operations, so commands must detect and recover partial transitions.
- Each multi-system transition is a durable, idempotent operation that resumes forward from observed state instead of attempting rollback.
- Multi-slice proposal publication stays sequential: preflight the declared graph, make each child's body and relationships durable, and add `ready` last. A retry reuses unambiguous existing records and continues; already-finalized children remain valid and eligible. V1 adds no rollback, hidden batch state, or transaction protocol.
- Manual backend edits are external state changes that the engine must validate before acting.
- Semantically equivalent external edits are reconciled; ambiguous or contradictory drift stops for human resolution rather than being overwritten.
- GitHub's existing labels may remain as compatibility mappings even when canonical state names differ.
- V1 deliberately retains the existing best-effort `wip` Claim under a single-operator assumption. Atomic Claim Tokens are deferred until real concurrent claiming justifies Git-ref coordination or another backend primitive.
- Interrupted V1 work resumes by stable Work Item identity from its worktree or an explicit resume command; ordinary queue selection skips `wip` projections.
- Merge authorization and execution stay outside the engine. For GitHub, the Submission carries an issue-closing reference so the human merge closes its issue immediately; the engine later derives Merged state from the backend when needed.
- Network reads and documented idempotent calls may retry with bounds; an ambiguous mutation is always observed and reconciled before any retry.
- Normal implementation pins a Target Snapshot at Work Start, and submission must contain that commit. Later target movement does not move this fixed point; only Synchronization Rework pins a newer snapshot, while ordinary finding-driven rework retains the existing one.

## Planned correction: Candidate-first workflow

The [candidate-first correction](../capabilities/work-item-lifecycle.md#planned-correction-candidate-first-workflow) supersedes the target-obligation and journal-style recovery policies above without changing backend independence or human merge authority. It selects from open queue/Claim projections and candidate-local Dependencies, with explicit issue/PR ownership rather than global name resolution; execution resumes from Git, artifacts, and visible feedback without hidden issue metadata. The destination is `main`, and merge conflicts remain human integration concerns rather than automatic Synchronization Rework. A CLI-owned, worktree-private Review Checkpoint governs the retained worktree's review budget, deliberately accepting a fresh count after checkpoint loss rather than introducing durable lifecycle storage. Retries reconcile observable effects, preserve later-stage Claims, and stop when evidence is ambiguous instead of promising reconstruction of every historical operation.
