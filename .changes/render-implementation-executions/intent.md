# Deliver Tailored Implementation Executions

## Why

Implement currently delivers a structured outcome containing largely generic
definitions and a separate facts object. The worker must reconcile repeated
instructions, choose the applicable procedure, and substitute facts itself.
That leaves resolved alternatives in the execution and makes an appended Work
Start section insufficient evidence that the complete instructions are correct.

## What

Deliver one engine-specialized Execution Skill as the default Markdown output
of Implement startup, with narrow later continuations and Markdown handoff
outcomes. Bind established facts where they are used and preserve genuine
future-dependent decisions. Explicit JSON remains an equivalent transport of
the same operation, not a second operation or authority for success.

This is approved slice 2 of #44, slug `render-implementation-executions`, under
ADR 0001's planned refinement. Its engine-native Dependency is the sibling
`render-deferred-resources`. Prerequisite #40 applies transitively; the approved
advisory `Blocked by #40` on the first child's issue does not replace that native
sibling Dependency. Dependencies must be Merged, not merely Ready for Merge.
The baseline is intentionally authored before #40 lands and specifies its
accepted metadata-only startup, not the marker-inspecting code at this branch's
creation point. ADR 0005 is accepted but pending after #44 and is not implemented
by this slice.

## Scope

- Default Execution Skill Markdown for `skl implement next`, its existing
  `start` alias, and both explicit-item and conventional-worktree resume.
- Default Markdown outcomes and applicable instructions for `implement submit`
  and `implement needs-human`, retaining explicit `--format json`.
- Explicit engine procedure inputs for initial work, resumed/draft progress,
  and finding-driven Rework; complete literal binding of established identities,
  references, locations, selected remote, and operation commands.
- Post-#40 metadata-only startup and worker-owned Git preparation, followed by
  narrow read-only inspection continuations or repairs for later facts.
- Specialization of the complete bundled Implement, TDD, Audit, Design, and
  Domain instructions through the sibling's shared module/template mechanism.
- Complete already-fetched evidence as labeled data, or precise selected-item
  retrieval instructions for pending evidence, without rendering-driven reads.
- Deferred, parameterized resources at their existing procedural steps, with
  literal known inputs and explained later inputs under their owning skills.
- Audit recipe selection from established adapter capabilities; a small runtime
  choice when capabilities are genuinely unknown.
- Accurate empty, waiting, repair, failure, and verified-handoff reports, with
  unchanged eligibility, Claims, transitions, and observational recovery.
- Direct installed Implement activation, read-only refusal of generic Implement
  retrieval, and ownership-safe disabling of the legacy Implementation Pi loop
  while preserving single-item Pi use.
- Representative complete rendered fixtures and focused checks at the existing
  CLI and installation seams, plus bounded usage and upgrade documentation.

## Out of Scope

- ADR 0005 changes to TDD, construction order, Audit, artifact conventions,
  delegation, testing policy, or pre-Audit integration.
- Blanket target synchronization, Target Snapshot, Synchronization Rework,
  automatic integration/conflict resolution, or agent merging removed by #37.
- Reimplementation of #40 selection/preparation or #39 handoff recovery; new
  execution cursors, journals, persisted rendering sessions, or metadata stores.
- An Implement-only renderer, capability registry, new fetcher, earlier evidence
  hydration, speculative harness integrations, or general composition framework.
- Watchdog execution specialization or a dependency on its sibling's landing
  order; shared Pi helper cleanup while another lane still calls it.
- Keeping the old Pi loop alive with JSON flags, redesigning queue orchestration,
  compulsory redesign/domain documentation work, or unrelated skill rewrites.
- New test frameworks, internal-helper test seams, benchmarks, exhaustive input
  combinations, or claims that rendered-fixture checks prove agent behavior.

## Definition of Done

- [x] DOD1: Startup and handoff commands default to applicable Markdown, with no separate facts assembly or exact-JSON worker relay; explicit JSON preserves the same operation, outcome, and effects. (B1, B15, B16)
- [x] DOD2: Explicit procedure inputs distinguish initial, resumed/draft, and finding-driven work; every established reference is bound without exceeding post-#40 metadata-only startup or inferring artifact progress from lifecycle labels. (B2, B3)
- [x] DOD3: After preparation, read-only inspection supplies narrow progress-specific continuations or repairs, preserves invocation identity, avoids duplicate Completion or ledger recreation, and refreshes integrity when required. (B4, B5)
- [x] DOD4: Complete bundled output is specialized once per included definition while preserving current task judgment, TDD, verification order, Audit severity/dispositions, scope, frozen ledger, human pause, and human-only integration/merge. (B6)
- [x] DOD5: Established execution capabilities select only supported Audit recipes; unknown capability retains a runtime choice, and invalid format/capability inputs fail before avoidable effects. (B7, B14)
- [x] DOD6: Already-fetched evidence is complete, labeled, provenance-preserving data; pending evidence has concrete complete retrieval instructions, and no-PR, empty, and failed evidence remain distinct. (B8, B9)
- [x] DOD7: Shared deferred resources receive literal settled inputs, explain genuinely later values, retain owner names and visibility, and are disclosed only at the appropriate step. (B10)
- [x] DOD8: Empty/waiting, repair, operational-failure, refused, and interrupted outcomes accurately state status and Claim certainty with applicable recovery; rendering preserves engine error semantics and #39 verified handoff/release-last behavior. (B11, B12, B13, B17)
- [x] DOD9: Installed Implement stubs run lane `next` directly and consume its full instructions; generic Implement retrieval refuses read-only while reasoning skills and Implement resources remain usable. (B18, B19)
- [x] DOD10: Installation disables existing owned legacy Implementation Pi loops, preserves user-owned files and single-item Markdown-reporting runners, and leaves any still-used shared queue helper intact. (B20, B21)
- [x] DOD11: B1-B21 have scenario-aligned checks at the approved seams, representative fixtures cover complete executions and continuations, existing relevant regressions pass, and usage/upgrade docs explain rebuilding and refreshing owned installations.

## Manual verification

None. The accepted verification surfaces are the in-process CLI, real temporary
Git repositories, controlled backend fixtures, and temporary installed homes.
