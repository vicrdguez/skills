# Issue Publication Prose

Write fresh human-facing descriptions of accepted proposal `widget-dashboard` and publish them through `skl`. Write them from the current private evidence; earlier bodies are not kept.

## Read the evidence

Read each slice the publication outcome lists with `skl ledger show --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/<slice>'`. It shows the frozen Contract, current state, Dependencies and attached issues. That evidence stays private.

Check: you have read every listed slice.

## Write the prose

Write one self-contained Markdown file per slice, plus one parent body when the proposal has several slices, in private temporary files outside the source tree and the ledger clone.

- Say what the work commits to, its current context, useful evidence, and what a human still owns, such as Manual Verification.
- Summarize the Contract, reports and decisions, and point to the private readback for detail.
- Let a parent body describe how its slices group together, leaving each child's detail to the child.

Check: every listed slice has a body file.

## Publish

```
skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget-dashboard' --issue <slice>=<body-file> [--issue <slice>=<body-file> ...] [--parent-body <parent-body-file>]
```

Repeat `--issue` for each slice whose issue should be current. `skl` updates the issues it recorded, creates missing ones and groups children under the parent. Publish only through this command.

The outcome reports each surface as `created`, `updated`, `missing_input`, `failed`, `uncertain`, `superseded` or `conflict`:

- `uncertain`: the create may have succeeded. Check the forge before publishing again, since a repeat can duplicate the issue.
- `superseded`: publish again with prose for the current view.
- `conflict`: report it to the user.

Check: you reported each surface's result to the user.
