# Inspect Private Reports

## Why

Detailed review findings belong primarily to agents, but the human needs optional access between rounds to detect flawed implementation directions or workflow gaps. W3 in superseded #63 / PR #78 exposed that value. Removing GitHub inline-publication scope must not make the original evidence inaccessible or force a human into every review.

## What

Expose current and historical implementation/Watchdog reports through `skl`, with a discoverable starting point for a selected Work Item, using the private Git ledger's existing files, history, and references.

## Scope

- Locate and retrieve current implementation/review reports at committed ledger HEAD without needing a worker Claim or Needs Human state.
- Retrieve exact historical report versions and reach earlier rounds through Git-backed history or recorded references.
- Expose report access through ordinary status/output while preserving opaque authored bodies and actionable absence/unavailability errors.
- Verify read-only, local-only access through public CLI operations and real ledger Git history.

## Out of Scope

- Forge publication, including both independently delivered publication slices and GitHub inline findings.
- A guided decision viewer, synthesized review summaries, automatic finding classification/resolution, notification infrastructure, or a new human approval gate.
- A report registry, history index, finding database, new report schema, source archive, or direct private-ledger navigation instructions for workers.
- Workflow state changes, Claims, decision intake, archival/retirement, merge observation, or fixing bug #80.

## Definition of Done

Source-carrier checkboxes below are endpoint verification only, not changes to accepted target Contracts or report contents.

- [x] Humans and agents can discover and retrieve current implementation/review reports for a selected Work Item, independently of Claim, lifecycle, and forge availability (B1, A1).
- [x] Earlier rounds and exact historical versions remain accessible through existing Git/reference evidence, with honest errors and no silent substitution (B2, A2).
- [x] Read operations preserve authored report bytes and all workflow/ledger state without a parallel report/finding store or new mandatory review gate (B3, A1-A2).
- [x] Existing Go/public-CLI/real-Git tests establish current/history selection, privacy, immutability, and failure behavior; the Full Gate passes (A3).

## Manual verification

None. The required textual retrieval, reference identity, and absence of mutation are observable through automated local CLI/Git checks.
