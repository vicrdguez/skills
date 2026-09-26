Status: prose_required

No public prose was supplied, so the current result of Work Item `widget-dashboard/foundation` is not presented.

Work Item: widget-dashboard/foundation (awaiting_review)
Current result: implement awaiting_review at `projects/widgets/proposals/widget-dashboard/foundation/implement-report.md`
Ledger commit: `0000000000000000000000000000000000000001`
Source: head 0000000000000000000000000000000000000002, target 0000000000000000000000000000000000000003
Branch: `widget-dashboard`

To present the current result, read the private evidence:

- `skl ledger show --commit '0000000000000000000000000000000000000001' --path 'projects/widgets/proposals/widget-dashboard/foundation/implement-report.md'`

Write fresh public prose following `skl skill --resource pull-presentation.md implement`, then run:

`skl ledger present --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --public-body <fresh-public-prose.md>`
