# Current implementation pull request prose

Write the pull request body for the Work Item's current implementation result in a new temporary file, from the private evidence the presentation command listed, and pass it to that command's `--public-body`. The command also shows the Work Item's state and its completed reviews.

The body replaces the whole pull request description. Write it complete, for a reader whose only source is this pull request, as though it were the first version they read:

- **Problem and solution:** the problem the change solves, what it delivers in broad strokes, and the scope it leaves out where a reviewer could expect it. For one slice of a larger change, add a sentence on this slice's part.
- **Status:** the Work Item's state after the latest completed phase, as a settled result, with the number of completed independent reviews:
  - `awaiting_review`: the change awaits independent review, which comes before any human merge.
  - `rework`: independent review asked for another implementation round; say what it must resolve.
  - `needs_human`: the change waits on a human decision; say what needs deciding, and separate what is delivered from what was planned.
  - `ready_for_merge`: independent review approved the reviewed revision, the only state that means approval; the human-owned checks and the merge remain with a human.
- **Verification:** what the checks establish, the gaps that remain, and each human-owned check, such as Manual Verification, described well enough that its owner knows what to check and why.
- **Decisions and concerns:** the significant decisions and trade-offs that still shape the change, each marked as settled or awaiting a human, and the concerns a reviewer should weigh. Follow a `decision` reference in a report's `ledger` frontmatter with `skl ledger show --commit <ledger-commit> --path <ledger-path>`.

Summarize each fact in your own words, by its use to the reader, whichever document holds it. Keep out bookkeeping, such as ledger paths and commits, `skl` commands, Claims, finding numbers, finding inventories and worker exchanges, and genuinely sensitive detail, such as credentials or unfixed security weaknesses; where that detail shapes scope or readiness, state its effect in general terms. Links to public context, such as merged pull requests or public docs, may supplement the explanation.

Check: the body covers each point above for a reader who sees only this pull request.
