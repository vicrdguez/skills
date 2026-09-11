# Skill distribution

`skl` gives supported Agent Harnesses access to the same repository-owned Skill Definitions and supporting resources without requiring a harness plugin package.

## Behaviors

- Installs the user-level binary from this repository through Go.
- Installs embedded common Skill Stubs into the Pi, Codex, Claude Code, and OpenCode user skill directories. OpenCode uses `~/.config/opencode/skills/<name>/SKILL.md` with independent regular files, not cross-harness links or copied resource trees.
- Refreshes only stubs carrying the `skl.stub/v1` ownership protocol and preserves unrelated harness files.
- Leaves OpenCode discovery configuration unchanged. After installing native stubs, users can remove only obsolete Pi or raw-source `skills.paths` entries, retain unrelated settings and skills, then restart OpenCode to verify native discovery.
- Retrieves a concrete Instruction Packet for an invoked skill as rendered Markdown or equivalent typed JSON.
- Includes guaranteed supporting Skill Definitions once per packet and retrieves conditional Skill Resources only when needed.
- Resolves definitions and resources only from the running binary's embedded catalog; Consumer Repository files cannot override them.
- Keeps Setup as a direct deterministic command rather than an agent skill.
- Embeds and installs Pi-only implementation and Watchdog queue prompts, fresh-context runner definitions, and a structured-outcome continuation check under `.pi/agent/`. Refreshes only `skl.pi/v1` assets and preserves user-owned files.
- Pi queues use semantic Work Start commands rather than selecting board records. They continue only after a verified stage handoff with the Claim released, and stop on `no_work`, the item limit, incomplete Claims, or ambiguous results. Implementation and Watchdog schedulers use separate Pi sessions.
- Watchdog packets include fixed review facts and historical contract files without bundling Audit or duplicating supporting definitions. Pi runners consume the packet manifest instead of preloading the same skills again.

## Out of scope

- Prebuilt releases, package marketplaces, automatic updates, and uninstall management in V1.
- Consumer Repository overrides of Skill Definitions or Workflow Mechanics.
- Starting or controlling Agent Harness sessions.
