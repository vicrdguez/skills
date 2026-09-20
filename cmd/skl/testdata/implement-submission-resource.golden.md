# Submission Result Document

This file is the invocation's Result Document, written at the destination named below. The engine publishes it unchanged and never parses, judges, or cross-checks the prose against the semantic command that carries the decision, so the document must stand on its own: state the evidence you established, the alternatives you weighed, and the outcome you recommend. Do not restate settled invocation facts as if you had re-derived them, do not claim work you did not do, and do not encode the decision as machine-readable fields for the engine to read back.
Write `/tmp/implement-result/submission.md` and replace this guidance with your own prose. The engine appends its machine-owned issue-closing footer (`Closes #<issue>`) to this body.

## Summary

Describe the implemented change and its scope.

## Verification

Map every behavioral scenario to its test, and record the focused checks you ran and the Full Gate results.

## Audit ledger --<fixed-point>...<audited-head>

For every finding: ID, axis, the severity Audit assigned, and the disposition `fixed`, `declined`, or `debt` with one line of reasoning and its evidence. Preserve the fixed point and head you actually audited.

