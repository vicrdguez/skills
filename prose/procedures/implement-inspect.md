# Implement inspection continuation

{{template "implement-facts" .}}

This read-only continuation resolves the prepared local branch and reports the facts and commands that apply next. It repeats no Contract document, selects no other work, and authorizes no Claim change, ledger mutation, or completion. Re-read the current source facts with `{{.InspectCommand}}` after edits and before handoff.

- Continue this Claim with `{{.ResumeCommand}}`.
{{if .SubmitCommand}}- Submit settled work with `{{.SubmitCommand}}`.
{{end}}{{if .PauseCommand}}- Pause on a consequential unresolved decision with `{{.PauseCommand}}`.
{{end}}{{if .ReleaseCommand}}- Release the reservation without destroying progress with `{{.ReleaseCommand}}`.
{{end}}
