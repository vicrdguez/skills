---
name: watchdog
description: Adversarial, guilty-until-proven validation of a claimed change in a fresh context -- lands it or bounces it, never editing code.
disable-model-invocation: true
---


Review and validate the changes in an adversarial, guilty-until-proven code review that earns its
place before the human approval. It sits between build and the human's merge: a **model** reviews; the **human** accepts. **The trust boundary (non-negotiable):** This skill **never runs in the context that built the change**.

**Direct invocation:** When invoked directly, execute the watchdog workflow in the current session. Do not launch `watchdog-runner`; the caller is responsible for starting this skill in a fresh session.

 This skill **edits no code**: on a pass it lands; on a fail it comments and bounces.
 It works on a single unit of work (ticket/PR).
 
 Use `docs/github.md` to know how to work with the Github board.
 
 
## Verify independently — never on trust

**Re-run the mechanical verification yourself** using `audit` — its Artifacts axis checklist is the single source of what must hold. Do not accept the implementor's green suite as sufficient: a green suite you did not run yourself does not count. The implementor already ran `audit` as their refactor step; that pass ran in the context that built the change and does not count either.

`audit` pins the baselines. Give it the PR base merge-base on a first review, or the previous summary's `Reviewed head` on a repeat; artifact integrity always runs against the proposal's `Artifact baseline`, whichever round this is. It runs the gate once per invocation, so do not run it again yourself.

## Review guilty-until-proven — behavior, style, hygiene

Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not
sufficient — weak tests pass too.

- **Judge test *strength*, not presence.** For each materialized test, ask: *would this test fail if
  the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A test
  that asserts nothing meaningful — tautological, over-mocked so it exercises the mock, asserting a
  constant — is a **finding**, even though it is green.
- **Ground style and hygiene in the target project's own quality skills** — its linters, formatters, style guides, and framework-convention skills installed in *that* repo — **not the model's priors**. If the project ships an `audit`/quality skill, run it.
- Read the diff for the ordinary defects too: correctness, edge cases, missed scenarios, mocking the
  code under test, plan drift against `plan.md`.
- Apply the principles from `design` and `domain`

If the change is high-stakes or considered critical you can do an **Independent test re-implementation**, writing the tests yourself from `behavior.md` and diffing intent. However,  is an **opt-in escalation**  that should be requested by the user explicitly, not the default. The standing default is this adversarial test-strength read.

### You are not an edge-case hunter - Aim for a single watchdog pass

Even when finding edge cases is important, especially for critical behaviors, the goal is not to run an infinite implement->watchdog->implement loop that keeps finding niche edge cases and never completes.

Find as many edge cases as you can and report at once. If the PR already has a watchdog pass, be very conservative and assess if what you see is really an edge case or if you are just splitting hairs. Edge cases should be reported sparingly and where meaningful. Focus on what is really blocking.

### Repeat review is incremental

The first review of a PR is complete: read all of it, batch every finding, publish them together. A repeat review is not a second complete review — restarting an unconstrained search is how a PR gets four rounds of new blockers and never converges. Instead:

1. Rerun the gate through `audit` and verify every still-active finding against the final state.
2. Read only `previous-reviewed-head...HEAD` for new defects, standards, hygiene, and regressions the rework introduced.
3. Scan the resulting whole only for the critical class — security, privacy, data loss, compatibility, accessibility, an unusable path.

Assign a new ID only for a defect the rework introduced or a critical discovery of that last kind. A pre-existing, noncritical thing you merely noticed this round is a `NOTE`, not another bounce. A finding that was `NOTE` last round cannot become `BLOCK` this round without new material evidence or a human's `BLOCK`.

**After two completed bounces, stop.** Do not open a third rework cycle: publish the ledger, pause at `needs-human`, and let a human break the tie.

## Findings

Every finding gets an ID, and it keeps that ID for the life of the PR — `W1`, `W2`, `W3`, assigned in order and encoding nothing else. The reader needs to see that this round's `W2` is last round's `W2`. `docs/github.md` holds the ledger format and the disposition grammar.

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
- an explicit mandatory project or language rule — `MUST`, `ALWAYS`, `NEVER` or equivalent — violated in changed code;
- a mandatory finding from a project-specific quality skill;
- material frozen behavior with no credible evidence behind it;
- a concrete maintenance hazard in changed code: duplicated domain policy, a misleading interface, unreachable behavior, hidden coupling, a flow that cannot be read, or a test that cannot prove the behavior it claims.

Project and language rules are authoritative and outrank the generic smell baseline in both directions — they can make something blocking that the baseline would only note, and a repo that documents nothing does not license the model's own priors. A generic smell with no rule and no concrete impact behind it is a `NOTE`.

