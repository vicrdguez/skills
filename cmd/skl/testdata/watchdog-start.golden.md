## Review Start

Review Work Item #7 in '<worktree>'. Its Submission is #11 on branch `widget`, selected from remote `origin`. The original reviewed head is `<head>`. This invocation has 0 completed reviews and is review number 1. The engine has acquired or resumed this Work Item's Claim; do not select another item.

Prepare: `git -C '<main>' fetch 'origin' '+refs/heads/widget:refs/remotes/origin/widget'` then `git -C '<main>' worktree add -b 'widget' '<worktree>' 'origin/widget'`. Safely reuse an existing worktree, preserving dirty files, its index, and branch progress. Never reset, stash, rebase, force-push, or merge merely to prepare the review.

Inspect after preparation: `skl watchdog inspect --repo '<worktree>' --remote 'origin' --item 7 --submission 11 --base 'main' --submission-body-sha256 'c37baae2c8ae04036b781111e4b4d46dd77d4d740e7530f1988a1f6fd9145705' --review-number 1 --reviewed-head '<head>' --result-directory '<result>'`. It resolves exact Artifact Baseline and Completion and the usable comparison. Read the historical ledger only at those endpoints. This startup used metadata only: it did not inspect local project objects or validate artifact or comparison facts.

Resume this Claim only with `skl watchdog resume --repo '<worktree>' --remote 'origin' --item 7`. Inspect existing work and visible evidence when resuming; worker reasoning is not persisted. A publication already begun must use the original fixed-number submit command and Result Documents.

The private Result Document directory is '<result>'. Write the summary to `<result>/summary.md`, optional inline anchors to `<result>/findings.json`, and a complete final PR body on pass to `<result>/submission.md`.

Keep the original reviewed head and review number in every later instruction and command. A later PR head or checkpoint drift requires explicit repair; it cannot silently replace this invocation's identity. No previous completed review reference is available. Supplied prior findings still retain their identities.
