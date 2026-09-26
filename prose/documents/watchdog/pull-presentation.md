# Current review pull request prose

Author fresh public prose for the Work Item's current review result only. Earlier public updates that never reached the pull request, including a missed implementation update, are not replayed, and no earlier body is recovered: describe the result as it stands now.

Read the private evidence with the exact `skl ledger show --commit <commit> --path <path>` references the presentation command supplied, including the consumed implementation report. The review report, its findings, and the Contract stay private; use them as evidence, never as the public body. Detailed findings are not published and no inline comments are emitted.

- For a `pass` result, state that independent review approved the reviewed revision, the verification that supports it, residual risks, and that merge remains a human decision. Ready presentation applies only while the pull request shows that reviewed revision.
- For a `rework` result, state in public terms that review requested another implementation round and what kind of outcome it awaits, without the private finding ledger.
- For a `needs_human` result, state that review is paused on a human decision, without the private question, options, or decision history.

Say that the remaining human verification obligations stay accessible privately through `skl`; public prose may omit private operational detail but cannot remove those obligations from human review. Do not paste reports, Contracts, private operational details, raw decision history, or findings. Write the prose to a new temporary file and pass it to the presentation command's `--public-body`; the file is transport input, never a registered record.
