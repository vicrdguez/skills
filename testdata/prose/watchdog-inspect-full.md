---
name: watchdog
description: Adversarial validation in a fresh context, passing review to the human merge boundary or returning findings.
disable-model-invocation: true
---

# Review source inspection

Repository: acme/widgets on remote `origin`
Work Item: widget-dashboard/foundation
Branch: `widget-dashboard`
Worktree: `/work/widgets/.worktrees/widget-dashboard`
Result Documents: `/tmp/skl-watchdog-result`
Claim: `0000000000000000000000000000000000000001`
Fixed reviewed implementation head: `0000000000000000000000000000000000000002`
Recorded Integration Target: `0000000000000000000000000000000000000003`
Previous reviewed revision: `0000000000000000000000000000000000000004`
Completed reviews: 2; this invocation is review number 3
Review scope: `full`
Prepared source head: `0000000000000000000000000000000000000002`
Prepared integrated target: `0000000000000000000000000000000000000003`


This read-only continuation reports the prepared source facts and the commands that apply next. It repeats no ledger document, selects no other work, and authorizes no Claim change, ledger mutation, or verdict. Re-read the current source facts with `skl watchdog inspect --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'` before handoff.

- Continue this Claim with `skl watchdog resume --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`.
- Submit the settled review with `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --outcome <pass|rework|needs-human>`.
- Hand a decision to a human with `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --outcome needs-human`.
- Release the reservation without destroying progress with `skl watchdog release --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --claim '0000000000000000000000000000000000000001'`.

