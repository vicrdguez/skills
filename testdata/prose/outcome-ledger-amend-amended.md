Status: amended

Proposal `widget-share` is amended in the Workflow Ledger in one commit. Issue publication never gates the amendment.

- Replaced `behavior.md` of Slice `foundation`

Project: widgets (acme/widgets)
Ledger commit: `0000000000000000000000000000000000000001` (projects/widgets)
Ledger push: pushed
Parent issue: attached acme/widgets#107

Slice `foundation`: Dashboard foundation (branch `widget-share`)
  Dependencies: none
  Issue: attached acme/widgets#105
  Readback: `skl slice show --repo '/work/widgets' --remote 'origin' --slice 'widget-share/foundation'`

These active Slices depend on a changed Slice. Ask the user to confirm that their Contracts still hold:
- `widget-share/followup` depends on `widget-share/foundation`

Issues not presented, for lack of a supplied body: `foundation`.

Some issues lack public prose. To publish them, write fresh prose following `skl skill --resource issue-publication.md --input 'proposal=widget-share' --input 'project=widgets' --input 'repo=/work/widgets' --input 'remote=origin' propose`, then run:

`skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget-share' --issue 'foundation='<body-file>`

Next, read each listed slice back with its readback command.
