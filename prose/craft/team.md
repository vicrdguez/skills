### Split the work

Read the whole Contract and the architecture it touches first. Then cut the work into cohesive patches, each with the files or modules it owns and the patches it depends on.

Check: every Contract item belongs to a patch with named ownership.

### Run the patches

Hand each patch to its own fresh-context implementer with:

- the worktree;
- the files or modules it owns;
- the acceptance criteria of its patch;
- the focused checks it runs.

An implementer writes only inside its ownership and reports its status, the files it changed, the checks it ran and its blockers.

Run patches with disjoint ownership in parallel, and stage a patch after the ones it depends on or overlaps. While patches run, watch only ownership, overlap and the worktree's integrity, and leave each finished patch as it is.

Check: every patch has reported.

### Reconcile once

When every patch is in, make one reconciliation pass over the combined change before Audit: fix formatting, compilation and interface failures, reconcile the patches where the combination needs it, and run the focused checks. Send a failure one patch caused back to its implementer as a narrow repair task.

Check: the combined change passes its focused checks.

Audit is the first holistic review of the combined change.
