Turn the understanding the user confirmed in Explore into an accepted Proposal: **tracer-bullet** vertical slices, each with its Contract and its blocking Dependencies. The decisions are already made; this is precise materialization.

## 1. Gather context

- Find Explore's user-confirmed final recap in this conversation, with every decision it names. It is your approved source. Without a confirmed recap, stop and suggest running `explore`; a superseded change also needs a fresh one.
- When the user passes a reference, such as a spec path or an issue, read it in full.
- Read `CONTEXT.md` and use its vocabulary. Respect the ADRs relevant to the change.

Check: you hold the confirmed recap and every decision it names.

## 2. Draft vertical slices

Break the work into tracer-bullet slices:

- Each slice cuts a narrow but complete path through every layer it touches, and is demoable and verifiable on its own.
- Each slice fits a single fresh context window.
- Give behaviors that deliver safe, useful results independently their own slices. Judge independence with declared Dependencies Merged and later slices absent.
- Combine independently useful behaviors only for a concrete cut in overall implementation or review burden. Shared files or a shared Workflow stage are not enough.
- Judge review burden by the behavior and the distinct correctness, failure and recovery concerns a reviewer must hold together. Use the agreed requirements and focused inspection of the repository, not line counts or detailed implementation plans. Different error cases alone need no separate slices.

Load the `design` skill, then sketch the seams at which each slice is verified:

- Keep every seam the user deliberately agreed.
- Where the seam choice was delegated, prefer an existing seam, and use the highest one that exposes the promised consequence.
- Before drafting, return to the user any consequential seam that is neither settled nor delegated.

Check: every slice has its behavior, its Dependencies and its verification seams.

## 3. Quiz the user

Present the breakdown as a numbered list. For each slice show:

- *Title*: a short descriptive name;
- *Blocked by*: the slices that must complete first, if any;
- *What it delivers*: the end-to-end behavior it makes work;
- *Review burden*: its concerns, and any concrete reason for combining independently useful behaviors.

Ask the user:

- Does the granularity feel right, too coarse or too fine?
- Are the blocking edges correct: does each slice depend only on slices that genuinely gate it?
- Should any slices be merged or split further?

Iterate until the user approves. A small change can be a single slice.

Check: the user approved the breakdown.

## 4. Write the Contracts

Write each slice's Contract documents. Acceptance records their exact bytes and freezes them: from then on they are read-only, progress lives in phase reports, and discoveries belong in findings or a renewed Proposal. So resolve every contradiction now, between the documents and between them and the project's rules; a downstream worker can only stop and ask.

| Document | When | Template |
| --- | --- | --- |
| `intent.md` | Always: the result, scope, exclusions, Definition of Done and Manual Verification. | `skl skill --resource intent.md propose` |
| `behavior.md` | Always: the binding rules and the scenarios that discriminate them. Load the `testing` skill to choose observable seams and credible evidence. | `skl skill --resource behavior.md propose` |
| `plan.md` | When the approved design pins architecture. | `skl skill --resource plan.md propose` |
| `tasks.md` | When sequencing, Dependencies or coordination need explicit tracking. | `skl skill --resource tasks.md propose` |

Keep the set compact: preserve the approved decisions, and add no obligation they lack. Give each section a descriptive, unique heading.

Label each independently tracked commitment: `B<n>` for a behavior rule, `A<n>` for an architectural commitment, `T<n>` for a warranted task, `M<n>` for human-owned Manual Verification. Label commitments only, never every paragraph or heading. Each label lives in one document; you assign it, and it stays stable from acceptance on.

An unambiguously implied case needs no text of its own. Resolve every consequential behavioral or architectural gap before acceptance: silence delegates nothing.

If writing the Contracts changes a slice boundary or Dependency, explain the discovery and return to step 3 with the revised breakdown. Elaboration within unchanged slices needs no renewed approval.

Check: every slice has `intent.md`, `behavior.md` and each warranted document.

## 5. Review fidelity

Run one bounded review in a fresh context across the whole slice set. Give the reviewer:

- the exact confirmed recap;
- every decision and ADR it names;
- every slice and its drafted documents.

The reviewer checks only whether the drafts omit, weaken, strengthen, contradict or invent obligations relative to those sources. It leaves the design, the accepted choices and any decision gap alone.

Correct each demonstrable transcription error against the approved source. Return to the user an unsettled consequential choice, a contradiction within the approved sources, or a proposed semantic or architectural change; when the resolution changes a boundary or Dependency, return to step 3. Faithful corrections need no second approval and no document-by-document reread.

Check: every review finding is corrected or resolved by the user.

## 6. Accept into the Workflow Ledger

1. Run `skl propose cleanup --repo <root>` and follow its outcome. It archives finished Proposals and removes safe merged source work itself.
2. Commit durable `CONTEXT.md` and ADR changes to the target branch.
3. Prepare one intake directory outside the source tree:
   - `proposal.md`: the durable description of the approved Proposal;
   - `proposal.json`: `{"proposal": "<kebab-name>", "parent_title": "<multi-slice only>", "slices": [{"name": "<slug>", "title": "<issue title>", "branch": "<planned source branch>", "depends": ["<sibling slug or proposals/<proposal>/<slice>"]}]}`;
   - one directory per slice, holding only its Contract documents.
4. Write a self-contained, human-facing issue body for each slice, plus a parent body for multi-slice work, in private temporary Markdown files.
5. Run `skl ledger accept --repo <root> --proposal-dir <dir> --issue <slice>=<body-file>`, repeating `--issue` per slice and adding `--parent-body <file>` for multi-slice work, and follow its outcome. Acceptance records each planned branch; it creates no branch or worktree.
6. Read each slice back with `skl ledger show --repo <root> --item <proposal>/<slice>`. Cite its ledger commit and path when you identify a document exactly.

Check: the readback shows every slice's accepted documents.
