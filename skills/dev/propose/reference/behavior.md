# {Change title} Behavior

<!--
Write authoritative, scoped named rules where consequential ambiguity needs a
decision. A rule governs its class of situations beyond the scenarios beneath it.
Use binding scenarios only to discriminate plausible interpretations, not to
inventory every simple case or prescribe one test or task per scenario.

Use Gherkin notation as readable prose; there are no .feature files or Cucumber
runtime. Given / When / Then / And / But help expose preconditions, actions, and
observable outcomes. Where relevant, consider state and side effects, prohibited
or unchanged effects, and failure, cancellation, retry, or partial completion.
These are precision aids, not compulsory headings.

Each rule and scenario has a readable, uniquely referenceable descriptive heading;
IDs are not required. Each scenario must be observable through the chosen module
interface rather than incidental implementation state. An unambiguously implied
case needs no extra scenario. Resolve any consequential behavior not settled by
the approved source before publication rather than inventing an obligation.

Frozen at the `[baseline] <slice-slug>` commit. Artifact Completion may only tick
existing non-manual boxes; review discoveries belong in findings or a new Proposal.
Delete this comment in the real file.
-->

## Rule: {scoped, consequential rule}

{State the authoritative rule, including its scope and material prohibited effects.}

### Scenario: {case that distinguishes plausible interpretations}
- Given {the relevant precondition}
- When {the action through an observable interface}
- Then {the contractually observable outcome}
- And {a further observable or unchanged outcome, when relevant}

---

### Example

```md
# Order Cancellation Behavior

## Rule: Cancellation is available only before shipment

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
