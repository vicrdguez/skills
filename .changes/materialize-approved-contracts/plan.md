# Materialize Approved Contracts Plan

## Authority and Prerequisites

[ADR 0005](../../docs/adr/0005-verify-contracts-instead-of-prescribing-test-order.md)
owns the accepted policy; [ADR 0001](../../docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md)
owns instruction delivery. This plan pins responsibility separation, not a new
rendering design.

- Native internal Dependency: `verify-contract-conformance` must be Merged.
- External prerequisites: #48 `render-deferred-resources`, #49
  `render-implementation-executions`, and #50 `render-watchdog-executions` must all
  be Merged. The approved publication header `Blocked by: #48, #49, #50` is the
  legacy external dependency projection, not a claim of native external edges.
- This branch starts at `c691fdf`, before those deliveries. Bring their landed
  code/modules into the work branch through ordinary Git preparation as needed
  before implementing this slice. A Merged status alone does not put code here.
  Add no engine startup target pin or extra Workflow transition for preparation.

## Responsibility Boundaries

| Owner | Contractual responsibility |
| --- | --- |
| Explore authored guidance | Produce the final consequential-rule, architecture, and delegated-choice recap for one semantic confirmation; preserve the existing explicit handoff to Propose. |
| Propose authored guidance | Materialize accepted decisions, explicitly supply approved context to one fresh-context fidelity review across the slices before publication, correct transcription errors, and return unresolved or changed semantics to the human. |
| Propose named templates | Keep intent concise; express authoritative rules and discriminating scenarios; require a plan for pinned architecture; make tasks conditional on useful coordination. Preserve existing endpoint integrity and implied-case freedom together. |
| Landed shared distribution | Embed and render authored content, include guaranteed definitions once, and expose named resources through the existing public interface. It does not approve meaning or judge fidelity. |

Use the coherent Skill Modules delivered by the prerequisites and their shared
Go `text/template` renderer, typed procedure contexts, and deferred-resource
interfaces. Explore/Propose reasoning and templates remain context-free. The
approved recap and referenced decisions are reviewer context, not new rendering
fields, tracked artifacts, or engine-parsed evidence. Keep substantive testing and
conformance guidance owned by `verify-contract-conformance`; use its delivered
`testing` references rather than recreating that policy here.

## Public Seams

- Preserve `skl skill explore`, `skl skill propose`, and their explicit
  `--format json` equivalents.
- Preserve `skl skill --resource <name> propose` for `reference/intent.md`,
  `reference/behavior.md`, `reference/plan.md`, and `reference/tasks.md`, without
  proposal-specific inputs. Keep resource lookup owner-relative and deferred;
  internal modules are not public resources.
- Reuse the foundation's typed input validation, inert supplied data, resource
  descriptions, inspection continuations, and metadata-only startup unchanged.
  No new Propose `--input` contract, renderer, publication API, or semantic gate.
  Do not restore standalone Implement/Watchdog definition retrieval, retired
  generic output, or disabled legacy loops.

## Allowed Variation

Private paths, Go types/fields, helper choices, and exact coherent module layout
are implementation choices after inspecting the landed code, not frozen guesses.
Wording and test organization may vary while preserving `behavior.md` and these
responsibilities. Pin only deliberately agreed interfaces; illustrative sketches
do not gain authority. Material changes to semantics, architecture, slice
boundaries, or Dependencies return to the human, not routine reapproval of faithful
artifacts. No `tasks.md` is needed for this slice: this plan and the DoD provide
the useful sequencing and completion criteria.

## Verification Strategy

1. Extend existing public skill/resource retrieval checks, including the landed
   successors of the CLI/catalog checks in `cmd/skl/main_test.go`. Retrieve complete
   Explore/Propose output and all four templates; check advertised commands,
   resource visibility, once-only inclusion including the delivered testing skill,
   context-free retrieval, and Markdown/explicit JSON equivalence.
2. Inspect actual rendered instructions and resources as cohesive documents against
   the scoped rules and scenarios. Check approval versus fidelity, correction versus
   missing decisions, agreed architecture versus sketches, and implied cases versus
   ledger changes. Use focused delivery assertions, not a substring test for every
   policy sentence, synthetic model responses, or one test per scenario.
3. Reuse the prerequisite foundation checks for representative Execution Skills,
   relevant continuations, resource contracts, and distribution invariants instead
   of duplicating their suites. Run normal project checks on the final result.
   Account for the rules, scenarios, and architectural obligations with grouped
   references to checks or rendered-resource inspection in the existing Verification
   evidence, including results and material limitations. These checks verify
   delivered guidance, not human approval behavior or model judgment; no live agent
   evaluation is required.
