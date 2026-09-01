# Retire change artifacts before review Plan

## Approach
Treat change artifacts as a branch-local implementation ledger with two immutable checkpoints: the proposal's `Artifact baseline` and the fully ticked `Artifact completion`. The implementor retires the files before review. Audit and watchdog inspect code and artifacts through separate inputs, so the review diff stays focused while contract verification remains reproducible.

The artifact commits remain reachable through the open feature branch during review and rework. Squash merging normally excludes them from `main`; a non-squash merge may retain their commits but still produces a final tree without the artifacts. After merge, the artifacts have no durability guarantee.

## Implementation decisions
- Use the existing slice branch; introduce no second branch, tag, issue copy, or artifact store.
- Artifacts remain present and tickable until the first complete implementation is ready for review.
- A draft `needs-human` PR may carry incomplete artifacts because it is not review-ready.
- The completed ledger is committed before deletion. The review PR body carries both `Artifact baseline: <sha>` and `Artifact completion: <sha>`.
- Baseline-to-completion integrity permits only existing `[ ]` to `[x]` replacements. Manual-verification boxes remain unchecked.
- Completion-to-review-head permits retirement of `.changes/<slug>/`; no later commit recreates or edits that path.
- Audit excludes `.changes/**` from the implementation diff supplied to reviewers and evaluates artifact integrity separately.
- Watchdog reads requirements from the baseline, completion claims from the completion commit, and implementation from the review head.
- Rework never restores artifacts. The watchdog finding ledger becomes its work ledger.
- Watchdog pass removes no artifacts because the implementor already retired them; it copies manual checks from the baseline and transitions the PR to `done`.
- Merged PR state replaces `.changes/archive/` as the garbage-collection proof. Cleanup removes the worktree and local and remote feature branches only after GitHub reports the PR merged.
- Squash remains preferred, not enforced.
- `CONTEXT.md`, ADRs, and capability docs remain durable and unaffected.

### Module shapes & seams

#### [MODIFIED] Proposal and implementation handoff
**Interface:** the slice branch, originating issue, artifact baseline, review PR body, and lifecycle labels.

The proposal still publishes frozen artifacts on the slice branch. The implementation stage owns the single retirement transition: complete ledger commit, PR metadata, deletion, then `review`. Its external seam is the observable review handoff: the two SHAs resolve, the artifact path is absent at the review head, and the label is `review`.

**Test strategy:** inspect the documented commands and transition criteria as one sequence; verify the review head has an empty diff for `.changes/<slug>/` against the PR base.

#### [MODIFIED] Audit and watchdog verification
**Interface:** implementation diff, deterministic gate result, artifact baseline/completion commits, and final head.

Implementation review receives a path-excluded code diff. Artifact verification remains a separate deterministic input. Watchdog uses the two checkpoints on first and repeat reviews, while critical whole-state verification continues against final code.

**Test strategy:** verify every artifact check has an explicit commit range and that no review path requires artifacts to exist at `HEAD`.

#### [MODIFIED] Landing and cleanup protocol
**Interface:** PR lifecycle state reported by GitHub.

A watchdog pass transitions directly to `done` without an archive commit. Later proposal cleanup trusts only a merged PR before removing its worktree and branches, which also works after squash merging where Git ancestry alone cannot prove the branch merged.

**Test strategy:** trace `review` → `done` → human merge → cleanup across the watchdog, GitHub reference, setup stamp, and README; search for stale `.changes/archive/` sentinels or archive-on-pass instructions.

## Sequence
1. Update proposal and artifact-template lifecycle language.
2. Update implementation submission and blocked-draft behavior, including the completion checkpoint and retirement.
3. Separate implementation diffs from artifact integrity in audit.
4. Update watchdog verification, rework assumptions, pass behavior, and manual-check retrieval.
5. Replace archive-based cleanup with merged-PR verification in the GitHub protocol and proposal stage.
6. Refresh setup-owned pipeline guidance and README terminology.
7. Run consistency searches and `git diff --check` across every `.changes` and archive reference.
