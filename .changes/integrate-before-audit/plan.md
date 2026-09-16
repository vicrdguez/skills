# Integrate Before Audit Plan

## Approach

Implement the pre-Audit integration portion of [ADR 0005](../../docs/adr/0005-verify-contracts-instead-of-prescribing-test-order.md) on the execution-specific distribution architecture in [ADR 0001](../../docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md). This changes authored worker policy, not Workflow Mechanics or the human merge boundary.

External prerequisites #48 (`render-deferred-resources`), #49 (`render-implementation-executions`), and #50 (`render-watchdog-executions`) must all be Merged before implementation. Use their delivered code in this work branch, bringing it in with ordinary Git preparation as needed while preserving artifact ancestry and existing progress. That preparation is distinct from the later per-submission target observation. Do not pin a future integration SHA or final private module layout now. This slice has no Dependency on `verify-contract-conformance` or the other new ADR 0005 slices. Reuse already-Merged #37/#46; do not reimplement their human-integration mechanics or rewrite published ledgers.

## Implementation Decisions

### Ownership and delivery

- Implement's coherent procedure modules own the late ordinary-Git observation, merge, conflict handling, and final-state verification obligations. Reach the same policy from first work, resumed progress, finding-driven Rework, and applicable inspection continuations. Resume determines remaining work from preserved Git, evidence, and feedback, not a new cursor or receipt; resuming alone does not mandate a second integration.
- Audit owns implementation-phase review of the integrated result. Watchdog independently verifies the fixed submitted result and accounts for integration effects in repeat review without rerunning Audit. Keep the Full Gate, ordinary finding dispositions, review limits, endpoint checks, and human merge boundary.
- The shared Go `text/template` renderer and typed procedure contexts delivered by the prerequisites bind established identities, selected remote, supported target name, and concrete commands. Only the worker acquires the integration SHA at the pre-Audit step. Keep startup metadata-only; do not add target lookup, target ancestry enforcement, a `TargetSnapshot` field, or hidden issue metadata to the engine.
- Retain the deferred public Implement resource `reference/submission.md`. Extend its existing Verification guidance, not its transport schema, to record the integrated SHA and evidence freshness. Bind known resource arguments through the existing typed input contract; leave actual results, dispositions, and integrated-SHA evidence as opaque agent-authored Markdown, never `--input` data or prose parsed by the engine.
- Preserve owner-relative resource lookup, repeated `--input name=value`, `--describe-inputs`, validation, inert supplied data, once-only included definitions, Markdown-first output, and explicit JSON equivalence. Keep plain standalone Implement/Watchdog definition retrieval disabled and legacy queue loops disabled; any affected surviving single-item runner uses the prerequisite-delivered procedures and existing capability handling.

### Reference semantics

| Reference | Responsibility |
| --- | --- |
| Review baseline | Existing round-specific comparison: first Audit uses the normal PR-base merge-base after integration unless explicitly overridden; repeat review retains the prerequisite-delivered previous-head and full-fallback behavior. |
| Artifact Baseline | Immutable accepted Implementation Ledger snapshot; integration and review rounds do not advance it. |
| Artifact Completion | The ledger endpoint with only permitted completion ticks, before retirement; Rework keeps the ledger retired. |
| Reviewed head | Fixed candidate revision for a Watchdog invocation; retain it through continuations and verdict commands, including the existing separate final Debt Marker head allowance. |
| Integrated target SHA | Worker-observed target commit actually merged at the late step, recorded as Verification evidence; not a review baseline, artifact endpoint, startup pin, or submission-enforcement field. |

Integration-aware review must distinguish changes caused by integration or conflict resolution from unrelated code merely inherited from upstream. The former receives the required regression and conformance review; the latter is not a new slice obligation just because it appears in an incremental comparison. Preserve the existing whole-change critical-risk scan and do not reopen accepted preferences without relevant new evidence.

### Module Shapes and Seams

Modify the prerequisite-delivered Implement and Watchdog Skill Modules and their applicable inspection continuations, the shared Audit guidance, and the deferred Submission resource. Current ownership is discoverable through `skills/dev/implement`, `skills/dev/audit`, `skills/dev/watchdog`, the catalog, and the Workflow command delivery paths. These are ownership pointers, not frozen future file splits or Go field names. Update directly affected user-facing guidance that currently assigns all integration to the human, distinguishing pre-Audit worker integration from post-approval human integration.

The agreed verification seams are complete public Execution Skill rendering, applicable inspection continuations, and named-resource retrieval. Ordinary Git remains the worker's integration seam. If a changed deterministic wrapper around that existing Git seam needs executable verification, exercise its public observable behavior with the project's existing temporary-Git tooling; do not add a merge service, backend protocol, or wrapper just to test these instructions.

## Verification Strategy

- Extend existing public rendering/CLI coverage for representative first implementation, resumed progress, and finding-driven Rework procedures plus their applicable inspection continuations. Verify known remote/target binding, late observation and merge ordering, and coherent continuation after preparation. Inspect complete rendered guidance for conflict handling, integration-aware review, evidence freshness, cutoff, and failure semantics rather than adding a substring assertion for every policy sentence.
- Retrieve the deferred Submission resource through generated public commands and inspect its Verification guidance. Reuse #48's resource-contract coverage for owner-relative lookup, typed inputs, validation, inert data, and deferred disclosure. Confirm no integrated SHA or ordinary result prose has become a rendering input, and no startup target acquisition was introduced.
- Reuse #49/#50 and #37/#46 coverage for complete first/repeat Watchdog delivery, fixed invocation heads, same-head review, previous-head comparison and fallback, endpoint integrity, metadata-only startup, opaque Result Document publication, recovery, and mergeability-independent human handoff. Adapt affected expectations without duplicating their suites or adding target-state enforcement. Preserve once-only inclusion, internal-module visibility, Markdown/JSON equivalence, and owned-stub behavior where touched.
- Run the repository's normal checks during implementation and record focused results, relevant inspection evidence, the Full Gate, and material limitations in Submission Verification. These checks demonstrate deterministic delivery and changed executable behavior, not model judgment. Deterministic command tests against isolated fixtures remain allowed; live Workflow exercises, agent evaluations, and exhaustive scenario/variant combinations are not required.

## Sequence

1. After all external prerequisites are Merged, prepare this branch with their delivered code and inspect the actual module and continuation layout. Preserve the published artifact endpoints; choose only incidental layout and implementation details locally.
2. Update the shared pre-Audit procedure and its first/resume/Rework routes, then align Audit, Watchdog, deferred Submission guidance, and directly affected documentation around `behavior.md`.
3. Verify the complete public deliveries and affected ordinary-Git behavior at the agreed seams, retain the existing mechanics, and collect final-state evidence through the normal implementation handoff.
