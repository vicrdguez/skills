# Use a backend-neutral state model without a CLI database

The Workflow Engine owns canonical states, eligibility, ordering, invariants, and transitions rather than treating GitHub labels as the domain model. One Work Item keeps the same identity from publication through merge while backend issues and pull requests are projections or attachments. A backend exposes normalized records and conditional mutation primitives, while its labels and fields are projections of engine state. Durable truth is reconstructed from the active backend together with Git branches and active change artifacts; the CLI keeps no private authoritative database. This preserves one set of mechanics for the initial GitHub backend and a later complete local backend without duplicating workflow policy.

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
