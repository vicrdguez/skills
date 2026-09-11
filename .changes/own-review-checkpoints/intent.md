# Make Review Checkpoints CLI-Owned

## Why

Review scope and convergence currently depend on hidden issue metadata, worker extraction of prior review heads, and a timeline-derived finding-bounce count. Those mechanisms conflate completed reviews with handoff attempts and make finding-driven Rework depend on a cached Git revision. The Workflow Engine should own these deterministic facts without creating a durable lifecycle database.

## What

Deliver approved candidate-first slice 3, with no blocking Dependencies. The CLI maintains one worktree-private Review Checkpoint for the latest completed Watchdog Review, supplies review scope and a fixed-number submission command, and counts completed reviews rather than finding bounces. Publication retries replay the intended round instead of incrementing it again. Agent Workers retain review judgment, findings, Git work, and project checks.

## Scope

- Store `<count>:<sha>` in `.watchdog` under the selected Work Item worktree's private Git directory resolved by `git -C <selected-worktree> rev-parse --absolute-git-dir`. Each Work Item has a dedicated worktree.
- Validate checkpoint data and submit inputs, replace the single checkpoint atomically, and distinguish absence from corruption or read failure.
- Supply completed count, full or incremental scope, the invocation's actual current PR head, and a concrete command carrying `--review-number` through existing Instruction Packet and Result Document context.
- Publish complete review evidence, record the intended number and actual reviewed SHA, then release `wip`. Reconcile retries using that fixed command and exact published observations; stop on ambiguity.
- Count pass, rework, and Needs Human reviews. Default limit two permits automatic Rework only for a failure below the limit; the count never rejects a passing review.
- Remove review-specific hidden issue fields `watchdog_head`, `reviewed_head`, and `review_round_head` as read/write obligations. Keep reviewed and final heads as explicit per-invocation facts, not a persisted in-progress pin.
- Remove implementer extraction and required diff-baseline use of a previous reviewed head. Adjust Audit only where it requires that previous-review cache; preserve its judgment, gate, artifact checks, and explicit fixed-point interface.
- Retain valid counts when previous Git objects are unavailable or non-ancestral. Retain the checkpoint across Rework, Needs Human, ledger retirement, and working-tree cleaning; remove it only after verified `done` publication.
- Prove the behavior through public CLI execution with real worktrees and the actual GitHub adapter under controlled HTTP faults.

## Out of Scope

- Slice 1's artifact marker protocol or endpoint-validation redesign.
- Slice 2's target policy and removal of conflict-driven Synchronization Rework. Until that slice is Merged, its current diversion still applies independently of review-count policy.
- Slice 4's general handoff recovery, removal of other hidden metadata or the implementation transition journal, and removal of timeline-based partial-handoff reconstruction.
- Slice 5's object-free startup, candidate discovery, or worktree-preparation redesign. This slice does not promise startup without existing artifact objects.
- Parsing PR timelines for Review Count, parsing worker prose to recover a round, or parsing source code to validate Debt Markers.
- A Claim Token, nonce, lease, new journal, second persistent metadata file, checkpoint backup, shared registry, or lifetime review cap across worktree loss.
- New dependencies, general repository/storage abstractions, submodule support, fallback storage in the main worktree, or a new review-limit configuration interface.
- Human merge authority, finding identity/disposition policy, automatic checkpoint migration, and rewriting or deleting old GitHub comments.

## Definition of Done

- [ ] D1: CLI invocations from any supported repository location read and update only the selected Work Item's worktree-private checkpoint; workers are not taught its path, format, or file operations. Covered by B1.
- [ ] D2: Watchdog next/resume supply consistent completed-count, scope, current-head, and concrete fixed-number handoff facts without recording an in-progress review; unavailable previous revisions widen scope without losing a valid count. Covered by B2 and B3.
- [ ] D3: Missing checkpoints start at zero; corrupt or unreadable checkpoints and invalid submission counts/SHAs cause actionable refusals without reset, publication, or Claim release. Covered by B4.
- [ ] D4: Every completed review, including Needs Human and a genuinely new review at the same SHA, consumes exactly one intended round; default two caps automatic failure-driven Rework, not pass. Covered by B5 and B6.
- [ ] D5: Complete evidence precedes atomic checkpoint replacement, which precedes Claim release. Faults and exact fixed-number retries preserve this order without duplicate evidence or double counting; conflicting or insufficient observations stop rather than guess. Covered by B7, B8, B9, and B10.
- [ ] D6: Explicit actual-reviewed and final-head inputs preserve existing Debt Marker pass safety, including pushed-head and ancestry checks, without a persisted in-progress head or CLI source parser. Covered by B11.
- [ ] D7: Finding-driven implementation and its Audit proceed from current code, historical artifacts, and feedback without a previous-review cache or extracted SHA; unrelated worker obligations remain intact. Covered by B12.
- [ ] D8: The checkpoint survives nonterminal work and cleaning, is deleted only after verified `done`, and cleanup failure is a warning rather than a failed verdict. Worktree loss/recreation intentionally resets the count without reconstruction. Covered by B13, B14, and B16.
- [ ] D9: Review-count consumption is independent of target/conflict policy, status observation, and partial-handoff reconstruction. Timeline history is not a count source, and no alternate completion caller invents a review or bypasses required checkpointing. Covered by B15.

## Manual verification

None.
