# Watchdog review of `widget-dashboard/foundation` (initial)

You review this change fresh. If this session built it, stop and ask the user to start a new session.

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-watchdog-result`
Claim: `0000000000000000000000000000000000000001`
Reviewed head: `0000000000000000000000000000000000000002`
Recorded Integration Target: `0000000000000000000000000000000000000003`
Review round: 1

## Prepare

Prepare the worktree with `skl watchdog prepare --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`, then inspect it with `skl watchdog inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`. Work only in `/work/widgets/.worktrees/widget-dashboard`. Continue this Claim with `skl watchdog resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`; release it with `skl watchdog release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'` only to abandon the review.

Check: inspection shows the source head `0000000000000000000000000000000000000002`.

## Supplied documents

You judge the Contract: `intent.md`, `behavior.md`, and any `plan.md` or `tasks.md`. `implement-report.md` is evidence to verify, never authority. Apply a recorded `decision.md` within the Contract.

`0000000000000000000000000000000000000004:projects/widgets/proposals/widget-dashboard/foundation/behavior.md`

```
# Dashboard foundation behavior

## B1: The dashboard lists every widget

The dashboard lists every registered widget with its current health.

### Scenario: An unhealthy widget is visible
- Given a registered widget whose last check failed
- When the operator opens the dashboard
- Then the widget is listed as unhealthy

```
`0000000000000000000000000000000000000004:projects/widgets/proposals/widget-dashboard/foundation/intent.md`

```
# Dashboard foundation

## Why

Operators check each widget's health one by one.

## What

Show every widget's health on one dashboard page.

## Definition of Done

- [ ] The dashboard lists every widget with its health (B1).

## Manual verification

- [ ] M1: Open the dashboard and confirm each widget's health by hand.

```
`0000000000000000000000000000000000000004:projects/widgets/proposals/widget-dashboard/foundation/implement-report.md`

```
---
schema: 1
outcome: awaiting_review
source:
  head: 0000000000000000000000000000000000000002
  target: 0000000000000000000000000000000000000003
ledger:
  claim:
    commit: 0000000000000000000000000000000000000005
    path: projects/widgets/proposals/widget-dashboard/foundation/state.json
  contract:
    - commit: 0000000000000000000000000000000000000006
      path: projects/widgets/proposals/widget-dashboard/foundation/behavior.md
    - commit: 0000000000000000000000000000000000000006
      path: projects/widgets/proposals/widget-dashboard/foundation/intent.md
---
# Implementation result

## Completion and evidence

| Item | Status | Evidence |
| --- | --- | --- |
| B1 | complete | dashboard listing check |
| M1 | incomplete | human-owned Manual Verification |

## Audit ledger

No findings.

```

## Review

1. Run the Full Gate yourself: the full suite, typecheck and lint. A green suite you did not run is not evidence.
2. Check the report's completion table: every `B<n>`, `A<n>` and warranted `T<n>` is `complete` or `incomplete`, and a missing entry is `incomplete`. Is each `complete` true at this head? Leave the `M<n>` Manual Verification items to the human.
3. Verify every Audit `F<n>` disposition: is each `fixed` true, each `declined` defensible and really a `JUDGEMENT`? A declined `HARD` is a `BLOCK`. Do not rerun Audit.
4. Review the complete change, `git diff 0000000000000000000000000000000000000003...HEAD`, against every Contract item. Apply the method and criteria below. Review against the recorded Integration Target, this round's cutoff: the merge and its conflict resolutions count, unrelated inherited target code does not.

Check: you hold the gate result and a judgement on every Contract item and every `F<n>`.

## Findings

A finding keeps its Work-Item-local `W<n>` across rounds: list resolved ones as resolved, and number new ones after the greatest. Each finding has one disposition, `BLOCK`, `HUMAN` or `NOTE`, and states:

- **Source**: the Contract obligation, project or language rule, or concrete hazard it comes from.
- **Evidence**: what goes wrong, and where.
- **Required outcome**: the observable result that resolves it, not an implementation.

## Verdict

You edit no functional code. Write the report as `skl skill --resource ledger-review.md --input result_directory='/tmp/skl-watchdog-result' --input round=1 --input reviewed_head='0000000000000000000000000000000000000002' watchdog` instructs, then submit with `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --outcome <pass|rework|needs-human>`:

- `pass` only when no `BLOCK` or `HUMAN` finding is active;
- `rework` when the review fails;
- `needs-human` when a human decision is required: `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --outcome needs-human`.

