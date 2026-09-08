## Work Start

Work Item: #7
Branch: widget
Worktree: <worktree>
Artifact Baseline: <baseline>
Resume: `skl implement resume --item 7 --target-snapshot <target> --remote 'origin'`

Before coding, use ordinary Git in the worktree: `git merge <target>`. The engine has not merged or run project checks.

Write the opaque Result Document using the named template, then run `skl implement submit --repo '<worktree>' --remote 'origin' --item 7 --body '<result>/submission.md'`. Refresh ledger integrity for Audit with `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7`.
