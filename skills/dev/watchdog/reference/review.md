# Review Result Documents

Write opaque Markdown in this invocation's private directory: `{{.ResultDirectory}}/summary.md` always, `{{.ResultDirectory}}/submission.md` on a pass, and optional `{{.ResultDirectory}}/findings.json` for structured anchors.

This is review round {{.Round}} of this Submission. The original reviewed head is `{{.ReviewedHead}}`; it remains the original reviewed head even after you commit permitted Debt Marker comments, which are reported separately through `--head`.

## summary.md

`summary.md` is the current finding ledger: stable `W<n>` identities, review round, reviewed head, evidence and dispositions. Finding IDs are local to the PR and monotonic — the same defect keeps its `W<n>` identity for the life of the Submission. A round of 1 does not prove no findings exist: a lost Review Checkpoint resets the retained count while existing findings keep their identities, so preserve every identity you are given. The latest summary is the durable ledger of every still-active finding, with `BLOCK`, `HUMAN`, or `NOTE` dispositions, and includes the full reviewed-head SHA so a repeat worker recovers its fixed point without CLI prose parsing.

## Human directives

An owner, member, or collaborator may direct `W1 WAIVE` (accepted without debt), `W1 BLOCK`, or `W1 NOTE` in a free-form comment posted after the finding. Trim the directive and interpret it case-insensitively; only comments after the finding count, and the latest authorized directive wins. `WAIVE` accepts the finding without debt, while `BLOCK` and `NOTE` keep their dispositions. Never infer a decision from reactions, silence, or deleted comments. No stage creates follow-up issues on its own; that remains human or Propose work.

This authorization and precedence guidance is available before you choose any disposition: the verdict and the finding choices are not inputs to this resource.

## findings.json

Optional `findings.json` transports structured anchors, not finding prose:

```json
[{"path":"src/example.go","line":12,"side":"RIGHT","body_file":"/absolute/private/directory/W1.md"}]
```

Use `LEFT` for an old-side line or `RIGHT` for a new-side line. Write the finding's opaque Markdown in its `body_file`; anchors use the submitted fixed head. Keep each finding's identity in its agent-authored body. The opaque summary and finding bodies stay separate from these optional anchors and from the semantic `--verdict` flag.

## On pass

`{{.ResultDirectory}}/submission.md` is the complete final PR body including the historical Manual Verification section verbatim with every checkbox unchecked. Preserve the Audit ledger and review evidence. The engine appends `Closes #<source-issue>`; no archive or source-issue close occurs at Ready for Merge.
