---
name: implement
description: Implement a single change following TDD based on artifacts in `.change/<slug>` for that change. After implementation, it checks for correctness and adherence to the artifacts; Then it presesents the work by pushing to the remote branch for review. It never  archives, or blesses the changes. 
disable-model-invocation: true
---

Implement a single change proposal, materializing each Gherkin scenario in `behavior.md` into an idiomatic test and following a red -> green loop using the `/tdd` skill at pre-agreed seams. 

Use `docs/github.md` to know how to claim a single unit of work from Github. Once you claimed it, the change details are described in `.changes/<slug>`. Remember that you can't claim items with the `wip` label.

If the issue does not have any child issues, work only on the worktree in `.worktrees/<slug>`. If the issue has child issues, all work is done in the parent issue's worktree.

Run typechecking regularly, single test files regularly and a full test suite once at the end.

Update the change artifacts to reflect the completed work.
