# Serve principles at two tiers

Judgment guidance that holds everywhere is written once by skl; a repository's own standing orders have no place the Workflow reads. Rename the craft tier to principles, with the deep-module craft named `modules`, and add a repo-owned tier at `.skl/principles/<name>.md`, one file per principle with a one-line statement. The engine pastes every statement into every Procedure it renders for that repository and serves the full file as a Skill Resource where a playbook steers judgment with it. This extends ADR 0008's composition to a second, repository-owned source.

## Status

Accepted after the proposal checkpoint of the design-workflow Proposal; implementation is pending.

## Consequences

- A project principle's statement is a constraint, not a trigger, short enough to stand in a standing-orders list; the body explains and gives examples. The engine checks presence and length only.
- Both tiers have the same form, a `statement` in frontmatter and the explanation below, so a human promotes a project principle that proves universal by moving the file into skl's tree. No tooling promotes, generates or edits a principle; a Consumer Repository never reaches skl's source.
- The Forbidden section of a Contract may cite a principle by name; lint resolves the name. Watchdog and the audit step read a cited principle in full before judging the line.
- The former `design` craft becomes the `modules` principle so that `design` can name the design shape; `domain` keeps its name. Content does not change with the rename.
- Reflect, the step that grows the project list from the human's challenges, is deferred; the ledger's annotations, dispositions, reports and verdicts are the dataset it will read.
