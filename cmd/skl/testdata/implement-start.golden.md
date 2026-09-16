## Work Start

Work Item: #7
Branch: widget
Worktree: <worktree>
Prepare: `git -C '<main>' fetch 'origin' 'widget'` then `git -C '<main>' worktree add '<worktree>' 'widget'`; safely reuse a clean existing worktree instead of recreating it
Inspect: `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` resolves the Artifact Baseline and Completion from the fetched history
Resume: `skl implement resume --item 7 --remote 'origin'`

Write the opaque Result Document using the named template, then run `skl implement submit --repo '<worktree>' --remote 'origin' --item 7 --body '<result>/submission.md'`. If pausing, run `skl implement needs-human --repo '<worktree>' --remote 'origin' --item 7 --reason <permitted-reason> --decision '<result>/decision.md'` and add `--body <result>/submission.md` when preserving implementation changes.
