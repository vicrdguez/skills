{{define "issue-description"}}Write one body per slice, plus a parent body when the Proposal has several slices, in private temporary Markdown files outside the source tree. Write each body for a collaborator whose only source is that one issue, and write it complete, as though it were the first version they read.

- **Slice body:** the problem, the intended outcome, the accepted scope and what it leaves out, and the broad approach. In a multi-slice Proposal, add a sentence or two on how this slice contributes to the overall change.
- **Parent body:** the overall problem, intended outcome, scope and broad approach, and how the slices group together. Each slice's detailed scope stays in its own issue.
- **Decisions and concerns:** the significant trade-offs and decisions, each marked as settled or still waiting on a human, and the concerns that still matter to a reader.
- **Human checks:** each human-owned check, such as Manual Verification, described in the body well enough that its owner knows what to check and why, plus any known gap in verification.

Judge each fact by its use to the reader, whichever document holds it, and summarize it in your own words. Keep out bookkeeping, such as ledger paths and commits, `skl` commands, Claims, finding numbers, report inventories and worker exchanges, and genuinely sensitive detail, such as credentials or unfixed security weaknesses; where that detail shapes scope or readiness, state its effect in general terms. Describe the work itself rather than its progress. Links to public context, such as merged pull requests or public docs, may supplement the explanation.

Check: every slice has a body file, plus a parent body for a multi-slice Proposal, and each body explains its work to a reader who sees only that issue.
{{end}}
