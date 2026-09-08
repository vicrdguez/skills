# Drain Pi Queues Through Shared Loop Skills

## Why

PR #19's Pi-only prompts duplicate continuation policy, trust worker-returned state, and stop after an arbitrary item limit. Operators need independent implementation and Watchdog lanes that use the Workflow Engine's verified dispatch contract, while preserving fresh worker and Audit contexts and native role configuration.

## What

Approved slice 3, `drain-pi-queues`, is blocked by `continue-verified-dispatches`, transitively by `wait-for-claimable-work`. Deliver authoritative shared `implement-loop` and `watchdog-loop` Skill Definitions and thin Pi discovery and worker adapters using the existing `pi-subagents` extension. Later OpenCode and Codex slices depend on these shared skill names; their adapters are not delivered here.

The supervisor requests a dispatch, launches one fresh worker with the CLI-supplied startup command, awaits termination, and invokes that dispatch's CLI-supplied continuation command. Only the CLI action authorizes another dispatch. Exact command syntax and action vocabulary belong to the predecessor, not this slice.

## Scope

- Embed and catalog the two loop skills separately from the existing single-item `implement` and `watchdog` skills; distribute them through existing Skill Stubs and instruction retrieval.
- Use the predecessor's bounded waiting and verified continuation, with a 15-minute idle window per request and no loop attempt or item cap. Preserve immediate `next` by default, optional bare `--wait` meaning 15 minutes, explicit wait durations, and configurable `--poll` defaulting to 30 seconds as predecessor-owned CLI behavior.
- Run independent lanes in separate supervisor sessions, at most one supervisor and one active Work Item worker per lane per project. Audit's two child reviewers are not competing queue workers.
- Retain `pi-subagents` for both queue delegation and the two parallel, fresh-context Audit axes. Preserve Audit judgment, aggregation, and single-item worker responsibilities.
- Install managed global Pi role definitions named `skl-implement`, `skl-watchdog`, `skl-audit-standards`, and `skl-audit-artifacts`. Default all to `openai-codex/gpt-6-astra`, with `thinking: low` for implementation and `thinking: high` for the other three, in role frontmatter.
- Support native project `.pi/settings.json` `subagents.agentOverrides.<role>.model` and `.thinking` without replacing the role persona or overriding those settings at dispatch. Leave the supervisor model untouched.
- Document prerequisites and actionable stops for missing delegation, exact configured models, credentials, supported thinking, nested Audit delegation, and effective execution limits. No silent model substitution, fallback execution, or worker work in the parent.
- Retire PR #19's old dedicated prompt and runner source, `queue-next.mjs`, fixtures, helper test, and their embedding/install path. Keep shared Workflow code and the extension mechanism. Previously installed obsolete files remain byte-for-byte untouched; removal instructions are manual, not migration code.
- Update installation/operation documentation, the lifecycle capability, and the Pi-only/shared distribution decision in ADR 0001. Preserve later-harness planned scope and the separate backend-independence project.

## Out of Scope

- Implementing or redesigning waiting, dispatch identity, handoff verification, action vocabulary, or recovery commands owned by the predecessors.
- OpenCode or Codex worker adapters, a replacement Pi extension, cross-harness model configuration, provider-neutral Backend changes, or new Workflow State.
- Automatic cleanup or migration of installed legacy files, rewriting project overrides, global role override settings, user permissions, execution limits, or the supervisor model.
- Same-lane competing consumers, atomic Claims, a shared status view, cross-supervisor coordination, automatic worker replacement or Claim recovery, and automatic merging.
- Live harness smoke tests, model-quality tests, model benchmarks, or claims of measured performance or universal model availability.

## Definition of Done

- [ ] A1: Both shared loop skills are discoverable and retrievable through the existing installation/catalog interface, remain separate from one-item skills, and contain the authoritative loop instructions rather than duplicating them in Pi adapters. Covered by B1.
- [ ] A2: Retrieved loop contracts require a fresh sequential worker using the returned startup command, followed by the returned continuation command after termination regardless of worker-reported success; only the CLI action controls further dispatch. Supervisors report items, CLI-confirmed outcomes, running totals, and stop reasons in their own conversations. Covered by B2.
- [ ] A3: Retrieved loop contracts request 15-minute bounded waiting without an attempt cap, retain the predecessor's immediate/optional-wait/poll contract, and define idle timeout as lane-local rather than global completion. Covered by B3.
- [ ] A4: Retrieved contracts preserve lane independence, one active Work Item worker per lane/project, cold worker contexts, the review-bounce allowance, and human merge authority. Covered by B4.
- [ ] A5: Retrieved contracts stop actionably on incomplete or ambiguous dispatch/continuation and operational failure, preserve Claims and partial work, and never infer handoff from worker prose or automatically replace/recover a worker. Covered by B5.
- [ ] A6: Installed Pi worker roles and retrieved Audit instructions retain extension-based delegation and both parallel cold Audit axes, with one authoritative set of judgment briefs and no Pi in-parent fallback. Covered by B6.
- [ ] A7: Installed managed roles have the exact Astra model and thinking defaults in frontmatter, no fallback model chain, and thin startup/axis instructions; no dispatch-level model pin defeats native overrides. Covered by B7.
- [ ] A8: Installation is repeatable and refreshes only currently managed assets, preserving unrelated files, unowned collisions, project overrides, global settings, permissions, limits, and supervisor configuration. Emitted guidance supports model-only and model-plus-thinking project overrides without persona replacement. Covered by B8.
- [ ] A9: Emitted Pi instructions identify prerequisites and halt with actionable repair guidance when delegation or the configured model/thinking profile is unavailable, without parent execution or silent substitution. Covered by B9.
- [ ] A10: Fresh installation omits all retired Pi-only assets, while reinstallation neither deletes nor rewrites pre-existing legacy copies. Their obsolete source/helper tests/install wiring are retired, shared Workflow code and extension use remain, and manual removal guidance names the old installed paths. Covered by B10 and C1.
- [ ] A11: README, ADR 0001, and the lifecycle capability accurately distinguish delivered shared/Pi behavior from later OpenCode/Codex slices, record native role overrides and prerequisites, and make no runtime-proof, performance, or model-diversity claim for the shipped same-model profile. Covered by D1-D3.

## Manual verification

None.