Do not turn every declaration, smell, edge case or review observation into a test or a blocker. The implementor's own `audit` pass is where ordinary polish, readability and navigability cleanup belongs, so very little of it should still be here.

### NOTE and debt

A `NOTE` is real and actionable but safe to carry. When it has a place in the code, it lives in two places at once — beside the offending code, using whatever comment syntax the language takes:

```text
DEBT(#<pr>/W<n>): one-line debt
```

and as one line in a root `DEBT.md`, keyed by `path · symbol` rather than a line number so it survives edits:

```md
- `#17/W2` — `path · symbol` — one-line debt
```

Create `DEBT.md` lazily, on the first item. Every marker has exactly one index entry and every entry has a live marker; removing the debt removes both. `audit` checks that. A note with no honest code location stays in the PR or an already-linked issue — do not invent a location to hang it on.

The implementor materializes surviving notes during rework. If a PR passes with notes outstanding and no rework round is coming, you may add the markers and index entries yourself as part of finalizing. That is bookkeeping, not review: run the formatter or parser for the files you touched, `git diff --check`, and the debt consistency check — not `audit`, and not the full suite, for comments.

## Human dispositions

A repository owner, member or collaborator can overrule any finding with `W<n> WAIVE`, `W<n> NOTE` or `W<n> BLOCK` in a PR comment, as described in `docs/github.md`. Honor the latest one posted after the finding. Never read a decision into a deleted comment, a reaction, or silence — an unanswered finding is unanswered, not waived.

Neither this skill nor the implementor opens follow-up issues. That stays human or `propose` work.

## Pass → land and mark done

When verification passes **and** no `BLOCK` or `HUMAN` finding is still active, **land the change** by
- Gathering the acceptance gap that is meant for the human to validate manually. These are the verification steps an agent can't run itself and the human acceptance checklist. Use [acceptance.md](./reference/acceptance.md) as the template, write as a new artifact for the change in `.changes/<slug>`. It is the one artifact added after the proposal, and it collects only what the human must check by hand — the template says what belongs in it.
- archiving the change **inside the branch** (`.changes/<slug>/` → `.changes/archive/<YYYY-MM-DD>-<slug>/`), commit, and finalize the PR body as the human acceptance checklist. Then **label the PR `done`** (swap off `review`). Push the archive commit to the PR branch.

Hand off through the Board reference: remove `review` and `wip` as it adds `done`. The change now awaits the **human's merge**. The watchdog does not merge.


## Pause → hand the decision to a human

Some things are genuinely not yours to decide. Pause when the frozen artifacts contradict each other, when they conflict with a mandatory project, language, security or accessibility rule, when what they require is impossible under the pinned constraints, when every safe fix would change frozen behavior, interface or scope, when a blocker is disputed, or when the two-bounce limit is reached.

Nothing else qualifies. An adjacent improvement, a sibling behavior nobody specified, optional polish, a preference between two workable implementations — decide those yourself or leave them alone. Absence of a decision in the artifacts is not a question for a human; it is a choice already delegated to whoever implements it.

Publish the ledger and the inline evidence first, then transition to `needs-human` through the Board reference and exit. Do not bounce an undecided question to an implementor: they cannot answer it either, and the PR will come straight back. A human requeues it once they have answered.

## Fail → bounce to rework, edit no code

When verification fails **or** the review surfaces a blocking issue:

1. **Leave findings as PR comments before the handoff** — a summary verdict comment plus inline comments anchored to the offending lines. Map `audit`'s findings onto them 1:1: each finding's file/line anchors an inline comment; the summary comment carries the verdict. Be specific: which check failed and where, which test is weak and why it wouldn't catch a regression, which quality-skill rule tripped. That summary is the canonical ledger and the durable record: it carries `Reviewed head: <sha>`, every active `W<n>` with its disposition, and the verdict. The next round reads it to know what was already found.
2. Through the Board reference, remove `review` and `wip` as it adds `rework` to hand it back to the implementor.
3. **Modify no code.** Fixing is the implementor's job; collapsing that boundary is exactly what this stage exists to prevent. Do not archive, do not mark as `done`.

## Confirm every handoff completed

After each handoff operation, verify its outcome. If a command fails before any remote side effect occurs—for example, local CLI argument validation—correct the command and retry once. 

If a remote side effect may have occurred, inspect GitHub before retrying to avoid duplicate comments or transitions. Stop when the outcome cannot be established, or when the final Board state is not `done`, `rework` or `needs-human` with `wip` removed. Never remove `wip` just to clean up a partial handoff.

## Hand-off

- **Pass:** report the PR URL labeled `done`, awaiting the human's merge (the acceptance gate).
- **Fail:** report the PR URL labeled `rework` with the findings summary.
- **Pause:** report the PR URL labeled `needs-human` with the decision awaiting an answer.
