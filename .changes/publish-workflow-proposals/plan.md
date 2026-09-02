# Publish Workflow Proposals Plan

## Approach
Extend the Workflow module with one Proposal publication operation whose interface accepts prepared slice facts, opaque body-file paths, and a small dependency graph. The module preflights the whole request using the concrete Repository and normalized Backend records, then performs simple blocker-first mutations. Each child reaches a valid visible state independently; `ready` is its final mutation.

Keep proposal authorship in the embedded Skill Definition. The CLI neither generates nor parses Markdown and never creates branches or proposal commits. Its sole Git mutation in the active workflow is safe cleanup that can be derived from Merged Work Items.

## Implementation decisions
- Expose a semantic Propose command family rather than generic issue, label, worktree, or GitHub wrappers.
- Accept a single slice or repeated `slug=body-file` inputs, optional parent title/body-file, and explicit `dependent:blocker` edges. Exact flag spelling may follow `urfave/cli` conventions, but there is no separate manifest format.
- Use the slice slug as the Work Item's stable pre-publication key and current issue title, preserving the existing protocol. Parent title and all bodies come from the agent.
- Treat body files as opaque bytes. The CLI owns only API transport and structured parent/Dependency relationships.
- Preflight every declared slice and graph edge before the first backend mutation.
- Check that `CONTEXT.md`, `docs/adr/`, and `docs/capabilities/` contain no uncommitted changes in the proposal's main worktree.
- Observe the target branch once for publication. Require every Artifact Baseline to contain that commit and every remote slice head to equal its local published head.
- Derive the baseline from first-parent trees: exactly one absent-to-present transition at `.changes/<slug>`, with `intent.md` and `behavior.md` plus any optional ledger files all introduced in that commit. Before implementation there is no deletion transition.
- Use GitHub native sub-issue and Dependency relationships. If native Dependencies are unavailable, append the existing deterministic `Blocked by` fallback to the otherwise opaque child body without interpreting its prose.
- Add `ready` last for each child. Blocked children still represent Ready for Implementation; eligibility is a separate engine decision.
- On retry, list records and reuse only a unique exact slug/title match whose structured relationships do not contradict the request. Zero matches creates; multiple or conflicting matches require human resolution. Add no identity marker or transaction record.
- Do not roll back successful child publication after a later failure.
- Identify cleanup candidates from Merged Workflow State, exact `.worktrees/<slug>` registration, and matching local branch. Remove only clean conventional candidates; never force removal and never delete a remote branch.
- Revise only deterministic mechanics in the Propose Skill Definition. Preserve its conversation synthesis, repository reading, tracer-bullet reasoning, seam quiz, artifact templates, and frozen-contract wording.

### Module shapes & seams

#### [MODIFIED] Workflow
**Interface:** publish a typed Proposal request containing an optional Coordination Item body, one or more prepared slice records, and Dependency edges; return a structured completed, fix-required, or needs-human outcome.

Workflow owns full preflight, ordering, state meaning, safe forward reconciliation, and the point at which each Work Item becomes Ready. Callers do not orchestrate Backend mutations.

Dependencies: concrete Repository for Git facts and cleanup; Backend port for normalized Work Items, Coordination Items, Dependencies, and conditional mutations; Instruction Catalog for the specialized Propose packet.

**Test strategy:** test complete CLI-visible scenarios through the Propose command using temporary Git repositories and the in-memory Backend. Assert final normalized workflow records, not mutation call order.

#### [NEW] Implementation Ledger history
**Interface:** inspect a slice ref and slug and return the unique Artifact Baseline plus current ledger lifecycle facts, or a precise invariant violation.

This critical internal module owns first-parent path-state traversal and mandatory baseline tree shape. It does not parse Markdown or commit messages.

Dependencies: concrete Repository Git operations.

**Test strategy:** table-driven tests create literal Git histories for absent-to-present, inherited paths, split introductions, recreations, and merges that change the path relative to first parent. Expected commit identities are fixed from the constructed graph.

#### [MODIFIED] Backend port and GitHub adapter
**Interface:** list/create Coordination Items and Work Items, attach sub-issues, add Dependencies, and project Ready state.

Keep backend records normalized; GitHub issue numbers and database IDs stay inside the adapter. Extend the port only with operations exercised by this slice.

**Test strategy:** workflow behavior uses the in-memory adapter. HTTP adapter tests verify native relationship payload mapping and retry reconciliation against an HTTP test server.

#### [MODIFIED] Repository
**Interface:** expose target ancestry, local/remote slice refs, first-parent trees, worktree registration/cleanliness, and safe conventional cleanup.

Repository hides Git command details but does not perform proposal authoring or ordinary branch creation.

**Test strategy:** use temporary real Git repositories through Workflow scenarios and focused ledger-history graph tests.

## Sequence
1. Add normalized Proposal, Work Item, Coordination Item, and Dependency records to Workflow and Backend.
2. Implement Git-only Artifact Baseline discovery and complete preflight diagnostics.
3. Implement safe Merged-worktree/local-branch cleanup.
4. Publish single-slice Work Items with `ready` last and forward retry.
5. Add multi-slice parent and Dependency publication in blocker-first order.
6. Replace deterministic publication prose in the embedded Propose Skill Definition and remove its `docs/github.md` dependency.
7. Update proposal lifecycle documentation and verify no duplicate proposal manifest exists.
