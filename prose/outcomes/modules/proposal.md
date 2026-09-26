{{- define "outcome-proposal"}}Project: {{.Project}} ({{.Repository}})
Ledger commit: `{{.Commit}}` ({{.HeadRef}})
Ledger push: {{.Push}}
{{- if .Parent}}
Parent issue: {{.Parent}}{{if .ParentPublication}}
Parent publication: {{.ParentPublication}}{{end}}{{end}}{{if .Bookkeeping}}
Publication bookkeeping: {{.Bookkeeping}}{{end}}
{{range .Slices}}
Slice `{{.Name}}`: {{.Title}} (branch `{{.Branch}}`)
  Dependencies: {{if .Dependencies}}{{range $i, $d := .Dependencies}}{{if $i}}, {{end}}{{$d}}{{end}}{{else}}none{{end}}
  Issue: {{.Issue}}{{if .IssuePublication}}
  Issue publication: {{.IssuePublication}}{{end}}{{if .Grouping}}
  Parent grouping: {{.Grouping}}{{end}}
  Readback: `{{.Readback}}`
{{end}}{{end}}
{{- define "outcome-authoring"}}Some issues lack public prose. To publish them, read each slice with its readback command above, write fresh prose following `{{.Guidance}}`, then run:

`{{.Continuation}}`
{{end}}
