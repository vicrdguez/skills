# Drain Codex Queues Plan

## Approach

Deliver approved slice 5 as a thin native Codex integration on top of Merged `drain-pi-queues`. No dependency on OpenCode. Inherit the predecessor's shared loop skills, role identities and retrieval contract, ownership rules, waiting syntax, dispatch startup command, and continuation command; do not implement alternative queue mechanics in this slice.

The predecessor supplies the shared replacement for the older Pi-specific adapters. Read its merged implementation before coding and inherit its exact command syntax and retrieval contract.

Respect `CONTEXT.md` and ADRs 0001 (embedded definitions and thin stubs), 0002 (engine-owned mechanics and best-effort Claims), 0003 (historical ledger contract), and 0004 (preserved agent judgment). The approved queue extension changes harness coverage, not Workflow State, review-bounce allowance, or human Merge Authority.

## Implementation decisions

- Shared skill identities are `implement-loop` and `watchdog-loop`, distinct from one-item skills. Native Codex skill activation is sufficient; add a thin native command shortcut only if a supported equivalent exists and earns its place. Deprecated custom prompts are not required.
- Reuse the confirmed shared role names `skl-implement`, `skl-watchdog`, `skl-audit-standards`, and `skl-audit-artifacts`. Audit roles match Pi's complete supplied-brief contract; introduce neither Codex-only aliases nor a new axis skill/resource retrieval interface.
- Use native delegation for supervisor-to-worker and implementation-to-Audit dispatch. Each Work Item and each Audit axis gets a new session context, not a fork carrying parent history. Pass explicit startup/task context and fixed evidence only. Verify the supported installed Codex version's actual spawn semantics during implementation; do not invent a freshness flag, unsupported TOML key, or alternate execution service.
- Separate supervisor sessions own the two lanes. Each lane dispatches at most one Work Item worker at a time per project. Audit's two parallel children are nested reviewers for that worker, not competing Work Item consumers. Close completed threads before further dispatch to avoid exhausting native capacity over an uncapped run.
- Execute the CLI-issued startup command exactly in its supplied repository context. The supervisor does not select board records, reconstruct resume commands, or preload every bundled skill. Honor packet manifests.
- After every dispatched worker outcome, use the dispatch-specific CLI-issued continuation command as the only authority for more work. A failed/interrupted worker permits continuation only when that command verifies the durable handoff. Ambiguous selection without dispatch identity requires a stop and explicit inspection, not a new selection attempt.
- Loop calls opt into bounded CLI waiting: 15 minutes per idle request, polling every 30 seconds, both configurable. Adopt predecessor flag syntax rather than treating shorthand such as `--wait15m` or `--poll30s` as literal command spellings. Bare one-item `next` remains immediate; no harness wait/retry loop or attempt cap is added.
- Preserve independent parallel Audit axes and their existing evidence/aggregation semantics. Codex queue execution must halt if nested independent delegation is unavailable, even if generic Audit historically offered an in-context fallback. Watchdog remains a separate fresh review and never reruns Audit.
- Install one standalone native TOML file per role in `~/.codex/agents/`, with `name`, `description`, `model = "gpt-6-astra"`, `model_reasoning_effort`, and `developer_instructions`. Implementation effort is `low`; Watchdog and both Audit axes use `high`. The supervisor's model and effort are untouched.
- `developer_instructions` are complete but thin startup adapters: workers run the supplied authoritative startup command; Audit roles consume their complete supplied axis brief and recorded checks, following explicit resource pointers in that brief if any. Match Pi: no separately retrievable axis instructions are required, and retrieving full Audit would recursively invoke the orchestrator rather than perform the assigned axis. They do not define Workflow Mechanics or duplicate shared judgment instructions.
- Install Codex roles as managed global defaults using valid TOML ownership comments and the existing ownership-marker refresh policy. Refresh owned files as managed defaults, including model/reasoning settings and startup guidance. Preserve and report unowned conflicts and leave unrelated files untouched. Do not add field-aware preservation of edits inside owned files, TOML configuration rewriting, or a cross-harness configuration preservation parser.
- Installation writes only needed owned user-level roles and Skill Stubs. Never modify `~/.codex/config.toml`, supervisor settings, user-wide permissions, trust, delegation enablement, nesting/concurrency limits, or Consumer Repository files. Existing native model/provider availability failures are surfaced with guidance; no silent substitution.
- Current official docs describe subagents as supported and enabled by default. User/managed settings or installed-version nesting limits can still block supervisor -> implementation -> two Audit children. Document the actual supported version and required capacity, with user-controlled remedies; do not automatically enable or widen anything.
- Pin native override semantics: a same-name `.codex/agents/<role>.toml` replaces the global role configuration file, not just selected fields. A model-only project file is insufficient. The approved documentation must include complete copyable thin role definitions including startup instructions for all four roles, referencing CLI behavior rather than duplicating it. Project trust is required for project configuration to load; `skl install` never manages these files.
- Native repository overrides are the customization boundary and remain user-owned complete definitions, never touched by the installer. Global installed roles are managed defaults, not a promise to preserve edits inside owned files. Documentation must explain retaining the complete thin startup contract when users update repository overrides. No hidden `skl` model registry or cross-harness model layer is introduced.
- Recommend model diversity between implementer and reviewers without enforcing it. The explicit shipped same-model profile is not diverse and has no measured quality advantage claimed here.

