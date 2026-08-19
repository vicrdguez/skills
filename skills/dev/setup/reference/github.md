# Github
Proposed changes for this repo live as Github issues and PRs. Use the `gh` CLI for ALL operations.

## Issue / PR labels

| Label | Rides on | Means → next role | color(hex) |
|---|---|---|---|
| `ready` | issue | proposed change awaiting an **implementor** | 0e8a16 |
| `wip` | issue or PR | additive Worker Claim. An agent is working on it | fbca04 |
| `review` | PR | built change awaiting a **reviewer** | 1d76db |
| `rework` | PR | reviewer bounced it back to the **implementor** after review | d93f0b |
| `needs-human` | issue or PR | automation paused for a narrow human decision | b60205 |
| `done` | PR | passed review, awaiting the **human's approval to merge** | 5319e7 |

*If a label is not present* create them:

```sh
gh label create "<name>" --repo "<owner>/<repo>" --description "<desc>" --color "<color>" --force
```

`--force` makes this safe to re-run: it creates the label if missing and updates it otherwise.



## Creating Issues

Used by `propose`. The issue is a **thin pointer** to the brief on the pushed branch, not a copy of it. Push the branch first (`git push -u origin <slug>`), then:

```sh
gh issue create --repo "<owner>/<repo>" \
  --title "<slug>" \
  --label "ready" \
  --body "Change proposed on branch \`<slug>\`. Brief: .changes/<slug>/ (intent.md · behavior.md · plan.md? · tasks.md?).
Artifact baseline: <proposal-commit-sha>"
```

The `Artifact baseline` line is the SHA of the commit that published the artifacts. Later stages diff against it to prove the artifacts were not rewritten mid-flight. Legacy changes without one fall back to the first commit containing `.changes/<slug>/`.

- **Child issues**: issue linked to the parent issue as a GitHub sub-issue (`gh api` on the sub-issues endpoint).
- **Blocking**: GitHub's native issue dependencies — the canonical, UI-visible representation. Add an edge with `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`, where `<blocker-db-id>` is the blocker's numeric database id (`gh api repos/<owner>/<repo>/issues/<n> --jq .id`, not the `#number` or `node_id`). GitHub reports `issue_dependencies_summary.blocked_by` (open blockers only — the live gate). Where dependencies aren't available, fall back to a `Blocked by: #<n>, #<n>` line at the top of the child body. A ticket is unblocked when every blocker is closed **and the blocker's PR is merged** — a blocker issue closes when its PR passes review before the human merges it, so also check the PR state (`gh pr list --head "<blocker-slug>" --state merged`) before claiming.


## Claiming work

Used by `implement` and `watchdog`. Claims issues labeled as `ready`, `rework` or `review` by adding `wip`. Anything carrying `needs-human` is never eligible: it stays paused until a human records the decision and relabels it back into the queue.

As an implementor:
- Always prefer `rework` PRs over `ready` issues.
- Fetch the oldest open `rework` PR without `wip`; if none, the oldest open `ready` issue without `wip`.
- After a successful claim, check out the slice's worktree (`.worktrees/<slug>`, creating it from the pushed branch if absent). On a first claim, merge up-to-date `main` into the branch; on a rework round sync nothing. Never rebase: it rewrites the `Artifact baseline` commit and orphans the previous `Reviewed head`.

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

Used by `implement`.

Does a PR already exist for this branch?

```sh
gh pr list --repo "<owner>/<repo>" --head "<slug>" --state open --json number,isDraft,url
```

If one exists, **update** it (below) instead of creating a new one. The PR body carries the implementor's `## Audit ledger`. It is part of the submission, not commentary: the reviewer verifies it and the human reads it.

**For work done on `ready` issues**: Open a `review` PR, and only then remove the `ready` label from the issue, and remove its Claim:

```sh
gh pr create --repo "<owner>/<repo>" --label "review" \
  --title "<title>" --body-file body.md --base main --head "<slug>"

gh issue edit <issue-number> --repo "<owner>/<repo>" --remove-label "ready,wip" 

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

That view truncates on a busy PR, so also page both comment streams — the latest summary comment is the finding ledger, the inline comments carry its evidence:

```sh
gh api --paginate "repos/<owner>/<repo>/issues/<pr-number>/comments"  # summaries and human dispositions
gh api --paginate "repos/<owner>/<repo>/pulls/<pr-number>/comments"   # inline, anchored to lines
```

## After review

Used by `watchdog`. Submit the review, as either needing `rework` or as `done` — or pause it for a human when the decision is genuinely not the reviewer's to make.

```sh
# reviewer bounces:  review + wip → rework
gh pr edit <pr-number> --repo "<owner>/<repo>" --remove-label "review,wip" --add-label "rework"

