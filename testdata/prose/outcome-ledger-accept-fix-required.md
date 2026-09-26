Status: fix_required

Refused: slice directory /tmp/test/008/widget-dashboard/foundation misses behavior.md

Repair: supply at least intent.md and behavior.md; add plan.md and tasks.md only when warranted

Make the repair, then rerun, correcting any argument the refusal names:

`skl ledger accept --repo '/work/widgets' --proposal-dir '/tmp/test/008/widget-dashboard' --issue 'foundation=/tmp/fixtures/issue.md'`
