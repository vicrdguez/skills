{{- define "outcome-phase"}}{{if eq . "implement"}}Implement{{else}}Watchdog Review{{end}}{{end}}
{{- define "outcome-notes"}}{{range .}}{{.Label}}: {{.Status}}{{if .Detail}} — {{.Detail}}{{end}}
{{end}}{{end}}
{{- define "outcome-handoff"}}{{if .AlreadyCompleted}}An earlier run already recorded this handoff, so nothing new was committed. {{end}}Your Claim is released.

Phase Report: `{{.Report.Path}}` at `{{.Report.Commit}}`
{{template "outcome-notes" .Notes}}{{if .Present}}
The pull request does not show this result yet. To present it later, without repeating the handoff, run `{{.Present}}`.
{{end}}{{end}}
{{- define "outcome-merge"}}Proposal Branch merge of `{{.Item}}`: {{if eq .Status "merged"}}squash-merged into `{{.Branch}}` as `{{.Commit}}`.{{else}}not merged, so it stays ready for a human to merge. Reason: {{.Reason}}{{end}}
{{end}}
{{- define "outcome-merges"}}{{range .}}{{template "outcome-merge" .}}{{end}}{{if .}}
{{end}}{{end}}
{{- define "outcome-previous"}}{{with .}}Claim `{{.Claim}}` on Work Item `{{.Item}}` {{if eq .Ending "awaiting_review"}}was submitted for review{{else if eq .Ending "pass"}}passed review{{else if eq .Ending "rework"}}was returned for rework{{else if eq .Ending "needs_human"}}was paused for a human decision{{end}}.

{{end}}{{end}}