# reviewer passes:  review + wip → done / closes the original issue
gh pr edit <pr-number> --repo "<owner>/<repo>" --remove-label "review,wip" --add-label "done"
gh issue close <issue-number> --repo "<owner>/<repo>" \
  --comment "Closes with #<pr-number>."

# reviewer pauses:  review/rework + wip → needs-human
gh pr edit <pr-number> --repo "<owner>/<repo>" --remove-label "review,rework,wip" --add-label "needs-human"
```


After a reviewer transition, inspect the labels before reporting success:

```sh
gh pr view <pr-number> --repo "<owner>/<repo>" --json labels --jq '[.labels[].name]'
```

### Feedback as PR comments

Reviewer findings go on the PR as comments. Inline comments anchor to a line; a summary comment carries the verdict and the ledger below.

```sh
# summary verdict comment
gh pr comment <pr-number> --repo "<owner>/<repo>" --body-file findings.md
# inline, anchored to a file+line (review API)
gh api "repos/<owner>/<repo>/pulls/<pr-number>/comments" \
  -f body="<finding>" -f commit_id="<head-sha>" -f path="<file>" -F line=<n> -f side=RIGHT
```

### The finding ledger

Finding IDs are local to the PR and monotonic — `W1`, `W2`, … — and encode nothing else: not the PR number, not the review round, not the severity or axis. The same defect keeps its ID for the life of the PR.

The **latest summary comment is the ledger**, and it is the durable record. It carries the head it was produced against, every still-active finding with its disposition, and the verdict:

```text
Reviewed head: 9f2c1ab

W1 BLOCK  — refund path retries without an idempotency key
W2 NOTE   — order total recomputed in two places
W3 HUMAN  — behavior.md requires a sync refund; the provider is async-only

Verdict: rework
```

Dispositions are `BLOCK` (must be resolved before a pass), `HUMAN` (needs one of the narrowly allowed decisions) and `NOTE` (nonblocking, actionable debt).

### Human dispositions

A repository owner, member or collaborator overrides a finding by restating its ID with a disposition, one per line, in any PR comment:

```text
W1 WAIVE          accepted as-is; nonblocking and leaves no debt marker
W2 BLOCK          make or keep it blocking
W3 NOTE           downgrade to tracked debt
```

Trailing text on the line is a free-form reason and may link an issue (`W2 NOTE — tracked in #24`). Matching is case-insensitive after trimming.

Only commands posted **after** the finding count, and the latest one wins. Never infer a decision from a deleted comment, an emoji reaction, or silence. No stage creates follow-up issues on its own; that stays human or `propose` work.

## Rare human-decision handoff

Scope is settled by the frozen artifacts, so this handoff is rare. Adjacent improvements, unspecified sibling behavior, optional polish and implementation preferences never justify it. It is for contradictory or impossible artifacts, a conflict with a mandatory project/language/security/accessibility rule, a fix that cannot avoid changing frozen behavior or interface, a disputed blocker, or the bounce cap.

**When no implementation work exists**, keep the decision on the originating issue and never open a PR just to ask a question:

```sh
gh issue comment <issue-number> --repo "<owner>/<repo>" --body-file decision.md
gh issue edit <issue-number> --repo "<owner>/<repo>" --remove-label "ready,wip" --add-label "needs-human"
```

**When implementation work exists**, preserve it first — push the branch and open or update the PR, draft because it is not review-ready:

```sh
git push -u origin <slug>

gh pr create --repo "<owner>/<repo>" --draft --label "needs-human" \
  --title "<title>" --body-file body.md --base main --head "<slug>"
```

Post the decision as a `HUMAN` finding with the current state, the options and a recommendation. Drop `ready`/`wip` from the source issue so it cannot be claimed again, drop `review`/`rework`/`wip` from the PR, and verify both objects before exiting.

A human requeues it explicitly: `rework` when implementation must continue, `review` when only final verification remains. Mark a draft ready first:

```sh
gh pr ready <pr-number> --repo "<owner>/<repo>"
```

Or they end it. **Supersede**: the change is not worth merging, and what it taught belongs in a fresh slice. Close the PR unmerged and the issue as not planned, then hand the slug to `propose`. Discarding a change is a proposal-level decision, which is why no other stage may take it.

```sh
gh pr close <pr-number> --repo "<owner>/<repo>" --comment "Superseded: <reason>."
gh issue close <issue-number> --repo "<owner>/<repo>" --reason "not planned" \
--comment "Superseded: <reason>."
# from main, never from inside the worktree being removed
git worktree remove .worktrees/<slug> && git branch -D <slug>
```

