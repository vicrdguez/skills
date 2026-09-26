<!-- dev-pipeline:start -->
## Workflow

Use `skl` for Workflow operations. Only a human merges.

## Simplicity

Choose the least complexity that satisfies the accepted scope, repository standards and required verification. Preserve invariants, error handling, security and accessibility. When a simpler approach would change the agreed behavior, raise the trade-off before reducing scope.

- **Understand and reuse.** Trace the affected behavior and its callers first, and fix causes at the boundary responsible for them. Before adding a mechanism, look for existing code, standard-library and platform features, and installed dependencies, and check that a candidate meets the required semantics and failure modes.
- **Justify structure.** Add abstractions, configuration, dependencies and test infrastructure for concrete current needs. Prefer the approach that leaves less for callers and maintainers to understand over the one with the fewest lines. Apply the deletion test while preserving behavior: if removing an abstraction removes complexity, simplify it; if it spreads responsibilities into callers, keep those responsibilities together. A boundary can earn its keep through encapsulation or testability, even with one production implementation.
- **Keep tests direct.** Test observable behavior at the agreed seams with the project's existing test tools, direct setup and expected results independent of the implementation. Keep each test that protects against a distinct meaningful regression. Simplify repeated setup and incidental coupling to the implementation. Remove a test only when its behavioral and failure-mode protection stays covered at the appropriate interface.
- **Review concrete alternatives.** Judge a simplification by the burden it removes: name the simpler alternative, the burden it removes, and why behavior and verification stay intact. A code-smell name alone does not justify it. Keep cleanup within the change's scope, and raise unrelated opportunities separately.
<!-- dev-pipeline:end -->

## Agent-visible prose

Agent-visible prose is everything a worker reads: skills, Skill Resources, stubs and Outcome Instructions.

- Write to this worker, about this invocation.
- If the CLI refuses it, don't forbid it.
- Don't name what the agent can't reach.
- One place per meaning across the bundle.
- Explain a reason only when it steers judgment.
- End every step on a clear check.
- writing-for-agents applies to `prose/**`.
- ADR and Contract wording is never transcribed into prose.
- Agent-visible prose is verified by its golden in `testdata/prose/`; regenerate the goldens after a prose change. Tests of rendered prose check structure: bound values, the resource manifest, emitted commands that run, and branch markers the golden journey does not reach. When a prose change breaks a wording assertion, delete that assertion.

Changes to `prose/**`, stubs or Outcome Instructions follow `docs/agent-prose.md`.
