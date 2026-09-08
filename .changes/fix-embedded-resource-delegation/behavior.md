# Fix embedded resource delegation Behavior

## Feature: Embedded resource delegation

### Rule: Instructions name CLI retrieval, not authoring-checkout paths

#### Scenario Outline: Retrieve a resource named by a standalone definition
- Given a Consumer Repository working directory without this repository's skill sources
- When the worker retrieves the `<skill>` definition through `skl skill <skill>`
- Then the applicable existing pointer names `skl skill --resource <resource> <skill>`
- And executing that resource command returns the expected embedded material
- And the surrounding mandatory or conditional use of that material is unchanged

Examples:

| skill | resource |
| domain | reference/CONTEXT-FORMAT.md |
| domain | reference/ADR-FORMAT.md |
| domain | reference/CAPABILITIES-FORMAT.md |
| design | reference/DEEPENING.md |
| design | reference/DESIGN-IT-TWICE.md |
| propose | reference/intent.md |
| propose | reference/behavior.md |
| propose | reference/plan.md |
| propose | reference/tasks.md |
| tdd | reference/tests.md |
| tdd | reference/mocking.md |
| writing-for-agents | SKILL-MECHANICS.md |

#### Scenario: Follow nested and cross-skill pointers
- Given a worker has fetched Design's `reference/DESIGN-IT-TWICE.md`, Propose's `reference/tasks.md`, or Writing for Agents' `SKILL-MECHANICS.md`
- When the worker reaches their existing supporting-material pointers
- Then Design's sibling reference names `skl skill --resource reference/DEEPENING.md design`
- And Propose's capability-format reference names `skl skill --resource reference/CAPABILITIES-FORMAT.md domain`
- And references back to the Design or Writing for Agents parent definition identify `skl skill design` or `skl skill writing-for-agents` when that definition is not already supplied
- And none of these pointers requires a relative `SKILL.md` file beside the fetched resource

#### Scenario: Preserve resource ownership in bundled definitions
- Given an Implement Instruction Packet includes TDD, Audit, Design, and Domain
- When a worker reaches a resource pointer inside an included definition
- Then the pointer uses that definition's owning skill rather than `implement`
- And it retrieves the same embedded material as the standalone resource command
- And no supporting definition needs duplicate activation merely to resolve a resource

#### Scenario: Hand the smell baseline to the Standards reviewer without a source path
- Given Audit has located Consumer Repository standards files
- When it prepares the Standards reviewer brief
- Then the brief provides `skl skill --resource reference/smells.md audit` and directs the reviewer to read its output
- And it does not demand an absolute filesystem path to the embedded smell baseline
- And the Consumer Repository standards-file paths, precedence, and review responsibilities are retained

## Feature: Source-independent distribution smoke check

#### Scenario: Install stubs and retrieve embedded instructions outside the source checkout
- Given a temporary user skill home and a temporary working directory without installed authoring sources
- When the existing CLI command seam runs installation and read-only skill/resource retrieval
- Then the supported harness locations contain thin CLI-delegating stubs rather than copied definitions or resource trees
- And rendered and typed skill retrieval expose the corrected delegation instructions
- And standalone, nested, cross-skill, and included-definition resource commands return meaningful expected content
- And test expectations do not read the source checkout or merely compare two calls to the same retrieval function
- And no workflow work is claimed, no real user settings are modified, and no GitHub operation is required
