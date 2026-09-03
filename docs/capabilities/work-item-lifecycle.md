# Work Item lifecycle

`skl` coordinates each Work Item through the repository-owned Workflow while Agent Workers retain reasoning, code changes, project checks, and ordinary Git work.

## Behaviors

- `skl propose cleanup` removes clean conventional worktrees and matching local branches only for backend-confirmed Merged Work Items; dirty, unexpected, remote, and unrelated Git state is preserved and reported.
- `skl propose publish` preflights every declared slice and Dependency before publication: durable documents must be committed, local and remote slice heads must match and descend from the observed target, and first-parent history must contain one complete Artifact Baseline at the published head.
- Publishes one Work Item for a single slice. Multi-slice Proposals add one Coordination Item, native sub-issue relationships, and explicit Dependencies in blocker-first order.
- Treats supplied parent and child Markdown as opaque transport. Each child receives `ready` only after its body and relationships are durable.
- Repeats publication forward after operational failure by reusing unambiguous records and relationships. Contradictory or ambiguous records enter Needs Human rather than being guessed, deleted, or rolled back.
- Selects eligible Work Items in the established queue order, applies the best-effort `wip` Claim, and supplies a concrete Instruction Packet.
- Requires every Dependency to be Merged before its dependent Work Item becomes eligible.
- Pins a Target Snapshot for new implementation and Synchronization Rework without chasing later target movement.
- Keeps Audit inside implementation and Watchdog Review in a fresh Worker Session.
- Validates Implementation Ledger history and retirement from Git before allowing Awaiting Review, without parsing agent-authored handoff prose.
- Carries agent-decided outcomes through semantic commands, retaining Claims and state for repairable deterministic refusals.
- Allows one finding-driven review bounce; a second failed review enters Needs Human, while Synchronization Rework does not consume that allowance.
- Leaves merge authorization and execution to the Merge Authority and arranges for GitHub to close the source issue when the accepted Submission is merged.
- Recovers partial backend transitions by observing current state and continuing forward rather than rolling back completed work.

## Out of scope

- Editing product code, performing ordinary commits or pushes, or running Consumer Repository Full Gates.
- Parsing or judging agent-authored Result Document Markdown.
- Atomic multi-worker Claims in V1.
- A complete Local Backend or automatic Agent Harness execution in V1.
