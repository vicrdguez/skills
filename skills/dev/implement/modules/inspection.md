{{define "inspection"}}# Implement Inspection Continuation

Repository: {{.Repository}} on the selected remote `{{.Remote}}`
Work Item: {{.WorkItemReference}}
Branch: `{{.Branch}}`
Worktree: `{{.Worktree}}`
Artifact Baseline: `{{.ArtifactBaseline}}`
{{if .ArtifactCompletion}}Artifact Completion: `{{.ArtifactCompletion}}`
{{end}}{{if .SuppliedArtifactBaseline}}Supplied pointers: Artifact Baseline `{{.SuppliedArtifactBaseline}}`{{if .SuppliedArtifactCompletion}} and Artifact Completion `{{.SuppliedArtifactCompletion}}`{{end}}. They are known identities, not validated contents or ancestry, and they stay on every command below without an invented override.
{{end}}
This is a read-only continuation of a single Work Item. It selects no other work and acquires no Claim: keep the Claim exactly as it is. Nothing below authorizes successful completion by itself.

{{if .Inspection.Violations}}## Reported violations

Inspection reported {{len .Inspection.Violations}} violation(s). Repair or stop before doing anything else; none of them authorizes completion.

{{range .Inspection.Violations}}- {{.}}
{{end}}
Repair the reported invariant at its responsible boundary and then run `{{.InspectCommand}}` again to observe the current evidence. Do not duplicate Artifact Completion, recreate a retired ledger, release the Claim, or restart selection. A repeated inspection re-reads the worktree: it never replays a cached success.

{{else}}## Observed progress

{{if eq .Inspection.Progress "baseline-only"}}Only the Artifact Baseline is resolved and no completed work is recorded. Implement the accepted scenarios at their pinned seams and follow red -> green before Audit. Artifact Completion and ledger retirement are not appropriate yet: create Completion only once every automated box is provably done and the accepted ledger content is otherwise unchanged.
{{else if eq .Inspection.Progress "provisional"}}The ledger is still present with partial completion ticks and preserved work after the Baseline. Inspect the remaining work and continue it without restarting the tasks already marked done; Artifact Completion belongs to the end of the whole implementation, not to this increment.
{{else if eq .Inspection.Progress "completion-present"}}A valid Artifact Completion already exists and the ledger is still present. Reuse that resolved Completion instead of creating a second one, and perform the still-required later step: remove the entire `.changes/{{.Branch}}/` ledger in a commit after Completion, then continue verification and handoff.
{{else if eq .Inspection.Progress "retired"}}{{if eq .Procedure "rework"}}The ledger was already retired during finding-driven Rework. Keep it absent and resolve the supplied findings against the current PR comparison; read the historical accepted artifacts at the resolved endpoints instead of recreating the ledger.
{{else}}The completed ledger is already retired. Keep it absent, reuse the resolved endpoints, and finish the remaining verification and handoff without recreating it.
{{end}}{{end}}
Read the historical accepted artifacts from the resolved endpoints rather than from the working tree:

{{if .ArtifactBaseline}}- `git -C {{quote .Worktree}} show {{quote (printf "%s:.changes/%s/intent.md" .ArtifactBaseline .Branch)}}`
- `git -C {{quote .Worktree}} show {{quote (printf "%s:.changes/%s/behavior.md" .ArtifactBaseline .Branch)}}`
- `git -C {{quote .Worktree}} show {{quote (printf "%s:.changes/%s/plan.md" .ArtifactBaseline .Branch)}}`
- `git -C {{quote .Worktree}} show {{quote (printf "%s:.changes/%s/tasks.md" .ArtifactBaseline .Branch)}}`
{{end}}{{if .ArtifactCompletion}}Read the completed task ledger from Artifact Completion while preserving the Baseline contract above:

- `git -C {{quote .Worktree}} show {{quote (printf "%s:.changes/%s/tasks.md" .ArtifactCompletion .Branch)}}`
{{end}}
{{end}}
Refresh integrity with `{{.InspectCommand}}` before editing, before Audit, and before handoff, wherever the current procedure needs current evidence.
{{end}}
