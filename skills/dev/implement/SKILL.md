---
name: implement
description: Implement a single claimed change following TDD, driven by the artifacts in `.changes/<slug>` for that change.
disable-model-invocation: true
---

Implement a single change proposal, materializing each Gherkin scenario in `behavior.md` into an idiomatic test and following a red -> green loop using the `tdd` skill at the seams pinned in the change artifacts.

Use `docs/github.md` to know how to claim a single unit of work from Github. Once you claimed it, the change details are described in `.changes/<slug>`. Remember that you can't claim items with the `wip` or `needs-human` label.

Work only in the claimed slice's worktree, `.worktrees/<slug>`. After claiming, rebase its branch onto up-to-date `main` before starting.

Work only in the claimed slice's worktree, `.worktrees/<slug>`. On a first claim, merge up-to-date `main` iinto its branch before starting; on a rework round sync nothing. Never rebase: it orphans the _Artifact Baseline_ and the previous _Reviewed head_, and every three-dot diff taking against them silently widens to the old merge-base.

Run typechecking regularly, single test files regularly and a full test suite once at the end.

Once the whole implementation is done and every scenario is green, run `audit` against the PR base merge-base — not the first commit of the claim, which `...HEAD` would leave out of the diff. On a rework round the fixed point moves; **Rework** below pins it. Refactoring happens here, deliberately kept out of the red -> green cycles. Apply its findings yourself, fix the `HARD` and on each `JUDGEMENT`, either fix it, deline it with a stated reason, or carry it as debt. Declining `HARD` is not yours to do. Keep the suite while doing so. This is the pass where ordinary cleanup and refactoring belongs — smells, readability, maintainability, making the code navigable for the next agent. Whatever survives it, the watchdog sees.

Tick off the `[ ]` boxes the work completed. That is the only edit the change artifacts allow: they froze when they were published, and nothing you learn while implementing gets written back into them.

Then present the work by pushing the branch and submitting it for review as described in `docs/github.md`. Never archive or bless the changes — that is the watchdog's job.

The work is done only when every `behavior.md` scenario has a materialized test, every `intent.md` "Definition of Done" box is demonstrably met, the `audit` findings are applied, the full suite is green, and the PR is labeled `review`.

## The scope is already decided

The artifacts say what this change is. Implement that, honor what they exclude, and treat sibling behavior they never mention as somebody else's work.

Read the callers of any shared code you change — a regression **this** change causes is yours to fix. A defect that was already there is not: leave it. Neither is an opportunity to tidy a sibling up while you are in the area, and neither is a question. Absence of a decision in the artifacts is a decision delegated to you, so use the pattern the project already uses and the smallest implementation that works.

## Rework

The latest watchdog summary is the ledger of what was found. Read it, the inline comments carrying its evidence, and any human disposition posted since — `docs/github.md` explains both.

Resolve every finding that is still `BLOCK`. Findings left as `NOTE` are debt, not work to skip: materialize the code-local ones as `DEBT(#<pr>/W<n>)` comments with their matching `DEBT.md` entries, exactly as the watchdog's contract describes. Open no follow-up issues — that stays human or `propose` work.

Review your own rework against `previous-reviewed-head...HEAD`. Do not re-clean the whole PR: a second sweep across untouched code grows the diff, adds regressions, and gives the next review more surface than it had last round.

Before resubmitting, post a comment mapping each finding to how it was resolved — the commit that did it and the evidence that it holds:

```text
Rework: abc123...def456
W1 resolved — def456; covered by <check>
W2 debt — DEBT(#17/W2) in <path · symbol>
```

That is a claim, not proof. The next watchdog verifies it independently.

## When only a human can decide

Rare, and never about scope. If the artifacts contradict each other, conflict with a mandatory project, language, security or accessibility rule, require something impossible under the pinned constraints, or leave no safe fix that does not change frozen behavior, interface or scope — stop and hand it over.

Finish everything that is not blocked first. Then follow the human-decision handoff in `docs/github.md`: the decision goes on the issue when no code exists yet, or on a draft PR carrying the completed work when it does. Never leave the question only in your own session.
