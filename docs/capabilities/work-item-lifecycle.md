# Work Item lifecycle

`skl` coordinates each Work Item through the repository-owned Workflow while Agent Workers retain reasoning, code changes, project checks, and ordinary Git work.

## Behaviors

- `skl propose cleanup` removes clean conventional worktrees and matching local branches only for backend-confirmed Merged Work Items; dirty, unexpected, remote, and unrelated Git state is preserved and reported.
- `skl propose publish` preflights every declared slice and Dependency before publication: durable documents must be committed, local and remote slice heads must match and descend from the observed target, and first-parent history must contain one complete Artifact Baseline at the published head.
- Publishes one Work Item for a single slice. Multi-slice Proposals add one Coordination Item, native sub-issue relationships, and explicit Dependencies in blocker-first order.
- Treats supplied parent and child Markdown as opaque transport. Each child receives `ready` only after its body and relationships are durable.
- Repeats publication forward after operational failure by reusing unambiguous records and relationships. Contradictory or ambiguous records enter Needs Human rather than being guessed, deleted, or rolled back.
- `skl implement next` selects the oldest eligible Rework before Ready Work Items, breaking age ties by the Backend's stable ordering fact. It skips Claims and Needs Human, reads back its additive `wip` Claim, and supplies one packet bundling Implement, TDD, Audit, Design, and Domain.
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

## Backend ownership

The command integration binds the Backend and selects the Git remote together, using a GitHub `origin`, the sole GitHub remote, or an explicit `--remote` when needed. Ambiguous selection is refused before mutation. Workflow Mechanics use that explicit remote for Git evidence and opaque Work Item, Submission, and Coordination Item references for Backend operations. The GitHub integration retains native issue numbers, relationship and closing syntax, and supplied opaque Markdown without migrating existing records.

The Workflow Engine owns eligibility, ordering, canonical transitions, Git evidence requirements, and interrupted-operation recovery. The Backend observes normalized durable records and materializes requested effects. Shared Work Item, Submission, and relationship identities are opaque; the integration preserves numeric GitHub CLI and packet representations, renders native references and closing footers, and supplies the existing numeric ordering fact separately.

## Proposal decomposition

- Prefers separate Work Items for behaviors that deliver safe, useful results independently. Independence is assessed after declared Dependencies are Merged, without requiring later Work Items; each delivery remains vertically complete.
- Combines independently useful behaviors only for a concrete reduction in overall implementation or review burden. Shared files or a shared Workflow stage are insufficient reasons.
- Judges slice size by the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together, using agreed requirements and focused repository inspection rather than line counts or exhaustive implementation planning. Different error cases alone do not require separate Work Items.
- Explains those review concerns in the existing breakdown approval, including the reason for combining independently useful behaviors. This remains Agent Worker judgment, not a Workflow Engine score or gate.
- Revisits the existing approval loop before publication when artifact elaboration materially changes proposed boundaries or Dependencies. Ordinary elaboration does not require renewed approval; frozen Work Items cannot be split during implementation under this guidance.
- Adds no separate skill, artifact, metric, or approval stage. This guidance is limited to Proposal decomposition, separate from the Consumer Repository simplicity standard.

## Planned extension: Independent queue draining

- Supports the same implementation and Watchdog queue workflow in Pi, OpenCode, and Codex, replacing the Pi-specific queue integration in PR #19 while preserving its shared Workflow mechanics.
- Retains Pi's existing subagent extension for worker and Audit delegation. OpenCode and Codex use native delegation; replacing Pi's execution mechanism is a separate change.
- Keeps implementation-loop and watchdog-loop entry points separate from the single-item skills, with shared workflow instructions and thin harness prompt shortcuts or commands where supported.
- Installs managed worker definitions with explicit role-specific defaults: Astra with low reasoning for implementation, and Astra with high reasoning for both Audit axes and Watchdog. It does not change the supervisor model, unrelated user-wide permissions, or execution limits. Missing delegation prerequisites require actionable guidance rather than running worker tasks in the supervisor context. Harness smoke tests are not part of this extension.
- Leaves role-specific model and supported reasoning settings in native harness configuration rather than Workflow Engine flags. Consumer-repository role configuration overrides installed defaults and remains outside installer ownership. Existing Pi runner model choices need not be retained.
- Documents native model-only overrides for Pi and OpenCode, and a complete thin project-level role definition for Codex, whose project role replaces the global definition. Codex overrides include startup instructions but do not duplicate shared Workflow policy; no cross-harness model configuration layer is added.
- Recommends, but does not enforce, different implementation and reviewer models to reduce correlated review blind spots. The shipped same-model profile does not provide model diversity or claim measured superiority; each role remains configurable.
- Runs independent implementation and Watchdog supervisors, each dispatching one fresh Agent Worker at a time. The Workflow Engine owns waiting, selection, Claims, and authoritative continuation decisions; workers retrieve their Instruction Packets using the startup command returned to the supervisor.
- Keeps `next` immediate by default. Bare `--wait` waits up to 15 minutes; an explicit duration overrides that maximum. Waiting checks for claimable work immediately and polls at a configurable interval, defaulting to 30 seconds, until work is claimed or the idle window expires.
- Starts a new idle window for each request. An idle timeout describes only that queue's lack of claimable work during the window, not global completion.
- Drains without an attempt cap, preserving the per-change review-bounce allowance and human merge boundary. Worker prose alone never establishes a completed handoff.
- Leaves handoff validation and continuation decisions to the Workflow Engine, not the supervisor. An interrupted or failed worker may be followed by more work only when the engine confirms its handoff completed; otherwise the supervisor stops and reports explicit recovery instructions without automatically replacing the worker.
- Supplies a worker startup command and a supervisor continuation command for each dispatch. Continuation verifies that dispatch's durable handoff before selecting or waiting for more work, including when the other lane has already advanced the Work Item.
- Preserves Claims and partial work on dispatch or handoff failure. The other lane remains independent.
- Stops after interrupted or ambiguous selection without blindly issuing another `next`, expiring Claims, or automatically recovering them. Recovery remains explicit and may require manual inspection when dispatch identity was not received.
- Reports forge-query failures as operational errors rather than empty-queue observations, retaining existing backend retries without adding a waiting-loop retry policy.
- Retires installation of the Pi-specific queue assets without adding automatic cleanup or migration logic. Previously installed copies are removed manually.
- Retains the single-operator assumption: at most one supervisor per lane per project. Shared status views, cross-supervisor coordination, and competing same-lane consumers are outside this extension.

## Out of scope

- Editing product code, performing ordinary commits or pushes, or running Consumer Repository Full Gates.
- Parsing or judging agent-authored Result Document Markdown.
- Atomic multi-worker Claims in V1.
- A complete Local Backend or automatic Agent Harness execution in V1.
