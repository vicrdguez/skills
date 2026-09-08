# Drain Pi Queues Through Shared Loop Skills Behavior

These scenarios verify emitted instruction contracts and installation through existing CLI seams. They do not execute an LLM supervisor, prove runtime compliance, test model quality, or repeat predecessor Workflow Engine tests. Each scenario maps to the identically numbered B task and A acceptance criterion.

## Feature: Shared Loop Instruction Contracts

#### Scenario: B1 Retrieve shared loops separately from one-item skills
- Given the embedded catalog and a temporary installation home
- When `skl install` installs Skill Stubs and `skl skill` retrieves `implement-loop`, `watchdog-loop`, `implement`, and `watchdog` in Markdown and JSON
- Then both loop names resolve to their authoritative shared definitions through the existing packet protocol
- And installed loop stubs retrieve those definitions rather than containing copied loop policy
- And loop packets do not bundle the single-item implementation or review work into the supervisor context
- And the existing one-item skills still describe processing only one Work Item
- And shared stub distribution does not claim to install OpenCode or Codex queue worker adapters.

#### Scenario: B2 Emit CLI-authorized dispatch and continuation ordering
- Given the merged predecessor's dispatch interface supplies a worker startup command and a supervisor continuation command
- When each shared loop's instructions are retrieved
- Then they require the supervisor to request a dispatch and give its startup command to exactly one fresh worker in the selected project
- And the worker retrieves and follows that one Work Item's packet without claiming another item or reloading bundled definitions
- And the supervisor retains the continuation command and invokes it only after that worker terminates, including a failed or interrupted worker
- And continuation is decided solely by the CLI action, not worker prose, exit success, returned Workflow State, or a supervisor reconstruction of handoff rules
- And the next worker can launch only when the CLI authorizes its dispatch, including when the other lane has already advanced the prior Work Item.
- And the supervisor reports the dispatched item, CLI-confirmed completed outcome and running totals between workers, and the CLI's reason when it stops, without creating a shared status system.

#### Scenario: B3 Emit bounded waiting without a queue attempt cap
- Given waiting is supplied by the predecessor rather than the Pi extension
- When each loop contract is retrieved
- Then initial selection and continuation request a fresh bounded idle window of 15 minutes and impose no item or attempt cap
- And the contract uses the predecessor's supported syntax and preserves `next` as immediate by default, bare `--wait` as 15 minutes, an explicit duration as an override, and configurable `--poll` with a 30-second default
- And the Workflow Engine checks immediately and polls within that window, with no supervisor polling or additional retry policy
- And the CLI tool invocation permits a timeout longer than the requested wait; an insufficient harness limit is reported rather than silently shortening the CLI window or repeatedly timing it out
- And an idle-timeout action stops only that lane and is reported as no claimable work during that window, not global completion.

#### Scenario: B4 Emit independent cold-context lanes
- Given implementation and Watchdog supervisors run in separate sessions for one project
- When both shared loop contracts and Pi role definitions are retrieved or installed
- Then they require at most one supervisor and one active Work Item worker per lane for that project
- And each dispatch starts a new worker context without conversation history from the supervisor or prior workers
- And each lane continues or stops independently without running the other lane or coordinating a shared status view
- And Audit reviewer children remain inside implementation rather than becoming queue consumers
- And no loop rule changes the per-change review-bounce allowance or human-only merge authority.

#### Scenario: B5 Emit safe stopping and explicit recovery instructions
- Given selection or continuation can be interrupted, ambiguous, or fail operationally
- When each shared loop contract is retrieved
- Then a known dispatch uses its returned continuation command to let the CLI verify durable handoff after worker termination, even if the worker failed
- And a CLI stop or recovery action is reported with its supplied recovery guidance while Claims and partial work remain intact
- And missing dispatch identity, malformed or unknown actions, or interrupted or failed selection stops without blindly issuing another `next`
- And the contract forbids automatic replacement, Claim expiry, release, or resume as a loop recovery strategy
- And forge-query errors remain operational errors rather than empty-queue observations, with no new supervisor retry policy
- And worker claims of success alone never authorize further work.

