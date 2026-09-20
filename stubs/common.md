{{.Frontmatter}}

<!-- skl-owned: {{.Protocol}} -->

Run `{{if or (eq .Name "implement") (eq .Name "watchdog")}}skl {{.Name}} next{{else}}skl skill {{.Name}}{{end}}`. Skip activation for every skill named in `included_skills`; its definition is already in the packet.{{if eq .Name "watchdog"}} Start Watchdog in a fresh session, process one Work Item, and report the verified result in normal Markdown.{{end}}
