# Decision Result Document

Write `{{.ResultDirectory}}/decision.md` and replace this guidance with your own prose. {{template "result-document" .}} The permitted decision `--reason` is carried by the semantic Needs Human command, not parsed from this file.

## Human Decision

State the frozen requirement, the mandatory project rule, or the blocking requirement that stops progress, the current Workflow State, and the completed work. Describe the options, their consequences, and your recommendation.
{{if .Preserve}}
Implementation work exists: push the branch and also write `{{.ResultDirectory}}/submission.md`, the applicable Submission instructions for the procedure this invocation established, then supply that file as `--body` on the Needs Human command so one draft Submission preserves the work.
{{else}}
There is no implementation work to preserve: do not write or supply `submission.md`.
{{end}}
Do not invent Completion, tick unfinished work, or retire an incomplete ledger during this pause.
