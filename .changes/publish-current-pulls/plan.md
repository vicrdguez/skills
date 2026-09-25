# Publish Current Phase PR Presentations: Architecture

## Authority and assumptions

The user-confirmed replacement Explore recap and ADR0007 govern this slice, replacing the affected durable-publication assumptions of ADR0006 while retaining local authority, report schema, ordinary Git replication, privacy, and human merge. ADR0005 governs verification. #61 (`run-ledger-delivery`, PR #76) and its prerequisites are merged. `publish-current-issues` and `inspect-private-reports` are independently useful and not blockers; existing exact-reference retrieval already supports authoring evidence. All three replacement slices belong under coordination issue #59, without dependency edges implied by grouping.

Source inspection used `33726b01404c78b5bd82ad361914e16e7418aa84`; it is context, not a fixed integration target. Preserve the already-landed human-decision behavior and ordinary later integration. Superseded #63 / PR #78 demonstrates the failure, but its stronger recovery/inline guarantees are not requirements for this replacement. Source Artifact Baseline carrier checks do not authorize mutation of target private Contracts.

## A1: Existing handoff, explicit publication, and adapter responsibilities

| Owner | Contractual responsibility |
| --- | --- |
| `cmd/skl` | Normal handoff integration, explicit current-view inputs, and Markdown/JSON outcomes |
| Existing ledger delivery/publication modules | Read current authoritative state/reports, preserve local handoff invariants, and retain established PR attachments through brief mutations |
| Existing source-publication path | Ordinary non-force publication of the recorded branch/revision without changing code or creating evidence |
| `setup.GitHubBackend` | HTTP/authentication reuse, actual object/source observation, current body effects, draft/ready mapping, bounded retries and uncertain-create handling |
| Existing implement/watchdog skill resources and renderer | Public prose instructions, current private references, and invocation guidance without public report dumps |

Current modules include `cmd/skl/delivery.go`, `ledger/delivery.go`, `ledger/delivery_publication.go`, `ledger/records.go`, `ledger/report.go`, and `setup/delivery_publication.go`. Modify causes in their existing responsibilities rather than adding a second delivery engine or wrapping retained reservations. Normal and explicit entrypoints must obey the same accepted semantics. Private helper layout and exact command spelling remain delegated.

## A2: No durable publication protocol

Retain `SliceState.Submission` as the established repository/number identity. Remove ongoing in-scope authority/writes for `PublicationState.Active`, `Source`, and `Pull`; do not introduce the superseded branch's `Phase`, `PublishedSource`, `PullBody`, or finding receipts. Current result selection uses ordinary state and report metadata through the existing schema reader. Do not change report schema or interpret arbitrary Markdown to create a second publication authority.

The issue slice owns Issue/Grouping/ParentPublication removal. Existing ledger `Push` information concerns replication of authoritative records and is not a new forge-publication exception. Scope removals to the PR path; do not require the sibling issue slice to land first or erase its unrelated facts while it remains on the prior implementation.

Old PR reservation fields must not gate the new operation. Use ordinary affected writes rather than a migration service, historical rewrite, or operator recovery ceremony. Network work never holds a worker Claim or the brief ledger mutation lock. Later local results, Decisions, and Claims survive attachment bookkeeping; known different attachments are not reassigned. No publication lease, lock-file substitute, persistent operation owner, deferred attempt queue, or corrective rollback transaction is permitted. Temporary stale public content and unresolved external uncertainty are accepted limitations, not invitations to strengthen guarantees.

## A3: Prose and instructions use existing seams

Public content remains agent-authored invocation input. Supply available report/source references, private retrieval instructions, and specialized current-phase authoring guidance through the existing catalog/resource renderer. No temporary-body registration or new config is required. A missing body prompts reauthoring rather than recovery of saved bytes. Keep complete human Manual Verification privately retrievable and full reports private. Do not turn the excluded inline-finding feature or future viewer into a prerequisite for ordinary PR presentation.

## A4: Verification strategy

Use public `skl` handoff/publication interfaces, real temporary source and ledger Git repositories, the production adapter against controlled HTTP, and shipped resource rendering. Existing `ledger/delivery_publication_test.go` and command-level delivery tests provide appropriate seams. Cover latest-result selection, obsolete reservations referencing different report paths/content, sequential offline local progress, lost create responses, safely bounded retries, non-force source publication/refusal, observed source identity before approval, preservation of later Claims/results, and absence of forbidden current persistence. Verify body privacy and native draft/ready effects through HTTP rather than only internal stubs.

The W3 sequence is behavioral regression evidence, not an instruction to resurrect the old reconciliation operation. Demonstrate the wrong behavior or a faithful persisted-interruption reproduction and its correction. Do not require the superseded W1 restorative rollback or W2 inline-publication behavior: ADR0007 deliberately changes those obligations. Retain required source/approval safety and private findings.

Group evidence against B1-B5 and A1-A4 without a scenario-to-test cardinality or construction-order prescription. Adjust tests of explicitly superseded guarantees while retaining relevant failure-mode protection. Use existing Go tooling, the normal Full Gate (`go test ./...`, `go vet ./...`, formatting/whitespace checks), independent Watchdog, and human-only merge. Tests do not promise live GitHub availability, distributed transactions, permanent source retention, or arbitrary prose quality. No new test framework or `tasks.md` is warranted.

New persistence, stronger guarantees, responsibility transfers, or additional coordination require human agreement rather than being called implementation choices.
