---
name: implement
description: Implement a single claimed change following TDD, driven by the artifacts in `.changes/<slug>` for that change.
disable-model-invocation: true
---

Implement a single change proposal, materializing each Gherkin scenario in `behavior.md` into an idiomatic test and following a red -> green loop using the `tdd` skill at the seams pinned in the change artifacts.

If this packet carries Implementation facts, continue with that Work Item. Otherwise run `skl implement next` and follow its concrete packet. `no_work` ends the invocation. Resume interrupted work with `skl implement resume --item <number>` or from its conventional worktree with `skl implement resume`; ordinary selection skips Claims. On resume, inspect the selected branch, preserved files, applicable artifacts, and visible feedback to determine what remains; do not expect a persisted execution cursor. Keep the packet's retry commands. If the invocation supplied `--artifact-baseline <full-sha>` or `--artifact-completion <full-sha>`, preserve those exact flags and full SHAs on every generated or manually run resume, inspect, submit, and Needs Human command for this Work Item; never add override flags for endpoints resolved from markers.

Work only in the packet's conventional worktree, fetching the pushed branch and creating or safely reusing that worktree with ordinary Git. Startup does not inspect project objects, so run the packet's inspect command after preparation to resolve the Artifact Baseline and Completion before touching artifacts or submitting. Preserve existing branch progress. Every new or updated Submission targets `main`; integration with `main`, conflict resolution, and merge belong to the human Merge Authority after review. Never rebase or force-push: rewriting history orphans the Artifact Baseline and previous Reviewed head, and silently widens later three-dot diffs.

Use the packet's selected `remote` for Git fetch/push and pass `--remote <name>` on every Implement command, including Needs Human and legacy resume. When inference is ambiguous, choose explicitly with `--remote` before claiming.

Run typechecking and single test files regularly. `audit` runs the full suite as its gate at the end of this stage, so don't run it separately first.

Once the whole implementation is done and every scenario is green, run the packet's artifact inspection command, preserving any explicit endpoint flags, then run `audit` **only once** against the PR base merge-base — not the first commit of the claim, which `...HEAD` would leave out of the diff — unless the caller supplied another explicit fixed point. On a rework round the fixed point moves; **Rework** below pins it. This first-pass Audit is provisional: it may compare the Baseline with the current ledger while automated boxes remain unchecked, and must report Completion and retirement as pending rather than infer either from intermediate commits. Refactoring happens here, deliberately kept out of the red -> green cycles. Apply its findings yourself. Fix the `HARD` and on each `JUDGEMENT`, either fix it, decline it with a stated reason, or carry it as debt. Declining `HARD` is not yours to do. Keep the suite green while doing so. This is the pass where ordinary cleanup and refactoring belongs — smells, readability, maintainability, making the code navigable for the next agent. Whatever survives it, the watchdog sees. Once all is fixed, **do not** run `audit` again, that would resoult on a never ending loop of findings-fixings.

Record what the pass decided. Every `audit` finding goes into the PR body under `## Audit ledger`: ID, axis, severity as `audit` assigned it, and `fixed` / `declined` / `debt` with one line of reasoning:

```text
## Audit ledger --<fixed-point>...<head>
A1  Standards  HARD       fixed    — order total computed in two places; extracted to `OrderTotal` (def456)
A2  Standards  JUDGEMENT  declined — "Feature Envy" on `Cart.price`: moving it splits the pricing rule across two modules
A3  Standards  JUDGEMENT  debt     — DEBT(#17/A3) `String` currency; typed when the payments slice lands
A4  Artifacts  HARD       fixed    — DoD item 3 had no test; added `cancel_shipped_order_test` (abc789)

```
A declined judgement call with a stated reason is a decision, not an omission. This ledger is what the watchdog verifies and the human approves; without it, the next context re-derives the same calls from scratch and files them as new findings.

Tick off every automated `[ ]` box, except those under `Manual verification`, using lowercase `[x]`. That is the only endpoint content difference the Implementation Ledger allows. While every artifact file still exists, commit Artifact Completion with subject `[completion] <slug>` (optional explanatory text may follow after a space), then remove the entire `.changes/<slug>/` ledger in a separate subsequent commit before review. The removal need not be Completion's immediate child. Keep both resolved endpoint commits reachable; new work uses exact `[baseline] <slug>` and `[completion] <slug>` subject prefixes, while an explicit markerless handoff keeps its supplied full SHAs. Validation compares those endpoints and ledger absence at the review head, not intermediate edits or a deletion commit's parent.

Then push the branch with ordinary Git. Write the PR body, including every Audit disposition, to `submission.md` in the packet's private Result Document directory; retrieve the writing instructions with `skl skill --resource reference/submission.md --input result_directory='<result>' --input procedure=initial implement`. Run the packet's `skl implement submit` command. A `fix_required` outcome retains the Claim and prose: repair the reported invariant, commit and push as needed, and retry the same handoff. A successful handoff may include a cleanup-only warning for a retained private directory; do not resubmit or roll back published work. Never bless the changes — that is the watchdog's job.

The work is done only when every `behavior.md` scenario has a materialized test, every `intent.md` "Definition of Done" box is demonstrably met, every `audit` finding carries a disposition in the PR ledger, the full suite is green and `skl implement submit` reports `awaiting_review`.

