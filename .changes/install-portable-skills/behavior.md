# Install portable skills Behavior

## Feature: Harness-independent skill distribution

### Rule: One binary distributes common Skill Stubs

#### Scenario: Install supported Skill Stubs
- Given `skl` was installed from this repository with Go
- And isolated user skill directories for Pi, Codex, and Claude Code
- When `skl install` is invoked
- Then every current Skill Definition except `dev-setup` has an owned stub in each supported harness directory
- And the stub content delegates to `skl`
- And no harness plugin manifest is required

#### Scenario: Refresh only owned stubs
- Given supported harness directories contain current `skl` stubs and unrelated user files
- When `skl install` is invoked again
- Then the owned stubs match the embedded versions
- And unrelated user files remain unchanged

### Rule: Embedded definitions are the authoritative instruction source

#### Scenario: Retrieve rendered skill instructions
- Given a non-workflow Skill Definition and its resources were embedded at build time
- When its instructions are requested in Markdown format
- Then the returned Instruction Packet contains the authoritative embedded instructions
- And unrequested Skill Resources are absent

#### Scenario: Retrieve equivalent typed instructions
- Given an Instruction Packet can be rendered as Markdown
- When the same invocation requests JSON
- Then the JSON identifies the same protocol, skill, included skills, facts, resources, and instructions
- And stdout contains only the requested payload

#### Scenario: Retrieve one named resource
- Given an embedded Skill Definition owns a named Skill Resource
- When that resource is explicitly requested
- Then `skl` returns that canonical embedded resource
- And unrelated resources are absent

### Rule: Guaranteed skill dependencies load once

#### Scenario: Bundle guaranteed supporting skills
- Given an invoked Skill Definition always requires one or more supporting Skill Definitions
- When `skl` builds its Instruction Packet
- Then each required definition appears exactly once
- And every bundled definition is named in `included_skills`
- And an installed stub instructs the harness not to reactivate an already included skill

#### Scenario: Ignore Consumer Repository overrides
- Given a Consumer Repository contains a file resembling an installed Skill Definition
- When `skl` returns an Instruction Packet
- Then the packet uses the definition embedded in the running binary
- And repository content cannot override Workflow Mechanics or agent behavior
