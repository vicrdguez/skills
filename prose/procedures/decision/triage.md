# Decision Rules

These are the rules `skl decision inbox` renders with its requests. Take request facts and apply commands from the inbox.

The inbox spans every Project in the configured ledger, wherever you run it. Narrow it only when the user names a Project, with `--project <name>`.

{{template "decision-triage" .}}

{{template "decision-authorization" .}}

{{template "decision-recording" .}}

{{template "decision-renewal" .}}
