# Behavioral and Regression Tests

## Check the promised consequence

Test observable behavior through an interface that callers rely on. A good check states what capability or failure mode exists and remains stable when the implementation changes without changing that behavior.

```text
GOOD: "user can checkout with a valid cart"
  arrange: a cart containing a known product
  act:     checkout through the public ordering interface
  assert:  the returned order is confirmed with the independently known total
```

Avoid checks that only exercise code, inspect private collaborators, or assert an internal call sequence.

```text
BAD: "checkout calls paymentService.process once"
```

That check can pass without establishing the promised checkout result and can fail after a behavior-preserving refactor.

## Use an independent expectation

Expected values come from an accepted rule, a trusted worked example or reference, or a justified property.

```go
// Independent worked example: the expected total is a known contract example.
if got := CalculateTotal([]Item{PricedItem(10), PricedItem(5)}); got != 15 {
    t.Fatalf("total = %d, want 15", got)
}
```

Do not recompute the expected value with the production algorithm or copy generated output back as the oracle. Snapshot and golden checks are useful only when their expected content was independently established and is reviewed when behavior intentionally changes.

## Choose an appropriate seam

"Through the interface" is recursive. A substantial internal module can expose a meaningful interface of its own; incidental plumbing does not earn a separately pinned test seam. Prefer an existing boundary that can observe the promised consequence and a plausible violation. Do not add duplicate layers merely because another seam is available.

## Demonstrate regression sensitivity

For a bug fix, show both facts:

1. the check detects the reported wrong behavior (or a faithful controlled reproduction of it); and
2. the check passes with the fix.

Writing the check first is often useful but is not itself evidence. A compile failure, missing fixture, or unrelated exception does not demonstrate sensitivity to the bug.

When the original reproduction is unavailable, preserve the relevant trigger and observable failure with a faithful isolated reproduction, captured-trace replay, or controlled fault injection. State what the substitute does not establish. Material uncertainty without credible protection requires a human decision.

## Assess retained protection

Judge the changed tests as a set:

- reuse an existing check when it already distinguishes the obligation;
- strengthen weak assertions instead of adding a parallel test;
- consolidate overlap when distinct behaviors and failure modes remain protected;
- remove a check only when its meaningful regression protection remains elsewhere;
- scrutinize removed or weakened assertions against plausible violations.

There is no required one-scenario/one-test mapping, per-test ledger, unique-bug quota, universal mutation score, or duplicate suite. Grouped evidence is valid when every accepted obligation remains accounted for.
