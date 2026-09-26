Status: {{.Status}}

{{if eq .Status "existing"}}Proposal `{{.Proposal}}` was already accepted unchanged, so nothing new was recorded.{{else}}Proposal `{{.Proposal}}` is accepted into the Workflow Ledger.{{end}} Issue publication never gates the accepted work.

{{template "outcome-proposal" .}}
{{if .Authoring}}{{template "outcome-authoring" .Authoring}}
{{else if .Unpublished}}Some issue surfaces are not current. To publish them later, without repeating acceptance, run `{{.Publish}}`.

{{end}}Next, read each slice back with its readback command.
