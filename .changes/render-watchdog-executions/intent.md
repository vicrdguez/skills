# Deliver Tailored Watchdog Executions

## Why

Watchdog workers currently combine generic instructions with separately delivered facts and commands. This leaves deterministic review choices to the worker, hides supplied evidence in transport, and couples normal reports to Pi's legacy JSON relay. Deliver one usable review procedure without changing who judges the work or controls Workflow transitions.

## What

Specialize the shared Watchdog Skill Definition into a complete Markdown Execution Skill for each `next` or `resume` invocation. Bind known review facts where used, defer only facts requiring preparation or judgment, and render verified `submit` outcomes with applicable next instructions. Retain explicit JSON delivery for programmatic callers.

This is approved slice 3, slug `render-watchdog-executions`, implementing ADR 0001's planned execution-specific refinement. Its only native Dependency is `render-deferred-resources`. #40 is a transitive external prerequisite; the approved advisory `Blocked by #40` note belongs on the Proposal's first child issue if published before #40 merges. The implementation-rendering sibling is independent and may land later. Required merged prerequisite code must be present before implementation begins.

## Scope

- Markdown-first `skl watchdog next`, `resume`, and `submit`, with explicit `--format json` and normal Markdown worker reports. Rendering consumes the engine result without repeating selection, Claim acquisition, or publication.
- An engine-owned typed review situation: exact known repository, Work Item, PR, branch, worktree, remote, result paths, fixed reviewed head, Review Count, review number, and previous completed review reference when available. Review history and usable comparison scope remain separate.
- Correct first full review, repeat incremental review, unchanged-head comparison, and repeat full fallback. A lost checkpoint can reset the count, not supplied finding identities or human dispositions.
- #40's metadata-only startup even when project objects exist locally. Supply all available references and concrete preparation/read commands; resolve local-only artifact and comparison facts afterward through a necessary narrow engine inspection continuation or repair.
- Coherent Watchdog Skill Modules using the resource sibling's shared Go `text/template` mechanism. Definitions are included once, private modules are not resources, and `reference/review.md` remains deferred until before dispositions, with settled `--input` values already bound.
- Complete opaque evidence already fetched for the selected item, shown once as labeled data with available provenance. Otherwise provide selected-repository/issue/PR-bound `gh` retrieval instructions for complete required streams, pagination, and failures. Historical ledgers are read after preparation at resolved endpoints.
- Preserve the active Watchdog procedure: a fresh caller-provided Worker Session; independent Full Gate, endpoint integrity, frozen-contract and test-strength verification; verification of Audit claims without rerunning Audit; stable findings, authorized human precedence, critical-class and convergence limits; only permitted Debt Markers; Post-Marker Check and actual final head; verbatim unchecked Manual Verification in the final PR body; human-only integration and merge.
- Explicit no-work, wait, refusal, and failure presentation, with truthful Claim certainty and verified outcomes. Preserve post-#35 review semantics and #39 release-last observational recovery and Claim protection.
- Existing execution-capability input selects an already-supported recipe only where relevant. Install direct Watchdog stubs across supported harnesses, refuse plain workflow-skill retrieval without selecting work, and disable the owned legacy Pi Watchdog loop while retaining one-item use.

## Out of Scope

- ADR 0005's testing, integration, review, or artifact redesign, including pre-Audit integration. Active pre-ADR 0005 templates and scenario-by-scenario red-green rules remain binding.
- New Watchdog `start` alias, generic standalone workflow-skill generation, new harness recipes, a capability registry, queue orchestration, or a replacement Pi loop.
- Target Snapshot, Synchronization Rework, automatic conflict routing, new Claim semantics, persisted rendering sessions, or a second full Execution Skill after preparation. #37's human integration policy remains intact.
- Startup project-object, history, marker, ancestry, or artifact-body inspection; eager fetching or worktree creation; new evidence fetchers; parsing opaque review prose to determine verdicts or finding identities.
- Functional fixes by Watchdog, rerunning Audit, automatic follow-up issues, or agent merge authority.
- A native Dependency on the implementation-rendering sibling, changing its still-supported loop, or removing the shared queue helper while that lane references it. Benchmarks, agent evaluations, and exhaustive combination frameworks are excluded.

## Definition of Done

- [ ] DOD1: `next` and `resume` return complete tailored Markdown instructions by default, with known identities and usable commands bound at their points of use and no facts/JSON wrapper or worker procedural assembly. Ordinary reports are Markdown. Rendering introduces no repeated Workflow effects. (B1, B3, B15)
- [ ] DOD2: First, incremental, unchanged-head, and full-fallback reviews preserve the correct count, review number, available previous reference, finding history, and review obligations. Resume preserves local progress and explicit endpoint overrides. (B2, B3)
- [ ] DOD3: Startup stays metadata-only with or without local objects and returns every available reference plus concrete preparation instructions, without claiming uninspected facts are validated. (B4)
- [ ] DOD4: Post-preparation inspection resolves exact artifact endpoints and comparison scope or gives a precise repair, supplies concrete historical-file read instructions at those endpoints, and preserves the invocation's item, reviewed head, round, remote, overrides, and result paths without another selection, Claim, or full skill. Drift cannot silently replace publication identities. (B5, B6)
- [ ] DOD5: Specialization preserves the complete active review procedure and findings rules, including independent verification, human authorization, convergence, permitted markers, original versus final head, and unchecked Manual Verification. No behavior redesign or engine judgment of opaque prose is introduced. (B1, B2, B9, B11)
- [ ] DOD6: Supplied PR body/Audit ledger and selected feedback are complete, once-only, provenance-preserving data, safe from template interpretation or workflow substitution. Missing delivery has concrete complete retrieval instructions; empty, failed, and truncated reads remain distinct. (B7, B8)
- [ ] DOD7: The Watchdog review resource is retrieved before dispositions through the shared parameterized-resource mechanism, with known values bound, only genuinely later-known inputs left to the worker, no premature verdict input, correct resource ownership, and no private-module exposure or duplicate inclusion. (B9)
- [ ] DOD8: Known execution capabilities specialize only existing applicable recipes; unknown capabilities retain genuine runtime choices without new harness behavior or an Audit invocation. (B10)
- [ ] DOD9: `submit` renders only verified engine outcomes and applicable instructions. Review-limit routing, authorized evidence, release-last recovery, retained Result Documents on failure, cleanup-only warnings, and truthful Claim certainty survive refusals and nonzero failures. Passing beyond the limit remains allowed. (B11, B12, B13, B14)
- [ ] DOD10: Explicit JSON is equivalent to Markdown in instructions, identities, outcomes, and effects; invalid presentation/runtime input fails before avoidable effects; `no_work` and `idle_timeout` are explicit without fabricated review instructions. (B15, B16, B17)
- [ ] DOD11: Plain `skl skill watchdog` refuses with direct-command guidance and no work selection; reasoning definitions and Watchdog resources remain retrievable. Owned Watchdog stubs across supported harnesses directly call `skl watchdog next` and preserve one-item and caller-freshness responsibilities. (B18, B19)
- [ ] DOD12: Installation refresh actually disables an existing marker-owned Pi Watchdog loop before launch or Claim, preserves user-owned files and the one-item runner's Markdown report, and leaves the other lane and shared helper usable while referenced. (B20)

## Manual verification

None. The required observations are available through the approved command, fixture, Git, and installer tests; this slice adds no human-only check.
