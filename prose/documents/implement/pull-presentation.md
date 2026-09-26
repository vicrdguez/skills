# Current implementation pull request prose

Write fresh public prose for the Work Item's current implementation result, as it stands now.

Read the private evidence with the `skl ledger show --commit <commit> --path <path>` references the presentation command supplied. Use it as evidence only: the report, its Audit ledger, the Contract and decision history stay private.

- `awaiting_review`: what the change delivers, the verification a human reader needs, material limitations and risks, and that independent review comes before any human merge.
- `needs_human`: implementation is paused on a human decision. Say why in public terms, leaving out the private question and options.

Say that the human verification obligations remain available privately through `skl`. Write the prose to a new temporary file and pass it to the presentation command's `--public-body`.
