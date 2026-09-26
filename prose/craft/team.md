### Split the work

Read the whole Contract and the architecture it touches first. Then cut the work into cohesive patches, each with the files or modules it owns and the patches it depends on.

Check: every Contract item belongs to a patch with named ownership.

### Run the patches

Hand each patch to its own fresh-context implementer with:

- the worktree;
- the files or modules it owns;
- the acceptance criteria of its patch;
- the focused checks it runs.

Each implementer edits only the files it owns and answers with a short report: done or blocked, what it changed, which checks it ran, and what stopped it.

Run patches with disjoint ownership in parallel, and stage a patch after the ones it depends on or overlaps. While they run, your job is traffic control: confirm each implementer kept to its files and the worktree is sound. Take each finished patch as it is; review and polish wait for Audit, the first look at the whole change.

Check: every patch has reported.

### Reconcile once

When every patch is in, make one pass that gets the combined change building and passing: format it, make it compile, fit the interfaces together, settle where the patches disagree, and run the focused checks. Send a failure that traces to one patch back to its implementer as a narrow repair task.
