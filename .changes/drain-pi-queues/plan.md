# Drain Pi Queues Through Shared Loop Skills Plan

## Approach

Materialize approved slice 3 only. `continue-verified-dispatches` must be Merged before implementation; it depends on `wait-for-claimable-work`. Consume their delivered CLI interface rather than inventing command names, dispatch fields, action values, or handoff checks here. The canonical scope is `docs/capabilities/work-item-lifecycle.md` under "Planned extension: Independent queue draining".

Move queue instructions into embedded `skills/dev/implement-loop/SKILL.md` and `skills/dev/watchdog-loop/SKILL.md`, registered as `implement-loop` and `watchdog-loop` in `catalog.go` and distributed through existing Skill Stubs. These names are the stable interface for subsequent OpenCode and Codex adapters. Keep each loop separate from the one-item skill and do not bundle the worker's judgment instructions into the supervisor packet.

Use Pi's explicit skill activation, `/skill:implement-loop` and `/skill:watchdog-loop`, as thin entrypoints. No replacement prompt shortcut is required. This avoids collisions with previously installed `/implement-loop` and `/watchdog-loop` prompts without adding migration logic. Dedicated `skl-*` role names similarly avoid PR #19's runner names.

## Implementation decisions

- The Workflow Engine owns selection, Claims, bounded waiting, handoff verification, and continuation. The supervisor is an executor of the CLI action: obtain dispatch, retain continuation command, start a fresh worker with the startup command, await termination, invoke continuation, obey its action. Worker failure is not itself a continuation decision; an already completed durable handoff can still permit another dispatch.
- Initial selection and continuation use a 15-minute idle window per request, no queue attempt cap, and the predecessor's poll default. Document the optional wait/poll CLI interface without implementing a second timer or retry loop. Do not confuse the lack of a Workflow attempt cap with unlimited Pi execution resources.
- Shared loops require the CLI tool invocation timeout to exceed its requested wait with allowance for an in-flight backend operation. Use the harness's supported invocation option; if its effective limit cannot accommodate the requested window, halt with actionable configuration guidance rather than silently changing the window. Cancellation or tool timeout remains ambiguous and follows explicit recovery.
- Report the dispatched item, CLI-confirmed completed outcome and running totals between workers, plus the CLI stop reason. This is conversation output, not a dashboard, persistent run registry, or supervisor inference from worker prose.
- Unknown actions, operational errors, or missing/ambiguous dispatch identity stop safely. Never blindly reselect, automatically replace workers, release or expire Claims, or resume interrupted work. Report the CLI's exact recovery guidance when available; missing identity requires explicit inspection rather than a fabricated command.
- Run one supervisor per lane per project and one fresh Work Item worker at a time in each lane. Retain project filesystem guidance, but not previous conversation history. Audit's two nested reviewers remain parallel fresh contexts. No supervisor performs worker implementation, review, or Audit itself.
- Install `agents/skl-implement.md`, `agents/skl-watchdog.md`, `agents/skl-audit-standards.md`, and `agents/skl-audit-artifacts.md` to `~/.pi/agent/agents/` through the existing owned-asset mechanism. Models and thinking belong in frontmatter: `openai-codex/gpt-6-astra`, implementation low, all reviewers high. No fallbackModels or supervisor model edits.
- Queue role bodies consume the supplied startup command and finish only one Work Item. They must not run their own `next`. Reviewer role bodies consume the assigned shared Audit axis brief. The shared definitions remain authoritative; adapters contain only activation, role identity, and necessary Pi execution metadata.
- Preserve `pi-subagents` for queue execution and Audit's existing parallel `workflowScript`/`runs.all` mechanism. Update only Pi's Audit dispatch instructions to select the dedicated roles and fail actionably without delegation. Preserve its axis briefs, deterministic checks once, missing-artifact handling, aggregation, and unrelated harness behavior. Make the generic sequential fallback explicitly inapplicable to Pi rather than removing unrelated harness support.
- Native project `.pi/settings.json` `subagents.agentOverrides` supports both `model` and `thinking`. Show a model-only override and a model-plus-thinking override using the new role names; changing these fields leaves the installed persona intact. Do not add per-launch model/thinking pins, a project role copy, global overrides, or Workflow Engine model flags.
- Require compatible Pi and extension capabilities, `skl` on worker PATH, authenticated access to the exact configured model, supported thinking, nested Audit delegation, and adequate effective limits. Existing asynchronous Audit requires a Pi package installation capable of background children, not a standalone-only executable. Use the installed extension's guidance/doctor/model mapping to diagnose prerequisites; do not build a new probing or configuration subsystem. Halt actionably on unmet requirements, without silently relaxing settings or selecting another model.
- Installation leaves global settings and Consumer Repository files entirely outside its ownership. Existing unowned file collisions are preserved under the current ownership rule; document inspection/removal or renaming by the user before relying on a conflicting role. New names reduce collision risk without guaranteeing absence of user files.
- Remove old source assets and their embedding/install targets: `prompts/implement-loop.md`, `prompts/watchdog-loop.md`, `prompts/queue-next.mjs`, `prompts/queue-next.test.mjs`, `prompts/queue-outcomes.json`, `agents/implement-runner.md`, and `agents/watchdog-runner.md`. Replace old helper-dependent Go assertions rather than preserving an obsolete test path. Retain `workflow/` mechanics and the `pi-subagents` mechanism.
- Installed legacy files are different from retired source. Never enumerate/delete/rewrite legacy installed copies, even if marked owned. Do not reuse their paths for replacement prompts. README must list the five old installed targets under `~/.pi/agent/` and explain manual removal and explicit shared-skill activation.
- Correct only directly affected references, including the old runner name in single-item Watchdog guidance, without changing its direct fresh-session behavior. This is not a rewrite of Implement, Audit, or Watchdog judgment.
- Record the shared-loop/Pi-adapter decision by revising ADR 0001's Pi-only queue statement and related distribution consequences. Do not modify ADR 0002's separately planned backend-independence correction. Preserve ADR 0003 ledger rules and ADR 0004's judgment/mechanics separation.
- README and capability docs must distinguish this shared/Pi delivery from future OpenCode/Codex adapters. Recommend optional implementation/reviewer model diversity without enforcing it; the shipped same-model profile provides no model diversity and has no measured superiority claim. Availability depends on the user's configured provider catalog.

