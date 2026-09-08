# Drain OpenCode Queues Plan

## Approach
Implement the already-approved fourth slice after `drain-pi-queues` is Merged. That predecessor owns shared `implement-loop` and `watchdog-loop` definitions and depends transitively on CLI waiting and verified dispatch continuation. This checkout still contains the earlier Pi queue assets; they are context, not the architecture to reproduce. Read the merged predecessor's concrete command/packet interface before wiring OpenCode; do not invent parallel command names or outcome fields here.

OpenCode adds a thin Harness Adapter at the existing distribution and instruction seams. `skl` installs definitions but never starts a harness. A user invokes a native command in each of two separate OpenCode sessions. Shared instructions ask the CLI for work; a dispatch supplies a fixed-Claim startup command and authoritative continuation command. The supervisor delegates through native Task, the worker retrieves its own packet, and the supervisor follows CLI continuation decisions.

Respect `CONTEXT.md`, ADRs 0001 (embedded definitions and stubs), 0002 (engine-owned state without a database), 0003 (retired ledgers), and 0004 (preserved judgment roles). The supported-harness extension updates the existing distribution documentation; it does not revise Workflow Mechanics or judgment policy.

## Implementation Decisions
- Reuse predecessor loop definitions and waiting/continuation commands verbatim. Loop startup enables CLI waiting; bare CLI `next` remains immediate. Bare `--wait` means up to 15 minutes and `--poll` defaults to 30 seconds; explicit values use the predecessor's accepted syntax and validation. There is no item/attempt cap, and each selection request has its own idle window.
- Keep native loop commands in the current primary session: no worker `agent`, model override, or forced subtask. Commands forward arguments through native `$ARGUMENTS` and retrieve shared instructions. The user supplies separate sessions and retains their supervisor model.
- Use native Task delegation with `task_id` omitted for every new worker and Audit axis. Resuming a Workflow Claim via the CLI startup command is not resuming a native Task session. Do not pass accumulated worker conversation, select twice, or make the supervisor implement/review.
- Worker prose and exit status are not inputs proving completion. After completion, error, or interruption with a known dispatch and terminated worker, use its continuation command; the CLI reads durable evidence. If dispatch identity was never received, stop for explicit recovery. Preserve Claims and partial work; never auto-release or blindly repeat selection.
- OpenCode Audit requires two parallel independent native subagents. Add only the OpenCode execution binding to the shared Audit instructions or their existing deterministic adapter resource mechanism. Preserve both briefs, once-only deterministic checks, separate aggregation, and Watchdog's no-Audit rule. The generic no-subagent fallback must not be used for this supported OpenCode path.
- Install four global native Markdown roles: implementation, Watchdog, Audit Standards, Audit Artifacts. Coordinate role filenames, command references, Task permissions, and documentation. Likely names are `skl-implement`, `skl-watchdog`, `skl-audit-standards`, and `skl-audit-artifacts`; these are candidate names, not a frozen naming requirement or a new cross-harness naming layer.
- Role frontmatter sets `mode: subagent`, `model: openai/gpt-6-astra`, and `reasoningEffort` low for implementation, high for the other three. Queue workers execute the supplied startup command; Audit reviewers consume their complete supplied axis brief and recorded facts, following its explicit resource pointers without invoking Audit orchestration again. Install only scoped permissions necessary to these managed definitions; explicitly allow the implementation role's Task calls to the two Audit roles.
- Default global discovery paths are `~/.config/opencode/agents/*.md`, `~/.config/opencode/commands/*.md`, and `~/.config/opencode/skills/<name>/SKILL.md`. Respect OpenCode's actual config-home discovery, including a non-default XDG config home; do not substitute macOS Go config-directory conventions. Treat `OPENCODE_CONFIG_DIR` as a native additional configuration layer, not a reason to overwrite arbitrary project/user configuration. Document its precedence and keep installation ownership limited to managed global assets.
- Reuse the existing installation ownership policy and protocol markers. Preserve unrecognized/user-owned collisions and unrelated files; report blocked required assets actionably. Reinstall updates managed definitions only, without automatic migration/cleanup of unrelated or previously installed adapters. No project file is installer-owned.
- Project role model/reasoning overrides belong in `.opencode/opencode.json`, under `agent` keyed by actual installed role names. The user-supplied verified native behavior is authoritative here: JSON overlays retain role instructions; project Markdown, even an empty body, replaces the prompt; root `opencode.json` may load before global Markdown, so it is not the recommended override location.
- Require native Task support, discoverable roles, usable configured provider/models, appropriate shell/worktree access, and explicit supervisor-to-worker permissions. Worker Audit additionally requires `subagent_depth >= 2` and worker-to-axis permissions. Document user-applied setup and halt if unavailable; never silently alter user-wide permissions, models, limits, or depth. Do not promise parser-only checks prove effective runtime access.

