Status: {{.Status}}

Refused: merging `{{.Conflict.Branch}}` at `{{.Conflict.Head}}` into `{{.Branch}}` conflicts in:
{{range .Conflict.Paths}}
- `{{.}}`{{end}}

The merge was aborted, and the worktree is clean at its prepared head `{{.Conflict.Prepared}}`. Your Claim `{{.Claim}}` is kept.

This review ends in `rework`, and the implementer resolves the conflict. Write the report as `{{.Report}}` instructs, with one `BLOCK` finding against the Slice's integration with `{{.Conflict.Branch}}` that names these paths. Then submit:

`{{.Submit}}`

Check: submit reports the Work Item returned for rework or waiting on a human.
