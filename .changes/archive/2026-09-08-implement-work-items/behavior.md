# Implement Work Items Behavior

## Feature: Implementation Work Start

### Rule: Selection is deterministic and Claims are resumable

#### Scenario: Claim the oldest eligible implementation work
- Given unclaimed Rework and Ready Work Items of different ages
- And some Ready Work Items have Dependencies that are not Merged
- When implementation work is requested
- Then the oldest eligible Rework Work Item is selected before every Ready Work Item
- Or the oldest eligible Ready Work Item is selected when no eligible Rework exists
- And `wip` is added without removing its lifecycle projection
- And blocked, already claimed, and Needs Human Work Items remain untouched

#### Scenario: Report no eligible implementation work
- Given every candidate is blocked, claimed, Needs Human, or absent
- When implementation work is requested
- Then the outcome is `no_work`
- And no Work Item is mutated

#### Scenario: Resume an interrupted Claim
- Given a Work Item already carries `wip` and has an unambiguous stable identity or conventional worktree
- When that Work Item is explicitly resumed
- Then the same Work Item and existing state are returned
- And no second Work Item is claimed

### Rule: Work Start specializes instructions but leaves implementation to the agent

#### Scenario: Start a new implementation from a Target Snapshot
- Given an eligible Ready Work Item and the current target commit
- When it is claimed for first-pass implementation
- Then the Instruction Packet pins that commit as the Target Snapshot
- And it instructs the Agent Worker to merge the snapshot with ordinary Git before coding
- And Implement, TDD, Audit, Design, and Domain appear exactly once in the packet
- And `skl` performs no target merge or project validation gate

#### Scenario: Start finding-driven Rework without target synchronization
- Given an eligible Rework Submission with a previous reviewed head and published findings
- When it is claimed for implementation
- Then the packet supplies the existing Submission, findings, human comments, and previous reviewed head
- And it instructs the Agent Worker to review only the rework diff
- And it does not require a newer Target Snapshot merge

## Feature: Implementation handoff

### Rule: Awaiting Review requires fixed Git evidence

#### Scenario: Submit a completed first implementation
- Given the Agent Worker has pushed a branch containing the pinned Target Snapshot and completed code
- And the Implementation Ledger follows one `absent* → present+ → absent*` first-parent lifecycle
- And Artifact Completion contains only permitted ticks with Manual Verification unchecked
- And its child deletion commit removes the ledger and every later head keeps it absent
- And the agent supplies an opaque PR body containing its Audit ledger
- When review submission is requested
- Then `skl` creates or reuses one PR at the pushed fixed head
- And it appends `Closes #<source-issue>` to the supplied body
- And the PR projects Awaiting Review without `wip` or `rework`
- And the source issue remains open without `ready` or `wip`

#### Scenario: Refuse an invalid implementation handoff
- Given the target snapshot is absent, the local and remote heads differ, the ledger history is invalid, or the observed Workflow State contradicts submission
- When review submission is requested
- Then the outcome is `fix_required` with exact failing invariants and repair instructions
- And the Claim and current Workflow State are retained
- And no partial review handoff is reported as complete

#### Scenario: Resubmit completed Rework
- Given the Agent Worker has pushed fixes for an existing Rework Submission
- And retired artifacts remain absent
- And its opaque body describes the current Audit and rework dispositions
- When review submission is requested
- Then the same Submission is updated at the pushed head
- And it projects Awaiting Review without `rework` or `wip`
- And no new PR or Implementation Ledger is created

### Rule: Human decisions preserve available work

#### Scenario: Pause before implementation exists
- Given a claimed Ready Work Item cannot proceed without a permitted human decision
- And no implementation work needs a Submission
- When the agent requests a Needs Human handoff with an opaque decision body
- Then the decision is published on the source issue
- And the issue projects Needs Human without `ready` or `wip`
- And incomplete artifacts may remain on its branch

#### Scenario: Pause while preserving implementation work
- Given completed work cannot proceed without a permitted human decision
- And the branch has been pushed
- When the agent requests a Needs Human handoff with an opaque decision and Submission body
- Then one draft Submission preserves the branch head
- And issue and Submission projections prevent ordinary claiming
- And incomplete artifacts may remain until implementation can become review-ready

### Rule: Opaque Result Documents are lifecycle transport

#### Scenario: Publish agent prose without validating it
- Given an Agent Worker writes a template-guided Markdown Result Document in its private temporary directory
- When a semantic Implement command requests a valid transition
- Then `skl` publishes the Markdown without parsing or judging it
- And the semantic command determines the requested transition
- And successful publication removes the temporary operation directory
- But a failed handoff retains it for repair
