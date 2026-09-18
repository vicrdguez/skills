# Tasks - Deliver Tailored Implementation Executions

This ledger uses the current pre-ADR-0005 convention: one behavioral task per
named scenario or outline in `behavior.md`, implemented as one red-green cycle
with a test or table-driven test at the seams pinned in `plan.md`. All boxes
start unchecked. Completion permits only existing non-manual `[ ]` to `[x]`
ticks; do not add, remove, reorder, or rewrite the frozen contract.

## Behavioral

- [x] B1 Startup transport does not repeat the operation. See behavior.md B1; DOD1.
- [x] B2 Procedure selection ignores incidental evidence. See behavior.md B2; DOD2.
- [x] B3 Metadata-only startup binds every already-established reference. See behavior.md B3; DOD2.
- [x] B4 Inspection continues the actual ledger progress. See behavior.md B4; DOD3.
- [x] B5 Inspection violations and stale integrity cannot imply readiness. See behavior.md B5; DOD3.
- [x] B6 The complete bundle preserves the current implementation contract. See behavior.md B6; DOD4.
- [x] B7 Audit recipes follow capabilities rather than harness names. See behavior.md B7; DOD5.
- [x] B8 Already-fetched evidence is complete data, not template source. See behavior.md B8; DOD6.
- [x] B9 Evidence availability has an explicit truthful path. See behavior.md B9; DOD6.
- [x] B10 Deferred resources bind settled facts without premature decisions. See behavior.md B10; DOD7.
- [x] B11 Empty and waiting outcomes do not invent an execution. See behavior.md B11; DOD8.
- [x] B12 Repair outcomes preserve observed Claim certainty. See behavior.md B12; DOD8.
- [x] B13 Operational failures retain error and recovery semantics. See behavior.md B13; DOD8.
- [x] B14 Invalid presentation inputs fail before avoidable effects. See behavior.md B14; DOD5, DOD8.
- [x] B15 Submission outcomes reflect one verified handoff. See behavior.md B15; DOD1, DOD8.
- [x] B16 Human-pause outcomes preserve the actual work. See behavior.md B16; DOD1, DOD8.
- [x] B17 Refused or interrupted handoffs remain observationally recoverable. See behavior.md B17; DOD8.
- [x] B18 Installed Implement activation directly reaches lane next. See behavior.md B18; DOD9.
- [x] B19 Generic Implement retrieval refuses without workflow effects. See behavior.md B19; DOD9.
- [x] B20 Installation disables owned legacy loops without collateral removal. See behavior.md B20; DOD10.
- [x] B21 The Pi runner reports one item in normal Markdown. See behavior.md B21; DOD10.

## Chores

- [x] C1 Confirm `render-deferred-resources` and transitive #40 are Merged, include required prerequisite commits in this early-created branch using ordinary Git as needed, and use their delivered interfaces without adding target synchronization.
- [x] C2 Run focused CLI/installation checks and the existing Full Gate at Audit, review complete representative rendering fixtures and focused invariants, and retain relevant endpoint, Claim, handoff, wait, and ownership regressions. DOD11.

## Docs

- [x] D1 Update bounded Implement usage and installation/upgrade documentation for Markdown defaults, JSON opt-in, generic retrieval refusal, deferred resources, one-item Pi reports, and legacy-loop disablement through `go install ./cmd/skl` then `skl install`; preserve truthful Watchdog sibling status and the pending ADR 0005 redesign. DOD11.
