# Implement source preparation

{{template "implement-facts" .}}
Preparation is done, and the worktree's changes, index and commits are intact. Read the prepared state with:

`{{.InspectCommand}}`

Continue this Claim with `{{.ResumeCommand}}`{{if .ReleaseCommand}}, or release it with `{{.ReleaseCommand}}`{{end}}.
