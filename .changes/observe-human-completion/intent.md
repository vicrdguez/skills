# Observe Human Completion

## Why

Private delivery can finish at Ready for Merge without a forge connection, but later work needs a durable distinction between review approval, an actual human merge, and abandonment. Dependency eligibility and proposal completion must use confirmed facts, not public presentation or a successful publication attempt.

## What

Observe the explicitly attached Submission through the forge adapter and record confirmed merge or unmerged closure in the Workflow Ledger as Merged or Superseded bookkeeping. Expose those facts through status and normal dependency selection without extending autonomous delivery beyond Ready for Merge.

This is accepted slice #5 of the six-slice private-ledger change described by ADR 0006. ADR 0005 governs contract-grounded verification; CONTEXT.md supplies the domain meanings.

## Scope

- Narrow observations of the owned Submission, its repository attachments, and its Integration Target; authoritative terminal facts and state recorded in the ledger.
- Status and dependency eligibility using locally confirmed facts, including offline operation and explicit uncertainty for unconfirmed blockers.
- Safe repetition and selected-record concurrency checks that preserve current or later Claims, phase results, and terminal work.
- Coordination Item accounting that distinguishes all-children-Merged delivery from mixed terminal retirement eligibility.
- Separate merge evidence from historical implementation/review source revisions, including squash merges and human integration changes.

## Out of Scope

- Engine merging, final integration, conflict resolution, conflict-driven Rework, or a new post-approval review phase.
- Human decision UI, decision inbox, publication catch-up, or rebuilding public issues, comments, labels, and bodies as workflow authority.
- Replacement-specific dependency remapping or validation, automatic replacement or re-slicing, and proposal archive movement or source cleanup.
- Polling daemons, webhooks, runners, scheduling, repository-wide history discovery, source-code mirrors, or source-history retention guarantees.
- New report schemas, report rewrites, duplicated review results, and migration or dual-authority machinery.

## Dependencies

The only direct Dependency is `run-ledger-delivery` (#2). It supplies the useful path: local Implement and Watchdog through Ready for Merge or paused/Rework outcomes, exclusive Claims, schema-1 Phase Reports, source preparation, normal draft-PR publication attempts, and explicit source/forge attachments. Its prerequisite `record-private-proposals` (#1) supplies configuration, accepted private records, and normal issue-publication attempts transitively, not as another direct edge.

Human decisions (#3) and publication recovery (#4) are independent siblings, not prerequisites. Records or attachments they later produce must be consumable through the same ledger facts and ordinary guards; neither interface is needed to observe a normally published Submission. `archive-terminal-proposals` (#6) depends on this slice and consumes its terminal bookkeeping.

## Definition of Done

These unchecked acceptance bullets are current publication carrier checkboxes for the `.changes`/baseline endpoint machinery. They do not authorize mutation of the target private Contract; ADR 0006 keeps that Contract frozen and records completion in Phase Reports.

- [x] B1-B2: Actual owned-Submission merge and unmerged closure become Merged/Superseded bookkeeping; Ready for Merge remains the autonomous delivery endpoint and public presentation never authorizes a transition.
- [x] B3, B5: Stored confirmed facts remain usable offline; unknown or failed observations never invent terminal state or satisfy Dependencies, and only Merged blockers permit normal eligibility.
- [x] B4: Repetition, interrupted recording, and concurrent selected-record changes cannot release a current/later Claim, overwrite later results, or resurrect terminal work.
- [x] B6: Only all children Merged establishes full proposal delivery; mixed terminal outcomes can be recognized as retireable without asserting completion.
- [x] B7-B8: Fixed-item reads stay within the selected record and required evidence; historical source and ledger references survive terminal observation unchanged without a source-retention guarantee.
- [x] A1-A5: The accepted responsibility boundaries and failure-aware verification are satisfied using the existing public CLI, real Git repositories, and controlled HTTP tools, with many-to-many evidence and no mandatory test-writing order.

## Manual verification

None. Controlled forge responses, CLI outcomes, ledger commits, source refs, and request boundaries cover the required observations; human merge remains an external authority, not an agent-performed acceptance check.
