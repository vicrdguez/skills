# Decouple Implementation Operations Behavior

## Feature: Backend-independent implementation Work Start

#### Scenario: B1 Preserve implementation eligibility and deterministic order
- Given normalized Rework and Ready Work Items with opaque identities, ages, Claims, and Dependency states
- When the next implementation Work Item is requested
- Then the engine selects the oldest eligible Rework before any eligible Ready Work Item using the established tie-break order
- And Ready Dependencies must be Merged, not merely Ready for Merge or closed
- And blocked, claimed, and Needs Human items remain untouched
- And an entirely ineligible queue returns no_work without mutation

#### Scenario: B2 Resume the same interrupted Claim
- Given a claimed Work Item with an unambiguous explicit identity or conventional worktree
- And its existing Target Snapshot or previous reviewed head is recorded
- When implementation is resumed using either supported invocation
- Then the same Work Item and recorded obligations are returned
- And no other Work Item is claimed
- And existing native identifiers and persisted metadata need no migration

#### Scenario: B3 Preserve first-pass and Rework packet semantics
- Given a new implementation or finding-driven Rework selected through the bound Backend
- When Work Start returns its Instruction Packet
- Then new implementation pins the observed Target Snapshot and instructs the worker to merge it
- And finding-driven Rework supplies its existing Submission and reviewed head without new target synchronization
- And Implement, TDD, Audit, Design, and Domain remain included exactly once
- And existing user-facing native references and commands remain usable
- And the CLI performs no merge or project validation gate

## Feature: Semantic implementation handoffs

#### Scenario: B4 Preserve inspection and handoff refusal from fixed evidence
- Given implementation history available through the concrete Repository and state observed through the bound Backend
- When inspection or review submission is requested
- Then inspection reports the established fixed head and ledger facts
- And submission refuses missing Target Snapshot ancestry, differing local/remote heads, invalid ledger history, or contradictory workflow state with the existing fix_required repairs
- And refusal retains the Claim, lifecycle state, and Result Documents without a completed handoff

#### Scenario: B5 Preserve first-pass and Rework review publication
- Given a completed, retired ledger and pushed fixed head satisfying the existing implementation obligations
- And the worker supplies an opaque Submission body
- When first-pass or Rework submission is requested
- Then exactly one appropriate Submission is created or reused, with Rework updating its existing Submission
- And GitHub publication retains the unchanged body plus the established source-issue closing reference
- And the Submission projects Awaiting Review without Claim or Rework
- And the first-pass source issue remains open without its Ready projection or Claim
- And native record references are rendered outside Workflow Mechanics

#### Scenario: B6 Preserve implementation-stage human handoffs
- Given a claimed implementation requires a permitted human decision
- When the worker supplies an opaque decision and, when code exists, a pushed head and Submission body
- Then a no-code decision is published on the source Work Item without a new Submission
- And existing code is preserved in one draft Submission
- And the existing Needs Human and resume-state semantics are retained
- And incomplete artifacts remain permitted only as established for this handoff
- And successful publication cleans the private Result Document directory while a failed handoff retains it

#### Scenario: B7 Recover an interrupted implementation handoff
- Given a handoff was interrupted after a backend mutation became durable
- When the same semantic handoff is retried using existing persisted records
- Then the engine reconciles observed progress and completes remaining permitted steps without duplicating the Submission
- And the original fixed-head obligation is checked throughout the handoff
- And a changed head or contradictory observation retains the Claim and reports the existing repair outcome
- And a previously completed handoff is recognized without repeating publication
