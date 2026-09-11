---
name: watchdog
description: Adversarial validation in a fresh context, passing review to the human merge boundary or returning findings.
disable-model-invocation: true
---


Review and validate the changes in an adversarial, guilty-until-proven code review that earns its
place before the human approval. It sits between build and the human's merge: a **model** reviews; the **human** accepts. **The trust boundary (non-negotiable):** This skill **never runs in the context that built the change**.

**Direct invocation:** When invoked directly, execute the watchdog workflow in the current session. Do not launch `watchdog-runner`; the caller is responsible for starting this skill in a fresh session.

 This skill **edits no functional code**: the sole exception is non-functional Debt Marker comments on pass.
 It works on a single unit of work (ticket/PR).
 
 If this packet has Watchdog facts, review that fixed Submission. Otherwise run `skl watchdog next`; `no_work` ends the invocation. Resume an interrupted Claim with `skl watchdog resume --item <number>`. Use the packet's selected remote and conventional worktree, fetching the branch and creating the worktree with ordinary Git if needed. Keep the fixed reviewed head; never rebase or force-push.
 
 
## Verify independently — never on trust

**Run the gate yourself** — the project's full suite, typecheck and lint — and check artifact integrity with `git diff <artifact-baseline> <artifact-completion> -- .changes/<slug>/`, where the only permitted change is a line whose `[ ]` became `[x]` outside Manual Verification. Resolve Artifact Completion as the first parent of the ledger's deletion commit. Verify that a separate subsequent commit removes the entire ledger, it remains absent through review and rework, and both snapshots remain reachable and inspectable in Git history. Read the contract from those snapshots, not the review head. Do not accept the implementor's green suite as sufficient: a green suite you did not run yourself does not count. **Do not re-run `audit`.** The implementor already ran it and published its ledger; a second pass with the same briefs on the same code returns the judgement calls they weighed and declined, which is a disagreement, not a defect.

Use the packet's supplied full or incremental comparison. Artifact integrity always runs against the proposal's `Artifact baseline`, whichever round this is.

## Review guilty-until-proven — claims, tests, contract

Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not
sufficient — weak tests pass too.

- **Verify the ledger, adversarially**. Every `fixed` claim: is it true at this head? Every `declined`: is the reasoning defensible, and was the finding actually a `JUDGEMENT`? A declined `HARD` is a `BLOCK`, that call was never the implementor's to make.
- **Judge test *strength*, not presence.** For each materialized test, ask: *would this test fail if the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A test that asserts nothing meaningful — tautological, over-mocked so it exercises the mock, asserting a constant — is a **finding**, even though it is green.
- **Prove the frozen requirements**. Every `intent.md` "Definition of Done" item is demonstrably met, every `behavior.md` scenario is materialized as a test that actually covers it.
- **Scan the whole for the critical class only**. Security, privacy, authorization, data loss, compatibility, accessibility, an unusable path.

If the change is high-stakes or considered critical you can do an **Independent test re-implementation**, writing the tests yourself from `behavior.md` and diffing intent. However, is an **opt-in escalation** that should be requested by the user explicitly, not the default. The standing default is this adversarial test-strength read.

### Repeat review is incremental

The first review of a PR is complete: read all of it, batch every finding, publish them together. A repeat review is not a second complete review — restarting an unconstrained search is how a PR gets four rounds of new blockers and never converges. Instead:

1. Rerun the gate yourself and verify every still-active finding against the final state.
2. Read only the packet's incremental comparison for regressions the rework introduced and for false claims in the updated ledger.
3. Scan the resulting whole only for the critical class — security, privacy, authorization, data loss, compatibility, accessibility, an unusable path.

Assign a new ID only for a defect the rework introduced or a critical discovery of that last kind. A pre-existing, noncritical thing you merely noticed this round is a `NOTE`, not another bounce. A finding that was `NOTE` last round cannot become `BLOCK` this round without new material evidence or a human's `BLOCK`.

Return the semantic `rework` verdict for a failing review. The engine records every completed review and routes a failure at the default limit of two to Needs Human; passing reviews are never capped.

A repeat review with no new commits is legal: a human resolved everything by disposition. Run the gate and artifact check, honor the dispositions, and pass or pause on what remains.

## Findings

Before assigning dispositions for any verdict, retrieve `skl skill --resource reference/review.md watchdog` for human-directive authorization and precedence, stable finding identities, and Result Document transport.

