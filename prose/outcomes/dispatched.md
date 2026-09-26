Status: {{.Status}}

{{template "outcome-previous" .Previous}}Work Item `{{.Item}}` is claimed for {{template "outcome-phase" .Phase}} with Claim `{{.Claim}}`.

Start a fresh subagent and give it this one instruction: run `{{.Worker}}` and follow its output.{{with .WorkerModel}} Run the subagent on model `{{.}}`.{{end}}{{with .WorkerThinking}} Set its thinking level to `{{.}}`.{{end}}

When the subagent returns, run:

`{{.Continue}}`
