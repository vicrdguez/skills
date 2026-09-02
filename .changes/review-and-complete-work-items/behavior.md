# Review and complete Work Items Behavior

## Feature: Watchdog Work Start

### Rule: Review begins from one fixed Submission in a fresh context

#### Scenario: Claim the oldest Awaiting Review Submission
- Given multiple unclaimed Awaiting Review Submissions
- When Watchdog work is requested
- Then the oldest Submission by creation time and stable identity is selected
- And `wip` is added without removing `review`
- And the packet pins its current head
- And it supplies historical ledger snapshots, audit content, earlier findings, and later human comments without rerunning Audit

#### Scenario: Report no eligible review work
- Given every Awaiting Review Submission is absent, already claimed, or Needs Human
- When Watchdog work is requested
- Then the outcome is `no_work`
- And no Submission is mutated

#### Scenario: Resume an interrupted Watchdog Claim
- Given an Awaiting Review Submission already carries `wip`
- When its Work Item identity is explicitly resumed
- Then the same fixed Submission is returned
- And no other Submission is claimed

## Feature: Review convergence

### Rule: The semantic verdict controls the transition while prose remains opaque

#### Scenario: Bounce the first failing review
- Given a first Watchdog review at a fixed head produces an explicit rework verdict
- And the agent supplies opaque summary and optional anchored finding bodies
- When the verdict is submitted
- Then the finding content is published without semantic parsing
- And the same Submission projects Rework without `review` or `wip`
- And exactly one completed finding-driven bounce is observable in GitHub history

#### Scenario: Pause the second failing review
- Given a Work Item already completed one finding-driven bounce
- And its repeat Watchdog review produces another rework verdict
- When the verdict is submitted
- Then the current opaque finding ledger is published
- And the Submission projects Needs Human without `review`, `rework`, or `wip`
- And no second automated bounce is issued

#### Scenario: Apply later human direction without interpreting it
- Given a Needs Human Submission has later authorized human comments
- And a human explicitly requeues it to Rework or Awaiting Review
- When the corresponding worker requests work
- Then `skl` supplies the comments verbatim
- And eligibility follows the explicit projection
- And no intent is inferred from prose, reactions, silence, or deleted comments

### Rule: A passing review reaches the human merge boundary

#### Scenario: Mark a verified Submission Ready for Merge
- Given Watchdog returns an explicit pass verdict at the pinned head
- And no semantic merge conflict is reported by GitHub
- And the agent supplies an opaque final PR body containing Manual Verification
- When the verdict is submitted
- Then the final body is published with `Closes #<source-issue>`
- And the Submission projects Ready for Merge through `done`
- And `review` and `wip` are absent
- And the source issue remains open
- And no `.changes/archive/` artifact is created

#### Scenario: Carry a code-local Note as debt on pass
- Given a passing review has a nonblocking code-local Note
- When Watchdog finalizes the Submission
- Then the Agent Worker may add a `DEBT(#<pr>/W<n>)` source comment
- And it runs the preserved formatter or parser check plus `git diff --check`
- And it pushes the final head before submitting the pass verdict
- But `skl` does not parse the marker, rerun Audit, or run the Full Gate

### Rule: Synchronization failure is not a review failure

#### Scenario: Route a reviewed merge conflict to Synchronization Rework
- Given Watchdog returns a pass verdict
- But GitHub reports that the pinned Submission no longer merges into the current target
- When the verdict is submitted
- Then the Work Item projects Synchronization Rework
- And its next Implement packet pins the newer Target Snapshot
- And the finding-driven bounce allowance is unchanged

## Feature: Human completion

### Rule: Merged means the accepted code entered the target branch

#### Scenario: Observe a human merge
- Given a Submission projects Ready for Merge and its source issue is open
- When the Merge Authority merges it in GitHub
- Then GitHub closes the source issue through the PR closing reference
- And the next `skl` observation reports the Work Item as Merged
- And its dependent Work Items become eligible if no other blocker remains

#### Scenario: Complete a multi-slice Coordination Item
- Given every child Work Item of a Coordination Item is Merged
- And the Coordination Item remains open
- When `skl` next observes workflow status
- Then it closes the Coordination Item
- And it reports the Proposal complete

### Rule: Status observes and reconciles without redesigning work

#### Scenario: Report normalized workflow status
- Given GitHub and Git contain Ready, claimed, review, rework, Needs Human, Ready for Merge, Merged, partial, and Superseded records
- When `skl status` is invoked
- Then each unambiguous Work Item is reported in canonical Workflow State
- And semantically equivalent partial transitions are reconciled forward when safe
- And contradictory records report Needs Human rather than being overwritten

## Feature: Pi queue continuation

### Rule: Each queue item uses a fresh Worker Session

#### Scenario: Continue a Pi queue after a verified handoff
- Given a Pi implementation or Watchdog queue adapter launches one fresh worker for one Work Item
- When the worker returns a structured verified terminal handoff for that stage
- Then the adapter may launch the next item in another fresh Worker Session
- But it stops on `no_work`, its item limit, an incomplete claimed handoff, or an ambiguous result
- And implementation and Watchdog queues remain in separate Pi sessions