### Native Override Example
Use the final coordinated role names in shipped documentation. This illustrative user-owned `.opencode/opencode.json` keeps prompts intact and makes delegation depth an explicit user choice:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "subagent_depth": 2,
  "agent": {
    "skl-implement": { "model": "openai/gpt-6-astra", "reasoningEffort": "low" },
    "skl-watchdog": { "model": "openai/gpt-6-astra", "reasoningEffort": "high" },
    "skl-audit-standards": { "model": "openai/gpt-6-astra", "reasoningEffort": "high" },
    "skl-audit-artifacts": { "model": "openai/gpt-6-astra", "reasoningEffort": "high" }
  }
}
```

Document a separate scoped `agent.<actual-supervisor>.permission.task` example allowing the corresponding worker role, and the implementation role's explicit axis permissions. Explain last-matching-rule precedence, adapt to the user's chosen supervisor, and preserve unrelated existing rules. Do not install either example as user-wide configuration. Users may choose different reviewer models; recommend that diversity without requiring it or benchmarking these defaults.

### Module Shapes & Seams

#### Modified: Skill Distribution
Interface: existing `skl install` and `skl skill [--format json] <name>` / named resource retrieval, backed by `distribution.go`, `catalog.go`, and `stubs/common.md`.

Dependencies: repository-owned embedded definitions/resources and predecessor loop definitions are local; filesystem/home/environment are the existing installation seam; OpenCode discovery conventions are the external native contract. Add OpenCode as an explicit supported destination, not incidental discovery via another harness's directory. Maintain manifest deduplication, equivalent packet forms, recognized ownership, and thin stubs.

Test strategy: extend `cmd/skl/main_test.go`'s `newAppWithSkillHome` temporary-home installation and instruction retrieval tests. Cover default and non-default native config homes with isolated environment values, first install, recognized refresh, idempotence, user-owned collisions, unrelated settings, and project files. Compare returned packet fields/instructions and rendered Markdown at the public CLI seam. Existing `cmd/skl/pi_test.go` demonstrates installation testing, not an OpenCode runtime simulator.

#### New: Native OpenCode Harness Adapter
Interface: installed native command/role documents, their parsable frontmatter, and retrieved instructions/resources. Dependencies: predecessor CLI dispatch commands are repository-owned; native Task and OpenCode config loading are external capabilities. Preserve one worker per dispatch, independent lane contexts and Audit axes, fixed-Claim retrieval, engine authority, role-local defaults, and user-controlled prerequisites.

Test strategy: parse installed Markdown/frontmatter and documented JSON through appropriate format/schema parsers to validate native asset types, model/reasoning fields, consistent role references, scoped Task permissions, and override example validity. Reuse available parser tooling; no OpenCode runtime launch or fake harness is needed. Review native prompt retention/precedence against the verified contract and official references rather than writing a toy config resolver that purports to prove OpenCode behavior.

Instruction-only scenarios are accepted by reviewing the actual delivered documents retrieved at these seams. Small targeted structural assertions are appropriate; blanket grep/phrase inventories are not proof of supervisor execution, independent contexts, polling, or safe recovery. Do not add new test seams, model benchmarks, live smoke tests, or manual checks. The predecessor owns runnable waiting/continuation semantics tests; this slice proves distribution, configuration validity, and faithful instruction delivery only.

### References
- `README.md`, especially installation and the existing Pi loop description.
- `docs/capabilities/skill-distribution.md` and `docs/capabilities/work-item-lifecycle.md`, especially Planned extension: Independent queue draining.
- `skills/dev/audit/SKILL.md`, `distribution.go`, `catalog.go`, `stubs/common.md`, and existing CLI installation/retrieval tests.
- Native agents, role permissions and provider options: https://opencode.ai/docs/agents/
- Native commands and argument/context behavior: https://opencode.ai/docs/commands/
- Native configuration layering and `subagent_depth`: https://opencode.ai/docs/config/
- Native Skill Stub discovery: https://opencode.ai/docs/skills/

## Sequence
1. Read the merged `drain-pi-queues` artifacts/contracts and reconcile concrete loop/resource names without changing this slice's scope or tests.
2. Extend installation through one temporary-home red-green cycle, adding native command/role assets and explicit OpenCode stub discovery.
3. Complete ownership and packet-equivalence checks through the existing CLI seams; wire thin native worker and Audit delegation instructions against predecessor commands.
4. Validate installed native documents and override/prerequisite examples through parsers; review instruction scenarios against their delivered documents.
5. Update README and the relevant capability documents, run existing repository checks and focused distribution/parser checks, and record only the evidence actually obtained. Manual verification remains None.
