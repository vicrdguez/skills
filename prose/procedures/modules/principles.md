{{define "project-principles"}}{{with principles}}
## Project principles

This repository's principles apply to this work. Read one in full with `skl skill --resource <name>.md principles`.

{{range .}}- `{{.Name}}`: {{.Statement}}
{{end}}{{end}}{{end}}
