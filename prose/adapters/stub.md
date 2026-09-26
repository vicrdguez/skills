{{.Frontmatter}}

<!-- skl-owned: {{.Protocol}} -->

Run `{{if or (eq .Name "implement") (eq .Name "watchdog")}}skl {{.Name}} next{{else}}skl skill {{.Name}}{{end}}`.{{if eq .Name "watchdog"}} Start Watchdog in a fresh session, process one Work Item, and report the verified result in normal Markdown.{{end}}
