# Implementation report result document

Write the report body at `{{.ResultDirectory}}/implement-report.md`. The engine adds the schema-1 frontmatter and every ledger reference; write only the Markdown body, and never hand-author `schema`, `outcome`, `source`, `ledger`, or `round`. {{template "result-document" .}}

## Summary

State the implemented change, its scope, and the procedure this report closes.
{{if eq .Procedure "resumed"}}Identify what was already complete and what this continuation added.{{end}}{{if eq .Procedure "rework"}}Identify the fixed point and the head this report closes.{{end}}

## Verification

Write the full current completion-and-evidence table for the reported code revision, not a delta:

- Label Agent-owned rows with their Contract identity: `B<n>` for behavior, `A<n>` for architecture, and `T<n>` for warranted tasks.
- Declare each row `complete` or `incomplete`. `complete` means the required outcome and applicable verification succeeded for this revision.
- Group many-to-many evidence: one evidence item may support several rows, and one row may need several checks. Reference concrete tests, commands, or appropriate inspection evidence with results and material limitations.
- List separately the human-owned `M<n>` checks and declare their current state without marking a human check complete on the human's behalf.
- Omitted entries are never complete and never imply completion. Recording an evidence gap does not make it acceptable, and prose assurance alone is not evidence for ordinary executable behavior.

Include the full source head and the integrated target SHA actually observed and merged, the single Full Gate result over the final functional state, and any material limitations.

## Audit ledger

Record every Audit finding as `F<n>` with its axis (`Standards` or `Contracts`), the severity Audit assigned, and its disposition (`fixed`, `declined`, or `debt`) with one line of reasoning and its evidence. Preserve the fixed point and the audited head you actually audited. {{if eq .Procedure "rework"}}Preserve every historical `F<n>` and `W<n>` identity; add no synthetic finding and advance the cumulative ledger to the newly audited head.{{end}}

## Paused decision (blocker step only)

When a permitted human decision blocks progress, include the question, the evidence, the options with their consequences, and your recommendation, and record the current incomplete status of the affected rows. Pause with the typed `needs_human` outcome rather than prose, and do not approve, merge, or manually release the Claim; the engine releases it atomically with the paused report.

## Public result document

Separately author `{{.ResultDirectory}}/public.md` as the deliberately public, human-facing body. Never use this private report or the worker exchange as the public body, and never publish by default. Public text may omit private operational detail but must acknowledge that the remaining human verification obligations stay accessible privately through `skl`; it may not remove those obligations from human review.
