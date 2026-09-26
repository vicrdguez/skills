{{define "watchdog-facts"}}Repository: {{.Repository}} on remote `{{.Remote}}`
Work Item: {{.Item}}
Branch: `{{.Branch}}`
Worktree: `{{.Worktree}}`
Result Documents: `{{.ResultDirectory}}`
Claim: `{{.Claim}}`
{{if .RequiredHead}}Reviewed head: `{{.RequiredHead}}`
{{end}}{{if .RecordedTarget}}Recorded Integration Target: `{{.RecordedTarget}}`
{{end}}{{if .PreviousReviewed}}Previous reviewed revision: `{{.PreviousReviewed}}`
{{end}}Review round: {{.ReviewNumber}}
{{if .ReviewScope}}Review scope: `{{.ReviewScope}}`
{{end}}{{if .SourceHead}}Prepared source head: `{{.SourceHead}}`
{{end}}{{if .SourceTarget}}Prepared integrated target: `{{.SourceTarget}}`
{{end}}{{if .FetchStatus}}Source fetch: {{.FetchStatus}}
{{end}}{{end}}

{{define "watchdog-incremental"}}`git diff {{.PreviousReviewed}}...HEAD` as a repeat review{{end}}

{{define "watchdog-full"}}the complete change, `git diff {{.RecordedTarget}}...HEAD`, against every Contract item{{end}}
