{{.Frontmatter}}

<!-- skl-owned: {{.Protocol}} -->

{{if .Loop}}Run `{{.Command}}` with each nonempty value below as one argument for its flag. Quote values for the shell without changing them; omit empty slots.{{if .AutoSlot}} For --auto, pass the bare flag when its value is true; omit it when false or empty.{{end}}
{{.Arguments}}Follow each Outcome Instruction until one tells you to stop.{{else}}Run `{{.Command}}` and follow the Execution Skill it returns.{{end}}
