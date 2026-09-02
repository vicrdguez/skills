# Publish Workflow Proposals Behavior

## Feature: Proposal publication

### Rule: The Agent Worker authors while the Workflow Engine publishes

#### Scenario: Retrieve concrete Propose instructions
- Given a confirmed shared understanding is ready to materialize
- When the Propose Skill Stub delegates to `skl`
- Then the Instruction Packet preserves slice, seam, and artifact-authoring judgment
- And deterministic GitHub publication is expressed through semantic `skl` commands
- And the packet contains no copied board protocol

#### Scenario: Publish one prepared slice
- Given one pushed slice branch descends from the observed target commit
- And its head is the unique commit that introduces the complete frozen Implementation Ledger
- And the agent supplies an opaque thin-pointer issue body
- When the Proposal is published
- Then one Work Item issue is created or unambiguously reused for the slice
- And `ready` is applied only after its body and branch facts are durable
- And the Artifact Baseline remains discoverable from Git history

#### Scenario: Publish a dependency-ordered multi-slice Proposal
- Given a preflight-valid set of pushed slice branches
- And agent-authored Coordination Item and child bodies
- And an acyclic graph of explicit Dependency edges
- When the Proposal is published
- Then one Coordination Item groups every child Work Item
- And children are created in blocker-first order
- And GitHub exposes each parent and Dependency relationship
- And each child receives `ready` only after its own relationships are durable

### Rule: Invalid preparation is repaired before publication

#### Scenario: Refuse an invalid Proposal preflight
- Given a durable-document path is dirty, a slice misses the observed target, a baseline is ambiguous or incomplete, or the Dependency graph is invalid
- When publication is requested
- Then the outcome is `fix_required` with the failing invariant and repair instruction
- And no GitHub record or label is changed

### Rule: Publication resumes forward without transactions

#### Scenario: Resume a partial publication
- Given an earlier publication created some valid children before an operational failure
- When the same Proposal is published again
- Then each unambiguous valid record is reused
- And unfinished children and relationships continue in dependency order
- And completed children remain Ready and eligible according to their Dependencies
- But an ambiguous existing record stops for human resolution instead of being guessed or deleted

### Rule: Cleanup never sacrifices user work

#### Scenario: Clean only safe Merged worktrees
- Given GitHub reports some prior Work Items as Merged
- And their conventional local worktrees include clean, dirty, and structurally unexpected candidates
- When proposal cleanup runs
- Then clean conventional worktrees and their matching local branches are removed
- And dirty or unexpected candidates are preserved and reported
- And remote branches and unrelated worktrees are untouched
- And safe proposal publication may continue

### Rule: Prose is opaque transport

#### Scenario: Publish supplied Markdown without interpretation
- Given the agent supplies parent and child Markdown bodies from its instruction templates
- When `skl` publishes the Proposal
- Then the supplied bodies are sent without semantic parsing or validation
- And no duplicate proposal document is written into the Consumer Repository
