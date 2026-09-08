# Native OpenCode Skill Stub Behavior

## Feature: Native OpenCode installation

#### Scenario: Install the common catalog in OpenCode's own directory
- Given a temporary user home without existing workflow skill installations or OpenCode path overrides
- When `skl install` runs through the CLI command seam
- Then every catalog skill has a regular `SKILL.md` under `.config/opencode/skills/<name>/`
- And each file retains the existing skill frontmatter and `skl.stub/v1` marker
- And each file delegates to the same `skl` command as the corresponding common stub for the other harnesses
- And the OpenCode stub contents do not reference another harness's installation or the authoring skill tree
- And no resource tree or OpenCode queue adapter is installed beside those stubs

#### Scenario: Refresh an owned OpenCode stub and repeat installation
- Given an installed OpenCode stub carrying the current ownership marker has stale content
- When `skl install` runs again
- Then that stub is refreshed to the current embedded common stub
- And a further installation leaves the resulting stub contents unchanged
- And no additional discovery configuration is required or written by the installer

#### Scenario: Preserve unowned skills and user configuration
- Given a same-name OpenCode skill file without the current ownership marker
- And unrelated OpenCode skill and configuration files
- When `skl install` runs
- Then those files remain byte-for-byte unchanged
- And missing nonconflicting native OpenCode stubs are still installed
- And the installer does not delete or rewrite an existing `skills.paths` setting

#### Scenario: Retain existing harness installations
- Given a temporary user home
- When `skl install` runs with native OpenCode support
- Then Pi, Codex, and Claude Code still receive their existing common stubs
- And the existing Pi queue assets remain confined to Pi's installation locations
- And OpenCode's stubs are independent regular files rather than links to any of those locations
