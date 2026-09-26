Status: {{.Status}}

Refused: {{.Reason}}
{{if .Repair}}
Repair: {{.Repair}}
{{end}}
{{if eq .ClaimState "kept"}}{{if .Claim}}Your Claim `{{.Claim}}` is kept. {{end}}{{else if eq .ClaimState "unchanged"}}This refusal changed no Claim. {{else if eq .ClaimState "none"}}No Claim was acquired. {{else if eq .ClaimState "uncertain"}}A Claim may have been acquired before the refusal. Run `{{.StatusCommand}}` to see which Work Items hold a Claim; if one does, tell the user which, and stop. {{end}}
{{- if .Rerun}}{{if .Repair}}Make the repair{{else}}Fix the cause{{end}}, then rerun:

`{{.Rerun}}`
{{else}}{{if .Repair}}Make the repair{{else}}Fix the cause{{end}}, then rerun the same command.
{{end}}