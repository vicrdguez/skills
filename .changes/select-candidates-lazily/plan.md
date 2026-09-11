# Candidate-first Work Start Plan

## Approach

Deliver the fast consumer path after `leave-integration-to-humans`, `own-review-checkpoints`, and `publish-handoffs-without-journals` are Merged. Those dependencies remove startup obligations and make label-only consumption safe. `validate-artifact-endpoints` is deliberately independent: workers invoke the existing artifact-inspection seam after Git preparation, regardless of which validator is currently behind it.

Separate lightweight candidate observation, ordered selection, selected feedback, acquisition/readback, and presentation. This is a small set of responsibilities behind existing workflow/integration seams, not a new queue service or cache framework. Keep eligibility and priority in Workflow Mechanics and GitHub representations in the integration. The merged publication-binding correction already introduced opaque IDs and presentation adapters; do not revert it or broaden this Work Item into the separate backend-independence project.

## Implementation decisions

- Use direct GitHub issue/PR connections with open-state and positive queue-label filters. GraphQL repository connections are appropriate for labelled PRs and bounded shallow batches; use existing REST operations where they already supply the needed exact data. Do not use Search results to prove queue emptiness or ordering.
- Negative `wip` filtering is local. Follow each required connection/page independently; GraphQL errors, resource truncation, and incomplete pagination are not empty results. Do not rely on multi-label arguments expressing OR.
- Rework ordering is PR creation time, then PR number; Ready uses issue creation time/number; Watchdog uses PR creation time/number. Numeric ordering remains a provider-supplied fact rather than parsing opaque IDs in the engine. Exhaust eligible Rework before Ready. Stop after selecting a candidate; do not prefetch every candidate's feedback.
- Only Ready candidates have gating Dependencies. Resolve native relationships and already-supported explicit dependency declarations on that candidate, then establish actual Merged evidence for referenced blockers only. Closed does not itself mean Merged. Unknown/inaccessible relationship observations are not implicit absence or satisfaction. Avoid broad terminal-item normalization to build a merged map.
- One explicit owning issue per PR and at most one active owning PR per issue replace branch/title joins. Reuse the existing engine-supplied closing reference and GitHub's explicit issue/PR relationships; ordinary prose mentions do not assign ownership. Validate repository-qualified identity and the selected attachment, not global historical name uniqueness. Renaming titles cannot change ownership. Selected invalid ownership is a refusal, not an automatic search or reassignment.
- First publication must establish and read back that association; subsequent publication, selection/resume, status attachment reporting, and safe cleanup consume it consistently. A new Ready issue may have no PR yet. Do not recreate or inspect historical PRs merely because their branch names match. Branch and slice pointers needed before first publication come from explicit proposal attachment facts, not treating issue display titles as stable identities.
- Refresh only selected records around acquisition, returning an observed claimed item to the workflow rather than causing another queue enumeration. Preserve additive labels and the prerequisite ambiguous-write policy. Existing best-effort/single-operator semantics remain; no new exclusive-claim claim is made.
- Retired target/review/transition metadata must not return as a discovery dependency. Opaque human comments can be supplied as feedback, but they do not redefine queue eligibility. Old metadata-looking text is not an authority for the new selector.
- `next` needs repository/remote and worktree-location metadata, not local project commits/trees/blobs. Return branch identity and concrete preparation/inspection commands. Do not run Git fetch, create a worktree, load endpoint file bodies, or inspect project ancestry before delivering startup instructions.
- Preserve the worker's code, index, branch, and uncommitted work. The worker owns safe fetching/worktree preparation and invokes endpoint validation afterward. Resume uses current branch work and explicit attachment identity; it never substitutes another item or reads a hidden execution journal.
- The checkpoint prerequisite supplies count, completed-review SHA, and stable review-submission facts through its CLI interface. If local objects are unavailable, do not fetch in the selector to validate the repeat range: preserve the available count and provide the worker/inspection path with the prior revision and full-review fallback rule after preparation. The agent never needs the checkpoint filename or format. A missing dedicated worktree has no retained checkpoint and follows the accepted count-zero/full-review behavior.
- Presentation occurs once per selected invocation through the existing presentation/packet path. Remove historical-file embedding from the startup contract; selected feedback may be read once here or through a concrete selected-only retrieval path, but it must be complete and cannot trigger another queue selection or second full skill packet.
- Do not globally change shared historical-listing helpers to open-only: proposal reconciliation, explicit status, and safe merged cleanup retain their own valid observation needs. Optimize the Work Start path and remove obsolete broad calls from it.

### Modified modules and approved test seams

**CLI integration and presentation:** `cmd/skl/implement.go`, `cmd/skl/watchdog.go`, `setup/presentation.go`, `catalog.go`, and paired skill instructions. Tests enter through the existing `newApp(...).Run(...)` command interface and inspect structured outcomes and packets. Preserve numeric GitHub user-facing identities and opaque engine IDs.

**Workflow selection:** `workflow/implement.go` and `workflow/watchdog.go` retain ordering, eligibility, and explicit resume. Use existing workflow/backend seams; replace the broad Work Start observation path rather than making `ImplementationItems` silently return incomplete records to its other callers.

**GitHub observation and ownership:** `setup/implementation.go`, `setup/watchdog.go`, and the relevant publication/status/cleanup integrations translate direct relationships and controlled observations. Use the actual GitHub adapter under the CLI with `httptest` or an injected HTTP transport. Requested endpoints and forbidden unrelated reads are observable protocol/performance requirements, not tests of private helper calls.

**Git environment:** temporary real repositories with configured remotes but deliberately missing selected branch objects prove deferred preparation. Dedicated dirty worktrees prove non-destructive resume. Reuse the accepted artifact and checkpoint interfaces; add no private-parser or test-only public seam.

### Performance verification

Record before/after fixtures for no candidates, only claimed candidates, blocked Ready prefixes, and successful selection with unrelated history. Capture HTTP request counts by purpose, bytes where practical, Git object-reading subprocesses, and repeated elapsed measurements with medians and tail samples. Controlled HTTP latency makes the effect of removed serialized requests visible without consuming live API quota. Never benchmark production `next` by repeatedly claiming live work. Structural assertions are the stable regression gate; report timing evidence without unsupported universal speedup or strict millisecond promises.

## Sequence

1. Confirm the three declared prerequisites are Merged and read their final interfaces; do not assume source snapshots in this baseline already include them.
2. Use the approved CLI/actual-adapter seam to implement one B scenario per red-green cycle, starting with filtered no-work and ordering, then lazy Dependencies and explicit ownership.
3. Wire selected acquisition/readback, deferred Git preparation, complete selected context, and same-item resume without reintroducing prerequisite metadata.
4. Verify publication-to-next handoff compatibility and checkpoint facts across both lanes; preserve other operations' historical observation requirements.
5. Run the focused and full existing checks, produce the performance comparison, and update only this slice's current capability/README/skill descriptions. Keep independent marker rollout and queue-supervisor plans distinct.

Use a controlled worker/toolchain cutover: an already-running worker must finish with its original CLI/instructions or explicitly resume under the new supported contract, not receive compatibility metadata. This Work Item's own completion commit should carry `[completion] select-candidates-lazily` before its ledger is retired so the artifact-endpoint slice can inspect the history if installed later.
