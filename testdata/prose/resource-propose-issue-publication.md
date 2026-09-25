# Issue Publication Prose

Author the current human-facing descriptions of accepted proposal `widget-dashboard`, then publish them through `skl`. Each publication needs fresh prose. `skl` neither writes nor summarizes it, and it keeps no earlier body for reuse. Do not look for the bytes of a lost or stale earlier body: write new prose from the current private evidence.

## Read the current private evidence

Read every slice's accepted record with `skl ledger show --repo '/work/widgets' --remote 'origin' --item 'widget-dashboard/<slice>'`, using the slice names the publication outcome lists. Its output carries the frozen Contract documents, current state, dependencies, and established issue attachments. That evidence stays private.

## Write the public prose

Write one self-contained Markdown file per slice, plus one parent body when the proposal has several slices. Keep them in private temporary files, outside both the source tree and the ledger clone.

- Say what the work commits to, the current context, useful evidence, and any obligations a human has, such as Manual Verification that a human still owns.
- Do not paste frozen Contracts, full reports, operational details, or private decision history. Summarize them instead and point to the private readback.
- A parent body describes the grouping of its slices. It does not repeat each child.
- Public text is presentation only. It never becomes workflow authority, and later edits made on the forge change no local record.

## Publish through skl

```
skl ledger publish --repo '/work/widgets' --remote 'origin' --proposal 'widget-dashboard' --issue <slice>=<body-file> [--issue <slice>=<body-file> ...] [--parent-body <parent-body-file>]
```

Repeat `--issue` for each slice whose presentation should be current. `skl` updates an issue it has already recorded and creates a missing one. It groups attached children under the parent. It records only attachments that it has established. Never publish with `gh` and never edit ledger records by hand.

The outcome reports each surface once: `created`, `updated`, `missing_input`, `failed`, `uncertain`, `superseded`, or `conflict`. None of these results changes acceptance, lifecycle, Claims, or Contracts, and none leaves a recovery obligation behind. An `uncertain` create may have succeeded on the forge, so check the forge before you publish again. A later publication can create a duplicate if the earlier create did succeed. Report a `conflict` to a human rather than retargeting it. After a `superseded` result, publish again with prose for the current view.
