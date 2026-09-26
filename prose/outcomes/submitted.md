Status: {{.Status}}

{{if eq .Status "awaiting_review"}}Work Item `{{.Item}}` is submitted for review.{{else if eq .Status "ready_for_merge"}}Work Item `{{.Item}}` passed review and is ready for a human to merge.{{else}}Work Item `{{.Item}}` is returned for rework.{{end}} {{template "outcome-handoff" .}}
You are done: report the handoff to the user, and stop.
