---
name: implement
description: Implement a single claimed change following TDD, driven by the artifacts in `.changes/<slug>` for that change.
disable-model-invocation: true
---

Implement a single change proposal, materializing each Gherkin scenario in `behavior.md` into an idiomatic test and following a red -> green loop using the `/tdd` skill at the seams pinned in the change artifacts.

Use `docs/github.md` to know how to claim a single unit of work from Github. Once you claimed it, the change details are described in `.changes/<slug>`. Remember that you can't claim items with the `wip` label.

Work only in the claimed slice's worktree, `.worktrees/<slug>`. After claiming, rebase its branch onto up-to-date `main` before starting.

Run typechecking regularly, single test files regularly and a full test suite once at the end.

Once the whole implementation is done and every scenario is green, run `/review` against the claim's first commit — refactoring happens here, deliberately kept out of the red -> green cycles. Apply its findings yourself (fix the hard violations; use judgement on the judgement calls) and keep the suite green while doing so.

Update the change artifacts to reflect the completed work, then present the work by pushing the branch and submitting it for review as described in `docs/github.md`. Never archive or bless the changes — that is the watchdog's job.

The work is done only when every `behavior.md` scenario has a materialized test, every `intent.md` "Definition of Done" box is demonstrably met, the `/review` findings are applied, the full suite is green, and the PR is labeled `review`.
