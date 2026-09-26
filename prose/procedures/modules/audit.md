{{define "audit-intro"}}Review the candidate along two independent axes: **Standards** and **Contracts**.
Audit is implementation-phase judgment, not a Claim, workflow transition, or
independent Watchdog Review. It edits no workflow records.

## Pin the comparison and Contract

{{end}}

{{define "audit-comparison"}} For finding-driven Rework, an
available ancestral previously reviewed revision can delimit the delta; an
unavailable or non-ancestral revision requires the full comparison, without
resetting completed-review accounting or findings.

Record and validate the fixed point, candidate head, exact
`git diff <fixed-point>...HEAD` command, and `git log <fixed-point>..HEAD --oneline`.
An empty diff is legal; judge the complete resulting implementation and current
evidence. Review integration and conflict-resolution effects. Unrelated changes
inherited unchanged from the recorded target are not scope creep; concrete
regressions and material risks remain reviewable. Do not chase later target
movement or repeat a completed target integration merely because work resumed.

## Establish shared facts once

1. Identify applicable `AGENTS.md`, standards documents, required tooling, and
   project quality skills. Retrieve `skl skill --resource smells.md audit`
   and `skl skill --resource acceptance.md audit`. Supply both review
   axes with the shared acceptance criteria.
2. Run the project's **Full Gate** once on this integrated candidate: its entire
   test, typecheck, and lint suite. Record the commands, head, results, and any
   limitations before reviewers begin. The reviewers do not rerun this gate.
3. Record the selected-input inspection result and exact frozen Contract
   references. The Contract is read-only; current completion belongs in the
   implementation report's full table of explicit `complete`/`incomplete`
   declarations. Missing entries do not imply completion. Manual Verification
   remains human-owned. Report missing or incompatible inputs as concrete gaps.

## Dispatch independent axes

{{end}}

{{define "audit-review"}}Neither reviewer writes source, changes a Claim, performs a handoff, or replaces
Watchdog. Each receives the fixed diff command, commit list, exact Contract and
consumed report references, standards sources, shared criteria, and recorded gate
and inspection facts. Frozen obligations outrank required tooling and CI, then
project standards and quality skills, then language/framework correctness,
security and accessibility, then the generic smell baseline.

{{template "craft/audit.md" .}}## Aggregate without reranking

Keep the reports under `## Standards` and `## Contracts`, preserving their
`HARD`/`JUDGEMENT` tags. Assign new Audit Findings `F<n>` after the greatest
existing `F<n>`; preserve historical identifiers. Record axis separately from
identity. Do not merge findings across axes or let one axis mask the other.
Report both gate and input-inspection facts, each axis's counts, and its worst
issue if any.

The implementation owner applies every `HARD`, and fixes, declines with a
reason, or carries as debt every `JUDGEMENT`, recording each disposition under
`## Audit ledger`. A functional edit after Audit requires affected checks and a
final Full Gate covering the final functional state, distinguished from the
original audited head. Audit runs exactly once in this Implement execution;
applying findings does not authorize rerunning it. Independent Watchdog follows
the private handoff, and only a human merges.
{{end}}
