# Run ledger-backed delivery

## Why

Implementation and independent review must exchange durable results without
depending on editable PR bodies, public comments, or successful forge calls.
One locally committed handoff must preserve the result and make the next phase
eligible without a human joining those operations together.

## What

Complete the project-scoped implementation and watchdog path for Work Items
accepted into the private Workflow Ledger. Keep both phases on one authoritative
Claim, report, reference, and transition model through Rework, Needs Human, or
Ready for Merge. Preserve independent verification and the human merge boundary.

ADR 0006 is the design authority. This `.changes` directory is the current
publication carrier for implementing that design, not the target ledger layout.
Its existing carrier checkboxes do not permit mutation of target private
Contracts. ADR 0005 governs verification: evidence may be many-to-many, and this
Work Item does not prescribe test organization or construction order.

## Dependencies

- `record-private-proposals`: configured Project binding, accepted frozen
  Contracts, committed ledger operations, and exact CLI readback.
- Issue #55, `integrate-before-audit`: integrate and record the observed target
  before Audit, adapting its network assumptions to the accepted local-first
  policy rather than implementing a competing integration procedure.
- The predecessor brings the applicable #44/#51 instruction and verification
  changes through #53. Preserve already delivered delegation behavior; #54 is
  not an additional blocker merely because it touches implementation guidance.

## Scope

- Project-scoped selection, exclusive non-expiring Claims, explicit resume and
  release, and post-Claim source preparation that preserves existing progress.
- A complete implement/watchdog consumer path, not an implementation-only
  producer waiting for a later review backend.
- Schema-1 `implement-report.md` and `watchdog-report.md`, exact source/ledger
  references, opaque Markdown evidence, and atomic local report/state handoffs.
- Completed-review accounting in reports, incremental/full review selection,
  repeated-request safety, and the existing automatic-rework limit.
- Frozen Contract consumption, current completion-and-evidence tables, Audit's
  Standards/Contracts axes and `F<n>` findings, `W<n>` watchdog findings, and
  short self-contained Debt Markers.
- Specialized worker instructions and deferred resources following #44, with
  Markdown defaults and explicit JSON outcomes for callers that request it.
- Normal human-facing PR publication attempts after implementation and review,
  with adapter-owned draft/readiness presentation and honest pending outcomes.

## Out of Scope

- The human inbox, conversational authorization, or decision-writing interface
  supplied by `resolve-human-decisions`; consume recorded directions without
  inventing human approval.
- Standalone publication catch-up, reauthoring lost public bodies, or selected
  inline publication, supplied by `recover-forge-publication`.
- New merge/closure observation, terminal proposal accounting, and archive
  operations supplied by their dedicated slices.
- Engine interpretation of completion prose, contract amendments, requirement
  databases, legacy marker discovery, source-code retention guarantees,
  generated result IDs, or a separate results/event journal.
- Distributed coordination, Claim expiry, loop launchers, HTTP services,
  permanent dual authority, automatic migration, or a Markdown parser.

## Definition of Done

- [x] B1: Project-scoped selection and exclusive Claims preserve lane priority, normal dependency gating, and concurrency across different slices.
- [x] B2: Claimed work prepares or safely reuses source workspaces and consumes the exact private Contract without source-tree contract machinery.
- [x] B3: Execution skills and deferred resources bind known facts and commands without worker-owned ledger bookkeeping or engine-resolvable procedure choices.
- [x] B4: Versioned phase metadata round-trips through the documented persisted format while Markdown remains unchanged and unsupported formats are refused explicitly.
- [x] B5: Local handoffs commit report and state together, survive interrupted delivery, and cannot count a retry twice or overwrite later work.
- [x] B6: Implementation supplies explicit current completion evidence and Audit dispositions without the engine judging prose or rewriting Contracts.
- [x] B7: Watchdog drives pass, rework, and human pause with durable completed-review counts, including same-code new reviews and retry distinction.
- [x] B8: Review scope and reviewed/final source revisions preserve the independent-review and permitted-marker rules.
- [x] B9: Resume/release and network-failure handling preserve source progress and fixed inputs without automatic expiry or fabricated availability.
- [x] B10: Normal PR attempts use explicitly authored public material and adapter-owned draft/readiness behavior without gating local success.
- [x] B11: Fixed-item operations do not reconstruct unrelated project history, inspect source markers, or use forge conversations as authority.
- [x] B12: Existing CLI and real-Git verification covers the complete delivery path and failure boundaries, with schema and execution-resource documentation available through `skl`.

## Manual verification

None. CLI behavior, persisted records, generated instructions, concurrency, and
forge interactions are observable with real local Git repositories and the
existing controlled HTTP test seam. Human judgment remains part of the operating
workflow, not an untestable implementation acceptance substitute.
