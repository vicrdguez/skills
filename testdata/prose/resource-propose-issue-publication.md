# Issue Publication Prose

Write fresh human-facing descriptions of accepted proposal `widget-dashboard` from its current evidence, and publish them through `skl`.

## Read the evidence

Run `skl browse documents --project 'widgets' --proposal 'widget-dashboard'`. It shows the Proposal description, every slice's Contract, current reports and any active Human Decision. When a report's consumed ledger references include a decision or earlier report that bears on the work, read it with `skl ledger show --commit <ledger-commit> --path <ledger-path>`.

Check: you have read the Proposal description and every slice's documents.

## Write the prose

Write one body per slice, plus a parent body when the Proposal has several slices, in private temporary Markdown files outside the source tree. Write each body for a collaborator whose only source is that one issue, and write it complete, as though it were the first version they read.

- **Slice body:** the problem, the intended outcome, the accepted scope and what it leaves out, and the broad approach. In a multi-slice Proposal, add a sentence or two on how this slice contributes to the overall change.
- **Parent body:** the overall problem, intended outcome, scope and broad approach, and how the slices group together. Each slice's detailed scope stays in its own issue.
- **Decisions and concerns:** the significant trade-offs and decisions, each marked as settled or still waiting on a human, and the concerns that still matter to a reader.
- **Human checks:** each human-owned check, such as Manual Verification, described in the body well enough that its owner knows what to check and why, plus any known gap in verification.

Judge each fact by its use to the reader, whichever document holds it, and summarize it in your own words. Keep out bookkeeping, such as ledger paths and commits, `skl` commands, Claims, finding numbers, report inventories and worker exchanges, and genuinely sensitive detail, such as credentials or unfixed security weaknesses; where that detail shapes scope or readiness, state its effect in general terms. Describe the work itself rather than its progress. Links to public context, such as merged pull requests or public docs, may supplement the explanation.

Check: every slice has a body file, plus a parent body for a multi-slice Proposal, and each body explains its work to a reader who sees only that issue.

## Publish

```
skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget-dashboard' --issue <slice>=<body-file> [--issue <slice>=<body-file> ...] [--parent-body <parent-body-file>]
```

Repeat `--issue` for each slice whose issue should be current. `skl` updates the issues it recorded, creates missing ones and groups children under the parent. Publish only through this command.

The outcome reports each surface as `created`, `updated`, `missing_input`, `failed`, `uncertain`, `superseded` or `conflict`:

- `uncertain`: the create may have succeeded. Check the forge before publishing again, since a repeat can duplicate the issue.
- `superseded`: publish again with prose for the current view.
- `conflict`: report it to the user.

Check: you reported each surface's result to the user.
