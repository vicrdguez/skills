---
name: implement
description: Implement a single claimed change against its accepted behavioral and architectural contract.
disable-model-invocation: true
---

{{if and .Implementation .Implementation.Inspection}}{{template "inspection" .Implementation}}{{else if .Implementation}}Implement Work Item {{.Implementation.WorkItemReference}} in `{{.Implementation.Worktree}}`. This Execution Skill already represents the engine's completed Work Start operation; do not select or claim another Work Item.

## Applicable procedure

{{if eq .Implementation.Procedure "rework"}}This invocation follows finding-driven Rework: preserve the existing Submission{{if .Implementation.Submission}} #{{.Implementation.Submission}}{{end}}, keep `.changes/{{.Implementation.Branch}}/` retired, and resolve the supplied Watchdog findings against the current PR comparison. Do not recreate or revise the Implementation Ledger or create another Artifact Completion. Map every finding to its resolution commit and supporting evidence in the Result Document, and update the `## Audit ledger` in the Submission body to the current head. Resolve every `BLOCK`; materialize code-local `NOTE` findings as `DEBT(#{{.Implementation.Submission}}/W<n>)` comments. This obligation holds even when the supplied feedback is empty or still pending: retrieve every required stream before concluding there are no findings.
{{else if eq .Implementation.Procedure "resumed"}}This invocation resumes existing work: inspect the branch, the preserved files, the applicable artifacts, and the visible feedback; do not restart completed work. Continue the remaining accepted scenarios. Preserve the attached draft Submission{{if .Implementation.Submission}} #{{.Implementation.Submission}}{{end}} rather than replacing it.
{{else}}This invocation starts the accepted change: read the accepted artifacts at their Artifact Baseline before changing code, then implement every accepted scenario. No implementation progress is presumed; branch names, comments, and a draft attachment do not change this procedure.
{{end}}
## Established work

Repository: {{.Implementation.Repository}} on the selected remote `{{.Implementation.Remote}}`
Work Item: {{.Implementation.WorkItemReference}}
Branch: `{{.Implementation.Branch}}`
Worktree: `{{.Implementation.Worktree}}`
Private result location: `{{.Implementation.ResultDirectory}}`, which this invocation already created

Prepare: `{{.Implementation.FetchCommand}}` then `{{.Implementation.WorktreeCommand}}`; safely reuse a clean existing worktree instead of recreating it, and preserve dirty files, the index, and existing branch progress. Never reset, stash, rebase, force-push, or merge the target merely to make preparation or presentation convenient.
Push: `{{.Implementation.PushCommand}}`
Inspect: `{{.Implementation.InspectCommand}}` resolves the Artifact Baseline, Artifact Completion, and current ledger progress from fetched history.
Resume: `{{.Implementation.ResumeCommand}}`; this is the only command for continuing the same Claim.
{{if or .Implementation.SuppliedArtifactBaseline .Implementation.SuppliedArtifactCompletion}}
Supplied Artifact Baseline `{{.Implementation.SuppliedArtifactBaseline}}` and Artifact Completion `{{.Implementation.SuppliedArtifactCompletion}}` are known pointers, not validated contents or ancestry. Their exact override flags are already present on every applicable command above and below; do not add an override for an endpoint resolved from history. Inspection validates them.
{{end}}
The dedicated worktree, selected project commits, and artifact objects may still be unavailable locally. Run Prepare, then Inspect. Do not infer their availability, contents, ancestry, or ledger progress from the branch name, Submission, or lifecycle label.

{{template "evidence" .Implementation}}

## Execute the change

Work only in `{{.Implementation.Worktree}}`. Every new or updated Submission targets `main`; integration with `main`, conflict resolution, and merge belong to the human Merge Authority after review. Never rewrite history: the Artifact Baseline{{if .Implementation.ArtifactBaseline}} `{{.Implementation.ArtifactBaseline}}`{{end}}, Artifact Completion{{if .Implementation.ArtifactCompletion}} `{{.Implementation.ArtifactCompletion}}`{{end}}, and prior Reviewed heads must remain reachable.

{{if eq .Implementation.Procedure "rework"}}1. Run Inspect before editing. It must confirm the retired ledger and resolved historical endpoints. If it reports a violation or any other progress, repair or stop according to that continuation; never recreate the ledger.
2. Read the accepted `intent.md`, `behavior.md`, `plan.md`, and completed `tasks.md` from the historical endpoint commands returned by inspection. Treat the supplied Watchdog summary and inline findings as evidence to resolve, not as new frozen requirements.
3. Resolve every active finding against the complete accepted contract. Organize implementation, verification, and in-scope refactoring as the work requires; preserve each finding identity, existing behavioral and failure-mode protection, and untouched scope. Run focused checks that distinguish each claimed resolution from the reported failure.
4. Select the latest applicable supplied review whose verdict caused the current Rework and use that review's `Commit` as the fixed point. Stop rather than guess when the applicable reviewed commit is missing or ambiguous.
5. Invoke the bundled Audit exactly once over `<reviewed-commit>...HEAD`. Apply every Audit `HARD` finding. For each `JUDGEMENT`, fix it, decline it with a reason, or carry it as debt. If a disposition changes code, run its affected checks and a final Full Gate before handoff. Do not invoke Audit again in this execution.
6. Run Inspect again after all edits. It must still report the same valid retired ledger and no violations.
{{else}}1. Run Inspect before editing. Artifact progress is genuinely unknown until that read-only continuation. Follow only the returned progress-specific instructions: continue a provisional ledger, reuse an existing Completion, and keep a retired ledger absent. Never duplicate Completion or recreate retired artifacts.
2. Read the accepted `.changes/{{.Implementation.Branch}}/` artifacts at the resolved Artifact Baseline. Deliver the complete accepted behavior and architecture, not merely enough behavior to satisfy the checks written so far. Account for every rule and scenario using suitable existing verification boundaries, grouped or reused tests, and appropriate inspection evidence. Preserve every explicit frozen obligation. For resumed work, first establish what remains and preserve completed progress.
3. Choose the construction order that best fits the change. Add or strengthen checks where they provide credible protection, and perform in-scope refactoring throughout implementation while preserving required behavior, architecture, and regression coverage. Run typechecking and focused tests regularly. When the complete accepted contract is implemented and focused evidence is green, run Inspect again for current integrity, then invoke the bundled Audit exactly once against the merge-base with `main` and the parent of this change's first commit. Audit runs the Full Gate once before its reviewers and remains the final quality and conformance pass.
4. Apply its findings yourself. Apply every Audit `HARD` finding. For each `JUDGEMENT`, fix it, decline it with a reason, or carry it as debt; record every disposition under `## Audit ledger` in the Result Document. A declined judgement call with a stated reason is a decision, not an omission. Do not rerun Audit after applying its findings.
5. Finalize artifacts only as the current inspection continuation permits. If no Completion exists and all automated work is complete, change only existing non-manual `[ ]` boxes to lowercase `[x]`, commit `[completion] {{.Implementation.Branch}}`, and then remove `.changes/{{.Implementation.Branch}}/` in a later commit. If Completion already exists, reuse it; if the ledger is retired, keep it absent. Run Inspect once more and require no violations before handoff.
{{end}}
Push with `{{.Implementation.PushCommand}}`. A push does not authorize review or merge.

