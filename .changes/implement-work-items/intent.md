# Implement Work Items

## Why
Implement currently spends agent context selecting and claiming work, checking Dependencies, synchronizing Git state, validating artifacts, and translating a completed implementation into GitHub issue and PR mutations. Those deterministic mechanics obscure the TDD and Audit judgment the Agent Worker should own.

## What
Add semantic Implement operations that select or resume one Work Item, return a concrete bundled Instruction Packet, validate workflow-owned Git invariants, and publish the agent's completed first-pass, rework, or Needs Human handoff without parsing its prose or running project checks.

## Scope
- Select the oldest eligible Rework Work Item before the oldest eligible Ready for Implementation Work Item, using stable Work Item identity as a tie-breaker
- Require every Dependency to be Merged before a Ready Work Item is eligible
- Skip `wip` and Needs Human projections during ordinary selection
- Apply the existing best-effort `wip` Claim under the V1 single-operator assumption
- Resume interrupted work by explicit stable Work Item identity or its unambiguous conventional worktree
- Return successful structured `work_available` and `no_work` outcomes
- Pin the target commit observed for new implementation and Synchronization Rework in the Instruction Packet
- Tell the Agent Worker to perform the existing target merge itself; normal finding-driven Rework performs no new synchronization
- Bundle Implement, TDD, Audit, Design, and Domain definitions once without changing their agent behavior
- Preserve one red-green commit per behavioral task and implementation-owned ordinary Git commits and pushes
- Preserve Audit's two judgment axes, parallel execution when available, sequential fallback, Full Gate, finding dispositions, and PR audit ledger
- Keep project tests, lint, typecheck, formatters, parser checks, and Audit entirely agent-run
- Derive Artifact Baseline, Artifact Completion, and deletion from first-parent Git tree transitions
- Validate frozen-ledger deltas, completion ticks, Manual Verification exclusions, deletion, post-deletion absence, and Target Snapshot ancestry
- Allow incomplete artifacts to remain only for an implementation-stage Needs Human draft
- Require the Agent Worker to commit and push Artifact Completion and the subsequent ledger deletion before normal submission
- Accept agent-authored issue comments and PR bodies as opaque Markdown in private temporary files
- Append the machine-owned `Closes #<issue>` footer to Submission bodies
- Create a new review PR for first-pass work or update the existing PR for Rework
- Transition first-pass `ready + wip` and Rework `rework + wip` into Awaiting Review only after all deterministic preconditions pass
- Retain the Claim and current state on `fix_required`, returning exact repair instructions to the current or resumed Agent Worker
- Publish a Needs Human issue handoff before code exists or a draft Submission when work must be preserved
- Adopt existing unambiguous issue, PR, branch, worktree, and artifact projections lazily

## Out of Scope
- Writing product code, tests, audit judgments, finding dispositions, commit messages, or Result Document prose
- Running or configuring Consumer Repository Full Gates
- Running Audit inside the CLI or controlling its subagents
- Parsing Audit ledgers, Result Documents, PR bodies, or human prose
- Rebasing, force-pushing, or merging branches on the Agent Worker's behalf
- Atomic Claims for concurrent workers
- Reviewing or approving a Submission
- Merging code

## Definition of Done
- [ ] Implement selection deterministically returns and claims the correct oldest eligible Rework or Ready Work Item, or reports `no_work` without mutation.
- [ ] Interrupted claimed work resumes by stable identity while ordinary selection continues to skip it.
- [ ] A new implementation receives a fixed Target Snapshot and concrete once-only bundled instructions while the Agent Worker retains all code, test, Audit, commit, and push behavior.
- [ ] A valid first-pass implementation with a completed and retired ledger becomes one Awaiting Review Submission whose source issue remains open and whose body will close it on merge.
- [ ] A valid Rework implementation updates the existing Submission and returns it to Awaiting Review without recreating artifacts or synchronizing the target again.
- [ ] Invalid target, remote-head, artifact, or state preconditions return `fix_required`, retain the Claim and state, and can be repaired and retried.
- [ ] An implementation-stage human decision is durably published on the issue when no code exists or on a draft Submission when work exists, then enters Needs Human.
- [ ] Implement and Audit Skill Definitions preserve their judgment behavior while removing only deterministic mechanics now owned by `skl`.

## Manual verification
- [ ] Run an Implement Skill Stub in a fresh Agent Harness context and confirm the worker follows the concrete packet, runs Audit, and reaches the expected GitHub handoff without consulting `docs/github.md`.
