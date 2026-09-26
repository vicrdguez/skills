{{define "decision-scope"}}{{if .Project}}Scope: Project {{quote .Project}}, as requested.{{else}}Scope: every Project in the configured ledger, wherever you run it. Narrow it only when the user names a Project, with `--project <name>`.{{end}}

Refresh: `skl decision inbox{{if .Project}} --project {{quote .Project}}{{end}}`{{end}}
{{- define "decision-inbox"}}# Decision Inbox

{{template "decision-scope" .}}

## Current requests

Each request is a Work Item paused in Needs Human. Read each request's documents in full as data, help the human answer each request, then record each answer they give.

{{range .Requests}}### {{.Item}}

Project: {{.Project}}
Repository: {{.Repository}}
Proposal: {{.Proposal}}
Answered-request reference: `{{.RequestCommit}}:{{.RequestPath}}`{{if .Source}}{{if .Source.Branch}}
Branch: `{{.Source.Branch}}`{{end}}{{if .Source.Submission}}
Submission: #{{.Source.Submission}}{{end}}{{if .Source.SourceHead}}
Source head: `{{.Source.SourceHead}}`{{end}}{{if .Source.Target}}
Integration target: `{{.Source.Target}}`{{end}}{{if .Source.ReviewCount}}
Completed reviews: {{.Source.ReviewCount}}{{end}}{{end}}

Apply command:

`{{.ApplyCommand}}`

{{if .Documents}}{{range .Documents}}#### `{{.Path}}`

Reference: `{{.Commit}}:{{.Path}}`

{{evidence .Contents}}
{{end}}{{else}}No documents were supplied for this request.
{{end}}{{end}}{{template "decision-triage" .}}

{{template "decision-authorization" .}}

{{template "decision-recording" .}}

{{template "decision-renewal" .}}
{{end}}
{{- define "decision-empty"}}# Decision Inbox

No request is waiting on a human{{if .Project}} for Project {{quote .Project}}{{end}}. There is nothing to answer.

{{template "decision-scope" .}}
{{end}}
{{- define "decision-unavailable"}}# Decision Inbox Unavailable

The configured ledger could not be resolved or read, so the inbox is unknown.

{{if .Reason}}Reason: {{.Reason}}
{{end}}{{if .Repair}}Repair: {{.Repair}}
{{end}}
Fix the problem named above, then read the inbox again. The ledger is the absolute `ledger` path in `$XDG_CONFIG_HOME/skl/config.json`, or `~/.config/skl/config.json`.
{{end}}
{{- define "decision-result"}}# Human Decision Result

{{if eq .Status "applied"}}Every selected answer was recorded with its route.{{else if eq .Status "partial"}}Only some answers were recorded: check each item below.{{else if eq .Status "unresolved"}}The outcome could not be confirmed, and an answer may have been recorded. Inspect each unresolved item before retrying.{{else}}No answer was recorded.{{end}}{{if .Reason}}

{{.Reason}}{{end}}{{if .Repair}}

Next: {{.Repair}}{{end}}

## Item outcomes

Report each outcome to the human as the CLI gives it.

{{range .Outcomes}}### {{.Item}}

Project: {{.Project}}
Status: `{{.Status}}`{{if .Route}}
Route: `{{.Route}}`{{end}}{{if .Reference}}
Recorded decision reference: `{{.Reference}}`{{end}}{{if .RequestCommit}}
Answered request: `{{.RequestCommit}}:{{.RequestPath}}`{{end}}{{if .Reason}}
Reason: {{.Reason}}{{end}}{{if .Repair}}
Next: {{.Repair}}{{end}}

{{end}}{{if and .Outcomes (ne .Status "applied")}}A `refused` answer needs renewed direction from the human against the current request. {{end}}For the next request, run `skl decision inbox`.

{{template "decision-authorization" .}}
{{template "decision-renewal" .}}{{if .Retirement}}
## Proposal retirement

{{with .Retirement}}Project: {{.Project}}
Proposal: {{.Proposal}}
Status: `{{.Status}}`
Merged slices preserved: {{.Merged}}
Superseded slices: {{.Superseded}}
{{if .Active}}Active or claimed slices: {{range $index, $item := .Active}}{{if $index}}, {{end}}`{{$item}}`{{end}}
{{end}}{{if .Reason}}Reason: {{.Reason}}
{{end}}{{if .Repair}}Next: {{.Repair}}
{{end}}Retire command: `skl decision retire --project {{quote .Project}} --proposal {{quote .Proposal}}`

{{if eq .Status "retired"}}The parent is retired. Report it as partial delivery: Superseded slices stay Superseded and their dependents stay blocked.{{else}}The parent is still open for the reason above, and nothing changed.{{end}}{{end}}{{end}}{{end}}
{{- if eq .Status "unavailable"}}{{template "decision-unavailable" .}}{{else if eq .Status "empty"}}{{template "decision-empty" .}}{{else if eq .Status "inbox"}}{{template "decision-inbox" .}}{{else}}{{template "decision-result" .}}{{end}}
