# Drain OpenCode Queues

## Why
OpenCode users need the same independent implementation and Watchdog queue draining as Pi, without putting worker reasoning in a supervisor's context or duplicating Workflow Mechanics in a harness script. Installation must make this a supported, discoverable path while respecting user configuration.

## What
Approved Slice 4 adds two thin native OpenCode supervisor commands, managed worker roles, and native delegation for both independent Audit axes. It consumes the shared `implement-loop` and `watchdog-loop` Skill Definitions and CLI dispatch contract delivered by its predecessors.

Blocked by: `drain-pi-queues`. That dependency supplies the shared loop definitions and transitively the CLI waiting, worker startup, and authoritative continuation contract. The blocker must be Merged, not merely Ready for Merge, before implementation starts.

## Scope
- Support OpenCode explicitly through `skl install`: global common Skill Stubs, native commands, and four managed worker roles.
- Keep `implement-loop` and `watchdog-loop` separate from the one-item `implement` and `watchdog` skills, with one independent supervisor session per lane per project.
- Delegate each claimed Work Item to one fresh native Task context; use the CLI-provided startup command to resume that fixed Claim and retrieve its Instruction Packet.
- Use only CLI decisions for selection, waiting, handoff verification, and continuation, including after worker errors.
- Inherit uncapped draining and predecessor `--wait`/`--poll` behavior: loop defaults of 15 minutes idle waiting and 30 seconds polling, configurable per invocation.
- Delegate Audit Standards and Artifacts to two independent fresh native contexts, in parallel, from the implementation worker.
- Ship role-local `openai/gpt-6-astra` defaults: implementation `reasoningEffort: low`; both Audit axes and Watchdog `reasoningEffort: high`.
- Preserve supervisor model selection, user-owned files, unrelated settings, and Consumer Repository overrides across reinstall.
- Document native discovery, separate session invocation, prerequisites, recovery, role defaults, and prompt-preserving project JSON overrides.

## Out of Scope
- Implementing the predecessor CLI waiting/dispatch contract or copying its Workflow Mechanics into OpenCode adapters.
- Launching or controlling OpenCode from `skl`; plugins, a new harness runtime, or a synthetic delegation harness.
- Replacing Pi's subagent extension, implementing Codex queue support, or redesigning one-item skills and judgment policy.
- A dashboard, private database, cross-supervisor coordination, competing same-lane consumers, automatic Claim recovery, or agent merge authority.
- Silent changes to user-wide models, permissions, execution limits, or delegation depth; installer-owned project overrides.
- Live harness smoke tests, model benchmarks, or claims of measured model superiority.

## Definition of Done
- [ ] DoD1: `skl install` installs discoverable OpenCode Skill Stubs, two native loop commands, and four thin native worker roles in the appropriate global discovery locations, respecting native config-home discovery. (B1)
- [ ] DoD2: Reinstall refreshes only recognized managed assets, is idempotent, and preserves user-owned collisions, unrelated files/settings, and all project overrides. (B2)
- [ ] DoD3: Loop instruction retrieval returns equivalent Markdown and JSON packets, without duplicating shared definitions or changing the separate one-item entry points. (B3)
- [ ] DoD4: Each loop's delivered instructions use an independent supervisor and one fresh native worker per CLI dispatch, resuming its fixed Claim through the supplied startup command rather than selecting again. (B4)
- [ ] DoD5: Delivered supervisor instructions consume the predecessor's authoritative continuation and waiting contract without an attempt cap or local lifecycle decisions; incomplete or ambiguous dispatches stop safely. (B5)
- [ ] DoD6: OpenCode Audit instructions delegate Standards and Artifacts in parallel to independent fresh roles, share deterministic facts, preserve separate reports, and never fall back to supervisor or same-context judgment when delegation is unavailable. (B6)
- [ ] DoD7: Parsed native roles carry the specified model/reasoning defaults and required scoped Task permissions, while native loop commands leave the supervisor model and context unchanged. (B7)
- [ ] DoD8: Documented `.opencode/opencode.json` role overrides set model/reasoning without replacing installed prompts; project Markdown prompt replacement and root JSON precedence caveats are explicit. (B8)
- [ ] DoD9: Delivered prerequisite instructions require available native delegation, depth of at least two for worker Audit, explicit Task permissions, and usable configured models; unavailable prerequisites halt with actionable setup/recovery guidance instead of rewriting user configuration. (B9)
- [ ] DoD10: README and relevant capability documentation describe OpenCode as supported, explain installation, both lane invocations and configurable waiting, defaults/overrides/prerequisites/recovery, and accurately limit verification claims to the approved seams. (B10)

## Manual verification
None.