## Feature: Thin Pi Delegation And Native Role Configuration

#### Scenario: B6 Emit extension-based workers and two independent Audit axes
- Given Pi uses the retained `pi-subagents` extension
- When managed roles are installed and the shared Audit packet is retrieved
- Then queue delegation uses `skl-implement` and `skl-watchdog` in fresh contexts and awaits completion
- And Pi Audit delegates Standards to `skl-audit-standards` and Artifacts to `skl-audit-artifacts` in parallel fresh contexts through the same extension
- And each reviewer receives its axis brief and the recorded deterministic checks from shared Audit instructions, without copying those judgment rules into its role definition
- And the existing independent-Audit missing-artifact handling and side-by-side aggregation remain intact
- And Pi's missing-delegation path halts actionably rather than using the generic sequential in-parent fallback
- And single-item Watchdog remains independent and does not rerun Audit.

#### Scenario: B7 Install role-specific global defaults without pinning dispatch models
- Given a fresh temporary home
- When `skl install` installs the four managed Pi roles
- Then their frontmatter assigns `openai-codex/gpt-6-astra` to every role
- And `skl-implement` has `thinking: low`
- And `skl-watchdog`, `skl-audit-standards`, and `skl-audit-artifacts` have `thinking: high`
- And no managed role includes a fallback model chain
- And their bodies delegate startup or the supplied Audit brief rather than duplicating shared Workflow policy
- And emitted delegation instructions select roles without supplying a per-run model or thinking override that defeats native project configuration
- And the installer writes no supervisor model or global `agentOverrides` defaults.

#### Scenario: B8 Preserve user configuration while refreshing managed assets
- Given a temporary home with stale currently owned assets, an unowned collision at a managed path, unrelated files, and global settings with supervisor, permission, and execution-limit choices
- And a temporary Consumer Repository has `.pi/settings.json` model-only and model-plus-thinking `subagents.agentOverrides` entries for the new roles
- When `skl install` runs twice and the installed entrypoint guidance is retrieved
- Then currently owned assets refresh and the second installation makes no further changes to them
- And the unowned collision, unrelated files, global settings, and project override bytes remain unchanged
- And guidance shows native project role model and supported thinking overrides without replacing persona/startup instructions
- And the installed frontmatter provides defaults beneath those native overrides rather than a competing configuration layer
- And a preserved conflicting file is addressed by actionable user guidance rather than overwritten or silently treated as the managed role.

#### Scenario: B9 Emit actionable prerequisite failures without fallback execution
- Given the installed Pi entrypoints and roles depend on available delegation, nested Audit support, and exact configured model/thinking settings
- When their instructions and prerequisite guidance are retrieved
- Then they require the installed `pi-subagents` mechanism and necessary tools in both supervisor and implementation-worker contexts before attempting the dependent work
- And they identify `skl` availability, compatible Pi/extension installation, background-run prerequisites when used for Audit, provider authentication, exact model availability, supported thinking, and effective execution limits
- And missing prerequisites halt with guidance identifying the missing capability and how the user can install, authenticate, configure a native role override, or resolve the limit
- And neither parent execution, a silent model substitution, nor an automatic global permission or limit change is an allowed remedy
- And unavailable exact models are not described as universally supported or replaced through fuzzy matching or a fallback chain.

## Feature: Source Retirement Without Installed-File Migration

#### Scenario: B10 Retire old installation targets but preserve legacy copies
- Given one empty temporary home and another containing legacy copies of `prompts/implement-loop.md`, `prompts/watchdog-loop.md`, `prompts/queue-next.mjs`, `agents/implement-runner.md`, and `agents/watchdog-runner.md` under `.pi/agent/`
- When `skl install` runs repeatedly for both homes
- Then the empty home receives the new shared loop stubs and dedicated roles but none of those retired files
- And every legacy copy in the other home remains byte-for-byte unchanged, including files carrying the old ownership marker
- And current installed entrypoints and roles do not invoke the old helper, prompts, or runner names
- And current entrypoint guidance names an unambiguous shared-skill invocation and tells users to remove obsolete installed copies manually before relying on legacy shortcut names
- And no automatic cleanup or migration path is added.