### Module shapes & seams

#### [MODIFIED] Embedded catalog and instruction retrieval

Interface: existing `SkillNames`, `BuildPacket`, `Packet.Markdown`, `Packet.JSON`, resource retrieval, and public `skl skill` commands. Dependencies: in-process embedded Skill Definitions and predecessor-owned CLI contracts. Invariant: one authoritative behavior definition per skill, consistent Markdown/JSON packets, and no worker instruction bundle in the supervisor's loop packet.

Test strategy: extend existing retrieval/catalog tests in `cmd/skl/main_test.go`, using `newAppWithSkillHome`, to exercise emitted contracts for B1-B5. Assert independently specified required instructions and guardrails through returned packets, not only equality against source files. Existing definition-equality assertions can prove distribution fidelity but cannot alone prove the required contract. Do not add a prompt interpreter or simulate LLM decisions.

#### [MODIFIED] Managed installation and thin Pi adapters

Interface: `skl install` through the existing temporary-home seam, plus retrieval from installed Skill Stubs. Dependencies: filesystem installation is the existing replaceable I/O seam; Pi and `pi-subagents` are external execution dependencies documented rather than launched by tests. Invariants: managed refresh is repeatable; unrelated/unowned files, all settings, and installed legacy files are preserved; roles select native defaults without copying Workflow policy.

Test strategy: adapt `cmd/skl/pi_test.go` and existing install tests for B6-B10. Use temporary homes and a temporary Consumer Repository, inspect installed frontmatter/body contracts, seed independently specified user/legacy bytes, and compare them after repeated installation. Validate override guidance/configuration shape and absence of dispatch pins, not the extension's live resolution algorithm. Do not install the extension, access the user's real home, authenticate a model, or launch Pi.

#### [MODIFIED] Shared Audit instructions

Interface: the retrieved Audit packet and its inclusion in the one-item Implement packet. Dependencies: existing shared judgment briefs and Pi extension delegation. Invariants: both applicable axes remain independently briefed and cold, checks run once before delegation, role bodies remain thin, and Pi cannot take the generic in-parent fallback.

Test strategy: B6 and B9 inspect the emitted public contract and installed roles through the same retrieval/install seams. Reuse existing bundled-definition coverage where applicable; no separate Audit runtime, model-quality test, or broad new suite.

## Sequence

1. Confirm the predecessor has merged and read its actual startup, continuation, action, wait, and recovery interface. Resolve these into the emitted instructions; do not infer syntax from PR #19.
2. Run one red-green cycle per B task in order, extending existing catalog/retrieval and installation tests. Keep checks at the pinned public seams; behavioral tests prove contracts and installation only.
3. Retire obsolete source and helper-dependent test/install wiring as replacements land. Keep installed legacy copies untouched and shared Workflow mechanics unchanged.
4. Update README, the scoped distribution ADR, and finally the distribution and lifecycle capabilities. Mark only delivered shared/Pi behavior as current; leave later-harness work planned.
5. Run `go test ./...` and `git diff --check`, plus any existing project gate required by repository guidance. No live harness smoke test or manual verification is required.
