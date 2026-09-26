# Implement source inspection

{{template "implement-facts" .}}
These are the prepared source facts. Re-read them with `{{.InspectCommand}}` after edits and before you submit.

- Continue this Claim with `{{.ResumeCommand}}`.
{{if .SubmitCommand}}- Submit settled work with `{{.SubmitCommand}}`.
{{end}}{{if .PauseCommand}}- Pause for a human decision with `{{.PauseCommand}}`.
{{end}}{{if .ReleaseCommand}}- Release the Claim with `{{.ReleaseCommand}}`.
{{end}}