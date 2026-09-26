# Audit

{{template "craft/audit.md" .}}## Pin the comparison

After the late target integration, the fixed point is `git merge-base <observed-target-sha> HEAD`.{{if and (eq .Procedure "rework") .PreviousReviewed}} When the previously reviewed revision `{{.PreviousReviewed}}` is an ancestor of `HEAD`, it is the fixed point instead, delimiting this round's delta; otherwise review the full comparison. Either way, keep every prior finding and the completed-review count.{{end}} The diff command is `git diff <fixed-point>...HEAD`, and the commit list is `git log <fixed-point>..HEAD --oneline`. An empty diff is legal: judge the complete implementation.

## Establish the facts once

1. Run the inspection command with `<observed-target-sha>` as its `--target`: `{{.InspectCommand}}`. Its output is the input-inspection result.
2. Run the Full Gate once on the integrated candidate: the project's entire test, typecheck and lint suite. Record the commands, head and results. Reviewers use this result and do not rerun it.

Check: you hold the Contract documents supplied above, the diff command, the commit list, the gate result and the input-inspection result.

{{template "audit-dispatch" .}}
