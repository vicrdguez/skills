# Watchdog review report

Write the report body at `{{.ResultDirectory}}/watchdog-report.md`, in Markdown. The engine adds the frontmatter. The report must stand on its own:

- the reviewed head `{{.ReviewedHead}}` and review round {{.Round}};
- the finding ledger: every `W<n>`, active and resolved, with its disposition, Source, Evidence and Required outcome;
- the verification evidence: the gate commands and results, and your judgement on each Contract item and each Audit disposition;
- the verdict you recommend, and why.

## Public body

Write `{{.ResultDirectory}}/public.md` separately, as the pull request body. `pass` leaves the Work Item `ready_for_merge`, `rework` leaves it `{{if .ReworkPauses}}needs_human{{else}}rework{{end}}`, and `needs-human` leaves it `needs_human`. Completed reviews, counting this one: {{.Round}}.

{{template "pull-brief"}}
