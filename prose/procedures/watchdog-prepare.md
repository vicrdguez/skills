# Prepare the reviewed source

{{template "watchdog-facts" .}}

Source preparation has completed for this Claim. It created or safely reused the exact planned worktree, preserving dirty files, the index, and existing branch progress without resetting, stashing, rebasing, or forcing work aside. Do not repeat preparation; it selected no other work and changed no Workflow State or private ledger record. Read the prepared state with:

`{{.InspectCommand}}`

Continue this Claim with `{{.ResumeCommand}}`{{if .ReleaseCommand}}, or release the reservation without destroying progress with `{{.ReleaseCommand}}`{{end}}.

