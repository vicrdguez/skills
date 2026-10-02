# Issue Publication Prose

Write fresh human-facing descriptions of accepted proposal `{{.Proposal}}` from its current evidence, and publish them through `skl`.

## Read the evidence

Run `skl browse documents --project {{quote .Project}} --proposal {{quote .Proposal}}`. It shows the Proposal description, every slice's Contract, current reports and any active Human Decision. When a report's consumed ledger references include a decision or earlier report that bears on the work, read it with `skl ledger show --commit <ledger-commit> --path <ledger-path>`.

Check: you have read the Proposal description and every slice's documents.

## Write the prose

{{template "issue-description" .}}
## Publish

```
skl ledger publish --repo {{quote .Repo}} --remote {{quote .Remote}} --proposal {{quote .Proposal}} --issue <slice>=<body-file> [--issue <slice>=<body-file> ...] [--parent-body <parent-body-file>]
```

Repeat `--issue` for each slice whose issue should be current. `skl` updates the issues it recorded, creates missing ones and groups children under the parent. Publish only through this command.

The outcome reports each surface as `created`, `updated`, `missing_input`, `failed`, `uncertain`, `superseded` or `conflict`:

- `uncertain`: the create may have succeeded. Check the forge before publishing again, since a repeat can duplicate the issue.
- `superseded`: publish again with prose for the current view.
- `conflict`: report it to the user.

Check: you reported each surface's result to the user.
