{{define "audit-diff"}} The diff command is `git diff <fixed-point>...HEAD`, and the commit list is `git log <fixed-point>..HEAD --oneline`. An empty diff is legal: judge the complete implementation.{{end}}

{{define "audit-dispatch"}}## Dispatch the reviewers

Give each reviewer its brief and everything the check above names, the Contract included. Both reviewers use the recorded gate result. The Standards reviewer retrieves the smell baseline with `skl skill --resource smells.md audit`; both reviewers retrieve the acceptance criteria with `skl skill --resource acceptance.md audit`. Reviewers read and report.

Run the axes at once as fresh-context subagents{{with .}}{{template "subagent-choice" .}}{{end}} when you can spawn them. Otherwise run them yourself in sequence, Standards first, finishing its report before starting Contracts.

Aggregate the reports as above. Check: the report carries every axis that ran, its tagged `F<n>` findings and the gate result.
{{end}}
