{{.Frontmatter}}

<!-- skl-owned: {{.Protocol}} -->

Run `{{if or (eq .Name "implement") (eq .Name "watchdog")}}skl {{.Name}} next{{else}}skl skill {{.Name}}{{end}}`.
