# Current implementation pull request prose

Author fresh public prose for the Work Item's current implementation result only. Earlier public updates that never reached the pull request are not replayed, and no earlier body is recovered: describe the result as it stands now.

Read the private evidence with the exact `skl ledger show --commit <commit> --path <path>` references the presentation command supplied. The implementation report, its Audit ledger, and the Contract stay private; use them as evidence, never as the public body.

- For an `awaiting_review` result, state what the change delivers, the verification that matters to a human reader, material limitations and risks, and that independent review follows before any human merge.
- For a `needs_human` pause, state that implementation is paused on a human decision and why in public terms, without the private question, options, or decision history.

Say that the remaining human verification obligations stay accessible privately through `skl`; public prose may omit private operational detail but cannot remove those obligations from human review. Do not paste reports, Contracts, private operational details, raw decision history, or inline findings. Write the prose to a new temporary file and pass it to the presentation command's `--public-body`; the file is transport input, never a registered record.
