Status: fix_required

Refused: slice widget-search/foundation was claimed before (it is awaiting_review), so its Contract, title and planned branch stay as recorded

Repair: restore its recorded intent.md in the intake; carry new obligations in an added Slice

Make the repair, then rerun:

`skl ledger amend --repo '/work/widgets' --proposal-dir '/tmp/amend-2/widget-search'`
