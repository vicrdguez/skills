# Publish Workflow Proposals

## Why
Propose currently makes an Agent Worker perform deterministic GitHub publication, dependency wiring, baseline recording, and cleanup from copied protocol prose. Those mechanics cost context, are easy to execute partially, and vary across harnesses.

## What
Add semantic Propose commands and specialized instructions that let the agent author and prepare approved slices while `skl` validates their fixed Git state and publishes their Work Items, Coordination Item, Dependencies, and Ready projections.

## Scope
- Preserve Propose's existing reasoning about tracer-bullet slices, seams, artifact content, parent decisions, and dependency edges
- Return concrete Propose instructions from the embedded Skill Definition
- Keep durable `CONTEXT.md`, ADR, and capability changes committed on the target branch before slice branches are published
- Let the Agent Worker create slice worktrees, branches, artifacts, commits, and pushes with ordinary Git commands
- Accept agent-authored parent and Work Item bodies as opaque Markdown files
- Accept child slugs and cross-slice Dependency edges as explicit command inputs rather than a checked-in proposal manifest
- Validate the complete declared Proposal before the first GitHub publication mutation
- Require fixed durable-document paths to contain no uncommitted changes
- Require each slice branch to descend from the target commit observed for publication
- Identify Artifact Baseline from the unique first-parent absent-to-present `.changes/<slug>` transition
- Require every baseline to introduce the complete mandatory artifact set at once and to be the published branch head
- Create one Coordination Item only for multi-slice Proposals
- Create child Work Items in dependency order, wire parent and native GitHub Dependency relationships, and apply `ready` last per child
- Keep blocked Work Items in Ready for Implementation while eligibility remains gated until every blocker is Merged
- Resume partial publication forward by reusing only unambiguous existing GitHub records
- Preserve completed children after a later publication step fails; perform no rollback or hidden batch transaction
- Before publication, remove clean conventional worktrees and matching local branches only for Work Items already observed Merged
- Preserve and report dirty, structurally unexpected, remote, and unrelated Git state without blocking otherwise valid publication
- Keep Work Item issue bodies as thin pointers authored from the supplied template; do not parse, generate, or duplicate artifact prose

## Out of Scope
- Deciding slice boundaries, titles, prose, architectural seams, or Dependencies
- Creating new worktrees, branches, commits, or pushes for the proposal
- Parsing or validating Markdown content
- A durable proposal manifest, CLI database, rollback protocol, or hidden publishing state
- Deleting remote branches
- Making blocked Work Items eligible before their Dependencies are Merged
- Implementing work or opening review Submissions

## Definition of Done
- [x] Propose instructions leave slice design and artifact authorship with the Agent Worker while replacing deterministic publication prose with semantic `skl` operations.
- [x] A valid single-slice Proposal becomes one Ready Work Item linked to its pushed branch and identifiable Artifact Baseline.
- [x] A valid multi-slice Proposal becomes one Coordination Item and dependency-ordered child Work Items with explicit parent and Dependency relationships.
- [x] Invalid durable-document, branch, baseline, artifact-shape, or graph preconditions refuse publication before the first GitHub mutation and explain the repair.
- [x] A failed partial publication can be retried forward without duplicate unambiguous records or rollback of valid completed children.
- [x] Proposal cleanup removes only safe local Git state for Merged Work Items and preserves every unsafe, remote, or unrelated candidate.
- [x] Work Item Markdown remains agent-authored and opaque to `skl`, with no proposal bundle or metadata file added to the repository.

## Manual verification
- [ ] Open the published GitHub Coordination Item and children and confirm that native parent and Dependency relationships are visible in the GitHub interface.
