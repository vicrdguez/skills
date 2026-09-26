# Review source inspection

{{template "watchdog-facts" .}}

These are the prepared source facts. Re-read them with `{{.InspectCommand}}` before you submit.

- Continue this Claim with `{{.ResumeCommand}}`.
{{if .SubmitCommand}}- Submit the review with `{{.SubmitCommand}}`.
{{end}}{{if .PauseCommand}}- Hand a decision to a human with `{{.PauseCommand}}`.
{{end}}{{if .ReleaseCommand}}- Release the Claim with `{{.ReleaseCommand}}`.
{{end}}
