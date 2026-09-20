# Submission Result Document

{{template "result-document" .}}
Write `{{.ResultDirectory}}/submission.md` and replace this guidance with your own prose. The engine appends its machine-owned issue-closing footer (`Closes #<issue>`) to this body.

## Summary

Describe the implemented change and its scope.

## Verification

Account for every rule, scenario, and architectural obligation using grouped many-to-many references to concrete tests, commands, or appropriate inspection evidence. One evidence item may support several obligations, and one obligation may require several observations; separate rows are not required.

Record results and material limitations, including the focused checks, Full Gate, and artifact inspection. Identify the selected remote and the full target SHA actually observed and merged at the late pre-Audit step. The integrated target SHA is Verification evidence written in this opaque Markdown, not a rendering input or engine metadata field, and it does not replace the review baseline, Artifact Baseline, Artifact Completion, or a Watchdog invocation's fixed reviewed head.

Evidence must cover the submitted functional state. If Audit dispositions changed functional code, record the affected checks and final Full Gate that covered that later state, distinguishing the audited head from the final verified head. If another target merge changed the candidate, record its integration-effects review and final-state checks rather than reusing evidence from the earlier state. Prose assurance alone is insufficient for ordinary executable behavior, and listing an evidence gap does not make it acceptable.

## Audit ledger --<fixed-point>...<audited-head>

For every finding: ID, axis, the severity Audit assigned, and the disposition `fixed`, `declined`, or `debt` with one line of reasoning and its evidence. Preserve the fixed point and head you actually audited.
{{if eq .Procedure "rework"}}
## Rework

Map every existing finding by its stable ID to the resolution commit and the evidence that holds, or to its linked Debt Marker. Assign each new Audit Finding the next monotonic identity: continue after the greatest existing `F<n>` or begin with `F1` when none exists. Preserve every historical finding identifier unchanged. Advance the existing cumulative Audit ledger to the newly audited head. When the focused Audit is clean, add no synthetic Audit Finding and still advance the cumulative ledger to the newly audited head. Do not add a round-specific provenance section.
{{end}}
