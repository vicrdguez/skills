# Decouple Setup and Proposal Publication Behavior

## Feature: Repository-bound Setup

#### Scenario: B1 Preserve Setup effects through semantic preparation
- Given a Consumer Repository with existing substantive agent guidance
- And the integration supplies its repository-bound Backend
- When Setup runs twice with the same choices
- Then the existing owned guidance, ignore rule, and canonical preparation outcome are preserved
- And the GitHub adapter maintains the established Workflow Projection labels
- And unrelated files and labels remain unchanged

#### Scenario: B2 Resolve repository context before Workflow operations
- Given repositories using a GitHub origin, a sole GitHub remote with another name, or ambiguous GitHub remotes
- When Setup or Proposal publication resolves its repository context using the existing invocation choices
- Then accepted choices bind the Backend and selected Git remote to the same repository
- And ambiguous choices are refused before mutation according to the command's established contract
- And selecting another remote does not redirect Git evidence to origin

## Feature: Backend-independent Proposal publication

#### Scenario: B3 Publish a single slice with an opaque identity
- Given a complete pushed slice and a repository-bound in-memory Backend returning an opaque Work Item identity
- When its Proposal is published through the command seam
- Then one Ready for Implementation Work Item references that slice and its Artifact Baseline
- And the supplied issue Markdown is preserved
- And the GitHub adapter retains the existing native issue number and artifact-reference representation for the equivalent GitHub publication

#### Scenario: B4 Preserve coordinated publication ordering and graph validation
- Given a Proposal with a dependent slice declared before its blocker
- When publication is requested
- Then the blocker is published before the dependent
- And both belong to one Coordination Item
- And each Work Item becomes Ready only after its body and declared relationships are durable
- But a cyclic, self-referencing, or unknown dependency graph is refused without backend mutation

#### Scenario: B5 Resume partial publication without duplicate records
- Given publication was interrupted after a Work Item or relationship became durable
- When the same Proposal is retried
- Then existing unambiguous records are reused by their opaque identities
- And remaining publication steps complete in the established order
- And conflicting records produce the existing repair or human-decision outcome rather than being overwritten

#### Scenario: B6 Preserve Git preflight independently of Backend binding
- Given a slice whose pushed head differs, whose target commit is missing, or whose Artifact Baseline is invalid
- When Proposal publication is requested with its repository-bound Backend and selected Git repository context
- Then the existing failing invariant and repair outcome are returned
- And no Proposal records are mutated
- And correcting the Git evidence permits the same publication to succeed
