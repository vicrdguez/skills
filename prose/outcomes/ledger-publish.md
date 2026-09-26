Status: {{.Status}}

The current issue view of proposal `{{.Proposal}}` was published; each surface's result is listed below. Acceptance, lifecycle and Claims are unchanged.

{{template "outcome-proposal" .}}
{{if .Authoring}}{{template "outcome-authoring" .Authoring}}{{else}}Report each surface's result to the user, and stop.
{{end}}