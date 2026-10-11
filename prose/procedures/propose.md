Turn the understanding the user confirmed in Explore into an accepted Proposal: **tracer-bullet** vertical slices, each with its Contract and its blocking Dependencies. The decisions are already made; this is precise materialization.

## 1. Gather context

- Find Explore's user-confirmed final recap in this conversation, with every decision it names. It is your approved source for responsibilities, consequences, accepted limits and delegated choices. Without a confirmed recap, stop and suggest running `explore`; a superseded change also needs a fresh one.
- When the user passes a reference, such as a spec path or an issue, read it in full.
- Read `CONTEXT.md` and use its vocabulary. Respect the ADRs relevant to the change.
- When the recap names an accepted Proposal to amend, this run is an **amendment**. Export that Proposal with `skl ledger export --repo <root> --proposal <proposal> --to <dir>` into an intake directory outside the source tree, and read its `proposal.json` and Contracts.

Check: you hold the confirmed recap and every decision it names, and for an amendment, the exported intake.

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

In an amendment, draft only the delta: added Slices, replaced Contracts, changed Dependencies and superseded Slices. Give every dependent of a replaced Slice a disposition: keep it, repoint its Dependencies, replace its Contract, or supersede it. A dependent of a superseded Slice is repointed or superseded; one in another Proposal stays as it is, so name it to the user as needing an amendment of its own Proposal.

Check: every slice has its behavior, its Dependencies and its verification seams, and in an amendment every such dependent has its disposition.

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

In an amendment, present only the delta and each dependent's disposition.

Iterate until the user approves. A small change can be a single slice.

Check: the user approved the breakdown.

## 4. Write the Contracts

Write each slice's Contract documents. Acceptance records their exact bytes. A Contract is read-only from its Slice's first Claim, and an amendment may replace it before then; once a Slice has been claimed, its Contract changes only by superseding the Slice and adding a replacement Slice under a new name. Progress lives in phase reports, and discoveries belong in findings or a renewed Proposal. So resolve every contradiction now, between the documents and between them and the project's rules; a downstream worker can only stop and ask.

| Document | When | Template |
| --- | --- | --- |
| `intent.md` | Always: the result, scope, exclusions, Definition of Done and Manual Verification. | `skl skill --resource intent.md propose` |
| `behavior.md` | Always: the binding rules and the scenarios that discriminate them. Load the `testing` skill to choose observable seams and credible evidence. | `skl skill --resource behavior.md propose` |
| `plan.md` | When the approved design pins architecture. | `skl skill --resource plan.md propose` |
| `tasks.md` | When sequencing, Dependencies or coordination need explicit tracking. | `skl skill --resource tasks.md propose` |

Keep the set compact: carry the approved responsibilities, architectural consequences, accepted limitations and bounds of delegation where they matter to an implementer absent from the conversation. A stronger guarantee with new coordination or recovery duties changes the agreement, even when the slice boundary stays the same. Give each section a descriptive, unique heading.

Label each independently tracked commitment: `B<n>` for a behavior rule, `A<n>` for an architectural commitment, `T<n>` for a warranted task, `M<n>` for human-owned Manual Verification. Label commitments only, never every paragraph or heading. Each label lives in one document; you assign it, and it stays stable from acceptance on.

An unambiguously implied case needs no text of its own. Compare elaborations, including inherited mechanisms they rely on, with the approved consequences. Delegate internal helpers that preserve them; return new blocking or recovery duties to the user before acceptance when the approved sources leave them unsettled. Silence delegates nothing.

In an amendment, edit the exported intake: write Contracts for added and replaced Slices, update the changed `depends` lists, and mark each superseded Slice with `"superseded": true`.

If writing the Contracts changes a slice boundary or Dependency, explain the discovery and return to step 3 with the revised breakdown. Faithful elaboration needs no renewed approval; a changed obligation needs human agreement even within an unchanged slice.

Check: every slice has `intent.md`, `behavior.md` and each warranted document, and in an amendment every added or replaced Slice has them.

## 5. Review fidelity

Run one bounded review in a fresh context across the whole slice set. Give the reviewer:

- the exact confirmed recap;
- every decision and ADR it names;
- every slice and its drafted documents; in an amendment, the changed Slices and the current accepted Contracts of their dependents.

The reviewer checks only whether the drafts omit, weaken, strengthen, contradict or invent obligations relative to those sources, and flags unsettled consequential choices. It leaves resolution of decision gaps and the design to the human.

Correct each demonstrable transcription error against the approved source. Return flagged decision gaps, contradictions within the approved sources and proposed semantic or architectural changes to the user before acceptance; when the resolution changes a boundary or Dependency, return to step 3. Faithful corrections need no second approval and no document-by-document reread.

Check: every review finding is corrected or resolved by the user.

## 6. Write the issue descriptions

In an amendment, the slices this step covers are the added and changed Slices that stay active.

{{template "issue-description" .}}
## 7. Record in the Workflow Ledger

1. Run `skl propose cleanup --repo <root>` and follow its outcome. It archives finished Proposals and removes safe merged source work itself.
2. Commit durable `CONTEXT.md` and ADR changes to the target branch.
3. Prepare one intake directory outside the source tree:
   - `proposal.md`: the durable description of the approved Proposal;
   - `proposal.json`: `{"proposal": "<kebab-name>", "branch": "<optional Proposal Branch>", "parent_title": "<multi-slice only>", "slices": [{"name": "<slug>", "title": "<issue title>", "branch": "<planned source branch>", "depends": ["<sibling slug or proposals/<proposal>/<slice>"]}]}`;
   - one directory per slice, holding only its Contract documents.
   The Proposal Branch defaults to `proposal/<kebab-name>` when omitted. In an amendment, the edited export is the intake.
4. Run `skl ledger accept --repo <root> --proposal-dir <dir> --issue <slice>=<body-file>`, repeating `--issue` per slice and adding `--parent-body <file>` for multi-slice work, and follow its outcome. Acceptance records the Proposal Branch and each planned Slice branch; it creates no branch or worktree. In an amendment, run `skl ledger amend --repo <root> --proposal-dir <dir> --issue <slice>=<body-file>` instead, repeating `--issue` per slice body from step 6 and adding `--parent-body <file>` for multi-slice work, and follow its outcome.
5. Read each accepted or changed slice back with `skl ledger show --repo <root> --item <proposal>/<slice>`. Cite its ledger commit and path when you identify a document exactly.

Check: the readback shows every accepted or changed slice's recorded documents.
