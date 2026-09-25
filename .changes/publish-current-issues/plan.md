# Publish Current Issue Presentations: Architecture

## Authority and assumptions

The approved replacement Explore recap and ADR0007 govern this slice. ADR0007 deliberately replaces ADR0006's pending-forge-publication tracking assumptions for the affected issue/parent paths; ADR0006's local authority, frozen Contracts, privacy, and ordinary Git replication remain. ADR0005 governs verification. #61 (`run-ledger-delivery`, PR #76) and its acceptance prerequisite are already merged. The other two replacement slices are independent, not blockers. All three are children of coordination issue #59; that grouping does not create dependency edges.

The target was inspected at `33726b01404c78b5bd82ad361914e16e7418aa84`. This is context, not a pin preventing ordinary later integration. #63 / PR #78 is failed-attempt evidence, not the inherited acceptance contract or a required implementation base. Preserve historical Contracts and reports. This source `.changes` carrier is distinct from target private-ledger storage.

## A1: One current-view behavior through normal and explicit publication

| Owner | Contractual responsibility |
| --- | --- |
| `cmd/skl` | Inputs, Markdown/JSON operation facts, normal acceptance integration, and explicit current-view publication |
| Existing ledger acceptance/publication modules | Read current authoritative records and retain established attachments through brief safe mutations; preserve local authority |
| `setup.GitHubBackend` | Repository-bound HTTP, attachment observation, issue/parent updates, grouping, bounded retries, and uncertain-create handling |
| Existing authored skills and resource renderer | Specialized agent prose guidance and private evidence access; no CLI-generated descriptions |

Current affected modules include `ledger/accept.go`, `ledger/publish.go`, `ledger/records.go`, `cmd/skl/ledger.go`, and the GitHub adapter. Exact helper decomposition and command spelling are delegated. Do not add an alternate acceptance engine or keep durable reservations in the normal path behind a simpler explicit wrapper. Neither path may delegate forge execution to an agent's direct `gh` command.

## A2: Strict persistence and concurrency seam

Retain existing `SliceState.Issue` and `ProposalMeta.ParentIssue` associations. Remove in-scope authority and ongoing writes for `PublicationState.Issue`, `Grouping`, and `ProposalMeta.ParentPublication`, including reserved states. Do not substitute differently named fields, local files, lock files, or another store. No temporary-body registry is introduced. Ordinary ledger `Push` bookkeeping concerns replication of authoritative records, not this forge-publication protocol; PR-specific `Source`, `Pull`, and `Active` are owned by the independent PR slice.

Current selected-record writes must preserve unrelated fields, later Claims/results, and different established attachments. Reuse existing brief mutation and competing-history safeguards; network calls occur outside those mutations. No long-lived publication coordination is permitted. Safety is bounded checks and honest outcomes, not cross-process/cross-machine exactly-once publication or a rollback transaction. Old in-scope reservation metadata does not require a migration service or operator recovery ceremony.

## A3: Existing evidence and instruction delivery

Consume current accepted documents through the existing ledger/readback interfaces and expose concrete private retrieval operations. Use existing skill/resource rendering for authoring instructions; no parallel renderer, prose generator, classifier, new configuration store, or public copy of private evidence. Public bodies are invocation inputs, not ledger records. Parent/child acceptance publication is a complete usable delivery even while PR publication still uses its preexisting implementation.

## A4: Verification strategy

Use public `skl` acceptance and publication commands over real temporary source/ledger Git repositories and the production GitHub adapter with the existing controlled HTTP seam. Existing `cmd/skl/ledger_test.go` and ledger/setup tests are starting seams, not requirements to retain superseded reservation guarantees. Verify observable identities, request outcomes, grouping, local state/Contract preservation, and absence of prohibited current persistence. Pair interruption/newer-work and obsolete-reservation cases with credible regression sensitivity. Exercise bounded retry behavior and uncertain create response loss without a promise of global deduplication.

Rendered resources and HTTP bodies establish specialization and absence of automatic private evidence export; they do not prove arbitrary agent prose quality. Group evidence many-to-many against B1-B5 and A1-A4. Adapt tests whose expected semantics ADR0007 explicitly replaces, retaining distinct relevant failure protection. Do not add a framework, duplicate suite, or prescribed test-writing order. Run the normal Full Gate (`go test ./...`, `go vet ./...`, applicable formatting/whitespace checks) and retain independent Watchdog and human-only merge.

No `tasks.md` is needed: there is no additional inter-slice sequence, and scenario count is not a task inventory. New persistence, coordination, responsibility transfer, or stronger delivery guarantees require human resolution rather than being framed as implementation discretion.
