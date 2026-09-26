# Prepare the reviewed source

{{template "watchdog-facts" .}}

Preparation is done. Read the prepared state with:

`{{.InspectCommand}}`

Continue this Claim with `{{.ResumeCommand}}`{{if .ReleaseCommand}}, or release it with `{{.ReleaseCommand}}`{{end}}.
