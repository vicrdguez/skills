# Record Private Proposals

## Why

Accepted work currently depends on source-branch artifact markers and forge records. ADR 0006 requires a private Workflow Ledger that keeps accepted Contracts available locally even when publication fails. This first slice must provide useful proposal intake and exact readback without waiting for ledger-backed execution.

## What

Configure an existing local ledger clone, associate a Consumer Repository with its Project, and accept single- or multi-slice Proposals through `skl`. Freeze their Contracts in a local Git commit, record dependencies and planned source branches, expose the accepted content through the public CLI, and attempt ledger replication and descriptive issue publication without making either authoritative.

These three proposal files are a temporary bootstrap carrier, published through the current source-branch `.changes/record-private-proposals/` baseline mechanism. The unchecked acceptance bullets below serve only that carrier's existing endpoint validator. The target private Contracts are frozen, read-only documents with descriptive, locally numbered B/A/T/M items, not mutable completion checklists or source baseline/completion snapshots.

## Scope

- Machine-local ledger configuration and source-repository Project identity, including same-name collision refusal.
- Complete local acceptance of proposal content, slice Contracts, dependencies, planned branch identities, and initial state in ADR 0006's layout.
- Public CLI acceptance/readback, explicit commit/path references, publication status, and actionable refusals.
- Initial ledger push and human-facing issue publication using temporary agent-authored bodies; durable pending information for failures.
- Propose instructions and resources for the new intake path, plus safe refusal of unsupported execution and preservation of non-adopted work.

## Out of Scope

- `run-ledger-delivery`: implementation, Watchdog, Claims, phase-report schemas, private execution evidence, and basic PR updates.
- Human decisions; publication catch-up and explicitly selected inline findings; human merge/closure observation and dependency unblocking; archive and safe cleanup. These remain slices 3 through 6 respectively.
- Source branch/worktree creation at acceptance, a complete Local Backend, hosting provisioning, migration/import tooling, permanent dual authority, or automatic adoption of existing work.
- YAML libraries or schema machinery before phase reports need them; new storage engines, services, scheduling, or test frameworks.

## Dependencies

- Blocked by issue #53, `materialize-approved-contracts`, which transitively waits for #52, #49, and #50. Implement against its merged result while preserving ADR 0005's accepted verification policy; current source terminology may lag that target.
- This is approved slice 1 of six. `run-ledger-delivery` depends on this slice and #55; it is not a prerequisite for this slice's intake/readback usefulness.

## Definition of Done

- [ ] B1-B2 and A1: configuration resolves the specified local clone, and Project identity follows the remote repository rather than checkout names, refusing different-repository name collisions safely.
- [ ] B3 and A2: single- and multi-slice proposals freeze complete Contracts and declared relationships locally in the exact ADR layout, without source preparation or accidental acceptance of invalid/changed work.
- [ ] B4 and A3: public CLI readback returns exact accepted content and explicit full commit/path references independently of forge availability, marker discovery, and later ledger activity.
- [ ] B5-B6 and A2/A4: local acceptance survives ordinary publication failures with honest pending state; successful attachments are retained and competing ledger history requires reconciliation rather than automatic merging or grants.
- [ ] B7 and A3: distributed Propose guidance authors frozen, locally identified Contracts and descriptive temporary issue bodies, preserving #53's fidelity and ADR 0005 verification requirements.
- [ ] B8 and A5: the intermediate release provides usable intake/readback, refuses unsupported execution without forge fallback, and preserves non-adopted work without migration or dual-authority machinery.
- [ ] A6: existing CLI/Git/HTTP checks provide many-to-many evidence for all rules, scenarios, and architectural commitments; the Full Gate and independent review remain required.

## Manual verification

None.
