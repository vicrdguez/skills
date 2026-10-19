{{define "cited-checks"}}{{range .}}- `{{.Name}}`{{with .Case}} `{{.}}`{{end}}, for {{.Covers}}: {{if .Missing}}a row this slice adds, missing from `.skl/verify.md` here{{else}}{{if eq .Kind "journey"}}perform the journey `{{.Run}}`{{else}}run `{{.Run}}`{{end}}; expect {{.Expectations}}; rank `{{.Rank}}`{{end}}.
{{end}}{{end}}
