## Work Start

Work Item: #7
Branch: widget
Worktree: <worktree>
Prepare: `git -C '<main>' fetch 'origin' '+refs/heads/widget:refs/remotes/origin/widget'` then `git -C '<main>' worktree add -b 'widget' '<worktree>' 'origin/widget'`; safely reuse a clean existing worktree instead of recreating it
Inspect: `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` resolves the Artifact Baseline and Completion from the fetched history
Resume: `skl implement resume --item 7 --remote 'origin'`

This invocation starts the accepted change: read the accepted artifacts at their Artifact Baseline before changing code, and implement every accepted scenario. No progress is preserved yet, and a draft Submission, a branch name, or a nonempty comment stream does not change this procedure.

Write the opaque Result Document using the named template, then run `skl implement submit --repo '<worktree>' --remote 'origin' --item 7 --body '<result>/submission.md'`. If pausing, run `skl implement needs-human --repo '<worktree>' --remote 'origin' --item 7 --reason <permitted-reason> --decision '<result>/decision.md'` and add `--body <result>/submission.md` when preserving implementation changes.
