# Triage Rules for the Decision Inbox

Reference for resolving current Needs Human requests. Read `skl decision inbox` for current request facts and bound commands before applying these rules. Reading this resource records nothing.

## Read ledger-wide, filter explicitly

The inbox is every Project's current Needs Human requests. It is resolved from the configured Workflow Ledger, so it works from a non-repository directory and from inside any source checkout without discovering or registering workspaces. `--project <name>` is the only narrowing; the working directory never narrows scope. A configured ledger that cannot be resolved or read is an access problem, reported as unavailable and never as an empty inbox. An empty inbox creates no work.

{{template "decision-triage" .}}

{{template "decision-authorization" .}}

{{template "decision-recording" .}}

{{template "decision-renewal" .}}
