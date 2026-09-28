# {Change title} Behavior

<!--
For consequential stateful behavior, first ask what remains true across
transitions, interruption and retry; state the governing rule for that class
of situations, beyond a single demonstrated sequence. Name a rule wherever
plausible interpretations have different consequences.

Select scenarios that tell those interpretations apart through the slice's
observable interface. State the precondition with Given, the action with When,
and the outcome with Then; add And or But for relevant unchanged or prohibited
effects, such as state or side effects after failure, cancellation or retry.
Scenarios bind behavior; they are neither a phase-permutation inventory nor a
test plan. Use only the cases that clarify the rule.

Delete this comment in the real file.
-->

## B1: {scoped, consequential rule}

{State what holds throughout the rule's scope, including accepted limitations and relevant effects it rules out.}

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
