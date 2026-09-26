# Contract-Grounded Testing

Testing establishes that delivered behavior and architecture satisfy their accepted contract. Choose the construction order, test organization, and verification boundaries that make that evidence credible; no universal test-first chronology or scenario-to-test cardinality is required.

When exploring a codebase, read `CONTEXT.md` (if it exists) so test names and interface vocabulary match the project's domain language, and respect applicable ADRs and repository standards.

## Verify observable behavior

Test through an interface that exposes the promised consequence without reaching through it into incidental implementation details. A suitable existing verification boundary is valid when it can observe the obligation and distinguish a plausible violation; do not add another test layer or redesign the architecture merely to create a preferred seam.

A substantial internal module may have its own interface and tests. The question is whether the seam represents behavior callers rely on, not whether it is the topmost interface.

A strong check:

- observes an accepted behavior or failure mode;
- would fail for a plausible implementation that violates it;
- keeps unrelated setup, imports, and infrastructure from masquerading as behavioral evidence;
- remains stable when implementation details change without changing behavior.

See `skl skill --resource tests.md testing` for examples and test-set assessment, and `skl skill --resource mocking.md testing` for boundary substitutes and controlled alternatives.

## Ground expected outcomes independently

Every expected result needs an independent expectation grounded in an accepted rule, a trusted example or reference, or a justified property. Do not derive the expected value by repeating the production algorithm, copying the template under test, or asserting only that execution occurred.

When exact examples are unavailable, state the property and why it follows from the contract. Material uncertainty about the promised result is a decision gap, not a reason to weaken the assertion.

## Establish regression sensitivity

For a bug fix, provide evidence that the regression check detects the reported wrong behavior and passes with the fix. The check may be authored before or after the fix; early reproduction is encouraged because it sharpens diagnosis, but writing order is not an acceptance criterion.

A failure caused only by unrelated setup, import, compilation, or execution errors does not establish sensitivity to the regression.

If the original failure cannot be reproduced reliably or safely, a faithful isolated reproduction, captured-trace replay, or controlled fault injection may establish protection when it preserves the relevant trigger and observable failure. Record the material limitations of that evidence. If material uncertainty remains without credible protection, seek a human decision rather than claim verification or silently carry the gap as debt.

## Assess the changed test set together

Required behavioral and failure-mode protection matters more than test inventory. Use `skl skill --resource tests.md testing` for the detailed retention, consolidation, and removal guidance; unrelated repository-wide test pruning remains outside the current change unless explicitly accepted.

## Report evidence honestly

Connect the obligations to concrete tests, commands, or appropriate inspection evidence, including results and material limitations. Many obligations may share evidence and one obligation may need several checks. Prose assurance alone is insufficient for ordinary executable behavior, and merely listing a gap does not make it acceptable.