On `pass`, you may add Debt Markers for `NOTE` findings: short, self-contained code comments with no PR number, finding ID or ledger reference. Then confirm `git diff` shows only comments, run the Post-Marker Check (each touched file's formatter or parser, plus `git diff --check`), commit, and submit with `--head <final-sha>`.

# Review method

Assume the implementation is **wrong until it proves otherwise**. A passing suite is necessary, not sufficient: weak tests pass too.

- **Judge test strength, not presence.** For each test, ask: *would this test fail if the behavior broke?* Mentally (or actually) break the behavior and check the test catches it. A green test that asserts nothing meaningful (tautological, over-mocked, asserting a constant) is a finding.
- **Scan the whole for the critical class**: security, privacy, authorization, data loss, compatibility, accessibility, an unusable path.

## What blocks

A finding can block for:

- a failing documented check;
- an unmet accepted behavior or Definition of Done item;
- incorrect behavior this change introduces;
- a material critical-class or reliability risk;
- a mandatory project or language rule (`MUST`, `ALWAYS`, `NEVER`) broken in changed code and absent from the Audit ledger;
- a mandatory finding from a project quality skill;
- material accepted behavior with no credible evidence, or a specific evidence gap: the obligation, a plausible violation, and why the current evidence cannot distinguish them;
- a test that cannot prove the behavior it claims;
- a false claim in the implementation report, or a `HARD` finding it declined.

Not every observation blocks. Ordinary polish belonged to the implementer's Audit, so little of it should remain. A `NOTE` from last round becomes `BLOCK` only on new material evidence or a human's `BLOCK`.

## Repeat reviews stay incremental

An unconstrained repeat search finds new blockers every round and never converges. Instead:

1. Verify every still-active finding against the final state.
2. Read only what changed since the previous reviewed revision, for regressions, integration effects and false claims in the updated report.
3. Scan the whole only for the critical class.

Assign a new `W<n>` only for a defect the rework introduced or a critical discovery. A pre-existing, noncritical thing you merely noticed is a `NOTE`, not another bounce.

# Contract Acceptance and Finding Criteria

Use these criteria for both implementation Audit and independent Watchdog Review. Sharing them does not invoke Audit again and does not replace either stage's fixed-head, fresh-context, artifact-integrity, or handoff responsibilities.

## Judge three concerns distinctly

Judge behavioral conformance, architectural conformance, and local implementation quality separately.

- **Behavioral conformance** — the delivered behavior and failure modes satisfy every accepted rule and scenario.
- **Architectural conformance** — the implementation honors accepted module, interface, seam, ownership, and other plan commitments.
- **Local implementation quality** — changed code follows mandatory standards and avoids concrete maintainability, security, accessibility, reliability, and compatibility harm.

A green suite is relevant evidence, not proof of all three concerns.

## Account for obligations with credible evidence

Every accepted obligation must be accounted for through grouped many-to-many references to concrete tests, commands, or appropriate inspection evidence. Several obligations may share evidence, and one obligation may require several observations. Ordinary executable behavior needs executable evidence; prose assurance alone is insufficient.

For each claimed check, ask whether it observes the promised consequence and would distinguish a plausible violation. Expected outcomes must be independent of the implementation. Additional executable challenges are warranted by concrete risk or uncertainty, not by a universal demand for another test layer, a one-scenario/one-test mapping, or a duplicate suite.

Assess changed tests together. Reuse, strengthening, consolidation, or removal is acceptable only while required behavioral and failure-mode protection remains covered. Scrutinize removed or weakened assertions for lost protection. Do not require a per-test ledger, a unique-bug quota, or a universal mutation score.

## Classify findings by consequence

A concrete contractual violation, material risk, or specific evidence gap can block even when all existing checks pass. An evidence-gap finding names the obligation, the plausible violation, and why existing evidence does not distinguish it. Merely wanting a different test organization, abstraction, or implementation is not an evidence gap.

An equally valid implementation that satisfies the frozen behavior, architecture, and mandatory standards is not a finding. A reviewer's preference alone neither blocks nor needs a Debt Marker. A concrete nonblocking shortcoming may be recorded as judgement or debt when it states the actual maintenance or product consequence.

During Audit, tag contractual violations, material risks, and specific evidence gaps as `HARD`; tag concrete nonblocking quality debt as `JUDGEMENT`. During Watchdog Review, map active blocking defects to `BLOCK`, unresolved consequential decisions to `HUMAN`, and safe actionable debt to `NOTE` under Watchdog's own disposition and authorization rules.

