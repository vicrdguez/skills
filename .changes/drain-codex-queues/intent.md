# Drain Codex Queues

## Why

Codex users need independent implementation and Watchdog queue supervisors without carrying one Work Item's reasoning into the next or moving Workflow Mechanics into harness instructions. The shared queue contract must work through Codex's native delegation and role configuration.

## What

Approved slice 5, `drain-codex-queues`, blocked by `drain-pi-queues` being Merged. That predecessor owns the shared `implement-loop` and `watchdog-loop` Skill Definitions, shared worker-role contract, and CLI dispatch/continuation contract. This slice delivers the thin Codex integration; it is not blocked by OpenCode.

Two separate Codex supervisor sessions invoke the shared loop skills. Each dispatches one fresh worker at a time for its lane and project. Implementation delegates the two Audit axes to independent fresh native subagents. Installed native role files supply worker defaults while the CLI remains authoritative for startup and continuation.

## Scope

- Codex discovery of the inherited loop skills, distinct from the existing one-item `implement` and `watchdog` skills.
- Native Codex worker and nested Audit delegation, with new-session contexts rather than forks of parent conversation history.
- Thin managed global defaults under `~/.codex/agents/<role>.toml`, reusing confirmed shared role identities `skl-implement`, `skl-watchdog`, `skl-audit-standards`, and `skl-audit-artifacts`.
- Explicit native `name`, `description`, `model`, `model_reasoning_effort`, and complete thin `developer_instructions` in each role file.
- `gpt-6-astra` defaults with low reasoning for implementation and high reasoning for both Audit axes and Watchdog; supervisor settings unchanged.
- Uncapped draining using the predecessor's authoritative worker startup and supervisor continuation commands, with bounded CLI waiting rather than harness polling.
- Loop waiting defaults of 15 minutes per request and 30 seconds between polls, both configurable through the inherited CLI syntax. Bare one-item `next` remains immediate.
- Ownership-marker installation and refresh of managed global defaults, preserving and reporting unowned conflicting files and leaving unrelated files, global `config.toml`, and all native repository overrides untouched.
- Documented trusted-project, complete-file native role overrides, including copyable thin startup instructions that reference CLI-delivered behavior.
- Actionable prerequisite and provider/model failure guidance without supervisor fallback or silent model substitution.
- Deterministic tests of installed files, parsed configuration, and retrieved instructions through existing CLI seams; updated distribution and lifecycle documentation.

## Out of Scope

- OpenCode integration, changes to Pi execution, or reimplementation of predecessor Workflow Mechanics and shared skill contracts.
- Attempt caps, new Claims or leases, competing same-lane supervisors, cross-supervisor coordination, automatic recovery, or automatic merge.
- A custom Codex execution service, plugin, deprecated custom prompts, or a command shortcut when native skill invocation is sufficient.
- A new `skl` model registry, field-aware preservation or rewriting of global model edits, a cross-harness configuration preservation parser, model-selection flags, provider fallback, or automatic edits to user-wide permissions, trust, feature switches, nesting, or concurrency limits.
- Installer writes to Consumer Repository `.codex/` files, automatic cleanup of legacy assets, or overwriting unowned role files.
- Live harness smoke tests, model-quality benchmarks, or claims that deterministic content tests establish actual LLM compliance or review quality.

## Definition of Done

- [ ] DOD1: Installed Codex loop entry points retrieve the inherited shared Skill Definitions, remain distinct from one-item entry points, and keep Workflow policy out of stubs and native role files.
- [ ] DOD2: Delivered Codex instructions require two separate supervisors, one fresh worker per lane/project at a time, new-session spawning without parent-history forks, and exact CLI-issued startup commands.
- [ ] DOD3: Delivered loop instructions use the CLI-issued continuation command after each dispatch, have no attempt cap, inherit configurable 15-minute/30-second bounded waiting, and stop safely on unresolved or ambiguous outcomes without inferring handoff from worker prose.
- [ ] DOD4: Delivered implementation/Audit instructions require two independent parallel fresh native Audit subagents nested below the implementation worker. Each consumes its complete supplied axis brief and recorded checks, following explicit resource pointers in that brief if any, matching Pi without a new axis skill/resource interface or recursive Audit invocation. Separate axis reports and shared deterministic facts are preserved; Watchdog remains independent without rerunning Audit.
- [ ] DOD5: Installation produces four valid thin native Codex role files with the approved explicit model/reasoning defaults and no supervisor or global policy changes.
- [ ] DOD6: Reinstallation refreshes owned global defaults and guidance idempotently under the existing ownership-marker policy, preserves and reports unowned conflicts, and leaves unrelated files, global `config.toml`, and all native repository overrides untouched.
- [ ] DOD7: Delivered guidance halts actionably on missing native delegation, nested capacity, roles, trust for project overrides, CLI access, or provider/model availability; it neither runs workers in the supervisor nor silently substitutes settings.
- [ ] DOD8: Documentation provides copyable complete project role overrides, explains same-name project-file replacement rather than field merging, requires project trust, and references official Codex documentation without introducing a model registry or duplicating Workflow policy.
- [ ] DOD9: Existing CLI install/retrieval tests and configuration parsers cover these delivered boundaries, the repository checks pass, and README plus skill-distribution and work-item-lifecycle capabilities describe the delivered Codex integration without claiming live behavioral proof.

## Manual verification

None.
