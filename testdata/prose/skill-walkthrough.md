# Walkthrough

Use the user's one Proposal name or `<proposal>/<slice>` reference for this tour.

## Resolve and read

Find the current repository's Project with `skl browse projects --format json`. Resolve the exact name with `skl browse proposal --project <project> --proposal <proposal> --format json` or `skl browse slice --project <project> --item <proposal>/<slice> --format json`. If it is unknown, stop: report that `<reference>` is not an accepted Proposal or Slice. Read `skl browse documents --project <project> --proposal <proposal> --format json` for a Proposal tour, or `skl browse documents --project <project> --item <proposal>/<slice> --format json` for a Slice tour. Read `skl browse slice --project <project> --item <proposal>/<slice> --format json` for every Slice in scope. Use `skl ledger show --repo <repo> --item <proposal>/<slice> --phase implement` and `--phase watchdog` when an exact report read is needed. Read the Proposal description, each included Slice's Contract, latest implementation and watchdog reports, report completion tables, Audit and finding ledgers, and merge facts. Keep absent reports and incomplete reads distinct from a clean record.

Check: the exact reference resolves, and the Proposal context and every included Slice's available evidence are identified.

## Trace delivery

Read the delivered code through git. For a Slice use its recorded merged range, or the pull request head against its merge base when unmerged. For a Proposal with a Proposal Branch, compare that branch with its merge base with `main`; otherwise use the union of its Slices' merged ranges. Use recorded source heads, targets and merge facts to identify the revisions; inspect `git merge-base`, `git diff` and `git show` at those revisions. Attribute each relevant hunk to the Slice that introduced or reshaped it using the Slice ranges and reports, including when later Slices altered earlier work. If a revision is unavailable locally or attribution is uncertain, say what evidence is missing instead of guessing.

Check: the relevant hunks have a source revision and defensible Slice attribution, or a stated evidence gap.

## Tour

Open with the Proposal's intent as context. In one pass over the delivered state, stop in this order across the included Slices:

1. Each `A<n>`: quote the architectural commitment, explain how the code meets it, show the relevant hunks and their Slice attribution.
2. Each `B<n>`: quote the behavior rule, explain how the code meets it, show the relevant hunks and their Slice attribution.
3. Each `F<n>` and `W<n>` whose recorded disposition is `declined` or `debt`: show the finding, disposition and recorded reasoning.
4. The union of open `M<n>` Manual Verification items: use the Contract and completion tables to identify what remains human-owned; quote each item verbatim. An absent report does not close an item.

Identify the Slice with each item so repeated labels remain distinguishable. Pause after **each** stop for the user's questions or permission to continue. Answer from the same Contract, reports and diff. When asked why a choice was made and the record gives no reason, say the record is silent and offer to note it for a follow-up. Treat a requested change as a follow-up Proposal, rather than applying it during this tour. This tour writes no files, commits, ledger records, forge comments or decisions; use read-only `skl browse`, `skl ledger show` and git inspections throughout.

Check: every stop was covered in order, with one pause per stop, and nothing was written.
