# Testing

Tests show that the change keeps its Contract. This is the reference that makes those tests worth keeping: what a good test is, where tests go, the anti-patterns, and what counts as evidence.

When exploring the codebase, read `CONTEXT.md` (if it exists) so test names and interface vocabulary match the project's domain language, and respect ADRs in the area you're touching.

## What a good test is

Tests verify behavior through public interfaces, not implementation details. Code can change entirely; tests shouldn't. A good test reads like a specification — "user can checkout with valid cart" tells you exactly what capability exists — and survives refactors because it doesn't care about internal structure.

Write tests before or after the code, whichever fits the change, and refactor whenever it helps.

See `skl skill --resource tests.md testing` for examples and `skl skill --resource mocking.md testing` for mocking guidelines.

## Seams — where tests go

A **seam** is the public boundary you test at: the interface where you observe behavior without reaching inside (full vocabulary in `design`). Tests live at seams.

Test at the seams the Contract pins. Where the Contract leaves the seam to you, prefer an existing one, and use the highest seam that exposes the promised consequence.

## Anti-patterns

- **Implementation-coupled** — mocks internal collaborators, tests private methods, or verifies through a side channel (querying the database instead of using the interface). The tell: the test breaks when you refactor but behavior hasn't changed.
- **Tautological** — the assertion recomputes the expected value the way the code does (`expect(add(a, b)).toBe(a + b)`, a snapshot derived by hand the same way, a constant asserted equal to itself). The tell: it passes by construction and can never disagree with the code. Take expected values from an independent source: an accepted rule, a trusted example, or a justified property.

## Red on the bug

A bug fix's check goes **red on the bug**: it fails on the reported wrong behavior and passes with the fix, in either writing order. A failure from setup, an import or compilation is not red on the bug.

When the original failure can't be reproduced reliably or safely, a faithful isolated reproduction, a captured-trace replay or controlled fault injection can stand in, as long as it keeps the trigger and the observable failure. State in your report what stays unverified. When a material failure has no credible evidence, take it to a human decision.

## Evidence

Map obligations to checks many-to-many: one check can cover several obligations, and one obligation can need several checks. Judge the changed tests as a set. Reuse, strengthen, consolidate or remove checks while every required behavior and failure mode stays protected.
