# Review source inspection

{{template "watchdog-facts" .}}

This read-only continuation reports the prepared source facts and the commands that apply next. It repeats no ledger document, selects no other work, and authorizes no Claim change, ledger mutation, or verdict. Re-read the current source facts with `{{.InspectCommand}}` before handoff.

- Continue this Claim with `{{.ResumeCommand}}`.
{{if .SubmitCommand}}- Submit the settled review with `{{.SubmitCommand}}`.
{{end}}{{if .PauseCommand}}- Hand a decision to a human with `{{.PauseCommand}}`.
{{end}}{{if .ReleaseCommand}}- Release the reservation without destroying progress with `{{.ReleaseCommand}}`.
{{end}}
