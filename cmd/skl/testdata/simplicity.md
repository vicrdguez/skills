## Simplicity

Choose the least complexity that satisfies the accepted scope, repository
standards, and required verification. Preserve relevant invariants, error
handling, security, and accessibility. When a simpler approach would change
the agreed behavior, raise that trade-off rather than silently reducing scope.

### Understand and Reuse

Trace the affected behavior and relevant callers before choosing a solution.
Fix causes at the boundary responsible for them, rather than patching symptoms
in individual callers.

Look for adequate existing code, standard-library features, native platform
capabilities, and installed dependencies before adding a mechanism. Check
that a candidate actually satisfies the required semantics and failure modes;
availability alone does not make it suitable.

### Justify Structure

Introduce abstractions, configuration, dependencies, and test infrastructure
for concrete current needs. Prefer the approach that leaves less for callers
and maintainers to understand, rather than the fewest lines or files.

Apply the deletion test while preserving behavior: if removing an abstraction
eliminates complexity, simplify it; if it spreads responsibilities into callers,
keep those responsibilities together. A boundary can earn its keep through
encapsulation or testability without multiple production implementations.

### Keep Tests Direct

Test observable behavior at the agreed seams, using the project's existing
test tools. Keep setup direct and expected results independent of the
implementation.

Ask what meaningful regression would lose protection if a test disappeared.
Keep distinct regression protection; simplify repeated setup and incidental
implementation coupling. Remove redundant or implementation-coupled tests
only when their behavioral and failure-mode protection remains covered at
the appropriate interface.

### Review Concrete Alternatives

Judge simplifications by the burden they remove, not lines saved.

For a proposed simplification, identify the simpler alternative, the burden
it removes, and why required behavior and verification remain intact.
A named code smell is not sufficient justification.

Keep cleanup within the change's scope; raise unrelated opportunities
separately.
