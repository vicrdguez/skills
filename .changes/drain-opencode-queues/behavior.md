# Drain OpenCode Queues Behavior

Scenarios specify either observable distribution behavior or the instructions delivered to agents. Instruction scenarios require faithful encoding and review of the retrieved instructions, not an invented runtime test. The approved automated seams are pinned in `plan.md`.

## Feature: Supported OpenCode Distribution

### Scenario: B1 Install native OpenCode discovery assets
- Given a temporary user home with no OpenCode assets and native config discovery resolved inside the temporary environment
- When `skl install` runs through the existing CLI interface
- Then OpenCode's global skills directory contains common Skill Stubs for the supported catalog, including `implement-loop` and `watchdog-loop`
- And its native `commands/` directory contains separate implementation-loop and Watchdog-loop entry points
- And its native `agents/` directory contains implementation, Watchdog, Audit Standards, and Audit Artifacts roles
- And the installed Markdown/frontmatter parses as the appropriate native asset type
- And a non-default native config home is respected rather than writing to the real user's home

### Scenario: B2 Preserve user ownership on reinstall
- Given an installation with stale recognized managed assets, user-owned files at managed destination names, unrelated global settings, and project overrides
- When `skl install` runs twice
- Then recognized managed assets are refreshed and the second run makes no further changes
- And user-owned collisions, unrelated files and settings, and all Consumer Repository overrides remain byte-for-byte unchanged
- And a preserved collision that prevents the required role or command from being available is reported with an actionable resolution rather than overwritten

### Scenario: B3 Retrieve shared loop instructions without changing one-item entry points
- Given the predecessor's shared loop Skill Definitions in the embedded catalog
- When each loop is retrieved through `skl skill <name>` and `skl skill --format json <name>`
- Then both forms express the same instructions, facts, resources, and included-skill manifest
- And the installed OpenCode entry points retrieve those shared definitions instead of carrying a second workflow implementation
- And `implement` and `watchdog` retain their one-item contracts
- And definitions already included in a packet are not activated again

## Feature: Native Delegation Instructions

### Scenario: B4 Dispatch a fresh worker for the fixed Claim
- Given either loop's installed command and retrieved instructions in its own dedicated OpenCode supervisor session
- When the CLI returns a dispatch with a worker startup command and continuation command
- Then the instructions direct the supervisor to launch exactly one corresponding native Task worker, omitting `task_id` to create a fresh context
- And the worker runs the supplied startup command in the supplied repository/worktree context to resume only that Claim and retrieve its Instruction Packet
- And the worker follows the one-item skill and exits after its handoff without running `next` or the outer loop
- And no previous worker conversation is passed into a later worker
- And the implementation and Watchdog supervisors neither perform worker judgment nor share a conversation

### Scenario: B5 Follow authoritative continuation and waiting decisions
- Given a dispatched worker has returned, failed, or been interrupted and the dispatch's continuation command is available
- When the supervisor follows the delivered continuation instructions
- Then it runs that exact CLI continuation command to verify the durable handoff before any further dispatch, rather than trusting worker prose
- And only a CLI decision authorizes another worker, including when the other lane already advanced the item
- And the loop uses predecessor waiting with no attempt cap, a new idle window per request, defaults of 15 minutes waiting and 30 seconds polling, and invocation overrides via `--wait` and `--poll`
- And an idle timeout is reported for that lane rather than as global completion
- And an incomplete handoff or operational error stops the lane with the CLI recovery instructions and preserves Claims and partial work
- And interrupted or ambiguous selection without a received dispatch identity stops for explicit inspection rather than blindly calling `next` again
- And no OpenCode adapter reimplements polling, eligibility, bounce policy, handoff validation, or automatic recovery

### Scenario: B6 Delegate both Audit axes independently
- Given an implementation worker reaches the shared Audit gate with native nested delegation available
- When it follows the OpenCode Audit instructions
- Then it records the existing deterministic gate and artifact-integrity facts once and supplies them with the appropriate brief to two parallel native Task calls
- And one fresh role reviews Standards and the other fresh role reviews Artifacts, omitting `task_id` for each
- And neither reviewer receives the other's review or substitutes for the other axis
- And the implementation worker aggregates both reports under their separate axes according to the shared Audit contract
- And neither axis acquires a Workflow Claim or changes Workflow State
- And Watchdog remains a separate fresh worker and does not rerun Audit

### Scenario: B7 Install role-local defaults without changing the supervisor
- Given the installed OpenCode role and command documents
- When their native frontmatter is parsed
- Then all four roles have `mode: subagent` and `model: openai/gpt-6-astra`
- And implementation has `reasoningEffort: low` while Watchdog and both Audit axes have `reasoningEffort: high`
- And the implementation role explicitly permits Task delegation to the two Audit roles using the same names referenced by the adapter
- And loop commands select neither a worker role nor a model and do not introduce an extra subtask context for the supervisor
- And no user-wide config file, supervisor model, or execution/depth limit is modified by installation

## Feature: Configuration Guidance

### Scenario: B8 Override project role settings without replacing prompts
- Given the installed global Markdown roles and the documented project `.opencode/opencode.json` example
- When the example JSON and role Markdown are parsed and reviewed against the native configuration contract
- Then the example sets `agent` entries keyed by the installed role names with only the requested `model` and `reasoningEffort` overrides
- And it leaves role instructions intact by omitting `prompt` and project Markdown replacements
- And the guidance explains that a same-name project Markdown role, even with an empty body, replaces the prompt
- And the guidance recommends nested `.opencode/opencode.json` because root `opencode.json` may load before global Markdown roles
- And the installer never owns or rewrites that project configuration

### Scenario: B9 Halt on unavailable native prerequisites
- Given a required role, model, Task permission, or delegation depth is unavailable for the selected lane
- When the supervisor or implementation worker follows the delivered prerequisite and failure instructions
- Then it halts and identifies the missing prerequisite with actionable native setup guidance
- And the implementation path requires `subagent_depth` of at least two and explicit permissions from supervisor to implementation worker and from that worker to both Audit roles
- And the Watchdog path requires its own native worker delegation permission
- And depth, supervisor permissions, and model/provider setup remain explicit user configuration, not automatic installer edits
- And a failure after selection preserves the Claim and uses the supplied continuation/recovery contract without substituting another worker or same-context Audit

### Scenario: B10 Document the supported operating path and evidence limits
- Given the shipped OpenCode adapter, shared loop instructions, and predecessor CLI contract
- When a user reads the README and relevant capability documentation
- Then the documentation identifies OpenCode as a supported `skl install` target and explains native discovery locations and ownership preservation
- And it shows the two native loop commands in separate sessions, default waiting and explicit `--wait`/`--poll` overrides, and the one-supervisor-per-lane operating assumption
- And it includes role defaults, project JSON override examples, delegation/model prerequisites, and explicit failure recovery
- And it recommends model diversity without enforcing it or claiming the same-model defaults provide it
- And it states that tests cover installation, instruction retrieval/equivalence, and parsed adapter configuration, not live harness execution or model quality
