# Install native OpenCode Skill Stubs

## Why
`skl install` currently writes common Skill Stubs for Pi, Codex, and Claude Code, but omits OpenCode. The local workaround points OpenCode at Pi's skill directory. Basic OpenCode discovery should use its own native directory and should not depend on Pi installation or another harness's files.

## What
Extend the existing common-stub installation to OpenCode's default global skill directory, preserve the existing ownership rules, and document how to remove the temporary cross-harness discovery override after installation.

## Scope
- Install the same catalog of common Skill Stubs under `~/.config/opencode/skills/<name>/SKILL.md` as part of the existing `skl install` command.
- Preserve the current frontmatter, `skl.stub/v1` ownership marker, and CLI delegation behavior, including Implement's existing Work Start entrypoint.
- Write self-contained native stubs, not symlinks or pointers to Pi, Codex, Claude, or authoring-source directories.
- Apply the current refresh, idempotence, and preservation behavior to owned and unowned OpenCode skill files.
- Keep installation for the other supported harnesses and Pi-only queue assets unchanged.
- Extend the existing command-level installation tests using temporary homes.
- Update the supported-harness lists in README, the skill-distribution capability, and ADR 0001 without redesigning distribution.
- Document the one-time local cutover: install the new stubs, remove only obsolete source-tree or Pi-directory discovery entries from OpenCode configuration, restart, and verify native discovery. The installer does not edit those settings.

## Out of Scope
- OpenCode queue prompts, workers, delegation, model configuration, or changes to the separate queue-draining design.
- Automatic configuration migration, deleting another harness's skills, or taking ownership of unowned files.
- A new plugin, harness framework, stub template, installer command, target-selection flag, or configuration-root discovery mechanism.
- Changes to Skill Definitions, resource delegation, packet schemas, Workflow Mechanics, or proposal #21.
- Adding the OpenCode executable as a dependency of the normal test suite.
- Editing personal OpenCode configuration during this proposal or as a tracked implementation change.

## Definition of Done
- [ ] A fresh `skl install` creates native OpenCode stubs for every catalog skill with the existing frontmatter, ownership marker, and CLI delegation, without depending on another harness's files.
- [ ] Reinstallation refreshes outdated owned OpenCode stubs and leaves current stubs unchanged in content.
- [ ] Unowned OpenCode skills and unrelated configuration files are preserved, while installation still creates missing nonconflicting stubs.
- [ ] Existing Pi, Codex, and Claude Code installation behavior and Pi-only queue assets remain intact; no OpenCode queue assets are introduced.
- [ ] Existing CLI-level tests prove native installation, refresh, idempotence, and preservation using temporary homes, and the full documented gate passes.
- [ ] Documentation lists OpenCode's native installation path and explains removal of the temporary cross-harness override without prescribing deletion of unrelated skills or configuration.

## Manual verification
- [ ] After installing the reviewed binary, remove only obsolete raw-source and Pi skill-directory entries from OpenCode's `skills.paths`, restart OpenCode, and confirm the workflow skills load from `~/.config/opencode/skills/` as thin CLI stubs without claiming work.
