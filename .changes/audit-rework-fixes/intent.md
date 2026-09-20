# Audit finding-driven Rework once

## Why
Finding-driven Rework currently skips Audit for the lifetime of the Submission, although the intended convergence rule was only to prevent one Implement worker from repeating `Audit -> fixes -> Audit`. A Rework fix can introduce a standards violation or regress the frozen Contract and send an avoidable defect into another Watchdog round.

## What
Give each finding-driven Rework execution one Audit specialized to the fixes made since the Watchdog-reviewed commit. Keep the initial implementation Audit unchanged, constrain Rework findings to consequences of the Rework delta, and continue the Submission's Audit Finding ledger with `F<n>` identities.

## Scope
- Render a Rework-specific Audit procedure inside the execution-specific Implement packet
- Select the applicable Watchdog review's `Commit` as the Rework Audit fixed point and stop rather than guess when the evidence is missing or ambiguous
- Review only `<reviewed-commit>...HEAD` during Rework
- Restrict Standards findings to violations or smells caused by the Rework delta
- Restrict Contracts findings to unresolved supplied Watchdog findings, Contract regressions caused by the delta, unnecessary behavior introduced by the fixes, and inadequate regression coverage at accepted seams
- Keep Watchdog findings as evidence and resolution targets without promoting them into frozen Contract Items
- Run the Full Gate once through Rework Audit
- Leave artifact endpoint and retirement inspection to Implement's existing before-and-after Inspect steps
- Permit one Audit invocation per Implement execution and prohibit a second Audit after applying its findings in that execution
- Re-run affected checks and the final Full Gate when applying Audit findings changes code
- Assign every newly produced Audit Finding an `F<n>` identity per ADR 0006: start at `F1` when no `F<n>` exists and otherwise continue after the greatest existing `F<n>`, while preserving historical identifiers in other formats unchanged
- Keep one cumulative Audit ledger and advance its audited head without adding round-specific provenance sections
- Verify behavior through rendered Implement executions and the deferred Submission resource

## Out of Scope
- New workflow state, invocation facts, backend transitions, or CLI operations
- Repeating Audit within one Implement execution
- Whole-change Rework review of unchanged code or unrelated Contract omissions
- Changes to initial, resumed, standalone Audit, Watchdog, or human Merge Authority behavior
- Repeating artifact endpoint inspection inside Rework Audit
- Renumbering historical Audit Findings or Contract Items
- Automating attachment of this Work Item to coordination issue #44

## Definition of Done
- [x] A finding-driven Rework execution directs the worker to run exactly one Audit over the delta from the applicable Watchdog-reviewed commit to the current head, and to stop when that commit cannot be selected unambiguously.
- [x] Rework Standards and Contracts reviews report only findings caused by or necessary to verify the Rework delta, without reopening unrelated unchanged code or whole-change omissions.
- [x] Rework Audit owns one Full Gate run while Implement retains artifact inspection before and after Rework; endpoint inspection is not duplicated inside Audit.
- [x] Applying Rework Audit findings does not trigger another Audit in the same execution, and code-changing dispositions require affected checks and a final Full Gate before handoff.
- [x] Initial and Rework Audits assign every new Audit Finding an `F<n>` identity, starting at `F1` when necessary and otherwise continuing after the greatest existing `F<n>`, while preserving historical identifiers unchanged.
- [x] The cumulative Submission Audit ledger advances its audited head without adding a round-specific ledger section or a synthetic entry for a clean Audit.
- [x] Initial and resumed implementation and standalone Audit retain their existing fixed points, review scope, deterministic checks, and two-axis behavior apart from the shared `F<n>` identity rule.

## Manual verification
None.
