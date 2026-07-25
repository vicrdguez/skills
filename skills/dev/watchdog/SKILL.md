---
name: watchdog
description: Adversarial, guilty-until-proven validation of a claimed change in a fresh context -- lands it or bounces it, never editing code.
disable-model-invocation: true
---


Review and validate the changes in an adversarial, guilty-until-proven code review that earns its
place before the human approval. It sits between build and the human's merge: a **model** reviews; the **human** accepts. **The trust boundary (non-negotiable):** This skill **never runs in the context that built the change**. 

 This skill **edits no code**: on a pass it lands; on a fail it comments and bounces.
 It works on a single unit of work (ticket/PR).
 
 Use `docs/github.md` to know how to work with the Github board.
 
 
## Verify independently — never on trust

**Re-run the mechanical verification yourself** using `/review` — its Artifacts axis checklist is the single source of what must hold. Do not accept the implementor's green suite as sufficient: a green suite you did not run yourself does not count. The implementor already ran `/review` as their refactor step; that pass ran in the context that built the change and does not count either.

## Review guilty-until-proven — behavior, style, hygiene

Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not
sufficient — weak tests pass too.

- **Judge test *strength*, not presence.** For each materialized test, ask: *would this test fail if
  the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A test
  that asserts nothing meaningful — tautological, over-mocked so it exercises the mock, asserting a
  constant — is a **finding**, even though it is green.
- **Ground style and hygiene in the target project's own quality skills** — its linters, formatters, style guides, and framework-convention skills installed in *that* repo — **not the model's priors**. If the project ships a `/review`/quality skill, run it.
- Read the diff for the ordinary defects too: correctness, edge cases, missed scenarios, mocking the
  code under test, plan drift against `plan.md`.
- Apply the principles from `/design` and `/domain`

If the change is high-stakes or considered critical you can do an **Independent test re-implementation**, writing the tests yourself from `behavior.md` and diffing intent. However,  is an **opt-in escalation**  that should be requested by the user explicitly, not the default. The standing default is this adversarial test-strength read.

### You are not an edge-case hunter - Aim for a single watchdog pass

Even when finding edge cases is important, especially for critical behaviors, the goal is not to run an infinite implement->watchdog->implement loop that keeps finding niche edge cases and never completes.

Find as many edge cases as you can and report at once. If the PR already has a watchdog pass, be very conservative and assess if what you see is really an edge case or if you are just splitting hairs. Edge cases should be reported sparingly and where meaningful. Focus on what is really blocking.


## Pass → land and mark done

When verification passes **and** the review finds no blocking issue, **land the change** by 
- Gathering the acceptance gap that is meant for the human to validate manually. These are the verification steps an agent can't run itself and the human acceptance checklist. Use [acceptance.md](./reference/acceptance.md) as the template, write as a new artifact for the change in `.changes/<slug>`.
- archiving the change **inside the branch** (`.changes/<slug>/` → `.changes/archive/<YYYY-MM-DD>-<slug>/`), commit, and finalize the PR body as the human acceptance checklist. Then **label the PR `done`** (swap off `review`). Push the archive commit to the PR branch.

Hand off through the Board reference: remove `review` and `wip` as it adds `done`. The change now awaits the **human's merge**. The watchdog does not merge.


## Fail → bounce to rework, edit no code

When verification fails **or** the review surfaces a blocking issue:

1. **Leave findings as PR comments before the handoff** — a summary verdict comment plus inline comments anchored to the offending lines. Map `/review`'s findings onto them 1:1: each finding's file/line anchors an inline comment; the summary comment carries the verdict. Be specific: which check failed and where, which test is weak and why it wouldn't catch a regression, which quality-skill rule tripped.
2. Through the Board reference, remove `review` and `wip` as it adds `rework` to hand it back to the implementor.
3. **Modify no code.** Fixing is the implementor's job; collapsing that boundary is exactly what this stage exists to prevent. Do not archive, do not mark as `done`.

## Confirm every handoff completed

After each handoff command, inspect the Board state. A successful handoff ends at `done` or `rework` with `wip` removed. If a comment, label operation, or Board lookup fails or leaves
any other state, report the exact incomplete Board state and stop. Do not present the review as successfully handed off, and never remove `wip` just to clean up a partial handoff.

## Hand-off

- **Pass:** report the PR URL labeled `done`, awaiting the human's merge (the acceptance gate).
- **Fail:** report the PR URL labeled `rework` with the findings summary.
