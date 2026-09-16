# Contract-grounded verification behavior

## Feature: One testing policy across delivery and acceptance

### Rule: Construction choices remain free within the accepted contract

The Workflow requires evidence of accepted behavior, architectural commitments, and mandatory standards rather than a particular order of test and implementation edits. An explicitly frozen obligation remains binding. Reading that contract after preparation remains worker judgment; rendering must not infer its contents at metadata-only startup.

#### Scenario: Deliver flexible construction guidance

- Given an implementation execution and its applicable inspection continuation
- When their instructions and included guidance are rendered
- Then they permit suitable existing verification boundaries, grouped or reused tests, and in-scope refactoring throughout implementation
- And they require the complete accepted behavior rather than only enough behavior to satisfy the tests written so far
- And they impose no global test-first sequence, one-scenario/one-test correspondence, or per-scenario red-green coordination
- And they preserve explicit obligations discovered in the frozen contract

#### Scenario: Distinguish delegated detail from an unresolved decision

- Given the worker will inspect the accepted behavioral and architectural obligations
- When the applicable scope guidance is delivered
- Then an unspecified detail is delegated only when its alternatives preserve those obligations and mandatory standards
- And an unambiguously implied case may be implemented and tested without rewriting the frozen scenario list
- But an unresolved consequential behavioral or architectural choice requires human resolution rather than being inferred from silence

### Rule: Tests establish the right expectation and detect the relevant wrong behavior

Expected results must be grounded in accepted rules, trusted examples or references, or justified properties rather than merely echoing the implementation. Test boundaries must expose the promised consequence and distinguish a plausible violation; exercising code alone does not establish that its effects were checked.

#### Scenario: Verify a regression without requiring test-first chronology

- Given a regression check may be authored before or after its fix
- When the testing guidance is retrieved or included
- Then it requires evidence that the check detects the reported wrong behavior and passes with the fix
- And unrelated setup, import, or execution failures do not establish that sensitivity
- And early reproduction is encouraged without making writing order an acceptance criterion

#### Scenario: Handle an unavailable original reproduction honestly

- Given the original failure cannot be reproduced reliably or safely
- When the regression-evidence guidance is delivered
- Then a faithful isolated reproduction, captured-trace replay, or controlled fault injection may establish protection if it preserves the relevant trigger and observable failure
- And the resulting evidence must state its limitations
- But material uncertainty without credible protection requires a human decision rather than a false verification claim or silent debt

### Rule: Retained protection matters more than test inventory

#### Scenario: Assess the changed test set together

- Given existing and changed checks overlap in the behavior they exercise
- When implementation and Audit testing guidance is rendered
- Then it directs reuse, stronger assertions, consolidation, or removal only while required behavioral and failure-mode protection remains covered
- And removed or weakened assertions receive scrutiny
- And it requires neither a new test for every scenario nor a per-test justification ledger, unique-bug quota, universal mutation score, or duplicate suite
- And unrelated whole-repository test pruning remains outside the change

### Rule: Acceptance criteria govern preferences and evidence gaps consistently

Watchdog reviews every obligation, uses additional executable challenges for concrete risk or uncertainty, and preserves its existing fresh-context, fixed-head, finding-identity, and bounded repeat-review responsibilities. Sharing criteria must not invoke Audit again during Watchdog Review.

#### Scenario: Distinguish an actual violation from another valid implementation

- Given a review execution may encounter either a contractual architecture violation or an alternative implementation that meets the contract
- When its acceptance and finding criteria are delivered
- Then a concrete contractual violation, material risk, or specific evidence gap can block even when the suite is green
- And an evidence-gap finding identifies the obligation, plausible violation, and why the existing evidence does not distinguish it
- But an equally valid implementation or a reviewer's preferred test organization is not a blocker
- And a concrete nonblocking shortcoming may be recorded as debt while a preference alone needs no Debt Marker
- And the same criteria are available to Audit and Watchdog without another Audit execution

### Rule: Evidence covers obligations without dictating test organization

#### Scenario: Render a complete deferred Verification contract

- Given either a first-implementation or Rework submission resource is retrieved at its required later step
- When the resource is rendered with its valid typed inputs
- Then Verification accounts for every rule, scenario, and architectural obligation through grouped many-to-many references to concrete tests, commands, or appropriate inspection evidence
- And it records results and material limitations without demanding a separate row for every obligation
- And prose assurance alone is insufficient for ordinary executable behavior, and listing a gap does not make it acceptable
- And existing Summary, Audit-ledger, Rework-only resolution, and applicable human-owned Manual Verification obligations remain intact
- And agent-authored evidence and decisions remain opaque Result Document content rather than rendering prerequisites or engine-parsed assertions

### Rule: The public testing surface is coherent and safely installed

#### Scenario: Upgrade owned discovery and retrieve the new policy

- Given a supported harness has an owned tdd stub and unrelated user-owned files
- When the updated skill distribution is installed and its testing entrypoints and resources are retrieved
- Then testing is the canonical testing skill and no owned stale stub directs the user to a removed invocation
- And current catalog, embedded resources, descriptions, inclusion metadata, and caller pointers agree
- And unrelated and unowned content is preserved and repeat installation is stable
- And included definitions load once, private Skill Modules remain private, and deferred resources retain their actual owner and timing
- And Markdown and explicit JSON use the post-#44 public interfaces without restoring standalone Implement/Watchdog retrieval or disabled legacy queue loops
