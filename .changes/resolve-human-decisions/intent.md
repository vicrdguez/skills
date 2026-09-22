# Resolve Human Decisions

## Why

Needs Human pauses need an explicit human answer, not a forge relabel or an agent's inference from discussion. A person should be able to consider related questions across Projects without finding each source checkout, and resume the right phase without losing accepted obligations or review history.

## What

Deliver a ledger-wide Decision Inbox and conversational Skill through `skl`. Present current requests with their exact references, record explicitly scoped human direction in `decision.md`, and atomically requeue or supersede the selected Work Item. Keep worker selection project-scoped and independent of this human-facing inbox.

## Scope

- Read current Needs Human requests across the configured Workflow Ledger, optionally filtered by Project, including when invoked outside a source checkout.
- Supply the conversational Skill with commitments, conflicts, evidence, options, recommendations, and exact request references for triage and related-question grouping.
- Accept a clear human answer naming the affected requests without a mandatory second confirmation; distinguish authorization from questions, discussion, and agent suggestions.
- Validate the still-current request and record its answer together with routing to Implement, Watchdog, or Superseded. Support same-code Watchdog continuation and retain the Review Count and automatic-rework limit.
- Preserve frozen Contracts and partial delivery when abandoning wrong-scope work. Require renewed proposal and re-slicing for changed obligations; permit human-directed parent retirement only when no active work or Claim remains.
- Deliver the exact recorded decision to the next worker through `skl`, with specialized Markdown instructions and explicit JSON output available to programmatic callers.

## Out of Scope

- Machine configuration, Project association, acceptance, and frozen-Contract storage supplied by `record-private-proposals`.
- The project-scoped Implement/Watchdog loop, Claim acquisition/resume/release, report schema, ordinary handoff mechanics, and normal publication attempts supplied by `run-ledger-delivery`.
- Forge catch-up and ambiguous-publication recovery, owned by `recover-forge-publication`; neither a forge write nor that slice is required to apply a local Human Decision.
- Actual merge/closure observation, dependency unblocking, and terminal aggregate observation, owned by `observe-human-completion`; archive and source-workspace cleanup, owned by slice 6.
- Contract amendment, automatic replacement creation, migration/import tools, dependency-remapping or replacement-validation graphs, a source-checkout registry, an inbox database, persistent chat cursors, a project worker or loop launcher, and public-comment authority.

## Dependencies

The sole direct Dependency is `run-ledger-delivery`. It already depends on `record-private-proposals` and #55, and supplies current Needs Human reports, exact ledger/source references, exclusive Claims, atomic local handoffs, and specialized worker outputs. This slice consumes those boundaries rather than duplicating them. It does not depend on `recover-forge-publication`, `observe-human-completion`, or archive cleanup.

## Definition of Done

This Proposal uses the current source `.changes` Artifact Baseline carrier based on `main` at `e582110`. The unchecked acceptance bullets below are carrier checkboxes for the current endpoint validator, not mutable target Contracts. The target Workflow keeps accepted Contracts frozen and records completion in Phase Reports. Detailed obligations are identified once in `behavior.md` and `plan.md`.

- [x] The inbox is ledger-wide by default, works without a source checkout, filters by Project, and gives exact current request context for triage and grouping (B1-B2, A1, A3).
- [x] Only explicitly scoped human answers authorize changes; current-request validation permits unrelated ledger commits and refuses stale or ambiguous answers (B3-B4, A2-A3).
- [x] A Human Decision and its selected route are committed atomically, with safe retry and per-item outcomes for multi-item direction (B5, B7, A2).
- [x] Implement and Watchdog receive the exact answer through `skl`; same-code continuation preserves independent judgment, completed-review history, and the existing rework budget (B6, A3).
- [x] Wrong obligations require renewed proposal; supersession preserves merged work and frozen Contracts, and guarded retirement never claims full delivery or unblocks dependents (B8, A4).
- [x] Focused public-CLI, real-Git, and rendered-instruction checks account for behavior and architecture using ADR0005's many-to-many evidence, without mandating construction order or new test infrastructure (A5).

## Manual verification

None. Authorization mechanics and instruction delivery are verifiable at the CLI, real local Git, and rendered Skill/resource boundaries; no live conversation evaluation is an acceptance gate.
