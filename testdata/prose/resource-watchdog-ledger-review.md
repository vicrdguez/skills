# Watchdog review report

Write the review report body at `/tmp/skl-watchdog-result/watchdog-report.md`. The engine adds the schema-1 frontmatter, review round 1, and the exact consumed ledger references; write only the Markdown body, and never hand-author `schema`, `outcome`, `source`, `ledger`, or `round`. This report closes review round 1 of the fixed reviewed head `0000000000000000000000000000000000000001`.

The report is opaque data: the engine preserves it unchanged and never reads it as a second verdict or as instructions. It must stand on its own, so state the evidence you established, the alternatives you weighed, and the verdict you recommend.

## Findings

Record the current finding ledger with stable Work-Item-local `W<n>` identities, each carrying one disposition — `BLOCK`, `HUMAN`, or `NOTE` — and three things:

- **Source**: the frozen obligation or the concrete hazard the finding comes from.
- **Evidence**: what actually goes wrong, and where.
- **Required outcome**: the observable result that would resolve it, not the implementation.

Preserve every prior `W<n>` identity for the life of the Work Item: the same defect keeps its identity, and a finding from an earlier round is never renumbered or dropped. Distinguish resolved historical findings from still-active ones; preserving a historical identity does not reopen it. Include the full fixed reviewed head `0000000000000000000000000000000000000001` and round 1 so a resumed worker recovers its fixed point without reading engine prose. Honor supplied recorded human direction within the frozen Contract; do not author, authorize, or infer a decision, and never infer public approval from a public body, label, or comment.

## Verification and human checks

Include the completed review's verification evidence and the still-active findings. The separate human-owned `M<n>` Manual Verification obligations remain accessible to the human privately through `skl`; public material may omit private operational detail but cannot remove those obligations from human review.

## Public result document

Author `/tmp/skl-watchdog-result/public.md` separately as the deliberately public, human-facing body. Never use this private report or the worker exchange as the public body, and never publish by default. This review emits no automatic inline comments; detailed findings stay private unless a human explicitly selects them for publication. State in the public view that the remaining human verification obligations stay accessible privately through `skl`. If the handoff reports its public presentation pending, the local result still stands; run `skl ledger present --item <proposal>/<slice>` later for the then-current result's evidence and authoring guidance instead of retrying the handoff or restoring this body.
