# Implementation report

Write the report body at `{{.ResultDirectory}}/implement-report.md`, in Markdown. Start at the first heading, with no frontmatter. The report must stand on its own, with these sections:

## Summary

What the change delivers.{{if eq .Procedure "resumed"}} What was already done, and what this round added.{{end}}{{if eq .Procedure "rework"}} Which findings this round resolved.{{end}}

## Verification

- The completion table for the whole change at the final head: every `B<n>`, `A<n>` and warranted `T<n>`, declared `complete` or `incomplete`. A missing row counts as `incomplete`.
- The evidence beside the rows it supports: tests, commands or inspection, with their results and limitations. One check can support several rows.
- The `M<n>` checks, listed separately and left to the human.
- The source head, the integrated target (marked local-only when the fetch failed), the Full Gate result over the final state, and any material limitations.

## Audit ledger

One entry per `F<n>`: its axis, the severity Audit gave it, its disposition (`fixed`, `declined` or `debt`), one line of reasoning and the evidence. Name the audited head, and list checks run after it separately.{{if eq .Procedure "rework"}} Keep every earlier `F<n>` and `W<n>` with its number.{{end}}
{{if eq .Procedure "rework"}}
## Finding map

Every active finding, `F<n>` or `W<n>`, with the commit that resolves it and the check that tells the fix from the reported failure.
{{end}}
## Blocking decision

Only when you pause: the question, the evidence, the options with their consequences, and your recommendation.

# Public body

Write `{{.ResultDirectory}}/public.md` separately, as fresh prose for humans: what the change delivers and the verification behind it, or, at a pause, that the work waits on a human decision. Leave out the report, the findings, the Contract and private operational detail. Say that the human verification obligations remain available privately through `skl`.
