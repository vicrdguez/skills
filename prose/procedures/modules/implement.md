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

{{define "implement-judgment"}}For additions and extensions, establish agreement for consequences such as authority, coordination, blocking later work, persistence lifetime and recovery ownership. Inspect an inherited mechanism when your change relies on its effects; leave unrelated inherited code alone. Its prior existence, private placement, small size or passing checks do not settle new consequences. Choose details freely within accepted behavior, architecture and mandatory standards, and implement clearly implied cases. If a consequential choice remains open, finish unblocked work and use the human-decision pause below; if resolving it changes frozen obligations, seek renewed proposal. Repair a concrete violation against the accepted outcome, but treat a reviewer's suggested mechanism or disposition as evidence, not permission for a new protocol.{{end}}
