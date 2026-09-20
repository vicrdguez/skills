# Focused Rework Audit Behavior

## Feature: Rework-specific Audit execution

### Rule: Each Implement execution may invoke Audit at most once

#### Scenario: Audit the fixes from the applicable Watchdog-reviewed commit
- Given a finding-driven Rework execution with one applicable Watchdog review that caused Rework
- And that review identifies its reviewed commit
- When the Implement Execution Skill is rendered
- Then it directs the worker to run Audit exactly once over `<reviewed-commit>...HEAD`
- And it directs the worker to stop rather than guess when the applicable reviewed commit is absent or ambiguous

#### Scenario: Review only consequences of the Rework delta
- Given a finding-driven Rework execution with supplied Watchdog findings
- When its bundled Audit is rendered
- Then the Standards axis may report only violations or smells caused by the Rework delta
- And the Contracts axis checks resolution of the supplied findings, Contract regressions caused by the delta, unnecessary behavior introduced by the fixes, and regression coverage at accepted seams
- But neither axis reopens findings against unrelated unchanged code or whole-change omissions
- And Watchdog findings remain evidence and resolution targets rather than frozen Contract Items

#### Scenario: Keep deterministic checks at their responsible stages
- Given a finding-driven Rework execution
- When its Implement and bundled Audit instructions are rendered
- Then Rework Audit owns one Full Gate run
- And Audit does not repeat artifact endpoint or retirement inspection
- And Implement retains Inspect before editing and after all edits

#### Scenario: Finish Audit findings without entering an Audit loop
- Given a finding-driven Rework execution has completed its one Audit
- When the worker applies an Audit Finding disposition that changes code
- Then it runs the affected checks and a final Full Gate before handoff
- But it does not invoke Audit again in that execution

## Feature: Cumulative Audit Finding ledger

#### Scenario: Identify findings from an initial Audit
- Given no `F<n>` Audit Finding exists in the Submission
- When an initial Audit produces findings
- Then its new findings receive monotonic identities beginning with `F1`

#### Scenario: Continue Audit Finding identities during Rework
- Given the Submission contains historical Audit Finding identities
- When Rework Audit produces additional findings
- Then every new finding continues after the greatest existing `F<n>` or begins with `F1` when none exists
- And historical identifiers in every format remain unchanged
- And the existing cumulative Audit ledger advances to the newly audited head
- But no round-specific provenance section is added

#### Scenario: Add no finding entries when focused Audit is clean
- Given Rework Audit produces no findings
- When the Submission Result Document is updated
- Then no synthetic Audit Finding is added
- And the cumulative Audit ledger advances to the newly audited head

## Feature: Existing Audit procedures remain stable

#### Scenario: Preserve non-Rework Audit behavior
- Given an initial or resumed implementation execution or a standalone Audit invocation
- When its instructions are rendered
- Then its existing fixed point, review scope, deterministic checks, and two-axis behavior remain unchanged apart from the shared `F<n>` identity rule
