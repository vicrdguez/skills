# Retire implementation ledgers before review

Treat `.changes/<slug>` Markdown as an ephemeral implementation ledger rather than durable project documentation. The implementer records an immutable Artifact Baseline, permits only completion ticks during first-pass implementation, records Artifact Completion, removes the directory in a further commit, and pushes those commits before asking the engine to move the Work Item into Awaiting Review. The engine validates the snapshots, permitted edits, removal, and reachable references before allowing that transition. Review and rework resolve the historical snapshots while durable knowledge remains in `CONTEXT.md`, ADRs, and capability documents. This avoids merging obsolete planning files without replacing readable agent-authored contracts with JSON or another artifact store.

## Consequences

- The implementer owns ledger edits, deletion, commits, and push; the engine validates baseline-to-completion edits, completion-to-review-head removal, and snapshot reachability deterministically.
- Snapshot identity comes from the slice branch's first-parent Git trees, never commit messages or Markdown: Artifact Baseline is the unique absent-to-present transition for `.changes/<slug>`, the deletion commit is the unique later present-to-absent transition, and Artifact Completion is that deletion commit's first parent.
- The valid path history is `absent* → present+ → absent*`. Merges must leave the ledger tree equal to their first parent; missing, repeated, imported, recreated, or post-deletion presence is repairable invalid state rather than something the engine guesses through.
- Artifact Baseline introduces the complete ledger at once. Between Baseline and Completion only permitted checkbox ticks may change; Completion has every agent-verifiable box checked and every Manual Verification box unchecked, and its child deletion commit removes the entire ledger.
- Review instruction packets expose historical ledger content even though it is absent from the review head.
- Rework uses review findings rather than recreating the ledger.
- Ledger commits have no durability guarantee after merge; Superseded Work Items retain a lightweight durable reference so later exploration can inspect the abandoned evidence.
- Cleanup relies on semantic Merged state, not an archive directory or Git ancestry.
- V1 stores no extra baseline identity, so rewritten-but-equivalent reachable history cannot be detected after a prohibited rebase or force-push. Those operations remain forbidden by agent instructions rather than motivating another metadata store.

## Scoped supersession: Endpoint validation

The endpoint-validation slice supersedes only the first-parent identity, `absent* -> present+ -> absent*` transition count, intermediate-edit and merge-tree constraints, and immediate-child deletion requirement above. Artifact identity now uses unique slice-scoped commit-subject prefixes `[baseline] <slice-slug>` and `[completion] <slice-slug>` in selected reachable history. Validation compares those endpoint path sets, mode `100644` blobs, and exact bytes except permitted lowercase completion ticks; requires endpoint ancestry, completed automated checks, unchecked Manual Verification, and ledger absence at the review head; and deliberately ignores restored intermediate edits. Completion still contains the ledger and a later commit removes it. Missing or ambiguous markers are never guessed; markerless work may supply full endpoint SHAs per relevant invocation, applying the same checks without rewriting history or persisted Adoption state.

The retirement-before-review decision, historical contract access, Rework behavior, and human-owned Manual Verification remain in force. Startup content delivery and the other [candidate-first corrections](../capabilities/work-item-lifecycle.md#planned-correction-candidate-first-workflow) remain planned in slices #2 through #5.
