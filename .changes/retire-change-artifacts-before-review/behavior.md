# Retire change artifacts before review Behavior

## Feature: Ephemeral change artifacts

### Rule: Artifacts are an implementation ledger, not durable documentation

#### Scenario: Track first-pass implementation in the artifacts
- Given a slice branch containing frozen artifacts at its recorded baseline
- When the implementor completes the first implementation pass
- Then the artifacts remain available while work is in progress
- And only existing agent-verifiable `[ ]` boxes become `[x]`
- And manual-verification boxes remain unchecked

#### Scenario: Preserve an incomplete human-decision handoff
- Given implementation cannot complete without a human decision
- When the implementor opens a draft PR labeled `needs-human`
- Then the incomplete artifacts may remain on the slice branch
- And artifact retirement waits until the change is ready for review

#### Scenario: Retire completed artifacts before review
- Given every agent-verifiable checkbox is ticked and committed
- When the implementor submits the change for review
- Then the PR body records the artifact baseline commit
- And the PR body records the artifact completion commit
- And `.changes/<slug>/` is deleted before the PR enters `review`
- And the PR's final diff contains no change artifacts

#### Scenario: Verify a retired contract
- Given a review PR with recorded artifact baseline and completion commits
- When watchdog verifies the change
- Then it reads requirements from the baseline commit
- And it confirms baseline-to-completion artifact changes are only permitted checkbox ticks
- And it confirms completion-to-HEAD removes the artifacts without replacing them
- And it independently proves the frozen requirements against the final implementation and tests

#### Scenario: Rework after artifact retirement
- Given watchdog bounced a review PR after its artifacts were retired
- When the implementor performs rework
- Then it reads the frozen contract and completed ledger from their recorded commits
- And it tracks rework through the watchdog finding ledger
- And `.changes/<slug>/` remains absent

#### Scenario: Land and clean up without an archive
- Given watchdog passes the review
- When it hands the PR to the human merge gate
- Then it copies manual verification from the frozen artifact snapshot into the PR body
- And it marks the PR `done` without creating `.changes/archive/`
- When the PR is merged
- Then its merged GitHub state permits removal of the worktree and local and remote feature branches
