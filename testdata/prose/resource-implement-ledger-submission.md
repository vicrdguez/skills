# Implementation report

Write the report body at `/tmp/skl-implement-result/implement-report.md`, in Markdown. Start at the first heading, with no frontmatter. The report must stand on its own, with these sections:

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

Write `/tmp/skl-implement-result/public.md` separately, as the pull request body. Submitting leaves the Work Item `awaiting_review`; pausing leaves it `needs_human`. Completed reviews: 0.

The body replaces the whole pull request description. Write it complete, for a reader whose only source is this pull request, as though it were the first version they read:

- **Problem and solution:** the problem the change solves, what it delivers in broad strokes, and the scope it leaves out where a reviewer could expect it. For one slice of a larger change, add a sentence on this slice's part.
- **Status:** the Work Item's state after the latest completed phase, as a settled result, with the number of completed reviews:
  - `awaiting_review`: the change awaits independent review, which comes before any human merge.
  - `rework`: independent review asked for another implementation round; say what it must resolve.
  - `needs_human`: the change waits on a human decision; say what needs deciding, and separate what is delivered from what was planned.
  - `ready_for_merge`: independent review approved the reviewed revision, the only state that means approval; the human-owned checks and the merge remain with a human.
- **Verification:** what the checks establish, the gaps that remain, and each human-owned check, such as Manual Verification, described well enough that its owner knows what to check and why.
- **Decisions and concerns:** the significant decisions and trade-offs that still shape the change, each marked as settled or awaiting a human, and the concerns a reviewer should weigh. Reach recorded decisions through the `decision`, `implement` and `watchdog` references in a report's `ledger` frontmatter, reading each with `skl ledger show --commit <ledger-commit> --path <ledger-path>`.

Write each point in your own words from whichever supplied document holds it, keeping what a reviewer can use. Leave in the reports what only the workflow needs: ledger paths and commits, `skl` commands, Claims, finding numbers, finding lists and worker exchanges. Hold back secrets and exploitable detail, such as credentials or an unfixed vulnerability; when one affects scope or readiness, say so without the detail. You may link public pages, such as merged pull requests or public docs, beside the explanation.

Check: the body covers each point above for a reader who sees only this pull request.
