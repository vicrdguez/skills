# Observe Human Completion Plan

## Accepted Architecture

ADR 0006 is authoritative for private state, bounded references, guarded local writes, and the human completion boundary. ADR 0005 governs verification freedom and evidence. Implement this slice on `run-ledger-delivery`, not directly on main's forge-authoritative mechanisms. Locations below identify existing responsibilities to adapt or reuse; they do not prescribe new command options, types, or module names for predecessor code not yet implemented.

### A1. Workflow Owns Completion Meaning

**Module:** Workflow, currently `workflow/status.go`, `workflow/selection.go`, and lifecycle definitions/callers in `workflow/implement.go`.

**Interface:** Status consumes normalized observations and stored facts; normal selection asks whether each named blocker is confirmed Merged. Preserve `ObserveStatus` and the `SelectionBackend`/`ImplementationDependencies` responsibility boundaries where adequate rather than introduce a second lifecycle engine.

**Responsibility:** Apply B1-B6 deterministically. Ready for Merge remains approval, not merge or conflict-free integration. Parent full delivery requires every child Merged, while terminal mixed outcomes remain distinguishable for later cleanup. Do not derive completion from `CloseCoordination`, public labels, or a reconstructed review comment. Selection retains its existing project scope, priorities, and exclusive Claim semantics.

**Seam and verification:** Drive `skl status` and public work-selection operations with real ledger records and controlled owned-PR responses. Cover merge, unmerged closure, unresolved blockers, all-merged versus mixed parents, and retained Claims together where useful. Assert persisted states and subsequent selection, not merely an in-memory status projection. Relevant existing protection lives in `cmd/skl/status_test.go` and `cmd/skl/candidate_selection_test.go`.

### A2. Forge Adapter Supplies Narrow Facts, Not Authority From Prose

**Module:** Forge adaptation currently in `setup/watchdog.go` (`ReviewSubmission`), `setup/selection.go` (fixed PR reads and blocker observations), `setup/status.go`, and repository identity in `github/remote.go`.

**Interface:** Given exact ledger-owned Submission/repository/target references, read only the necessary forge record and return confirmed merge, confirmed unmerged closure, still-open, or an explicit observation failure. Adapt the existing read seam or narrow it if review-specific hydration brings irrelevant public state along. The core must not need GitHub label names, body parsers, timeline searches, or draft presentation to interpret completion.

**Responsibility:** Validate the returned identity and repository/target context before accepting facts. Supply the confirmed accepted source head separately from any merge revision when available. Remove status/dependency reliance on inventory and public ownership inference for this ledger path; `submissionOwner`, `closingReferences`, and `ListMergedWorkItems` are existing legacy mechanisms, not the new authority. An absent attachment or failed observation cannot imply abandonment. No merging endpoint, source integration, publication catch-up, or polling is added.

**Seam and verification:** Reuse `httptest` and the existing controlled HTTP transport patterns in `cmd/skl/status_test.go`, `cmd/skl/cleanup_rework_test.go`, and `setup/watchdog_test.go`. Check exact owned-record requests, identity/target mismatch refusals, malformed/inaccessible responses, and no broad inventory requests or forge merge writes. Expected outcomes come from literal fixture facts, not the production normalization helpers.

### A3. Ledger Mutation Owns Durability And Stale-Write Protection

**Module:** The private Git ledger record/read/mutation boundary supplied by #1/#2, used by Workflow status and selection. Main's `workflow/ledger.go` currently implements source-tree artifact endpoints; that is not an instruction to reuse marker/history scanning for private completion facts.

**Interface:** Read current selected state and explicit evidence references; conditionally record terminal evidence and resulting state together using the predecessor's brief serialized mutation protocol. Network observation stays outside the mutation lock, followed by selected-record revalidation. Use ordinary local commit and remote-push attempts, with pending replication and competing-history handling inherited rather than reimplemented.

**Responsibility:** Preserve current Claims, phase results, review counts, Contract bytes, and full commit/path references. Reject stale selected-record writes; do not reject solely because an unrelated slice committed. Repetition recognizes already committed completion, and later handoffs cannot requeue terminal work. Store new merge evidence separately from report source revisions. A locally failed mutation is not reported as durably recorded; an unavailable push does not undo a successful local commit. Competing remote ledger decisions still require explicit reconciliation before new work is granted under predecessor policy.

**Seam and verification:** Public CLI operations against separate real Git source and ledger repositories establish successful local recording, response interruption/retry, push unavailability, competing history, and a later Claim/result interleaved during controlled observation. Inspect committed selected records, exact report bytes/references, source refs, and subsequent operations. Preserve unrelated mutations and expose uncertainty without inventing successful persistence.

### A4. CLI Presents Stored Facts And Bounded Uncertainty

**Module:** CLI status and work-start entrypoints in `cmd/skl/status.go`, `cmd/skl/implement.go`, and existing presentation in `setup/presentation.go`; predecessor-provided record/reference retrieval remains the private access boundary.

**Interface:** Preserve public `skl` entrypoints and predecessor-established specialized Markdown/default and explicit JSON presentation, including non-work and repair outcomes. Precise extensions, if any are required, remain implementation choices; this plan does not invent flags or a new observation command family.

**Responsibility:** Make recorded completion, unavailable refresh, blocked eligibility, and full-delivery versus mixed-terminal parent accounting distinguishable. Fixed-item reads must not call the existing whole-project loader merely to rediscover one attachment. Project summaries use current ledger membership. Normal #2 publication provides an independently useful observation path; later #3/#4 outputs are consumed as ordinary ledger records without calls to those interfaces. No cleanup or automatic archive hook is invoked by status.

**Seam and verification:** Assert user-visible results through the public CLI and request logs for selected-record paths. Make unrelated records/endpoints unavailable to prove locality. Verify the same stored merge still drives selection offline, while an unknown blocker does not. Exercise late attachment availability as an ordinary record update rather than require sibling UI implementation.

### A5. Verification Remains Contract-Grounded And Many-To-Many

**Module:** Existing Go checks in `cmd/skl`, `setup`, and `workflow`, using public CLI fixtures, temporary real Git repositories/worktrees, and controlled HTTP. Reuse predecessor ledger fixtures in those tools rather than add a harness, daemon, or evaluation framework.

**Interface:** Verification evidence relates B1-B8 and A1-A4 to observable CLI outcomes, committed ledger contents and references, source safety, and bounded network access. The implementation report accounts for the whole obligation set with many-to-many mappings; neither one test per scenario nor TDD ordering is mandated.

**Failure-aware coverage:** A small group of end-to-end flows can cover normal merge/closure, offline dependency use, and parent accounting together. Targeted fault/interleaving cases cover stale Claims/results, interrupted responses and mutations, remote failures, foreign/unknown identities, and absent historical source objects. Retain independent regression protection from existing ownership, candidate-selection, status, and integration-policy checks while replacing their obsolete public-authority assumptions; avoid assertions on per-helper calls or incidental file organization.

**Checks:** Run focused affected Go tests and the existing repository-wide `go test ./...` plus `git diff --check`; preserve the repository's Full Gate, Audit, and independent Watchdog obligations. Concrete bug fixes must demonstrate regression sensitivity under ADR 0005. No manual verification, live GitHub mutation, new test infrastructure, source retention mechanism, or scenario-mirroring `tasks.md` is required.
