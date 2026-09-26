# Implementation report

Write the report body at `/tmp/skl-implement-result/implement-report.md`, in Markdown. The frontmatter is added on submit. The report must stand on its own, with these sections:

## Summary

What the change delivers.

## Verification

- The completion table for the whole change at the final head: every `B<n>`, `A<n>` and warranted `T<n>`, declared `complete` or `incomplete`. A missing row counts as `incomplete`.
- The evidence beside the rows it supports: tests, commands or inspection, with their results and limitations. One check can support several rows.
- The `M<n>` checks, listed separately and left to the human.
- The source head, the integrated target (marked local-only when the fetch failed), the Full Gate result over the final state, and any material limitations.

## Audit ledger

One entry per `F<n>`: its axis, the severity Audit gave it, its disposition (`fixed`, `declined` or `debt`), one line of reasoning and the evidence. Name the audited head, and list checks run after it separately.

## Blocking decision

Only when you pause: the question, the evidence, the options with their consequences, and your recommendation.

# Public body

Write `/tmp/skl-implement-result/public.md` separately, as fresh prose for humans: what the change delivers and the verification behind it, or, at a pause, that the work waits on a human decision. Leave out the report, the findings, the Contract and private operational detail. Say that the human verification obligations remain available privately through `skl`.