Each carries one disposition — `BLOCK`, `HUMAN` or `NOTE` — and three things:

- **Source**: the frozen requirement, the project or language rule, or the concrete hazard it comes from.
- **Evidence**: what actually goes wrong, where. Not a category name.
- **Required outcome**: the observable result that would resolve it. Not an implementation — choosing that is the implementor's job.

### What blocks

A finding can block for:

- a failing documented check;
- an unmet frozen behavior or "Definition of Done" item;
- incorrect behavior this PR introduces;
- material security, privacy, authorization, data-loss, compatibility, accessibility or reliability risk;
- an explicit mandatory project or language rule — `MUST`, `ALWAYS`, `NEVER` or equivalent — violated in changed code and absent from the ledger;
- a mandatory finding from a project-specific quality skill;
- material frozen behavior with no credible evidence behind it;
- a test that cannot prove the behavior it claims;
- a false claim in the implementor's ledger, or a `HARD` finding they declined.

Do not turn every declaration, smell, edge case or review observation into a test or a blocker. The implementor's own `audit` pass is where ordinary polish, readability and navigability cleanup belongs, so very little of it should still be here.

### NOTE and debt

A `NOTE` is real and actionable but safe to carry. When it has a place in the code, it lives beside that code, using whatever comment syntax the language takes:

```text
DEBT(#<pr>/W<n>): one-line debt
```

The marker is the record and `grep -rn 'DEBT('` is the index. There is no second copy to keep in sync. A note with no code location stays in the PR or an already-linked issue, do not invent a location to hang it on.

The implementor materializes surviving notes during rework. If a PR passes with notes outstanding and no rework round is coming, you may add the marker entries yourself as part of finalizing. Verify each `DEBT(#<pr>/W<n>)` names the correct stable finding and only non-functional comments changed. Commit and push the final head, record it, then run the formatter or parser for the files you touched and `git diff --check`, not `audit` and not the full suite just for comments. Supply this pushed final SHA with `--head` while retaining the packet's original `--reviewed-head`. The CLI verifies Git identities, not source comments or project checks.

## Pass -> Ready for Merge

When verification passes **and** no `BLOCK` or `HUMAN` finding is still active, mark the change **Ready for Merge**:
- Read the historical `intent.md` with `git show <artifact-baseline>:.changes/<slug>/intent.md`. Copy its `Manual verification` section into the PR body verbatim, with every checkbox unchecked, as the human's checklist. You tick nothing in it: by definition those are the checks no agent can run.
- Keep the retired Implementation Ledger absent; do not restore or archive it.

Write the complete final PR body to the packet's `submission.md`, and submit the packet's semantic command with `--verdict pass --body <absolute-submission.md>`. The engine appends the issue-closing footer and reports `ready_for_merge`, or `rework` for a merge conflict with a fresh Target Snapshot. Ready for Merge leaves the source issue open until GitHub observes the merge. The change now awaits the **human's merge**. The watchdog does not merge.


## Pause → hand the decision to a human

Supply the current ledger and inline evidence with `--verdict needs-human` and exit on the verified outcome. Do not bounce an undecided question to an implementor: they cannot answer it either, and the PR will come straight back. A human answers three ways: requeue to `rework` when implementation must continue, to `review` when only verification remains, or supersede, closing the PR unmerged and letting `explore` revisit the slice before `propose` cuts its replacement. Say which you recommend and why. Read the supplied raw human comments and their association metadata; interpret authorized directives yourself. Prose alone never requeues work.

## Fail → bounce to rework, edit no code

When verification fails **or** the review surfaces a blocking issue:

1. **Write all findings before the handoff**, preserving stable agent-authored `W<n>` identities and the reviewed head in the summary.
2. Submit the packet's command with `--verdict rework`. It publishes the opaque summary and inline bodies before applying the convergence transition.
3. **Modify no code.** Fixing is the implementor's job; collapsing that boundary is exactly what this stage exists to prevent. Do not archive, do not mark as `done`.

## Confirm every handoff completed

Use the structured `skl` outcome: only `ready_for_merge`, `rework`, or `needs_human` with the Claim released completes review. A `fix_required` outcome retains the Claim: repair only the reported deterministic precondition and retry the same semantic command with the same Result Documents. The engine rereads ambiguous writes before retrying; never mutate projections yourself to clean up a partial handoff. Return the final CLI JSON unchanged to a queue adapter; stop on an error or an unverifiable result.
