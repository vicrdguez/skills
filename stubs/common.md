{{.Frontmatter}}

<!-- skl-owned: {{.Protocol}} -->

Run `{{if eq .Name "implement"}}skl implement next{{else}}skl skill {{.Name}}{{end}}`. Skip activation for every skill named in `included_skills`; its definition is already in the packet.
