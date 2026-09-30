# Watchdog review report

Write the report body at `/tmp/skl-watchdog-result/watchdog-report.md`, in Markdown. The engine adds the frontmatter. The report must stand on its own:

- the reviewed head `0000000000000000000000000000000000000001` and review round 1;
- the finding ledger: every `W<n>`, active and resolved, with its disposition, Source, Evidence and Required outcome;
- the verification evidence: the gate commands and results, and your judgement on each Contract item and each Audit disposition;
- the verdict you recommend, and why.

## Public body

Write `/tmp/skl-watchdog-result/public.md` separately, as the pull request body. `pass` leaves the Work Item `ready_for_merge`, `rework` leaves it `rework`, and `needs-human` leaves it `needs_human`. Completed reviews, counting this one: 1.

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
