# Run ledger-backed delivery plan

## Approach

Use ADR 0006 and the approved six-slice decomposition. The predecessor already
provides configured Project identity, accepted Contracts, exact ledger readback,
and local commit/replication operations. This slice gives those records their
complete implementation and independent-review consumer path. Do not emulate
old GitHub labels, PR bodies, or timestamp receipts as the new authority.

Both worker phases belong together because Claim ownership, consumed report
references, completion accounting, and the state made eligible by a handoff must
agree. A storage helper or an implementation-only producer is not this slice's
deliverable. Human decision creation, explicit public catch-up, external terminal
observation, and archival remain separate approved capabilities.

This Work Item's `.changes` directory is only the current publication carrier.
Exercise the new interface in isolated configured ledger/source repositories;
do not automatically migrate the project's live queue or reinterpret its
already published Contracts while implementing this change.

## Implementation decisions

### A1 - Keep authority behind the existing CLI seam

- Modify the Workflow Engine's selected-item operations to use the predecessor's
  ledger records, rather than maintaining permanent forge/ledger authority modes.
- Preserve project-scoped selection and existing lane priority. Required
  dependencies use recorded Merged facts; future observation APIs are not part of
  this slice, and no replacement-specific dependency machinery is introduced.
- Use the current CLI, Repository, presentation, and forge transport Modules
  where adequate. Backend Interface shapes that assume authoritative issue/PR
  state, body equality, or successful remote publication must change rather than
  be preserved through a misleading adapter.
- Exact command spelling, private helpers, and mechanical record fields are
  delegated. Do not add a daemon, HTTP service, checkout registry, or loop runner.

### A2 - Make local state changes the handoff unit

- Reserve each Work Item exclusively across local CLI callers, without holding
  that mutation serialization during reasoning, tests, or network operations.
- Validate the current selected-item preconditions under the same local write
  discipline that records the report and resulting state. Do not require the
  entire ledger tip to remain unchanged throughout a worker's execution.
- Commit the phase result and state/Claim update together before treating the
  handoff as locally complete. Preserve results on failures; never rewind a
  later Claim or count the same completed review again.
- Reconcile a known completed effect from available exact evidence, or return
  concrete repair/refusal when uncertain. Do not build a general transition
  journal, invocation database, or history reconstruction mechanism.
- Reuse normal Git identities and the predecessor's replication policy. Remote
  outages leave pending replication; known competing ledger history requires
  reconciliation before additional work is granted.

### A3 - Keep report formats explicit and small

- Persist `implement-report.md` and `watchdog-report.md` with documented schema-1
  YAML frontmatter. Use `go.yaml.in/yaml/v3` pinned initially to `v3.0.5` for YAML;
  use small standard-library delimiter extraction, not a Markdown parser.
- The common `outcome` and `source`/`ledger` namespaces preserve repository
  meanings. Ledger inputs use full commit/path references to the versions read,
  not discovered introduction commits. Archive-safe historical paths remain
  meaningful at those revisions.
- The engine supplies known metadata and review bookkeeping; worker-selected
  outcomes and evidence enter through the supplied semantic command. Preserve
  Markdown as data, not template source or a second outcome parser.
- Reject incompatible required metadata and unknown schema versions explicitly.
  Document meanings, types, requiredness, and ownership through the existing
  resource mechanism. Implement only schema 1 now; do not invent hypothetical
  future adapters or rewrite historical reports.
- Preserve the ability to read already emitted versions when a later schema is
  actually introduced. Reading metadata is not judging completion evidence.

### A4 - Preserve source preparation and verification responsibility

- Prepare branches/worktrees only after Claim, using planned source identity;
  reuse progress on resume and rework. Source Git state stays in source repos.
- Preserve #55's pre-Audit integration and observed-target evidence. When needed
  source inputs are local but networking fails, use the last observed target
  honestly; do not require remote equality merely to persist a local result.
