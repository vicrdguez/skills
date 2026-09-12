---
name: implement
description: Implement a single claimed change following TDD, driven by the artifacts in `.changes/<slug>` for that change.
disable-model-invocation: true
---

Implement a single change proposal, materializing each Gherkin scenario in `behavior.md` into an idiomatic test and following a red -> green loop using the `tdd` skill at the seams pinned in the change artifacts.

If this packet carries Implementation facts, continue with that Work Item. Otherwise run `skl implement next` and follow its concrete packet. `no_work` ends the invocation. Resume interrupted work with `skl implement resume --item <number>` or from its conventional worktree with `skl implement resume`; ordinary selection skips Claims. Keep the packet's retry commands.

Work only in the packet's conventional worktree, creating it from the pushed branch with ordinary Git if needed. Preserve existing branch progress; integration with `main`, conflict resolution, and merge belong to the Merge Authority after review. Never rebase or force-push: rewriting history orphans the Artifact Baseline and previous _Reviewed head_, and silently widens later three-dot diffs.

Use the packet's selected `remote` for Git fetch/push and pass `--remote <name>` on every Implement command, including Needs Human and legacy resume. When inference is ambiguous, choose explicitly with `--remote` before claiming.

Run typechecking and single test files regularly. `audit` runs the full suite as its gate at the end of this stage, so don't run it separately first.

Once the whole implementation is done and every scenario is green, use ordinary Git to identify the merge-base with `main`, then run `audit` against it — not the first commit of the claim, which `...HEAD` would leave out of the diff. On a rework round the fixed point moves; **Rework** below pins it. Refactoring happens here, deliberately kept out of the red -> green cycles. Apply its findings yourself, fix the `HARD` and on each `JUDGEMENT`, either fix it, decline it with a stated reason, or carry it as debt. Declining `HARD` is not yours to do. Keep the suite green while doing so. This is the pass where ordinary cleanup and refactoring belongs — smells, readability, maintainability, making the code navigable for the next agent. Whatever survives it, the watchdog sees.

Record what the pass decided. Every `audit` finding goes into the PR body under `## Audit ledger`: ID, axis, severity as `audit` assigned it, and `fixed` / `declined` / `debt` with one line of reasoning:

```text
## Audit ledger --<fixed-point>...<head>
A1  Standards  HARD       fixed    — order total computed in two places; extracted to `OrderTotal` (def456)
A2  Standards  JUDGEMENT  declined — "Feature Envy" on `Cart.price`: moving it splits the pricing rule across two modules
A3  Standards  JUDGEMENT  debt     — DEBT(#17/A3) `String` currency; typed when the payments slice lands
A4  Artifacts  HARD       fixed    — DoD item 3 had no test; added `cancel_shipped_order_test` (abc789)

```
A declined judgement call with a stated reason is a decision, not an omission. This ledger is what the watchdog verifies and the human approves; without it, the next context re-derives the same calls from scratch and files them as new findings.

Tick off the `[ ]` boxes the work completed, except those under `Manual verification`. That is the only content edit the Implementation Ledger allows. Record Artifact Completion in a commit, then remove the entire `.changes/<slug>/` ledger in a separate subsequent commit before review. Keep Artifact Baseline and Completion reachable in Git history; retirement does not relax the content freeze.

Then push the branch with ordinary Git. Write the PR body, including every Audit disposition, to `submission.md` in the packet's private Result Document directory; retrieve the template with `skl skill --resource reference/submission.md implement`. Run the packet's `skl implement submit` command. A `fix_required` outcome retains the Claim and prose: repair the reported invariant, commit and push as needed, and retry the same handoff. Successful publication removes the temporary directory. Never bless the changes — that is the watchdog's job.

The work is done only when every `behavior.md` scenario has a materialized test, every `intent.md` "Definition of Done" box is demonstrably met, every `audit` finding carries a disposition in the PR ledger, the full suite is green and `skl implement submit` reports `awaiting_review`.

## The scope is already decided

The artifacts say what this change is. Implement that, honor what they exclude, and treat sibling behavior they never mention as somebody else's work.

Read the callers of any shared code you change — a regression **this** change causes is yours to fix. A defect that was already there is not: leave it. Neither is an opportunity to tidy a sibling up while you are in the area, and neither is a question. Absence of a decision in the artifacts is a decision delegated to you, so use the pattern the project already uses and the smallest implementation that works.

## Rework

The latest watchdog summary is the ledger of what was found. Read the packet's summary and inline comments carrying its evidence, and any human disposition posted since. Association and commit facts identify their source; interpreting findings remains your judgment.

For an adopted comment-based review, a `fix_required` response supplies the comments while retaining the Claim. Read its latest watchdog summary and pass the recorded full `Reviewed head` through `skl implement resume --item <number> --reviewed-head <sha>`. The engine validates the supplied Git identity without parsing findings or human prose.

Read the retired Implementation Ledger from its historical Artifact Baseline and Completion; rework must not recreate or revise it.

Resolve every finding that is still `BLOCK`. Findings left as `NOTE` are debt, not work to skip: materialize the code-local ones as `DEBT(#<pr>/W<n>)` comments, exactly as the watchdog's contract describes. Open no follow-up issues — that stays human or `propose` work.

Review your own rework against `previous-reviewed-head...HEAD`. Do not re-clean the whole PR: a second sweep across untouched code grows the diff, adds regressions, and gives the next review more surface than it had last round.

Before resubmitting, include a mapping of each finding to its resolution in the Result Document: the commit that did it and the evidence that it holds:

```text
Rework: abc123...def456
W1 resolved — def456; covered by <check>
W2 debt — DEBT(#17/W2) in <path · symbol>
```

That is a claim, not proof. The next watchdog verifies it independently. Update the `## Audit ledger` in the PR body in the same push: it must describe the current head, not the first one.

## When only a human can decide

Finish everything that is not blocked first. This handoff is for contradictory or impossible artifacts, a mandatory project/language/security/accessibility conflict, an unavoidable change to frozen behavior or interface, a disputed blocker, or the bounce cap; adjacent improvements and implementation preferences do not justify it. Write `decision.md` using `skl skill --resource reference/decision.md implement`, then run `skl implement needs-human --item <number> --reason <reason> --decision <absolute-file>`. Reasons are `contradictory_artifacts`, `mandatory_rule`, `frozen_interface`, `disputed_blocker`, and `bounce_cap`.

When implementation work exists, push it and also supply `--body <absolute-submission.md>` from the same private directory to preserve one draft Submission. Incomplete artifacts may remain during this pause. The semantic command publishes the decision and preserves the state to resume; never leave the question only in your own session.
