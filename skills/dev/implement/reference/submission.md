# Submission Result Document

{{template "result-document" .}}
Write `{{.ResultDirectory}}/submission.md` and replace this guidance with your own prose. The engine appends its machine-owned issue-closing footer (`Closes #<issue>`) to this body.

## Summary

Describe the implemented change and its scope.

## Verification

Map every behavioral scenario to its test, and record the focused checks you ran and the Full Gate results.

## Audit ledger --<fixed-point>...<audited-head>

For every finding: ID, axis, the severity Audit assigned, and the disposition `fixed`, `declined`, or `debt` with one line of reasoning and its evidence. Preserve the fixed point and head you actually audited.
{{if eq .Procedure "rework"}}
## Rework

Map every existing finding by its stable ID to the resolution commit and the evidence that holds, or to its linked Debt Marker. Assign each new Audit Finding the next monotonic identity: continue after the greatest existing `F<n>` or begin with `F1` when none exists. Preserve every historical finding identifier unchanged. Advance the existing cumulative Audit ledger to the newly audited head. When the focused Audit is clean, add no synthetic Audit Finding and still advance the cumulative ledger to the newly audited head. Do not add a round-specific provenance section.
{{end}}
