Status: {{.Status}}

{{with .Result}}{{if eq $.Status "presented"}}The current result of Work Item `{{.Item}}` is presented.{{else if eq $.Status "prose_required"}}No public prose was supplied, so the current result of Work Item `{{.Item}}` is not presented.{{else if eq $.Status "uncertain"}}The presentation of Work Item `{{.Item}}` may or may not have reached the forge.{{else}}The current result of Work Item `{{.Item}}` is not presented yet; the committed result is unaffected.{{end}}

Work Item: {{.Item}} ({{.Lifecycle}})
Current result: {{.Phase}} {{.Outcome}}{{if .Round}}, review round {{.Round}}{{end}} at `{{.Report.Path}}`
Ledger commit: `{{.Report.Commit}}`{{if $.Source}}
Source: {{$.Source}}{{end}}
Branch: `{{.Branch}}`{{if .Submission}}
Submission: {{.Submission.Repository}}#{{.Submission.Number}}{{end}}{{if .Claimed}}
A later Claim is active; presentation leaves it unchanged.{{end}}
{{end}}{{template "outcome-notes" .Notes}}
{{- with .Guidance}}
{{if eq $.Status "uncertain"}}Check the forge for the pull request before presenting again. {{end}}To present the current result, read the private evidence:
{{range .Evidence}}
- `{{.}}`{{end}}

Write fresh public prose following `{{.Authoring}}`, then run:

`{{.Continue}}`
{{else}}
You are done: tell the user, and stop.
{{end}}