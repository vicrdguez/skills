{{define "ledger-audit"}}
Review the candidate along two independent axes: **Standards** and **Contracts**.
Audit is implementation-phase judgment, not a Claim, workflow transition, or
independent Watchdog Review. It edits no workflow records.

## Pin the comparison and Contract

{{if .Delivery}}Work Item: `{{.Delivery.Item}}`; source worktree: `{{.Delivery.Worktree}}`.
The exact accepted Contract and consumed reports were supplied as labeled data
in this execution. Use those references, not public descriptions or source
markers. Refresh the selected Claim and prepared source facts with
`{{.Delivery.InspectCommand}}` before Audit, substituting the exact newly integrated
SHA for its target argument rather than retaining an older preparation target.
{{else}}Use the caller's exact Contract references or supplied Contract documents.
For ledger-managed work, retrieve them through `skl ledger show --repo <source-repository> --item <proposal>/<slice>`
or its supplied exact commit/path command. Ask for missing originating context;
report an unavailable Contracts axis explicitly rather than inventing obligations.
{{end}}
{{if .Delivery}}After the required late target integration, resolve `git merge-base <recorded-integrated-target-sha> HEAD`.
That normal PR-base merge-base is the default fixed point. Use a supplied fixed
point when the invocation establishes one.{{else}}Use the caller's specified fixed point (commit, branch, tag, or merge-base),
resolved to its full commit identity. Ask if no comparison was supplied;
standalone Audit does not authorize source fetching or merging.{{end}} For finding-driven Rework, an
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
   project quality skills. Retrieve `skl skill --resource reference/smells.md audit`
   and `skl skill --resource reference/acceptance.md audit`. Supply both review
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

{{if .Delivery}}{{if eq .Delivery.Capability "claude-agents"}}Use the established parallel Agent mechanism: one message with two fresh
`general-purpose` Agent calls, one per axis.
{{else if eq .Delivery.Capability "pi-subagents"}}Use the established Pi asynchronous subagent mechanism to dispatch both axes in
fresh contexts in parallel; keep their write responsibilities read-only.
{{else if eq .Delivery.Capability "sequential"}}Use the established sequential fallback, Standards first and Contracts second,
keeping each axis's findings separate.
{{else}}Make one bounded check for a supported helper mechanism. When available,
dispatch both axes as parallel fresh subagents; otherwise run Standards then
Contracts sequentially. A harness name alone establishes no capability.
{{end}}{{else}}Use supported parallel fresh subagents when available; otherwise perform
Standards then Contracts sequentially. Keep the two judgments independent.
{{end}}
Neither reviewer writes source, changes a Claim, performs a handoff, or replaces
Watchdog. Each receives the fixed diff command, commit list, exact Contract and
consumed report references, standards sources, shared criteria, and recorded gate
and inspection facts. Frozen obligations outrank required tooling and CI, then
project standards and quality skills, then language/framework correctness,
security and accessibility, then the generic smell baseline.

### Standards brief

Read the diff and relevant callers. Report every documented-standard violation
and material local-quality concern in changed code or integration effects. Cite
the standard and file/hunk. For a simplification, name the concrete simpler
alternative, the burden it removes, and why required behavior and verification
remain intact. A smell name alone is insufficient. Tag each finding `HARD` or
`JUDGEMENT`; generic smells are always `JUDGEMENT`, while an explicit mandatory
rule or concrete hazard may justify `HARD`. Skip tooling-enforced observations.
Exclude unrelated target additions inherited unchanged. Keep the report under
500 words, compressing rather than omitting findings.

### Contracts brief

Judge the complete final implementation against every accepted behavior,
scenario, Definition of Done, and architecture commitment. Check the current
completion-and-evidence table, not merely the latest delta. Accept grouped
many-to-many evidence, construction freedom, and test reuse or consolidation
when required behavior and failure-mode protection remain. Check removed or
weakened assertions for lost protection without demanding per-test bookkeeping.

Report missing, partial, contradicted, or out-of-scope behavior; frozen
architectural violations; selected-input integrity failures; and specific
coverage/evidence gaps as `HARD`. Quote the obligation and identify the plausible
violation existing evidence cannot distinguish. A red Full Gate is `HARD`.
A plan divergence is `JUDGEMENT` unless it breaks an accepted obligation. Review
integration effects while excluding unrelated inherited target additions. Prior
findings and recorded human directions are evidence within the frozen Contract,
not amendments or new requirements. Keep the report under 500 words, compressing
rather than omitting findings.

## Aggregate without reranking

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
