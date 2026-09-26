{{define "audit-dispatch"}}## Dispatch the reviewers

Give each reviewer its brief and the facts you recorded. The Standards reviewer retrieves the smell baseline with `skl skill --resource smells.md audit`; both reviewers retrieve the acceptance criteria with `skl skill --resource acceptance.md audit`. Reviewers read and report; they edit nothing.

Run the axes at once as fresh-context subagents when you can spawn them. Otherwise run them yourself in sequence, Standards first, finishing its report before starting Contracts.

Aggregate the reports as above. Check: the report carries every axis that ran, its tagged `F<n>` findings and the gate result.
{{end}}
