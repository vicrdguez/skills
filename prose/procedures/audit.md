# Audit

{{template "craft/audit.md" .}}## Pin the comparison

Use the fixed point the user gave: a commit, branch, tag or merge-base. When none was given, ask for one. Resolve it with `git rev-parse <fixed-point>` before going further.{{template "audit-diff" .}}

## Find the Contract

When the branch belongs to a ledger Work Item, read its Contract with `skl ledger show --item <proposal>/<slice>`. Otherwise use the issue or path the user supplies. When there is no Contract, skip the Contracts axis and say so in the report.

## Run the gate once

Run the project's full test, typecheck and lint suite once and record the commands, head and results.

Check: you hold the diff command, the commit list, the gate result and the Contract, or the note that there is none.

{{template "audit-dispatch"}}
