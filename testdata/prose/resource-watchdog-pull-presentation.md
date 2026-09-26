# Current review pull request prose

Write fresh public prose for the Work Item's current review result, as it stands now.

Read the private evidence with the `skl ledger show --commit <commit> --path <path>` references the presentation command supplied, including the implementation report. Use it as evidence only: the reports, findings, Contract and decision history stay private.

- `pass`: independent review approved the reviewed revision. Name the verification behind it and the residual risks, and say that merge remains a human decision.
- `rework`: review requested another implementation round. Say what kind of outcome it awaits.
- `needs_human`: review is paused on a human decision.

Say that the human verification obligations remain available privately through `skl`. Write the prose to a new temporary file and pass it to the presentation command's `--public-body`.
