{{template "audit-intro" .}}Work Item: `{{.Item}}`; source worktree: `{{.Worktree}}`.
The exact accepted Contract and consumed reports were supplied as labeled data
in this execution. Use those references, not public descriptions or source
markers. Refresh the selected Claim and prepared source facts with
`{{.InspectCommand}}` before Audit, substituting the exact newly integrated
SHA for its target argument rather than retaining an older preparation target.

After the required late target integration, resolve `git merge-base <recorded-integrated-target-sha> HEAD`.
That normal PR-base merge-base is the default fixed point. Use a supplied fixed
point when the invocation establishes one.{{template "audit-comparison" .}}{{if eq .Capability "claude-agents"}}Use the established parallel Agent mechanism: one message with two fresh
`general-purpose` Agent calls, one per axis.
{{else if eq .Capability "pi-subagents"}}Use the established Pi asynchronous subagent mechanism to dispatch both axes in
fresh contexts in parallel; keep their write responsibilities read-only.
{{else if eq .Capability "sequential"}}Use the established sequential fallback, Standards first and Contracts second,
keeping each axis's findings separate.
{{else}}Make one bounded check for a supported helper mechanism. When available,
dispatch both axes as parallel fresh subagents; otherwise run Standards then
Contracts sequentially. A harness name alone establishes no capability.
{{end}}
{{template "audit-review" .}}