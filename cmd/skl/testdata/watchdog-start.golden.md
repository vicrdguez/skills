## Review Start

Work Item: #7
Submission: #11
Worktree: <worktree>
Reviewed head: <head>
Artifact Baseline: <baseline>
Artifact Completion: <baseline>
Completed reviews: 0
Review number: 1
Scope: full
Resume: `skl watchdog resume --repo '<worktree>' --remote 'origin' --item 7`

Use the supplied historical files, opaque PR body, prior findings, and human comments. Review the invocation's current head; rerun the Full Gate, active-finding verification, artifact checks, and whole-change critical-class scan. The engine has not run Audit or project checks.

Write `summary.md`, optional anchored findings, and on pass `submission.md` in <result>. Run `skl watchdog submit --repo '<worktree>' --remote 'origin' --item 7 --review-number 1 --reviewed-head <head> --summary '<result>/summary.md' --verdict <pass|rework|needs-human>`. Pass also requires `--body <result>/submission.md`; optional inline inputs use `--findings <result>/findings.json`. After permitted Debt Marker comments, commit and push, run the Post-Marker Check, and supply `--head <final-sha>` while retaining the original `--reviewed-head`.

Review the full PR comparison; no usable retained reviewed revision is required or fetched.
