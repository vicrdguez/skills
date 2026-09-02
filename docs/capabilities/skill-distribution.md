# Skill distribution

`skl` gives supported Agent Harnesses access to the same repository-owned Skill Definitions and supporting resources without requiring a harness plugin package.

## Behaviors

- Installs the user-level binary from this repository through Go.
- Installs embedded common Skill Stubs into the Pi, Codex, and Claude Code user skill directories.
- Retrieves a concrete Instruction Packet for an invoked skill as rendered Markdown or equivalent typed JSON.
- Includes guaranteed supporting Skill Definitions once per packet and retrieves conditional Skill Resources only when needed.
- Keeps Setup as a direct deterministic command rather than an agent skill.

## Out of scope

- Prebuilt releases, package marketplaces, automatic updates, and uninstall management in V1.
- Consumer Repository overrides of Skill Definitions or Workflow Mechanics.
- Starting or controlling Agent Harness sessions.
