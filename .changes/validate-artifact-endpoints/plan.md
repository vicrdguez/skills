# Validate Marked Artifact Endpoints Plan

## Approach

This is approved slice #1, `validate-artifact-endpoints`, with no blocking Dependencies. Implement one coherent artifact contract across the existing CLI, rather than adding a validator that publication, startup, or handoff can bypass. Baseline preparation remains Agent Worker Git work; `skl` validates and transports facts. All implementation and documentation changes described here are future work.

The inspected base is `2f68e43`. It includes the #32 opaque Work Item/Submission identities and repository-bound publication integration. Current paths are `workflow/ledger.go`, `workflow/proposal.go`, `workflow/implement.go`, `workflow/handoff.go`, `workflow/watchdog.go`, `workflow/review.go`, `cmd/skl/{main,implement,watchdog}.go`, and `setup/presentation.go`; command rendering no longer lives in the workflow module. Do not regress those identity or repository-binding corrections or expand this slice into the remaining backend-independence migration.

## Implementation Decisions

### Marker Identity And Scope

- New proposal preparation creates a complete ledger commit with subject prefix `[baseline] <slice-slug>` at the pushed publication head. Completion preparation uses `[completion] <slice-slug>` while all artifact files still exist. A subsequent ordinary commit removes the ledger before review. No issue number, placeholder issue, CLI-created commit, or amend is needed.
- Match the literal, case-sensitive prefix at subject start, followed by the exact slug token. End of subject or a space introducing optional explanatory text terminates the token. Body text, leading decoration, and longer slug names do not match.
- The relevant history is the commits reachable from the selected fixed branch or Submission head, including merge parents, for this exact slug. Do not use `--all`, a repository-wide issue/PR ownership audit, a target cutoff, or first-parent tree-transition discovery. A single commit reached through several parents is still one marker.
- The slug is the declared proposal slice and its existing branch/ledger attachment, not a numeric or opaque Work Item ID. Distinct Work Items must use distinct slugs; duplicate matching markers in relevant history refuse with their SHAs. Do not introduce a global uniqueness service or take over slice #5's attachment resolution.
- Missing required markers are actionable failures. Distinguish absence from ambiguity and unavailable Git evidence. An explicit input never resolves ambiguity by selecting one candidate.

### Snapshot Invariants

- A baseline is a commit containing the accepted ledger directory with at least `intent.md` and `behavior.md`. Optional files are frozen by their presence at Baseline; their entire relative path set participates in comparison. Every entry is mode `100644`, type `blob`; reject executable files, symlinks, gitlinks, and a non-directory ledger root.
- Endpoint path sets and bytes are identical except that an existing unchecked Markdown task box may become a lowercase checked box outside Manual Verification. Preserve `ledgerCheckbox` syntax and `ledgerBoxes` heading scope; do not normalize whitespace, case, newlines, order, or prose before comparing. Manual Verification stays unchecked at both endpoints. Every automated checkbox is complete at Completion.
- Resolve available full commit identities and require Baseline to be an ancestor of or equal to Completion, and every resolved endpoint to be reachable from the inspected head. Equality is permitted for explicit historical evidence already satisfying completion. Marked new work normally has separate baseline and completion commits.
- Completion contains the artifacts. Review requires a later head where the entire ledger path is absent, which establishes retirement after Completion. Do not require deletion to be Completion's immediate child or discover a unique deletion SHA. Keep the existing present/retired phase vocabulary where useful; any legacy deletion output must not imply first-parent-derived Completion.
- Do not enforce an `absent/present/absent` transition count, first-parent merge-tree equality, monotonic intermediate ticks, or content validity at every commit. Endpoint-invalid edits still fail; intermediate edits restored by the compared endpoint pass. Metadata scans for marker uniqueness and ancestry are not content audits.

### Phase Requirements

| Invocation/phase | Required artifact evidence |
| --- | --- |
| New proposal publication | Unique marked Baseline at the pushed head, complete accepted path set, valid baseline shape and manual boxes; no Completion required |
| Inspect or implementation startup before Completion | Valid Baseline and reachable current head; when the ledger is present, compare Baseline with that head as a provisional endpoint allowing unfinished automated boxes; report no Completion and do not claim review readiness |
| Inspect with Completion present | Validate the named pair, report present until head retirement, then retired; invalid or incomplete Completion is a violation |
| Implementation submit, finding-driven Rework startup, Watchdog startup and submit | Valid completed pair and absence at the fixed review/handoff head; Watchdog's optional final Debt Marker head also keeps artifacts absent |
| Implementation Needs Human | Unambiguous valid Baseline and available, validly ordered supplied endpoints; no requirement for finished automated boxes, Completion, or retirement merely to preserve an incomplete implementation |

