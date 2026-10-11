Status: {{.Status}}

{{if eq .Status "existing"}}Proposal `{{.Proposal}}` already matches the intake, so nothing new was recorded.{{else}}Proposal `{{.Proposal}}` is amended in the Workflow Ledger in one commit.{{end}} Issue publication never gates the amendment.
{{if eq .Status "amended"}}
{{if .DescriptionReplaced}}- Replaced `proposal.md`
{{end}}{{if .ParentTitleChanged}}- Set the parent title to "{{.ParentTitle}}"
{{end}}{{range .Amended}}{{$name := .Name}}{{if .Added}}- Added Slice `{{.Name}}`
{{end}}{{range .Contract}}- Replaced `{{.}}` of Slice `{{$name}}`
{{end}}{{if .Title}}- Changed the title of Slice `{{.Name}}`
{{end}}{{if .Branch}}- Changed the planned branch of Slice `{{.Name}}`
{{end}}{{if .Dependencies}}- Changed the Dependencies of Slice `{{.Name}}`
{{end}}{{end}}{{end}}
{{template "outcome-proposal" .}}
{{if .Confirm}}These active Slices depend on a changed Slice. Ask the user to confirm that their Contracts still hold:
{{range .Amended}}{{$name := .Name}}{{range .Dependents}}- `{{.}}` depends on `{{$.Proposal}}/{{$name}}`
{{end}}{{end}}
{{end}}{{if .Unpresented}}Issues not presented, for lack of a supplied body: {{range $i, $s := .Unpresented}}{{if $i}}, {{end}}`{{$s}}`{{end}}.

{{end}}{{if .Authoring}}{{template "outcome-authoring" .Authoring}}
{{else if .Unpublished}}Some issue surfaces are not current. To publish them later, without repeating the amendment, run `{{.Publish}}`.

{{end}}{{if .Slices}}Next, read each listed slice back with its readback command.{{else}}Report this outcome to the user, and stop.{{end}}
