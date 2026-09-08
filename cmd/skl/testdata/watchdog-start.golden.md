## Review Start

Work Item: #7
Submission: #11
Worktree: <worktree>
Reviewed head: <head>
Artifact Baseline: <baseline>
Artifact Completion: <baseline>
Completed finding bounces: 1
Resume: `skl watchdog resume --repo '<worktree>' --remote 'origin' --item 7`

Use the supplied historical files, opaque PR body, prior findings, and human comments. Work in this fresh Worker Session at the fixed reviewed head. The engine has not run Audit or project checks.

Write `summary.md`, optional anchored findings, and on pass `submission.md` in <result>. Run `skl watchdog submit --repo '<worktree>' --remote 'origin' --item 7 --reviewed-head <head> --summary '<result>/summary.md' --verdict <pass|rework|needs-human>`. Pass also requires `--body <result>/submission.md`; optional inline inputs use `--findings <result>/findings.json`. After permitted Debt Marker comments, commit and push, run the Post-Marker Check, and supply `--head <final-sha>` while retaining the original `--reviewed-head`.
