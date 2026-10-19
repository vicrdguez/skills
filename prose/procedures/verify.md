# Verify `{{.Repository}}`

Verify this repository as its skill below directs, and rank each result on its ladder, strongest first: {{range $index, $rank := .Ranks}}{{if $index}}, {{end}}`{{$rank}}`{{end}}.{{with .Channels}} Retrieve the craft of each channel you verify through:

{{range .}}- `{{.}}`: `skl skill --resource {{.}}.md verify`
{{end}}{{end}}
{{.Body}}
