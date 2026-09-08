# Decouple Review and Completion Operations Behavior

## Feature: Backend-independent Watchdog operations

#### Scenario: B1 Preserve review selection and fixed evidence
- Given Awaiting Review Work Items with opaque identities, ages, Claims, and Submission heads
- When the next review is requested or an existing review Claim is explicitly resumed
- Then the engine preserves the established oldest-eligible ordering or resumes the same Claim
- And the packet pins the existing review head and supplies historical artifacts and opaque review context
- And an ineligible queue returns the established no-work outcome without mutation
- And native references remain usable without the engine interpreting their representation

#### Scenario: B2 Preserve semantic verdict outcomes
- Given a valid fixed-head review and the worker's opaque result bodies
- When the worker requests pass, a first finding-driven failure, or a second finding-driven failure
- Then the engine permits Ready for Merge, Rework, or Needs Human respectively under the established rules
- And the adapter publishes the existing native findings, labels, Manual Verification body, and source-issue closing reference as appropriate
- And Claims are released only after the durable handoff
- And no CLI operation performs Audit, Watchdog judgment, a project gate, or the human merge

#### Scenario: B3 Preserve human pause and requeue observations
- Given a Work Item paused for a human with an existing resume state and opaque findings and human comments
- When status or a supported resume operation observes an explicit valid requeue to Rework or Awaiting Review
- Then the engine uses the normalized state to preserve the established resume semantics
- And the worker receives the unchanged human comments and relevant source facts
- And prose is not interpreted as a state mutation or finding disposition by the engine
- And contradictory observations retain the existing refusal or human-decision outcome

#### Scenario: B4 Preserve Synchronization Rework obligations
- Given an accepted Submission no longer merges cleanly into its target
- When the Workflow observes that condition through the established operation
- Then the engine requests Synchronization Rework with a fresh Target Snapshot
- And the finding-driven bounce allowance is unchanged
- And the worker remains responsible for merging and validating the code

## Feature: Backend-independent completion and cleanup

#### Scenario: B5 Derive completion from semantic observations
- Given Work Items observed as Ready for Merge, actually Merged, or Superseded by the bound Backend
- When status reconciles the existing lifecycle
- Then Ready for Merge is not treated as Merged
- And only actual Merged blockers satisfy Dependencies
- And a Coordination Item completes only after every child is Merged
- And Superseded work retains the established observation and reference behavior
- And valid state is not rewritten merely to obtain status

#### Scenario: B6 Preserve safe local cleanup
- Given normalized Merged and unmerged Work Items with conventional local branches and worktrees
- And Git evidence includes clean removable state and state protected by the established safety policy
- When cleanup is requested from the permitted location
- Then only the established safe Merged local state is removed
- And unmerged, dirty, ambiguous, or otherwise protected local state is preserved with its existing report
- And remote branches are not deleted
- And native backend reference syntax is not needed to evaluate local Git safety

#### Scenario: B7 Resume interrupted review and completion mutations
- Given a permitted review or completion transition was interrupted after a native write became durable
- When the same semantic operation is retried with its existing identity and fixed evidence
- Then the engine reconciles observed progress and resumes remaining permitted steps
- And existing findings, Submissions, and Coordination Items are not duplicated
- And changed evidence or conflicting state cannot be reported as a completed handoff
- And existing persisted records remain usable without migration
