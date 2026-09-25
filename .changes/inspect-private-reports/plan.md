# Inspect Private Reports: Architecture

## Authority and assumptions

The approved replacement Explore recap and ADR0007 require optional private inspection while deferring an agent-guided viewer. ADR0006 already makes Git the report store and preserves exact consumed-input references; ADR0005 governs verification. #61's ledger/report foundation is merged. Neither sibling publication slice is a blocker, and this slice must be usable without them. All three belong under coordination issue #59, which is grouping rather than a dependency.

The inspected target was `33726b01404c78b5bd82ad361914e16e7418aa84`. Existing `ShowReference` can retrieve an exact commit/path, while `ShowItem` returns accepted Contracts. The landed Needs Human inbox exposes current blocking reports for its own purpose but is not a general report browser and must not become a prerequisite. This slice fills a CLI access gap over existing storage, not an architectural storage gap. The target observation is context, not a pin against later integration.

## A1: Existing read interfaces, not another subsystem

| Owner | Contractual responsibility |
| --- | --- |
| `cmd/skl` | Work-Item selection inputs, current/historical retrieval and discoverability, equivalent Markdown/JSON facts |
| Existing ledger readback/report modules | Resolve committed current report paths and exact Git references, retrieve original document contents, expose existing historical evidence without mutation |
| Existing report schema | Preserve outcome/source/consumed-input/round semantics and opaque Markdown; no new finding schema |

Current seams include `cmd/skl/ledger.go`, `ledger/readback.go:ShowReference`, `ledger/delivery.go:CurrentReport`, and `ledger/report.go`. Existing status output and the read-only Needs Human report access can be reused where suitable without changing the inbox's scope or adding an approval step. Command spelling and internal helpers are delegated. Reuse the existing machine configuration and repository/Project selection; do not introduce new credentials, configuration stores, registries, or direct-ledger-edit instructions.

## A2: Current files and Git history are sufficient

Current retrieval reads the report at committed HEAD and returns an exact ledger commit/path reference. Earlier versions come from available path-scoped Git history or existing consumed-input references. Do not mandate a generated history index or duplicate every past report in new files. Preserve existing input references for execution provenance; do not reinterpret the history browser as a way for workflow mechanics to infer missing authoritative state.

The interface transports original report contents and recorded facts. It is not a prose summarizer, architectural-decision classifier, finding-resolution engine, or notification/viewer product. Source-code references and ledger references remain distinct; historical source retention is not added. Missing evidence has an honest refusal, not a fallback to unrelated revisions or forge prose. A retrieval has no mutation phase, no Claim, and no network publication.

## A3: Verification strategy

Use existing public CLI tests with real temporary ledger Git repositories, isolated configuration, and reports produced through the existing schema/phase facilities. Appropriate starting points include `cmd/skl/ledger_test.go`, ledger readback/report tests, and the existing inbox's read-only report tests. These are available seams, not a prescription for internal mocks or test organization.

Verify Work-Item-to-current-report access and discoverability, exact historical identity, authored-byte preservation, source-versus-ledger reference distinction, an unavailable historical version, an absent current report, unrelated commits, working-tree edits, later Claims/results, Ready-for-Merge access, and operation without forge access. Observe unchanged Git HEAD, records, and relevant dirty files rather than only asserting that a helper ran. Group evidence against B1-B3 and A1-A3; do not require a test per scenario, a new framework, or an agent-evaluation harness.

Run the normal Full Gate (`go test ./...`, `go vet ./...`, formatting/whitespace checks), preserve independent Watchdog and human-only merge, and report limits of available Git history honestly. Do not add `tasks.md` merely to restate scenarios. Changing storage, introducing a viewer or finding authority, or transferring responsibility beyond the approved interface requires human resolution.
