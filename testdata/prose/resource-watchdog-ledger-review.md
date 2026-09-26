# Watchdog review report

Write the report body at `/tmp/skl-watchdog-result/watchdog-report.md`, in Markdown. The engine adds the frontmatter. The report must stand on its own:

- the reviewed head `0000000000000000000000000000000000000001` and review round 1;
- the finding ledger: every `W<n>`, active and resolved, with its disposition, Source, Evidence and Required outcome;
- the verification evidence: the gate commands and results, and your judgement on each Contract item and each Audit disposition;
- the verdict you recommend, and why.

## Public body

Write `/tmp/skl-watchdog-result/public.md` separately, as fresh prose for humans: the verdict and the verification behind it. Leave out the findings, the Contract and private operational detail. Say that the human verification obligations remain available privately through `skl`.
