Status: {{.Status}}

{{if eq .Status "no_work"}}Nothing needed cleanup.{{else if eq .Status "completed"}}Cleanup finished.{{else}}Cleanup is incomplete; the repairs below say why.{{end}}
{{with .ArchiveRepair}}
Archive refused: {{.Reason}}{{if .Repair}}
  Repair: {{.Repair}}{{end}}{{end}}
{{- with .Archive}}{{range .Archived}}
Archived proposal: {{.Proposal}} ({{if .FullyDelivered}}fully delivered{{else}}retired without full delivery{{end}}) at `{{.Commit}}`{{if .Resumed}}
  Finished an interrupted archive move{{end}}{{end}}{{range .Kept}}
Kept active proposal: {{.Proposal}}: {{.Reason}}{{end}}{{range .Repairs}}
Archive repair for {{.Proposal}}: {{.Reason}}
  Repair: {{.Repair}}{{end}}{{with .Replication}}
Ledger replication: {{.Status}}{{if .Detail}} — {{.Detail}}{{end}}{{end}}{{end}}
{{- with .SourceRepair}}
Source cleanup refused: {{.Reason}}{{if .Repair}}
  Repair: {{.Repair}}{{end}}{{end}}
{{- with .Source}}{{range .Removed}}
Removed local source work: {{.}}{{end}}{{range .Preserved}}
Preserved local source work: {{.Branch}}: {{.Reason}}{{end}}{{range .Failed}}
Source removal failed: {{.Branch}}: {{.Reason}}{{end}}{{end}}

{{if eq .Status "fix_required"}}Make each repair, then rerun `{{.Rerun}}`.{{else}}Continue with your next step.{{end}}
