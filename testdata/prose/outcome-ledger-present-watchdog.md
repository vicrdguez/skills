Status: prose_required
no public prose was supplied; author it for the current result from the private evidence below
Work Item: widget-dashboard/foundation (rework)
Current result: watchdog rework, review round 1 at projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md
Ledger commit: 0000000000000000000000000000000000000001
Source: head 0000000000000000000000000000000000000002, target 0000000000000000000000000000000000000003, reviewed 0000000000000000000000000000000000000002
Branch: widget-dashboard
Private evidence: `skl ledger show --commit '0000000000000000000000000000000000000001' --path 'projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md'`
Private evidence: `skl ledger show --commit '0000000000000000000000000000000000000004' --path 'projects/widgets/proposals/widget-dashboard/foundation/implement-report.md'`
Authoring guidance: `skl skill --resource reference/pull-presentation.md watchdog`
Present with fresh prose: `skl ledger present --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/foundation' --public-body <fresh-public-prose.md>`
