# Review source inspection

Repository: acme/widgets on remote `origin`
Work Item: widget-alerts/foundation
Branch: `widget-alerts`
Worktree: `/work/widgets/.worktrees/widget-alerts`
Result Documents: `/tmp/skl-watchdog-result`
Claim: `0000000000000000000000000000000000000001`
Reviewed head: `0000000000000000000000000000000000000002`
Recorded Integration Target: `0000000000000000000000000000000000000003`
Review round: 1
Review scope: `full`
Prepared source head: `0000000000000000000000000000000000000004`
Prepared integrated target: `0000000000000000000000000000000000000003`
Merged `proposal/widget-alerts` head: `0000000000000000000000000000000000000005`


These are the prepared source facts. Re-read them with `skl watchdog inspect --repo '/work/widgets' --remote 'origin' --item 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'` before you submit.

- Continue this Claim with `skl watchdog resume --repo '/work/widgets' --remote 'origin' --item 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --result-directory '/tmp/skl-watchdog-result'`.
- Submit the review with `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --run-metadata '/tmp/skl-watchdog-result/run.json' --outcome <pass|rework|needs-human> --head '0000000000000000000000000000000000000004'`.
- Hand a decision to a human with `skl watchdog submit --repo '/work/widgets' --remote 'origin' --item 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001' --body '/tmp/skl-watchdog-result/watchdog-report.md' --public-body '/tmp/skl-watchdog-result/public.md' --run-metadata '/tmp/skl-watchdog-result/run.json' --outcome needs-human --head '0000000000000000000000000000000000000004'`.
- Release the Claim with `skl watchdog release --repo '/work/widgets' --remote 'origin' --item 'widget-alerts/foundation' --claim '0000000000000000000000000000000000000001'`.