- Required missing source inputs, dirty/unrecorded result ambiguity, wrong
  revisions, or stale execution inputs receive concrete repair/refusal rather
  than invented evidence or destructive reset.
- Contracts are fixed inputs supplied through `skl`, not source-tree material to
  discover, tick, or remove. Retire old endpoint/marker machinery from this path.
- Preserve Full Gate, Audit, independent Watchdog, and post-marker checks.
  Applicable ancestry checks concern the selected source evidence, not arbitrary
  historical discovery or permanent source-object retention.

### A5 - Adapt complete execution skills, not generic facts wrappers

- Build on delivered #44 modules and deferred resources. Supply all known
  references and commands at their applicable step, with explained placeholders
  only for facts genuinely unavailable before worker preparation or judgment.
- Default to Markdown and retain explicit JSON, including no-work and repair
  outcomes. Do not restore the retired worker-to-loop JSON passthrough contract.
- Normal worker procedures must not depend on understanding the ledger's local
  path or record layout. Inputs arrive through `skl`, not direct private Git reads.
- Preserve ADR 0005 construction freedom, many-to-many verification, and
  already delivered delegated-work capability without granting subagents extra
  Claims. This slice does not prescribe one test for every scenario or item.
- Verification reports state full current complete/incomplete declarations.
  Audit axes are Standards and Contracts with `F<n>` findings; watchdog uses
  stable Work-Item-local `W<n>` findings. Historical IDs are not rewritten.

### A6 - Store review accounting in the committed result

- Replace `.watchdog` with report metadata for completed round and reviewed code.
  Starting, resuming, publishing, or recreating a worktree does not advance/reset
  it. A distinct authorized execution at the same code revision is a new review,
  not a duplicate solely because its source SHA matches.
- Keep the existing default of two completed rounds before another failing
  rework result needs human direction. Explicit Needs Human counts as a completed
  review; pass is allowed at any round. Human continuation does not reset count.
- Read required supplied decision evidence for continuation, but leave its
  authoring/authorization/requeue interface to `resolve-human-decisions`.
- Use incremental review only with an available ancestral previous reviewed
  revision; otherwise full review retains count. Keep reviewed/final code
  distinct when permitted Debt Marker comments are added on pass.

### A7 - Attempt basic publication without making it the ledger

- Ordinary phase handoffs accept separately authored temporary human-facing
  bodies and attempt appropriate source/forge publication. Private reports are
  never a fallback body, and source mismatches must not be presented as approval
  of code that was not reviewed.
- GitHub alone translates readiness to draft/non-draft presentation. Record
  attachments and pending work without retaining public prose in ledger files.
- Failure leaves local success intact and temporary content usable when retained.
  Standalone catch-up after lost prose and explicitly selected inline findings
  belong to `recover-forge-publication`.
- Keep the complete private Manual Verification obligations accessible to the
  human; public presentation may omit private operational details, not the
  existence of remaining obligations. Debt Markers are concise and self-contained.

### A8 - Verify at the agreed observable seams

- Primary seams: public `skl` commands and outcomes, real source/ledger Git
  repositories and worktrees, documented persisted report format, rendered
  execution skills/resources, and the existing controlled forge HTTP adapter.
- Existing orientation points include `workflow/implement.go`,
  `workflow/selection.go`, `workflow/handoff.go`, `workflow/review.go`,
  `setup/presentation.go`, and the CLI handlers. Their current signatures and
  forge-shaped internals are not frozen architectural commitments.
- Reuse real-Git helpers, candidate-selection tests, later-Claim protection,
  review-scope/count tests, and controlled lost-response tests. Update tests that
  intentionally assert worktree-loss reset or forge publication as authority;
  retain their relevant behavioral/failure protection at the new seam.
- Exercise a complete locally accepted implementation, independent review,
  rework, and pass, plus concrete pause/refusal cases. Include actual concurrent
  callers, unrelated ledger activity, publication failure, repeated handoff,
  and source-reviewed/final distinctions. Expected results must be independent
  of implementation computations; no new test framework or helper-mock fleet.