## Result and handoff

Write the opaque Submission body at `{{.Implementation.ResultDirectory}}/submission.md`. Retrieve its instructions only when verification and dispositions are settled:

`skl skill --resource reference/submission.md --input result_directory={{quote .Implementation.ResultDirectory}} --input procedure={{.Implementation.Procedure}} implement`

{{if eq .Implementation.Procedure "rework"}}Include a `Rework: <supplied-reviewed-head>...<current-head>` line and one `W<n> resolved — <commit>; covered by <check>` (or permitted debt) line for every supplied finding. Retain and update the existing Audit ledger rather than replacing the Submission with a new one.
{{else}}Include the implementation summary, grouped contract-aligned verification, Full Gate result, artifact inspection, material limitations, and complete Audit ledger.
{{end}}
Submit only with:

`{{.Implementation.SubmitCommand}}`

A `fix_required` result retains this Claim and prose. Repair the reported invariant and retry the same command. Work is complete only when it reports `awaiting_review`; independent Watchdog review follows, and only a human may merge. Never bless the changes — that is the watchdog's job.

## The scope is already decided

Implement the complete accepted behavior and architecture, honor what the artifacts exclude, and treat sibling behavior they never mention as separate work. Use suitable existing verification boundaries, grouped or reused tests, and in-scope refactoring when they preserve the contract and relevant regression protection. Read callers of shared code you change and fix regressions this change causes. Do not tidy unrelated code. An unspecified detail is delegated only when its alternatives preserve accepted behavior, architecture, and mandatory standards; an unambiguously implied case may be implemented and verified without rewriting the frozen scenario list. A consequential unresolved behavioral or architectural choice requires the human-decision path rather than an assumption that silence grants permission.

Respect the applicable Audit dispositions: Apply its findings yourself. A declined judgement call with a stated reason is a decision, not an omission. During Rework, preserve those recorded dispositions and never invoke Audit more than once in the same execution.

## When only a human can decide

Finish all unblocked work first. Pause only for contradictory or impossible artifacts, a mandatory project/language/security/accessibility conflict, an unavoidable frozen-interface change, a disputed blocker, or the bounce cap. The permitted reasons are `contradictory_artifacts`, `mandatory_rule`, `frozen_interface`, `disputed_blocker`, and `bounce_cap`.

Retrieve the decision template at the decision point with:

`skl skill --resource reference/decision.md --input result_directory={{quote .Implementation.ResultDirectory}} --input preserve=<true|false> implement`

Choose this later value at the decision point, setting `preserve` to `true` when implementation work exists that the draft Submission must preserve and to `false` otherwise. Publish the decision with `{{.Implementation.NeedsHumanCommand}}`; when preserving changes, append `--body` with the established Result Document `{{.Implementation.ResultDirectory}}/submission.md`. A pause never invents Completion, ticks unfinished work, retires a live ledger, approves, or merges.
{{else}}Implement one accepted change through its conventional worktree. Start only with `skl implement next`, or resume an explicit existing Claim with `skl implement resume --item <number>`. Follow the returned Execution Skill: it binds the selected repository, remote, Work Item, branch, worktree, result location, procedure, evidence, and commands.

Implement the complete accepted behavioral and architectural contract using suitable verification boundaries and credible grouped evidence. Choose construction order and perform in-scope refactoring as the work requires while preserving explicit frozen obligations, existing work, and relevant regression protection. Never rewrite branch history, and leave integration and merge to the human Merge Authority. Run focused checks during implementation and the Full Gate through Audit. A normal implementation runs Audit once, records every disposition, ticks only permitted automated boxes, commits Artifact Completion, and must "remove the entire `.changes/<slug>/` ledger in a separate subsequent commit before review" before it pushes and submits its opaque Result Document. Never bless the changes — that is the watchdog's job.

Finding-driven Rework reads the retired Implementation Ledger from its historical endpoints; rework must not recreate or revise it. It resolves the supplied findings, keeps the retired ledger absent, and runs one focused Audit per Implement execution. Only the engine's `awaiting_review` outcome completes implementation. Use Needs Human solely for a permitted decision automation cannot make; a pause does not approve or merge work.
{{end}}
