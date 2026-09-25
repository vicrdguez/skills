# Publish Current Phase PR Presentations

## Why

A completed implementation/review cycle must not depend on recovering an obsolete public update. W3 in superseded #63 / PR #78 showed an implementation-publication reservation preventing the latest Watchdog result from being presented after local work advanced. ADR0007 replaces that coordination model rather than requesting another reservation-recovery patch.

## What

Publish the current local phase result through ordinary handoffs and explicit `skl` invocation, using agent-authored prose, existing PR associations, ordinary safe source publication, and best-effort draft/ready presentation without durable forge-publication tracking.

## Scope

- Simplify normal PR publication after local implementation/Watchdog handoff and add explicit current-view publication without replaying phases.
- Select current committed results, preserve local authority and later work, and remove PR publication reservations, pending/source receipts, and prose registration.
- Reuse ordinary non-force source publication and the GitHub adapter's native PR body and draft/ready presentation.
- Provide current private references and specialized guidance for fresh public prose, with bounded retry/uncertainty behavior and public CLI verification.

## Out of Scope

- Issue/parent presentation, independently owned by `publish-current-issues`; that slice is not a prerequisite.
- Inline findings, a guided decision viewer, a notification system, a queue/background publisher, durable recovery, rollback transactions, or stronger delivery guarantees.
- Human-decision intake, completion observation, changing Claims/review budget, source-code changes merely to publish, or policy changes to private-ledger replication.
- Workflow-design bug #80 and reopening or continuing the old Contract of #63.

## Definition of Done

The source carrier's checkboxes are endpoint verification only; target accepted Contracts remain frozen and ordinary reports carry completion evidence.

- [x] Both normal handoff and explicit publication present current local results without replaying phases or gating local progress (B1, A1).
- [x] PR associations are retained without in-scope durable coordination, pending-publication/source records, phase selectors, or prose registration; obsolete reservations cannot block current work (B2, A2).
- [x] Ordinary non-force source publication and draft/ready effects obey recorded source identity and bounded best-effort semantics while preserving later local results and Claims (B3-B4, A1-A2).
- [x] Current prose and private-evidence guidance are available without CLI-authored summaries, durable bodies, or automatic private-review export (B5, A3).
- [x] Grouped public CLI/Git/HTTP and rendered-resource evidence covers the obligations, regression sensitivity, and Full Gate without a new framework or stronger guarantees (A4).

## Manual verification

None. Required observations are available through local Git, controlled HTTP, and resource rendering; live GitHub reliability and arbitrary agent judgment are not claimed.
