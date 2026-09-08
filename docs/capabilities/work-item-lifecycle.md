# Work Item lifecycle

`skl` coordinates each Work Item through the repository-owned Workflow while Agent Workers retain reasoning, code changes, project checks, and ordinary Git work.

## Behaviors

- `skl propose cleanup` removes clean conventional worktrees and matching local branches only for backend-confirmed Merged Work Items; dirty, unexpected, remote, and unrelated Git state is preserved and reported.
- `skl propose publish` preflights every declared slice and Dependency before publication: durable documents must be committed, local and remote slice heads must match and descend from the observed target, and first-parent history must contain one complete Artifact Baseline at the published head.
- Publishes one Work Item for a single slice. Multi-slice Proposals add one Coordination Item, native sub-issue relationships, and explicit Dependencies in blocker-first order.
- Treats supplied parent and child Markdown as opaque transport. Each child receives `ready` only after its body and relationships are durable.
- Repeats publication forward after operational failure by reusing unambiguous records and relationships. Contradictory or ambiguous records enter Needs Human rather than being guessed, deleted, or rolled back.
- `skl implement next` selects the oldest eligible Rework before Ready Work Items, breaking age ties by stable identity. It skips Claims and Needs Human, reads back its additive `wip` Claim, and supplies one packet bundling Implement, TDD, Audit, Design, and Domain.
- `skl implement resume --item <number>` resumes only that Claim; from its conventional worktree, `skl implement resume` resolves the unambiguous identity without selecting other work.
- Requires every Dependency to be Merged before its dependent Work Item becomes eligible.
- Pins a Target Snapshot for new implementation and Synchronization Rework without chasing later target movement.
- Keeps Audit inside implementation and Watchdog Review in a fresh Worker Session.
- `skl implement inspect --item <number>` resolves fixed Git evidence and the ledger's first-parent `absent* -> present+ -> absent*` lifecycle. Every present commit preserves paths and prose except monotonic non-manual completion ticks; merges preserve the first-parent ledger tree.
- `skl implement submit --item <number> --body <absolute-file>` requires a pushed fixed head containing the Target Snapshot and completed ledger retirement. It creates or updates one Awaiting Review Submission, appends `Closes #<issue>`, and leaves the source issue open. Finding-driven Rework updates the same Submission without a new target merge or ledger.
- `skl implement needs-human` publishes the agent's permitted decision on the issue before code exists, or preserves pushed work in one draft Submission when supplied a body. It retains the exact state to resume and permits incomplete artifacts during the pause.
- Result Documents live in engine-created private temporary directories. Their bytes remain opaque; successful publication removes the directory, while failure retains it for repair.
- Carries agent-decided outcomes through semantic commands, retaining Claims and state for repairable deterministic refusals.
- Allows one finding-driven review bounce; a second failed review enters Needs Human, while Synchronization Rework does not consume that allowance.
- `skl watchdog next` claims the oldest unclaimed Awaiting Review Submission by creation time and Work Item identity, preserving `review`; `resume --item <number>` retains the fixed reviewed head rather than selecting another item.
- Supplies the historical Artifact Baseline and Completion files, opaque Audit-bearing PR body, prior summaries and anchored findings, and raw human comments in one concrete Watchdog packet. The worker owns finding identities, human directive interpretation, test-strength review, the Full Gate, and independent artifact verification; neither the CLI nor Watchdog reruns Audit.
- `skl watchdog submit` accepts a semantic pass, rework, or Needs Human verdict independently of opaque summary and inline body files. Structured anchors carry source path, line, side, and fixed commit; ambiguous publication responses are reconciled by exact content and anchor rereads.
- A pass transports the complete agent-authored final PR body with Manual Verification and appends `Closes #N`, projecting Ready for Merge through `done` without closing the source issue. Permitted Debt Marker comments require a pushed final head and worker-owned Post-Marker Check, not CLI source parsing.
- A conflicting pass pins the current target for Synchronization Rework, represented by `rework` plus `sync`; implementation removes `sync` when it hands back to review. Label history distinguishes synchronization from completed finding-driven bounces.
- Leaves merge authorization and execution to the Merge Authority and arranges for GitHub to close the source issue when the accepted Submission is merged.
- `skl status` reports canonical states, derives Merged from backend merge evidence, and closes a Coordination Item only after all children are Merged. Superseded Work Items retain their lightweight branch references. Valid state and contradictory projections are not overwritten; only unambiguous partial handoffs reconcile forward.
- Needs Human retains its resume state and existing Submission. Explicit human relabeling chooses Rework or Awaiting Review; comments alone do not change eligibility.
- Recovers partial backend transitions by observing current state and continuing forward rather than rolling back completed work.

## Planned extension after issue #3: Proposal decomposition

- Prefers separate Work Items for behaviors that deliver safe, useful results independently. Independence is assessed after declared Dependencies are Merged, without requiring later Work Items; each delivery remains vertically complete.
- Combines independently useful behaviors only for a concrete reduction in overall implementation or review burden. Shared files or a shared Workflow stage are insufficient reasons.
- Judges slice size by the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together, using agreed requirements and focused repository inspection rather than line counts or exhaustive implementation planning. Different error cases alone do not require separate Work Items.
- Explains those review concerns in the existing breakdown approval, including the reason for combining independently useful behaviors. This remains Agent Worker judgment, not a Workflow Engine score or gate.
- Revisits the existing approval loop before publication when artifact elaboration materially changes proposed boundaries or Dependencies. Ordinary elaboration does not require renewed approval; frozen Work Items cannot be split during implementation under this guidance.
- Adds no separate skill, artifact, metric, or approval stage. This extension is limited to Proposal decomposition, separate from the Consumer Repository simplicity standard.

## Out of scope

- Editing product code, performing ordinary commits or pushes, or running Consumer Repository Full Gates.
- Parsing or judging agent-authored Result Document Markdown.
- Atomic multi-worker Claims in V1.
- A complete Local Backend or automatic Agent Harness execution in V1.