Provisional inspection loads only the current endpoint, never intermediate snapshots. Audit before final ticks can therefore report current contract integrity without inventing a Completion. It must label completion/retirement evidence pending rather than claim readiness. Needs Human remains a preservation path, not approval: incomplete work and reported provisional artifact violations may be carried to the human, as today. Do not gate that pause on the full review contract or silently label an incomplete snapshot a finished Completion. Invalid or ambiguous endpoint identity still refuses; no-body pauses retain the existing check that implementation outside the ledger or an existing Submission requires a pushed draft body.

All callers of `InspectLedger` must agree on the phase they require. Preserve fixed local/remote heads, Claims, opaque prose, and existing transition behavior. Refusals use existing public outcome conventions (`fix_required`, or inspection violations) with concrete slice, endpoint/path, and fetch/explicit-input/human-resolution guidance. Do not recommend rewriting or force-pushing history to manufacture markers.

### Explicit Markerless Evidence

- Add `--artifact-baseline <full-sha>` and `--artifact-completion <full-sha>` only to `implement inspect`, `implement next`/its existing `start` alias, `implement resume`, `implement submit`, `implement needs-human`, `watchdog next`, `watchdog resume`, and `watchdog submit`. They are not global flags and are not accepted by `propose publish`, `cleanup`, `status`, `setup`, or `skill`.
- An input substitutes only for an absent marker of that endpoint in the selected history. A unique existing marker is authoritative and cannot be overridden; multiple markers refuse even if explicit values are supplied. A missing endpoint may be supplied while the other is uniquely marked. No automatic inference from first appearance, deletion parent, issue body, or old journal is allowed.
- Require available full commit SHAs, not symbolic refs, abbreviations, tags naming non-commits, or silently fetched replacements. Apply the same endpoint and ancestry checks as marked work, with the phase-specific incomplete allowance for Needs Human. Baseline-only progress can supply only Baseline; review needs both endpoints resolved.
- Existing queue selection stays unchanged. Explicit inputs on current `next` apply only to its selected item; they do not choose an item, migrate a queue, or let ordinary markerless `next` succeed without operator input. Explicit resume keeps its existing claimed-item requirements.
- Carry caller-supplied values separately from automatically resolved endpoint facts through transient instruction facts to `setup/presentation.go`. Generated resume, inspect, and handoff commands must retain the supplied flags, as must actionable continuation instructions and worker-directed Needs Human/Audit invocations. Do not automatically turn every discovered SHA into an override or add these values to existing persisted transition records.
- Add no Adoption command, record, fallback, or marker-writing operation. Workers on the former workflow may finish with the old CLI. Switching an existing worker to the new CLI is an explicit handoff using accepted endpoint SHAs; keep that worker on its old CLI until that handoff, rather than silently upgrading its protocol mid-session.

### Scope And Independent Delivery

The endpoint contract ships without slices #2 through #5. Keep target pins/containment and Synchronization Rework until #2, existing review counts/history until #3, transition journals and reconciliation until #4, and queue discovery/local-object startup prerequisites plus embedded Watchdog artifact files until #5. Preserve those behaviors only until their owner slices land, not as new permanent mandates in this proposal. If a sibling lands first, integrate with its current interface rather than reintroducing removed policy.

No new marker-related global backend reads are permitted; the existing broad `ImplementationItems`/publication discovery behavior is not redesigned here. This distinction keeps marker lookup local without accidentally requiring candidate filtering as a Dependency.

The planned capability and ADR correction paragraphs describe the whole candidate-first change. For this approved scope, ADR 0003's first-parent identity, transition-count, intermediate-edit/merge-tree mandates, and immediate-child deletion requirement are superseded by this endpoint contract, not constraints the implementation must preserve. Its retirement, historical contract, and human-verification meanings remain. ADR 0004's lazy automatic Adoption mandate is superseded only for markerless artifact evidence. The final docs task must explicitly mark these scoped supersessions in the ADRs themselves. Do not claim ADR 0002's target/journal mandates, ADR 0004's synchronization/review policies, or startup content delivery have been superseded by this slice.

### Module Shapes And Seams

