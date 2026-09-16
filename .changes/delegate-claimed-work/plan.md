# Delegate Claimed Work Plan

## Decisions And Prerequisites

Follow [ADR 0005](../../docs/adr/0005-verify-contracts-instead-of-prescribing-test-order.md) for optional delegation and owner accountability, and [ADR 0001](../../docs/adr/0001-embed-skill-definitions-behind-harness-stubs.md) for the post-#44 rendering and capability boundaries. Architectural decisions, not incidental implementation sketches, are binding.

- Native Dependency: `verify-contract-conformance` must be Merged.
- External blockers: #48 `render-deferred-resources`, #49 `render-implementation-executions`, and #50 `render-watchdog-executions` must be Merged, not merely Ready for Merge. These references do not assert native external Dependency edges.
- During implementation, use ordinary Git preparation to bring the landed prerequisites into the existing work branch while preserving its progress and artifact endpoints. Do not implement against the pre-#44 sources, recreate their machinery, or introduce a startup target snapshot/pin. Startup remains metadata-only; this preparation does not replace the separate late pre-Audit integration policy.

## Responsibilities

- Modify the landed Skill Modules composing complete Implement Execution Skills for initial work, resumed work, and Rework, plus relevant inspection continuations and included testing guidance where delegation-specific wording is needed. Use #48's shared renderer and #49's typed procedure contexts; choose concrete private paths and field names from the landed code, not from this proposal.
- Reuse the adapter-established capability seam and supported instruction recipes. Specialize known cases and retain bounded runtime discovery only for genuinely unknown capability. Shared policy remains harness-independent; this slice adds no capability registry, scheduler, or new Workflow role.
- Update the surviving single-item `agents/implement-runner.md` restriction that formerly allowed helpers only for Audit. Preserve its post-#44 one-item lifecycle and Markdown reporting; do not restore scheduler continuation, JSON forwarding, or disabled Pi loops.
- Keep the Workflow Engine responsible for identity, Claims, eligibility, and transitions. The owner alone coordinates helper assignments, resolves write conflicts, integrates results, and submits. Helper execution is not another Workflow lane or Claim.
- Reuse installed-stub and owned-adapter distribution so the changed guidance reaches supported installations without overwriting user-owned files or duplicating definitions.

## Public Interfaces

Existing Implement startup/resume operations and relevant inspection continuations remain the delivery interfaces. Preserve selected Work Item identity, remote, established artifact endpoints and any explicit endpoint flags through their concrete commands. Do not restore standalone Implement/Watchdog definition retrieval or add a delegation CLI mode.

Use the existing typed capability inputs to choose supported recipes, never the harness name as proof of tools. A helper brief is agent-authored context, not a new engine-parsed schema: it conveys assignment bounds, working location, authoritative contract inputs, required observations, and write responsibility, with implementation preferences explicitly subordinate to those obligations. Helpers return contributions and evidence to the owner rather than a final Workflow result.

Where affected, preserve #48's owner-relative named resources, repeated `--input name=value`, `--describe-inputs`, input validation, inert supplied data, and deferred retrieval timing. Keep included definitions once-only. No new public resource contract is required solely to carry a helper brief.

## Sibling Ownership

`verify-contract-conformance` owns the testing skill transition, verification policy, acceptance/finding criteria, grouped evidence, and published-contract precedence. Consume that policy without redesigning it or weakening Audit through optional-delegation fallback. `materialize-approved-contracts` owns Explore/Propose fidelity and artifact production. `integrate-before-audit` owns late target observation, integration, and evidence freshness. This slice neither rewrites their contracts nor migrates already-published ledgers.

## Verification Strategy

- Extend existing rendering and CLI checks to inspect representative complete Implement Execution Skills and relevant continuations across initial work, resume, and Rework. Exercise supported, unsupported, and unknown capability inputs at the existing seams, including cases where harness identity alone would suggest the wrong recipe.
- Inspect delivered guidance for bounded contractual inputs, unchanged identity/authority, non-conflicting writes, owner integration, and final-state verification. Check that optional serial fallback does not remove or downgrade the applicable mandatory Audit mechanisms. Use grouped coverage rather than a substring assertion per policy sentence or one test per scenario.
- Reuse foundation checks for once-only inclusion, resource visibility and retrieval, deferred timing, concrete commands, and equivalent explicit JSON where touched. Avoid duplicating the entire #48/#49/#50 test matrix.
- Extend existing installation checks for the surviving runner and affected owned guidance, including upgrades, preservation of user-owned content, and continued disabling of legacy loops.
- Run the repository's normal project checks and Full Gate during implementation. Record concrete evidence and material limitations through the existing Verification section; no model evaluation, live agent experiment, or new test infrastructure is required. These checks establish instruction delivery and interface behavior, not actual agent compliance.