### Module shapes & seams

#### Modified: Skill Distribution

Public seam: `skl install`, backed by `Install(home)` in `distribution.go` and embedded role/stub assets. It depends on the embedded catalog and native Codex file schema. Preserve the existing install ownership boundary and temporary-home test approach. Extend existing CLI install checks for parsed TOML, exact defaults, thin startup content, ownership-marker refresh of global defaults, preserved and reported unowned collisions, unchanged repository overrides and global `config.toml`, and idempotence. Parsing validates delivered TOML in tests; it does not introduce a runtime configuration preservation or rewriting feature. Do not test private helper organization.

#### Modified: Instruction Retrieval and Codex Adapter Content

Public seam: `skl skill <name>`, its JSON form, named resources, and the predecessor's authoritative dispatch packet interface. `catalog.go`, shared definitions, and any necessary Codex-specific resource supply instructions without a second policy source. Extend existing retrieval/manifest tests to cover discoverability, startup/continuation authority, fresh native session instructions, nested axis separation, bounded waiting, and safe-stop guidance. Reuse predecessor CLI tests for the deterministic queue decisions rather than duplicating them as a Codex scheduler.

#### Modified: Native Configuration Documentation

Public seam: README's copyable TOML role examples and prerequisite instructions. Parse examples with the available configuration parser and check required fields/startup references alongside installed templates. These checks validate delivered syntax and content, not Codex's live selection, model behavior, or context isolation. Native same-name replacement and project trust are external supported-version facts, not functionality to implement inside `skl`.

## Sequence

1. After `drain-pi-queues` is Merged, resolve its exact shared role, ownership, retrieval, waiting, and dispatch contracts. Confirm the installed supported Codex version's native role loading, complete-file project replacement, fresh spawning, and nested delegation semantics against official docs and available version/schema evidence. If unsupported, halt actionably instead of inventing configuration.
2. Add scenario-led red-green checks at existing install and retrieval seams, then implement the smallest native role/stub and adapter-content changes needed. Preserve one-item behavior and other harnesses.
3. Verify owned refresh and native overrides, parse installed/example TOML, and run repository checks. No live harness launches, smoke tests, benchmarks, or additional manual-verification gate.
4. Update README, `docs/capabilities/skill-distribution.md`, and `docs/capabilities/work-item-lifecycle.md` last, replacing stale Pi-only claims only as warranted by the merged predecessor and this delivery.

## References

- https://developers.openai.com/codex/subagents : native role schema, global/project role locations, delegation availability, and model settings.
- https://developers.openai.com/codex/config-basic : native configuration precedence and trusted-project prerequisite.
- https://developers.openai.com/codex/config-reference : supported configuration keys and delegation limits for the implementation's supported version.
- Official URLs may redirect; record the supported version and current source when implementing. Do not infer a live behavior guarantee from configuration parsing or instruction assertions.
