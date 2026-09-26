# {Change title} Behavior

<!--
Write a named rule wherever a consequential ambiguity needs a decision. A rule
governs its whole class of situations, beyond the scenarios beneath it.

Add a scenario only when it tells plausible interpretations of a rule apart.
An unambiguously implied case needs none. Each scenario:
- is observable through the interface the slice is verified at, never through
  incidental implementation state;
- states its precondition with Given, its action with When, and its observable
  outcome with Then, adding And or But for further or unchanged outcomes;
- considers, where relevant, state and side effects, effects that must not
  happen, and failure, cancellation, retry or partial completion.

Scenarios are readable Gherkin prose, with no .feature files or runner. They
bind behavior, never tests: one scenario is not one test or one task.

Delete this comment in the real file.
-->

## B1: {scoped, consequential rule}

{State the rule, its scope, and the effects it rules out.}

### Scenario: {case that distinguishes plausible interpretations}
- Given {the relevant precondition}
- When {the action through an observable interface}
- Then {the observable outcome}
- And {a further observable or unchanged outcome, when relevant}

---

### Example

```md
# Order Cancellation Behavior

## B1: Cancellation is available only before shipment

A customer may cancel an unshipped order. Cancellation makes the order cancelled
and initiates a full refund. Once shipped, an order rejects cancellation and
remains unchanged.

### Scenario: Cancel an unshipped order
- Given a customer has an unshipped order
- When the customer cancels the order
- Then the order becomes "cancelled"
- And a refund for the full payment is initiated

### Scenario: Reject cancellation after shipment
- Given the order has shipped
- When the customer attempts to cancel the order
- Then cancellation is rejected with reason "already shipped"
- But the order remains "shipped" and no refund is initiated
```
