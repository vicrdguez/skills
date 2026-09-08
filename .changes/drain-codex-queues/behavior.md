# Drain Codex Queues Behavior

These scenarios specify observable installed configuration and CLI-delivered instruction contracts. Content and parser checks establish what is delivered, not whether a live LLM follows it. Scenario identifiers map to `tasks.md`; DOD references map to `intent.md`.

## Feature: Shared Codex queue entry points

### Scenario: B1 - Discover loop skills separately from one-item skills
- Given the merged predecessor supplies shared `implement-loop` and `watchdog-loop` definitions
- When `skl install` runs against a temporary user home and the installed Codex skill retrieval commands are executed
- Then both loop stubs retrieve their shared authoritative definitions through `skl`
- And `implement` and `watchdog` remain separate one-item entry points
- And installed stubs do not contain duplicate Workflow policy or require deprecated custom prompts
- And the packet manifest prevents reactivation of already bundled definitions
- And this satisfies DOD1 and DOD9

### Scenario: B2 - Deliver fresh one-worker-per-lane native dispatch instructions
- Given a Codex supervisor retrieves either loop's instruction packet
- When its native dispatch instructions are inspected
- Then they require separate implementation and Watchdog supervisor sessions and at most one active Work Item worker per lane per project
- And each dispatch creates a new native Worker Session rather than resuming an earlier worker or forking parent conversation history
- And the dispatch supplies only the explicit task context needed to execute the CLI-issued startup command in the supplied repository context
- And the worker executes that command to retrieve its authoritative Instruction Packet instead of independently selecting another Work Item
- And completed worker threads are closed before further dispatch so uncapped draining does not accumulate open threads
- And this satisfies DOD2

### Scenario: B3 - Deliver authoritative continuation and bounded waiting instructions
- Given the Codex loop instructions are retrieved through `skl`
- When their queue continuation contract is inspected
- Then they require the dispatch-specific CLI-issued continuation command before any subsequent worker, even after an interrupted or failed worker
- And continuation depends on the CLI's durable handoff decision, not worker prose, including when the other lane already advanced the Work Item
- And they impose no attempt cap and request bounded waiting with a default 15-minute idle window and 30-second polling interval using predecessor-supported configurable syntax
- And each request starts a new idle window while bare one-item `next` remains immediate
- And the supervisor does not implement its own selection, polling, handoff checks, or Workflow transitions
- And this satisfies DOD3

### Scenario: B4 - Deliver safe stop and recovery boundaries
- Given a loop's CLI result or native worker outcome cannot establish a safe next dispatch
- When the installed and retrieved Codex failure guidance is inspected
- Then idle timeout is reported as no claimable work in that lane's bounded window, not global completion
- And query failures remain operational errors rather than empty-queue observations
- And incomplete handoffs, dispatch failures, and interrupted or ambiguous selection preserve Claims and partial work and report CLI recovery guidance or explicit inspection when dispatch identity is unavailable
- And the supervisor stops without blind `next` retries, replacement workers, Claim expiry, or automatic recovery unless the CLI confirms the previous durable handoff completed
- And the other supervisor remains independent and merge authority remains human-owned
- And this satisfies DOD3

## Feature: Native independent review delegation

### Scenario: B5 - Deliver nested independent Audit axes
- Given an implementation worker receives its bundled Audit instructions and the installed native Audit roles
- When their Codex delegation contract is retrieved and inspected
- Then implementation delegates Standards and Artifacts in parallel to two distinct fresh native subagents nested beneath that worker
- And each axis consumes its complete supplied brief and recorded fixed evidence, including deterministic gate and integrity checks produced once by the invoking Audit, following explicit resource pointers in the brief if any
- And the roles match Pi's supplied-brief contract without a new axis skill/resource retrieval interface or retrieving full Audit to recursively invoke its orchestrator
- And neither axis forks the implementation worker's conversation history or receives the other axis's reasoning
- And the existing Audit aggregation preserves both reports separately
- And Watchdog runs in its own fresh Worker Session without rerunning Audit
- And unavailable nested delegation requires an actionable halt instead of sequential in-context review
- And this satisfies DOD4

## Feature: Native role distribution and ownership

### Scenario: B6 - Install complete thin Codex roles with explicit defaults
- Given a temporary home without installed Codex roles
- When `skl install` runs and the four installed `~/.codex/agents/<role>.toml` files are parsed
- Then their role names match the inherited implementation, Watchdog, Audit Standards, and Audit Artifacts contract
- And every file contains `name`, `description`, `model`, `model_reasoning_effort`, and complete thin `developer_instructions`
- And every model is `gpt-6-astra`, with `low` effort for implementation and `high` for the other three roles
- And worker startup instructions execute the CLI-issued startup command and follow its packet while Audit role instructions consume the complete supplied axis brief and recorded checks, following explicit resource pointers in the brief if any, without retrieving full Audit or requiring a separate axis skill/resource interface
- And no role file duplicates shared Workflow policy or changes supervisor settings, global permissions, delegation switches, or limits
- And this satisfies DOD5 and DOD9

### Scenario: B7 - Refresh managed global defaults while preserving repository overrides
- Given a temporary home with stale owned role defaults and guidance, an unowned role-name collision, and unrelated Codex configuration
- And a Consumer Repository with complete project role overrides and other `.codex/` files
- When `skl install` is run twice
- Then the existing ownership-marker policy refreshes owned global role defaults and guidance and owned stubs without field-aware preservation of edits inside managed role files
- And the second installation makes no further changes
- And the unowned collision, unrelated home files, global `config.toml`, and every project file retain their bytes
- And the unowned collision is reported rather than overwritten, with actionable prerequisite guidance if it leaves a required role unavailable
- And this satisfies DOD6 and DOD9

### Scenario: B8 - Deliver actionable prerequisite failures without fallback
- Given the installed Codex entry points, roles, and prerequisite documentation
- When their failure instructions are inspected for missing CLI access, missing or invalid roles, disabled delegation, insufficient nesting or thread capacity, untrusted project overrides, or unavailable configured provider/model
- Then they direct the user to the specific prerequisite and native configuration or installation remedy and halt the affected lane
- And they explain that current Codex releases support subagents by default but user or managed configuration can disable or constrain them
- And they require user action rather than silently editing global configuration, permissions, trust, or execution limits
- And they forbid supervisor execution of worker tasks, loss of independent Audit axes, and silent model/provider substitution
- And this satisfies DOD7

### Scenario: B9 - Document complete trusted-project native overrides
- Given the Codex override examples delivered in README
- When each copyable role template is parsed and its startup instructions are inspected
- Then it is a complete `.codex/agents/<role>.toml` definition with the same `name` as its global role, description, editable native model/reasoning fields, and complete thin `developer_instructions`
- And the instructions reference authoritative CLI-delivered behavior rather than embedding Workflow policy
- And the text explicitly states that the same-name project role replaces the global configuration file, so model-only project files do not inherit global startup instructions
- And the text requires project trust and states that installation never creates or refreshes project overrides
- And the text identifies installed global roles as managed defaults refreshed by ownership marker and native repository role files as the customization boundary
- And it references the official Codex subagents and configuration docs and explains native override precedence without a new `skl` model registry
- And model diversity is recommended rather than enforced or claimed by the shipped same-model defaults
- And this satisfies DOD8 and DOD9
