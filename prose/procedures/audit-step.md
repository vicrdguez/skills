# Audit

{{template "craft/audit.md" .}}## Pin the comparison

After the late target integration, the fixed point is `git merge-base <observed-target-sha> HEAD`.{{if and (eq .Procedure "rework") .PreviousReviewed}} When the previously reviewed revision `{{.PreviousReviewed}}` is an ancestor of `HEAD`, it is the fixed point instead, delimiting this round's delta; otherwise review the full comparison. Either way, keep every prior finding and the completed-review count.{{end}}{{template "audit-diff" .}}

## Establish the facts once

1. Run the inspection command with its `--target` replaced by `<observed-target-sha>`; the target it shows here is the preparation target: `{{.InspectCommand}}`. Its output is the input-inspection result.
2. Run the Full Gate once on the integrated candidate: the project's entire test, typecheck and lint suite. Record the commands, head and results.

Check: you hold the Contract documents supplied above, the diff command, the commit list, the gate result and the input-inspection result.

{{template "audit-dispatch" .Reviewer}}
