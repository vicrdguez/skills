Status: pending

The current result of Work Item `widget-dashboard/foundation` is not presented yet; the committed result is unaffected.

Work Item: widget-dashboard/foundation (rework)
Current result: watchdog rework, review round 1 at `projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`
Ledger commit: `0000000000000000000000000000000000000001`
Source: head 0000000000000000000000000000000000000002, target 0000000000000000000000000000000000000003, reviewed 0000000000000000000000000000000000000002
Branch: `widget-dashboard`
Public presentation: pending — forge construction unavailable: forge unavailable in isolated delivery test

To present the current result, read the private evidence:

- `skl ledger show --commit '0000000000000000000000000000000000000001' --path 'projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md'`
- `skl ledger show --commit '0000000000000000000000000000000000000004' --path 'projects/widgets/proposals/widget-dashboard/foundation/implement-report.md'`

Write fresh public prose following `skl skill --resource pull-presentation.md watchdog`, then run:

`skl ledger present --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --public-body <fresh-public-prose.md>`