## The scope is already decided

The artifacts say what this change is. Implement that, honor what they exclude, and treat sibling behavior they never mention as somebody else's work.

Read the callers of any shared code you change — a regression **this** change causes is yours to fix. A defect that was already there is not: leave it. Neither is an opportunity to tidy a sibling up while you are in the area, and neither is a question. Absence of a decision in the artifacts is a decision delegated to you, so use the pattern the project already uses and the smallest implementation that works.

## Rework

The latest watchdog summary is the ledger of what was found. Read the packet's summary and inline comments carrying its evidence, and any human disposition posted since. Association and commit facts identify their source; interpreting findings remains your judgment.

Read the retired Implementation Ledger from its historical Artifact Baseline and Completion; rework must not recreate or revise it. Preserve any explicit endpoint flags and full SHAs from the handoff on inspection, resume, resubmission, and Needs Human commands.

Resolve every finding that is still `BLOCK`. Findings left as `NOTE` are debt, not work to skip: materialize the code-local ones as `DEBT(#<pr>/W<n>)` comments, exactly as the watchdog's contract describes. Open no follow-up issues — that stays human or `propose` work.

Review your own rework against the ordinary PR comparison and focus on the supplied findings. Do not re-clean untouched code: it grows the diff, adds regressions, and gives the next review more surface.

Before resubmitting, include a mapping of each finding to its resolution in the Result Document: the commit that did it and the evidence that it holds:

```text
Rework: abc123...def456
W1 resolved — def456; covered by <check>
W2 debt — DEBT(#17/W2) in <path · symbol>
```

That is a claim, not proof. The next watchdog verifies it independently. Update the `## Audit ledger` in the PR body in the same push: it must describe the current head, not the first one.

## When only a human can decide

Finish everything that is not blocked first. This handoff is for contradictory or impossible artifacts, a mandatory project/language/security/accessibility conflict, an unavoidable change to frozen behavior or interface, a disputed blocker, or the bounce cap; adjacent improvements and implementation preferences do not justify it. Write `decision.md` using `skl skill --resource reference/decision.md --input result_directory='<result>' --input preserve=<true|false> implement`, setting `preserve` to `true` when implementation work exists that the draft Submission must preserve and to `false` otherwise; then run `skl implement needs-human --item <number> --reason <reason> --decision <absolute-file>`. Reasons are `contradictory_artifacts`, `mandatory_rule`, `frozen_interface`, `disputed_blocker`, and `bounce_cap`.

When implementation work exists, push it and also supply `--body <absolute-submission.md>` from the same private directory to preserve one draft Submission. Incomplete artifacts may remain during this pause; preserve any supplied endpoint flags, but do not invent Completion, tick unfinished work, or retire the ledger. The semantic command publishes the decision and preserves the state to resume; never leave the question only in your own session.

## Work Start

Repository: acme/widgets on the selected remote `origin`
Work Item: #7
Branch: `widget`
Worktree: `<worktree>`
Private result location: `<result>`, which this invocation already created

Prepare: `git -C '<main>' fetch 'origin' '+refs/heads/widget:refs/remotes/origin/widget'` then `git -C '<main>' worktree add -b 'widget' '<worktree>' 'origin/widget'`; safely reuse a clean existing worktree instead of recreating it, and preserve dirty files, the index, and existing branch progress. Never reset, stash, rebase, force-push, or merge the target merely to make preparation or presentation convenient.
Push: `git -C '<worktree>' push 'origin' 'widget'`
Inspect: `skl implement inspect --repo '<worktree>' --remote 'origin' --item 7` resolves the Artifact Baseline and Completion from the fetched history.
Resume: `skl implement resume --item 7 --remote 'origin'`

This invocation starts the accepted change: read the accepted artifacts at their Artifact Baseline before changing code, and implement every accepted scenario. No progress is preserved yet, and a draft Submission, a branch name, or a nonempty comment stream does not change this procedure.

The dedicated worktree, the selected project commits, and the artifact objects may still be unavailable locally. Preparing the branch and running the Inspect command establish them; do not derive any of them from the branch name, from the presence of a Submission, or from a lifecycle label.

## Evidence

### Already-fetched feedback

The invocation supplied no fetched feedback. That is not proof that none exists.

### Pending retrieval

No Submission is attached to this Work Item, so it has no Submission body, discussion, review summary, or inline finding. Do not invent those commands or treat their absence as an empty review.
- Source Work Item comments: `gh api --paginate repos/acme/widgets/issues/7/comments`

A collection that was fetched completely and is empty is `fetched empty`. A stream the invocation did not supply is `pending`. A read that failed, that returned an error, or whose pagination stopped early is a `retrieval failure` to repair or retry. These three are different facts, and only the first may be reported as no findings.


Write the opaque Result Document using the named template, then run `skl implement submit --repo '<worktree>' --remote 'origin' --item 7 --body '<result>/submission.md'`. If pausing, run `skl implement needs-human --repo '<worktree>' --remote 'origin' --item 7 --reason <permitted-reason> --decision '<result>/decision.md'` and add `--body <result>/submission.md` when preserving implementation changes.


