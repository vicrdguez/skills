# Install the repository-wide simplicity standard Behavior

## Feature: Setup delivers a standing simplicity standard

### Background:
- Given a valid Consumer Repository supported by the existing Setup test infrastructure
- And backend validation and the existing Setup choices are satisfied

#### Scenario: Fresh Setup installs the approved standard by default
- Given the repository has no `AGENTS.md`
- When `skl setup` runs for the repository
- Then the managed workflow block contains the exact Simplicity section in `plan.md` once
- And the existing Workflow entrypoint instructions remain present
- And no simplicity-specific option or confirmation is required

#### Scenario: Setup refreshes owned guidance without rewriting user guidance
- Given `AGENTS.md` contains a valid pre-change managed workflow block
- And user-authored guidance appears before and after that block
- When `skl setup` runs for the repository
- Then the managed block contains the exact approved Simplicity section once
- And the existing Workflow entrypoint instructions remain present
- And the user-authored text before and after the block is byte-for-byte unchanged

#### Scenario: Repeating Setup does not duplicate or change the standard
- Given Setup has installed the approved Simplicity section
- When `skl setup` runs again with the same existing Setup choices
- Then `AGENTS.md` is byte-for-byte unchanged
- And its managed block contains the approved Simplicity section once

## Feature: Instruction retrieval replaces only Audit's Ponytail integration

### Background:
- Given the embedded catalog includes Audit with only the Ponytail removals enumerated in `plan.md`

#### Scenario Outline: Direct Audit retrieval preserves ordinary review without Ponytail
- When the public `skl skill` command retrieves `audit` in <format> format
- Then its Audit definition is the approved post-removal text
- And it contains no Ponytail discovery, invocation, special dispatch, output, word allowance, or aggregation instructions
- And it retains the ordinary two-axis review, standards and smell baseline, checks, finding classifications, and reporting instructions
- And it does not inject the new Simplicity section

Examples:
| format |
| Markdown |
| JSON |

#### Scenario Outline: Implementation packets bundle the same Audit without a new simplicity dependency
- When the public `skl skill` command retrieves `implement` in <format> format
- Then its bundled Audit definition equals the Audit definition retrieved directly
- And the remaining supporting definitions, resources, and manifest membership are unchanged by this change
- And it does not inject the new Simplicity section or add a simplicity skill or resource

Examples:
| format |
| Markdown |
| JSON |
