Status: {{.Status}}

{{if eq .Status "awaiting_review"}}Work Item `{{.Item}}` is submitted for review.{{else if eq .Status "ready_for_merge"}}Work Item `{{.Item}}` passed review{{if and .Merge (eq .Merge.Status "merged")}} and was squash-merged into its Proposal Branch{{else}} and is ready for a human to merge{{end}}.{{else}}Work Item `{{.Item}}` is returned for rework.{{end}} {{template "outcome-handoff" .}}{{with .Merge}}{{template "outcome-merge" .}}{{end}}
You are done: report the handoff to the user, and stop.
