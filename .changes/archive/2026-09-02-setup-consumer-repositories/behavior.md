# Set up Consumer Repositories Behavior

## Feature: Consumer Repository setup

### Rule: Setup resolves and validates its target before mutation

#### Scenario: Infer a GitHub Consumer Repository
- Given a Git repository whose `origin` is a GitHub remote
- And valid authentication is available from the supported token chain
- When `skl setup` is invoked from a nested directory
- Then it prepares the Git root as the Consumer Repository
- And it uses the target branch reported by GitHub
- And it writes no workflow configuration file

#### Scenario: Refuse an ambiguous GitHub remote
- Given a Git repository with multiple GitHub remotes and no GitHub `origin`
- When `skl setup` is invoked without an explicit remote
- Then it reports the ambiguity
- And neither repository files nor GitHub labels are changed

### Rule: Setup owns only explicit repository surfaces

#### Scenario: Maintain the owned AGENTS block
- Given `AGENTS.md` contains user-authored guidance outside one valid workflow marker pair
- When `skl setup` is invoked
- Then it replaces only the marked workflow block with the minimal bootstrap
- And the user-authored guidance remains byte-for-byte unchanged

#### Scenario: Refuse malformed AGENTS ownership markers
- Given `AGENTS.md` contains a missing, nested, reversed, or duplicate workflow marker
- When `skl setup` is invoked
- Then it reports the malformed ownership boundary
- And `AGENTS.md` remains unchanged

#### Scenario: Offer a safe CLAUDE symlink migration
- Given `CLAUDE.md` is absent or contains only the exact legacy `@AGENTS.md` import
- When the user accepts the Setup offer
- Then `CLAUDE.md` is a relative symlink to `AGENTS.md`
- But substantive existing `CLAUDE.md` content is never replaced without explicit authorization

#### Scenario: Retire legacy setup artifacts
- Given `.gitignore` contains unrelated entries
- And `docs/github.md` may exist
- When `skl setup` completes
- Then `.worktrees/` appears exactly once in `.gitignore`
- And unrelated ignore entries remain unchanged
- And `docs/github.md` is absent

### Rule: Setup is repeatable

#### Scenario: Repeat setup without drift
- Given a Consumer Repository has already completed Setup
- When `skl setup` is invoked again with the same choices
- Then the owned repository content is unchanged
- And the required workflow labels have their canonical names, colors, and descriptions
- And unrelated GitHub labels remain unchanged
