# Github
Proposed changes for this repo live as Github issues and PRs. Use the `gh` CLI for ALL operations.

## Issue / PR labels

| Label | Rides on | Means → next role | color(hex) |
|---|---|---|---|
| `ready` | issue | proposed change awaiting an **implementor** | 0e8a16 |
| `wip` | issue or PR | additive Worker Claim. An agent is working on it | fbca04 |
| `review` | PR | built change awaiting a **reviewer** | 1d76db |
| `rework` | PR | reviewer bounced it back to the **implementor** after review | d93f0b |
| `done` | PR | passed review, awaiting the **human's approval to merge** | 5319e7 |

*If a label is not present* create them:

```sh
gh label create "<name>" --repo "<owner>/<repo>" --description "<desc>" --color "<color>" --force
```

`--force` makes this safe to re-run: it creates the label if missing and updates it otherwise.



## Creating Issues

Used by `/propose`. The issue is a **thin pointer** to the brief on the pushed branch, not a copy of it. Push the branch first (`git push -u origin <slug>`), then:

```sh
gh issue create --repo "<owner>/<repo>" \
  --title "<slug>" \
  --label "ready" \
  --body "Change proposed on branch \`<slug>\`. Brief: .changes/<slug>/ (intent.md · behavior.md · plan.md? · tasks.md?)."
```

- **Child issues**: issue linked to the parent issue as a GitHub sub-issue (`gh api` on the sub-issues endpoint).
- **Blocking**: GitHub's native issue dependencies — the canonical, UI-visible representation. Add an edge with `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`, where `<blocker-db-id>` is the blocker's numeric database id (`gh api repos/<owner>/<repo>/issues/<n> --jq .id`, not the `#number` or `node_id`). GitHub reports `issue_dependencies_summary.blocked_by` (open blockers only — the live gate). Where dependencies aren't available, fall back to a `Blocked by: #<n>, #<n>` line at the top of the child body. A ticket is unblocked when every blocker is closed **and the blocker's PR is merged** — a blocker issue closes when its PR opens for review, so also check the PR state (`gh pr list --head "<blocker-slug>" --state merged`) before claiming.


## Claiming work

Used by `/implement` and `/watchdog`. Claims issues labeled as `ready`, `rework` or `review` by adding `wip`

As an implementor:
- Always prefer `rework` PRs over `ready` issues.
- Fetch the oldest open `rework` PR without `wip`; if none, the oldest open `ready` issue without `wip`.
- After a successful claim, check out the slice's worktree (`.worktrees/<slug>`, creating it from the pushed branch if absent) and rebase the branch onto up-to-date `main` before starting.

```sh
gh pr list --repo "<owner>/<repo>" --label "rework" --state open \
  --search "-label:wip sort:created-asc" --limit 1 \
  --json number,headRefName,title --jq '.[0]'
```

```sh
gh issue list --repo "<owner>/<repo>" --label "ready" --state open \
  --search "-label:wip sort:created-asc" --limit 1 \
  --json number,title --jq '.[0]'
```


As a reviewer
- Fetch the oldest open `review` PR without `wip`.

```sh
gh pr list --repo "<owner>/<repo>" --label "review" --state open \
  --search "-label:wip sort:created-asc" --limit 1 \
  --json number,headRefName,title --jq '.[0]'
```

**IMPORTANT**: Add the `wip` label without removing the lifecycle label. Do not fetch or touch the change unless the command succeeds:

```sh
gh issue edit <issue-number> --repo "<owner>/<repo>" --add-label "wip" # ready issue
gh pr edit <pr-number> --repo "<owner>/<repo>" --add-label "wip"       # rework PR or review PR
```

Failed or interrupted stays claimed. A human requeues manually.

## Submit for review

Used by `/implement`. 

Does a PR already exist for this branch?

```sh
gh pr list --repo "<owner>/<repo>" --head "<slug>" --state open --json number,isDraft,url
```

If one exists, **update** it (below) instead of creating a new one.

**For work done on `ready` issues**: Open a `review` PR, close the `ready` issue, and only then remove its Claim:

```sh
gh pr create --repo "<owner>/<repo>" --label "review" \
  --title "<title>" --body-file body.md --base main --head "<slug>"

gh issue close <issue-number> --repo "<owner>/<repo>" \
  --comment "Built and opened for review in #<pr-number>."

gh issue edit <issue-number> --repo "<owner>/<repo>" --remove-label "wip"
```



**For work done on `rework` PRs**: Swap `rework + wip` -> `review`.

```sh
# implementor re-presents after rework:  rework + wip → review
gh pr edit <pr-number> --repo "<owner>/<repo>" \
  --remove-label "rework,wip" --add-label "review"
```

The implementor reads the bounce feedback with:

```sh
gh pr view <pr-number> --repo "<owner>/<repo>" --comments
```

## After review

Used by `/watchdog`. Submit the review, as either needing `rework` or as `done`

```sh
# reviewer bounces:  review + wip → rework
gh pr edit <pr-number> --repo "<owner>/<repo>" --remove-label "review,wip" --add-label "rework"
# reviewer passes:  review + wip → done
gh pr edit <pr-number> --repo "<owner>/<repo>" --remove-label "review,wip" --add-label "done"
```


After a reviewer transition, inspect the labels before reporting success:

```sh
gh pr view <pr-number> --repo "<owner>/<repo>" --json labels --jq '[.labels[].name]'
```

### Feedback as PR comments

Reviewer findings go on the PR as comments. Inline comments anchor to a line; a summary comment carries the verdict.

```sh
# summary verdict comment
gh pr comment <pr-number> --repo "<owner>/<repo>" --body-file findings.md
# inline, anchored to a file+line (review API)
gh api "repos/<owner>/<repo>/pulls/<pr-number>/comments" \
  -f body="<finding>" -f commit_id="<head-sha>" -f path="<file>" -F line=<n> -f side=RIGHT
```

