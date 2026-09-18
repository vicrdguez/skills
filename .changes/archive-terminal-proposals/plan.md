# Archive Terminal Proposals Plan

## Accepted Architecture

ADR 0006 pins private layout, whole-proposal archival, reference preservation, ordinary Git replication, and separate source cleanup. ADR 0005 pins contract-grounded verification without a mandated test order. Build only on `observe-human-completion`; the private record and mutation responsibilities it inherits from #1/#2 are reused, not recreated. Existing locations below are responsibility anchors at main, not invented names for future predecessor modules or CLI options.

### A1. Workflow Owns Archive Eligibility And Explicit Coordination

**Module:** Workflow cleanup currently in `workflow/proposal.go` (`Cleanup`, `CleanupOutcome`, and the cleanup side of `Backend`), with status/parent accounting in `workflow/status.go` as adapted by #5.

**Interface:** The explicit cleanup operation returns distinct archive, source-removal, source-preservation, and repair outcomes through the existing public CLI. It consumes current proposal membership and confirmed terminal facts from the private ledger, not `ListMergedWorkItems`' forge-history discovery.

**Responsibility:** Apply B1 and B4-B6. Archive only whole terminal/unclaimed proposals; preserve full delivery versus Superseded retirement. Do not equate archive eligibility with source-deletion eligibility or require source deletion to finish the ledger move. No merge hook, replacement remapping, or source/ledger cross-repository transaction is added. Only the selected proposal's relevant state participates in mutation preconditions.

**Seam and verification:** Run public cleanup against real ledger/source repositories with all-Merged, mixed-terminal, claimed-terminal, active, and unknown-member records. Assert complete path movement or preservation, unchanged states, and independently observable source outcomes. Use the same cleanup path for normal #5 completion and later ordinary sibling records; do not require decision/catch-up interfaces as fixtures.

### A2. Private Ledger Owns Safe Movement And Historical Lookup

**Module:** The private Git ledger layout, identity/read interface, and serialized mutation boundary supplied by #1/#2 and consumed by #5. Main's `workflow/ledger.go` owns legacy `.changes` endpoint inspection, not the target archive mechanism.

**Interface:** Move `projects/<repo>/proposals/<proposal>` as a complete directory to `projects/<repo>/archive/<proposal>` through ordinary CLI-owned ledger mutation. Continue current lookup by stable proposal/Work Item identity and historical lookup by the exact full commit/path already stored. Resolve a known proposal against its active/archive locations without discovering it through history or unrelated inventories.

**Responsibility:** Revalidate complete membership, terminal state, absence of Claims, owned paths, and destination identity inside the existing short mutation boundary. Preserve every document and its bytes, including schema-1 report metadata, historical input references, decisions when present, and pending-publication facts. Do not rewrite embedded earlier paths, create empty replacement records, split children across locations, or add a redundant archive index. Use whole-directory movement and existing Git commit/recovery support; an ambiguous partial tree or distinct destination is a repair outcome, not permission to overwrite. Remote push follows local commit, with pending/unavailable and competing-history cases handled by predecessor semantics.

**Seam and verification:** Through public cleanup and record retrieval, capture original document contents and a prior full commit/path reference, archive, then retrieve both historical and current versions and check dependency resolution. Real Git/filesystem cases cover matching retry, distinct destination, relevant concurrent mutation, move/commit failure, interrupted response, push failure, and unrelated ledger changes. Observe committed versus uncommitted outcomes without asserting helper names or requiring arbitrary history scans.

### A3. Source Cleanup Keeps Its Existing Independent Safety Checks

**Module:** Concrete Git cleanup in `workflow/proposal.go` (`Cleanup` and `registeredWorktrees`), using explicit source attachments and confirmed accepted source-head evidence supplied by #2/#5. Existing source preparation responsibility is in `setup/preparation.go`; existing forge-derived merge discovery is in `setup/github.go`.

**Interface:** Evaluate actually merged, unclaimed, explicitly owned source work against its registered worktree location, cleanliness, and exact accepted source head. Only that safe path may remove the local worktree and local branch. Stop using `ListMergedWorkItems` timeline/body reconstruction as cleanup authority on the private-ledger path.

