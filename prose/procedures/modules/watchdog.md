{{define "watchdog-facts"}}Repository: {{.Repository}} on remote `{{.Remote}}`
Work Item: {{.Item}}
Branch: `{{.Branch}}`
Worktree: `{{.Worktree}}`
Result Documents: `{{.ResultDirectory}}`
Claim: `{{.Claim}}`
{{if .RequiredHead}}Fixed reviewed implementation head: `{{.RequiredHead}}`
{{end}}{{if .RecordedTarget}}Recorded Integration Target: `{{.RecordedTarget}}`
{{end}}{{if .PreviousReviewed}}Previous reviewed revision: `{{.PreviousReviewed}}`
{{end}}Completed reviews: {{.ReviewCount}}; this invocation is review number {{.ReviewNumber}}
{{if .ReviewScope}}Review scope: `{{.ReviewScope}}`
{{end}}{{if .SourceHead}}Prepared source head: `{{.SourceHead}}`
{{end}}{{if .SourceTarget}}Prepared integrated target: `{{.SourceTarget}}`
{{end}}{{if .FetchStatus}}Source fetch: {{.FetchStatus}}
{{end}}{{end}}
