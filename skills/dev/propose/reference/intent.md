# {title}

<!--
Capture intent so precisely that an implementer needs no further clarification.
Keep each line tight. Delete this comment in the real file.

Frozen once published: the only later edit is ticking a Done box `[ ]` → `[x]`.
-->

## Why
<!--
The problem and motivation from the user's perspective. 
-->

## What
<!--
What this change introduces, the solution. Detailed, but to the point.  Focus on clarity and avoid redundant explanations.
-->

## Scope
<!--
LONG Bullet list of what this change DOES.
-->


## Out of Scope
<!--
Bullet list of the what this change deliberately DOES NOT do.
-->

## Definition of Done
<!--
Observable, testable acceptance criteria. The conditions that determine if this change is complete or not. EVERY Gherkin scenario in `behavior.md` should trace back to a line here.

Use markdown task-bullets: `- [ ]`
-->

## Manual verification
<!--
What a human must check by hand because no agent can run or observe it: visual checks, third-party dashboards or sandboxes, credentials the agent lacks, production-like data. Frozen like everything else: the reviewer copies it into the PR body, nobody authors it after the fact. "None" is a valid answer.

Use markdown task-bullets `- [ ]`
-->

------
## Example

```md
# Add order cancellation

## Why
Customers can't cancel an order after placing it, which drives avoidable support load.

## What
Let a customer cancel an order before it ships; refund the full amount automatically.

## Scope
- Customer-initiated cancellation of unshipped orders
- Automatic full refund on cancellation

## Out of Scope
- Partial cancellation
- Admin-initiated cancellation
- Post-shipment returns

## Definition of Done
- [ ] A customer can cancel an order while it is unshipped, and the order becomes "cancelled".
- [ ] Cancelling an unshipped order initiates a full refund.
- [ ] Cancelling a shipped order is rejected and leaves the order unchanged.

## Manual verification
- [ ] Cancel an order and confirm the refund shows as initiated in the Stripe test dashboard
```
