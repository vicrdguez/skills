Status: {{.Status}}

Proposal `{{.Export.Proposal}}` is exported as an intake directory. The ledger is unchanged.

Project: {{.Export.Project}} ({{.Export.Repository}})
Ledger commit: `{{.Export.Commit}}`
Destination: `{{.Export.Destination}}`
Slices: {{range $i, $s := .Export.Slices}}{{if $i}}, {{end}}`{{$s}}`{{end}}

The destination holds `proposal.json`, `proposal.md` and one directory of Contract files per Slice. Report the destination and ledger commit to the user, and stop.