**Modified Workflow module:** `workflow/ledger.go` owns marker resolution, endpoint validation, phase facts, and localized violations behind the existing inspection interface, extended only as needed for explicit evidence and phase policy. Its Git dependency remains concrete process execution, not a new Repository interface. Reuse endpoint file reading and checkbox rules; remove the old per-commit content loop rather than maintaining two validators. `artifactBaseline` in `workflow/proposal.go` applies publication-specific head requirements. `workflow/implement.go`, `handoff.go`, `watchdog.go`, and `review.go` consume the same result and require their appropriate phase.

**Modified CLI and instruction module:** `cmd/skl/{main,implement,watchdog}.go` parse only relevant flags and invoke Workflow behavior. `catalog.go` holds transient endpoint/command facts and rendered guidance; `setup/presentation.go` formats provider-native IDs and concrete commands with existing quoting and remote behavior. Backend identity and repository binding remain integration-owned. Existing Backend adapters cover state/transport variation; no new seam is justified for Git or endpoint tests.

**Agreed test seam:** use `newApp(...).Run` in `cmd/skl/main_test.go`, `implement_test.go`, and `watchdog_test.go` with real temporary Git repositories. Reuse `proposalRepository`, `prepareSlice`, `runGit`, `implementationMemory`, and the existing public CLI helpers as appropriate; adjust marker-era fixtures rather than bypassing flags with direct calls to `InspectLedger`, `permittedTicks`, or packet constructors. For publication binding, opaque transport, and request effects, reuse the controlled `httpRoundTripFunc` adapter and `setup.NewGitHubBackend` pattern in `cmd/skl/main_test.go`; the current real HTTP requeue fixture is in `cmd/skl/watchdog_requeue_test.go`. No new test-only production seams, mock Git graph, private-helper suite, or Cucumber runtime.

One scenario is one named public behavior test; outlines are table-driven cases within it. Shared phase/marker cases may exercise several existing commands in the same test to prove the full path, rather than spawning a separate test for each helper or flag. Update obsolete direct history tests so they do not retain superseded expectations; keep unrelated coverage intact. Instruction checks use public `skl skill`/resource retrieval and generated packets for marker creation, Audit provisional integrity, endpoint-only Watchdog checks, override propagation, and unchanged unrelated policies.

### Performance Verification

B14 uses real short and long histories with the same small artifact path set, including a representative history of at least 1,000 unrelated commits and restored intermediate artifact churn. Exercise `implement inspect` through the CLI and use native Git tracing to verify artifact tree/blob reads reference only resolved endpoints/provisional head and head presence. Compare subprocess/content-load growth between history sizes: no per-history-commit process or content-read slope is acceptable. Metadata-only marker enumeration and Git's ancestry traversal are allowed; avoid an exact brittle total subprocess count or internal-helper call assertions.

Leave a runnable Go benchmark at the same CLI seam, named `BenchmarkArtifactEndpointInspection`, reporting history sizes and normal benchmark measurements (and trace-derived work counts where useful). Fixture creation stays outside the measured interval. Record representative before/after CLI data in the implementation's Result Document; do not gate tests on elapsed milliseconds. Run `go test ./...`, the focused B1-B14 public behavior tests, and `go test ./cmd/skl -run '^$' -bench '^BenchmarkArtifactEndpointInspection$' -benchmem` during implementation verification. No benchmark is run or claimed by this proposal-writing task.

## Sequence

1. Implement B1-B3 as public CLI red-green cycles establishing marker preparation, scope, and diagnostics; replace the old discovery path at its shared owner.
2. Implement B4-B8 as public CLI red-green cycles for endpoint invariants, retirement, restored history, and provisional inspection/current startup.
3. Implement B9-B13 as public CLI red-green cycles carrying the same evidence through submission, incomplete pause, Watchdog, and explicit-input continuation.
4. Implement B14 and collect representative-history CLI measurements, then run the complete existing Go gate without changing unrelated Workflow expectations.
5. Complete the final paired-guidance/ADR/capability task. Update `skills/dev/propose/SKILL.md`, `skills/dev/implement/SKILL.md`, `skills/dev/audit/SKILL.md` (including deterministic checks and the Artifacts brief), `skills/dev/watchdog/SKILL.md`, applicable resources, and generated instructions. Update `docs/capabilities/work-item-lifecycle.md` and the scoped supersession notes in ADRs 0003/0004; consult the existing capability format resource. Leave all other planned candidate-first policies planned, and verify guidance through the public CLI.
