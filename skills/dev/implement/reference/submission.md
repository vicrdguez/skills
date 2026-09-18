# Submission Result Document

Write `{{.ResultDirectory}}/submission.md` and replace this guidance with your own prose. The engine publishes the file unchanged and appends its machine-owned issue-closing footer (`Closes #<issue>`); it does not parse, judge, or cross-check your prose against the semantic command.

## Summary

Describe the implemented change and its scope.

## Verification

Map every behavioral scenario to its test, and record the focused checks you ran and the Full Gate results.

## Audit ledger --<fixed-point>...<audited-head>

For every finding: ID, axis, the severity Audit assigned, and the disposition `fixed`, `declined`, or `debt` with one line of reasoning and its evidence. Preserve the fixed point and head you actually audited.
{{if eq .Procedure "rework"}}
## Rework

Map every existing finding by its stable ID to the resolution commit and the evidence that holds, or to its linked Debt Marker. Keep the Audit ledger current for this head.
{{end}}
