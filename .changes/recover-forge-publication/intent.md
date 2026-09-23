# Recover Forge Publication

## Why

Local acceptance and phase handoffs must remain useful when the forge is unavailable. Once those records exist, an operator or agent needs to publish the latest human-facing view without repeating implementation or review, losing private prose boundaries, or duplicating an external effect whose response was lost.

## What

Deliver explicit CLI recovery for pending issue and PR publication through `skl`. Reuse current temporary bodies when available; when they are lost or stale, supply specialized instructions and exact private evidence references so an agent can author a fresh human-facing body. Recover observable effects safely and preserve authoritative local work.

## Scope

- Recover pending initial descriptive issue publication from `record-private-proposals`, including the multi-slice parent where applicable, and pending PR creation or presentation updates from `run-ledger-delivery`.
- Publish only the latest relevant local view, not a replay of every missed phase or review round. Inspect pending status and report published, already-satisfied, pending, prose-needed, stale, or ambiguous results distinctly.
- Reuse applicable temporary agent-authored prose; recover missing prose through specialized Skill/resource output, never CLI-generated summaries or durable public-body snapshots.
- Reconcile ambiguous creates/updates against observable forge effects, protect current attachments and publication requests, and avoid duplicate effects when their identity can be established.
- Keep GitHub draft/ready mapping in the forge adapter, preserve local Workflow State, Review Count, Claims, Contracts, and reports, and keep detailed worker evidence private by default.
- Support explicit optional publication of selected actionable inline findings anchored to the reviewed code; expose complete Manual Verification obligations privately while retaining appropriate human-facing checks.

## Out of Scope

- Local acceptance, machine config, Project identity, or frozen-Contract storage owned by slice 1; the Implement/Watchdog loop, report schema, exclusive Claims, local handoffs, normal publication attempts, and ordinary draft-to-ready publication owned by slice 2.
- Human Decision inbox or authorization, owned by `resolve-human-decisions`. This slice neither requires it nor treats public comments as decisions.
- Actual merge/closure observation, terminal aggregate status, and dependency unblocking, owned by `observe-human-completion`; archive and workspace cleanup, owned by slice 6.
- Re-running implementation, tests, Audit, or Watchdog to republish; resetting state, review budget, or Claims; waiting for publication as a Workflow gate.
- Persisted `issue.md`, `pr.md`, or outbox prose; replay logs; prose-generation or summarization in the CLI; automatic publication of every finding; new credentials infrastructure; a daemon, loop consumer, scheduler, HTTP server, or direct forge mutation instructions outside `skl`.

## Dependencies

The sole direct Dependency is `run-ledger-delivery`. Through its existing Dependencies on `record-private-proposals` and #55, it supplies local acceptance, authoritative reports/state, pending-publication records, forge attachments, normal publication attempts, and specialized instructions. Recovery consumes those facilities and handles pending acceptance publication as well as phase publication. There is no Dependency on `resolve-human-decisions`, `observe-human-completion`, or archive cleanup.

## Definition of Done

This Proposal uses the current source `.changes` Artifact Baseline carrier based on `main` at `e582110`. The unchecked acceptance bullets below are carrier checkboxes for the current endpoint validator, not mutable target Contracts. The target Workflow keeps Contracts frozen and stores completion in Phase Reports. Detailed obligations are identified once in `behavior.md` and `plan.md`.

- [x] Operators and agents can inspect and recover pending initial issue and phase PR publication through `skl` without rerunning work or waiting on another slice (B1-B2, A1-A2).
- [x] Recovery reuses applicable temporary prose or supplies specialized authoring instructions and exact private references when it is missing or stale; no public-body snapshot or CLI-authored summary is persisted (B2-B3, A2, A4).
- [x] Observable ambiguous effects and repeated recovery do not duplicate publication; stale requests and changed attachments cannot overwrite newer work or clear newer pending publication (B4-B5, A2-A3).
- [x] GitHub alone owns draft presentation, and recovery never resets local state, reports, Review Count, or Claims or observes completion on behalf of slice 5 (B1, B5-B6, A1-A3).
- [x] Public bodies remain descriptive human content, complete Manual Verification remains privately accessible, and only explicitly selected actionable reviewed-code findings are eligible for optional inline publication (B7-B8, A3-A4).
- [x] Public CLI checks with real local Git and the existing controlled forge HTTP seam, plus rendered Skill/resource checks, provide many-to-many evidence without new frameworks or a prescribed construction sequence (A5).

## Manual verification

None. Local preservation, GitHub HTTP effects including draft changes and ambiguous responses, and delivered authoring guidance can be verified at controlled CLI/HTTP and rendered-resource seams without a live forge or human-only gate.
