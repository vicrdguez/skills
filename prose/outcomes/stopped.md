Status: {{.Status}}

{{with .Ending}}Claim `{{.Claim}}` on Work Item `{{.Item}}` {{if eq .Ending "held"}}is still held, so its worker returned before a phase handoff{{else}}was released before any phase handoff{{end}}.{{end}}

{{if .Resume}}Tell the user, and give them both commands to choose from:

- resume the interrupted work: `{{.Resume}}`
- release the Claim: `{{.Release}}`

Then stop.{{else}}Tell the user, and stop.{{end}}
