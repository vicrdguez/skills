{{define "implement-facts"}}Repository: {{.Repository}} on remote `{{.Remote}}`
Work Item: {{.Item}}
Branch: `{{.Branch}}`
Worktree: `{{.Worktree}}`
Result Documents: `{{.ResultDirectory}}`
Claim: `{{.Claim}}`
{{if .RequiredHead}}Required head: `{{.RequiredHead}}`
{{end}}{{if .SourceHead}}Source head: `{{.SourceHead}}`
{{end}}{{if .SourceTarget}}Integrated target: `{{.SourceTarget}}`
{{end}}{{if .RecordedTarget}}Recorded Integration Target: `{{.RecordedTarget}}`
{{end}}{{if .PreviousReviewed}}Previous reviewed revision: `{{.PreviousReviewed}}`
{{end}}{{if .ReviewScope}}Review scope: `{{.ReviewScope}}`
{{end}}{{if .FetchStatus}}Source fetch: {{.FetchStatus}}
{{end}}{{end}}

{{define "subagent-choice"}}{{with .Model}} with `{{.}}`{{end}}{{with .Thinking}} at `{{.}}` thinking{{end}}{{end}}

{{define "implement-judgment"}}Decide an unspecified detail yourself when every option keeps the Contract, and implement a case the Contract clearly implies. When the Contract leaves a consequential choice open, pause as described below.{{end}}
