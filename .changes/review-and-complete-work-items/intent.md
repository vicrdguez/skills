# Review and complete Work Items

## Why
Watchdog currently spends fresh agent context reconstructing queue order, Claims, review rounds, GitHub mutations, and completion state from protocol prose. Its valuable work is adversarial judgment; deterministic review handoffs and human-merge observation belong in the Workflow Engine.

## What
Add semantic Watchdog and status operations that supply a fixed review packet to a fresh Agent Worker, publish its opaque findings under an explicit verdict, enforce convergence, and carry the Work Item through Ready for Merge, human merge, Dependency release, and Coordination Item completion.

## Scope
- Select and best-effort claim the oldest unclaimed Awaiting Review Submission
- Return structured `work_available` and `no_work` outcomes and support explicit resume by Work Item identity
- Pin the Submission head being reviewed and supply Artifact Baseline, Artifact Completion, audit ledger, earlier findings, and later human comments as available facts or opaque content
- Preserve Watchdog as an independently invoked fresh Worker Session
- Preserve its adversarial contract proof, test-strength review, critical-risk scan, Full Gate, and independent artifact verification
- Preserve the rule that Watchdog never reruns Audit
- Preserve full first review and incremental repeat review behavior
- Accept agent-authored summary and inline finding bodies as opaque Markdown with structured anchor inputs
- Let the semantic Watchdog verdict request pass, rework, or Needs Human without parsing the prose
- Preserve stable agent-authored `W<n>` finding identities and pass raw human comments to the Watchdog Worker
- Apply one finding-driven bounce; send a second failed review to Needs Human
- Keep Synchronization Rework separate so merge-conflict repair does not consume the review-failure bounce
- Permit Watchdog to add only non-functional `DEBT(#<pr>/W<n>)` source comments for passing code-local Notes
- Keep Debt Marker correctness, formatter/parser checks, and `git diff --check` in Watchdog instructions rather than CLI validation
- On pass, accept an agent-authored final PR body containing the Manual Verification checklist and append the machine-owned issue-closing footer
- Transition a passing, mergeable Submission to Ready for Merge through the existing `done` label without closing its source issue
- Create no artifact archive and keep the retired ledger absent through review and rework
- Detect a non-mergeable accepted Submission and create Synchronization Rework with a newly pinned Target Snapshot
- Derive Merged only after GitHub reports that the human merged the Submission
- Let GitHub close the source issue through `Closes #N` in the merged PR
- Treat Dependencies as satisfied only after semantic Merged state
- Close a Coordination Item on the next CLI observation after every child is Merged
- Preserve Needs Human resume state and accept explicit human relabeling to Rework or Awaiting Review
- Recognize Superseded Work Items and retain their lightweight branch reference for later Explore
- Expose normalized workflow status without mutating valid state
- Install updated Pi-only implementation and Watchdog queue adapters that continue with fresh Worker Sessions only after verified handoffs
- Adopt unambiguous legacy and partially transitioned review records lazily

## Out of Scope
- Performing Watchdog judgment, interpreting findings, or deciding human directives
- Parsing or validating finding, audit, Result Document, issue, or PR Markdown
- Rerunning Audit or executing any Consumer Repository gate from the CLI
- Functional code edits by Watchdog
- Automatic merge, merge authorization, or Manual Verification completion
- More than one automated finding-driven bounce
- Atomic concurrent-worker Claims or Agent Harness identity attestation
- Automated queue draining outside Pi in V1
- Deleting remote branches or guaranteeing post-merge artifact-commit retention

## Definition of Done
- [ ] Watchdog deterministically claims or resumes the oldest eligible Awaiting Review Work Item and returns a fixed concrete packet to a fresh Worker Session, or reports `no_work` without mutation.
- [ ] A first failing review publishes the agent's opaque findings and transitions the same Work Item to Rework with its Claim released.
- [ ] A second failing review publishes the current ledger and enters Needs Human instead of issuing another bounce.
- [ ] A passing review with no active blocking or human verdict becomes Ready for Merge, carries Manual Verification and the issue-closing footer, and creates no artifact archive.
- [ ] Passing code-local Notes may be materialized only as Debt Marker comments under preserved Watchdog checks, without CLI Markdown or source parsing.
- [ ] A merge conflict after review creates Synchronization Rework with a fresh Target Snapshot and does not spend the finding-driven bounce.
- [ ] Human merge is observed as Merged, closes the source issue through GitHub, releases dependent Work Items, and closes a fully merged Coordination Item on the next observation.
- [ ] Human comments and explicit requeue projections resume Needs Human work in the correct Rework or Awaiting Review state without CLI interpretation of prose.
- [ ] Pi queue adapters preserve fresh-context separation and stop or continue only from structured verified Workflow outcomes.

## Manual verification
- [ ] Run one implementation and Watchdog queue iteration in separate Pi sessions and confirm that each Worker Session is fresh and the loop follows only structured `skl` outcomes.
- [ ] Merge a Ready for Merge PR in GitHub and confirm its source issue closes immediately and `skl status` subsequently reports Merged and updates any completed Coordination Item.