**Responsibility:** Preserve dirty, changed-head, unknown-head, unowned, wrong-location, unmerged, and Superseded source work. A target squash SHA is not the accepted source head, and source-head ancestry or a surviving upstream ref is not required once merge and exact head are confirmed. Never delete remote branches. Source safety-check or removal failure must remain visible without undoing an already committed archive. Retain existing local registration inspection; do not add a cross-project source registry, source archive mirror, or stronger source-retention guarantee.

**Seam and verification:** Retain and adapt the real-Git regression protection in `cmd/skl/main_test.go` (`TestCleanOnlySafeMergedWorktrees`, `TestCleanMergedBranchWithPrunedUpstream`, and `TestCleanupPreservesUnacceptedLocalHead`) and the ownership cases in `cmd/skl/cleanup_rework_test.go`. Supply authority from real ledger records and #5 facts rather than public body instructions. Check source bytes and refs, remote refs, squash identity, unexpected locations, and independently successful archival with preserved/failed source cleanup.

### A4. CLI And Propose Expose Explicit Cleanup Without Hidden Lifecycle Work

**Module:** Cleanup command wiring in `cmd/skl/main.go`, existing presentation in `setup/presentation.go`, and Propose guidance in `skills/dev/propose/SKILL.md` with its existing distribution checks in `cmd/skl/main_test.go`.

**Interface:** Preserve `skl propose cleanup` as an independently callable public entrypoint. Preserve predecessor specialized Markdown/default and explicit JSON conventions, including non-work and repair outcomes; exact new output structure is delegated within those boundaries. Propose retains the explicit cleanup call before preparing new slices. This proposal adds no invented flags or archive command family.

**Responsibility:** Present ledger archival separately from removed/preserved source work and pending replication. Do not silently turn a preserved worktree into an archive failure or hide a destructive-operation failure behind overall success. Instructions route mechanics through `skl`, not direct filesystem moves by workers. Status and completion observation never call archival automatically. Later human-decision/publication records remain consumable without their UI being available, and unfinished public publication is not an archive precondition.

**Seam and verification:** Public CLI tests cover independent invocation, no eligible proposals, repair outcomes, offline/pending publication, and distinct archive/source results. Reuse existing skill resource/distribution tests to check Propose's cleanup-before-preparation guidance; this is an instruction distribution check, not an agent-behavior evaluation. Verify ordinary status/Watchdog/merge observation leaves proposal paths unchanged until explicit cleanup.

### A5. Verification Protects Outcomes And Failure Boundaries Together

**Module:** Existing Go test tools in `cmd/skl`, `workflow`, and `setup`, including real temporary source and ledger Git repositories, local remotes/worktrees, and controlled HTTP (`httptest` or the existing transport seam) where forge interaction matters.

**Interface:** The implementation report supplies many-to-many evidence for B1-B6 and A1-A4. A whole-proposal flow may cover completeness, reference retrieval, archived dependency resolution, and offline replication together; focused fault cases preserve distinct collision, stale-Claim, interrupted-move, and source-deletion protection. One scenario need not produce one test, and one obligation may need several evidence sources.

**Failure-aware coverage:** Assert actual on-disk trees, Git refs, exact historical bytes, subsequent CLI reads, and absence of remote branch deletion rather than internal helper sequences. Use existing controlled failures and direct setup, not a new fault-injection framework. Expected identities and contents are literal independent fixture values. Preserve meaningful existing regressions while retiring only assumptions superseded by ledger authority; cleanup of unrelated test suites is out of scope.

**Checks:** Run focused affected Go tests, existing repository-wide `go test ./...`, and `git diff --check`, retaining Full Gate, Audit, and independent Watchdog obligations. Demonstrate regression sensitivity for any concrete bug fixed, without requiring TDD order. No `tasks.md` is needed to mirror the scenarios, no live GitHub writes are required, and no Manual Verification remains uncovered by the agent-observable seams.
