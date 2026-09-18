# Archive Terminal Proposals

## Why

Completed or abandoned proposals should leave the active ledger area without erasing their evidence or treating abandoned code as merged. Archiving ledger records and deleting safe merged source work are different operations with different safety conditions.

## What

Extend explicit cleanup to move an entire terminal, unclaimed Proposal from `projects/<repo>/proposals/<proposal>` to `projects/<repo>/archive/<proposal>`. Preserve lifecycle meaning and exact historical report references, and retain existing safe merged-workspace cleanup as an independent outcome.

This is accepted slice #6 of the six-slice private-ledger change described by ADR 0006. ADR 0005 governs contract-grounded verification; CONTEXT.md distinguishes full delivery, supersession, and Claims.

## Scope

- Whole-proposal archival only when every slice is terminal and unclaimed, with all-Merged delivery distinct from mixed or wholly Superseded retirement.
- Explicit cleanup through the public CLI, independently callable and invoked by Propose before preparing new slices, never automatically after merge.
- Durable ledger archive mutation and harmless retry using predecessor identity, serialization, local-commit, and push semantics.
- Historical exact commit/path retrieval and current identity resolution after movement, without rewriting reports or Contracts.
- Separate safe cleanup of actually merged source work, preserving dirty, unknown, unmerged, Superseded, unowned, or changed-head work and all remote branches.
- Useful archival with offline forge access, pending publication, or a retained source worktree, without decision UI or catch-up prerequisites.

## Out of Scope

- Inferring or changing lifecycle because a path moved, marking abandoned work delivered, or creating merge evidence in cleanup.
- Human decision UI, publication recovery, successful forge publication as an archive gate, or replacement-specific remapping/validation.
- Automatic post-merge cleanup, deleting unmerged or Superseded source work, remote branch deletion, or global source-workspace discovery.
- Source archive mirrors, permanent source retention, ledger compaction/retention policy, history scans, administrative importers, or automatic replacement/re-slicing tools.
- New report schemas, rewriting historical inputs, separate archive manifests duplicating proposal membership, or new test infrastructure.

## Dependencies

The only direct Dependency is `observe-human-completion` (#5). It provides the authoritative Merged/Superseded facts, full-delivery distinction, and safe terminal observations this cleanup consumes. Its chain through `run-ledger-delivery` (#2) and `record-private-proposals` (#1) supplies private layout, stable identity, report references, exclusive Claims, source attachments, and ordinary ledger mutation/push semantics; these are transitive foundations, not additional direct edges.

Human decisions (#3) and publication recovery (#4) remain independent siblings, not prerequisites. Cleanup must accept their later ordinary terminal records or attachment/publication updates without depending on those interfaces. A proposal completed through #2 and observed through #5 is already an independently useful archive path.

## Definition of Done

These unchecked acceptance bullets are current publication carrier checkboxes for the `.changes`/baseline endpoint machinery. They are not permission to mutate the target private Contract, whose accepted obligations remain frozen under ADR 0006.

- [ ] B1-B2: Explicit cleanup moves only whole terminal/unclaimed proposals to the archive, preserves full-delivery versus Superseded meaning, and retains every proposal document and exact earlier commit/path reference.
- [ ] B3-B4: Source deletion still requires actual confirmed merge, exact accepted head, clean worktree, and expected owned location; all unsafe source work and remote branches are preserved, independently of archive success.
- [ ] B5: Repetition and interrupted moves cannot overwrite a distinct archive, lose individual files, clear a later Claim, or silently claim success on an ambiguous partial result.
- [ ] B6: Cleanup works independently and in Propose before new slices, never as a merge hook, with predecessor local-ledger/push behavior and no publication-success or sibling-UI precondition.
- [ ] A1-A5: Existing CLI, Workflow, private-ledger, and Git cleanup responsibilities satisfy the accepted architecture and failure-aware many-to-many verification without new test infrastructure or mandated TDD order.

## Manual verification

None. Public CLI results, real source/ledger Git state, exact historical reads, controlled remote failures, and instruction-distribution checks cover the required observations.
