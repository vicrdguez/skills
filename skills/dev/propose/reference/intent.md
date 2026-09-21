# {title}

<!--
State the desired result compactly. Keep detailed rules and scenarios in
behavior.md rather than repeating them here. Delete this comment in the real file.

Frozen when `skl ledger accept` records the proposal. Accepted files stay
read-only: progress and completion evidence live in phase reports, never in
completion ticks or edits here.
-->

## Why
<!-- The problem and motivation from the user's perspective. -->

## What
<!-- The result this change introduces, without restating behavior.md. -->

## Scope
<!-- What this change includes. -->

## Out of Scope
<!-- What this change deliberately excludes. -->

## Definition of Done
<!--
Observable completion outcomes. Give each independently tracked outcome its
descriptive local label (B<n> for behavior rules, A<n> for architectural
commitments, warranted T<n>) and map them many-to-many to rules, scenarios,
and checks; do not create one item per scenario or test.

Use Markdown task bullets: `- [ ]`.
-->

## Manual verification
<!--
Human-owned checks an agent cannot run or observe, such as visual checks,
third-party dashboards, unavailable credentials, or production-like data.
Label each check M<n>. Frozen like every other obligation. "None" is a valid
answer.

Use Markdown task bullets `- [ ]` when checks exist.
-->

---
## Example

```md
# Add order cancellation

## Why
Customers cannot cancel an order after placing it, which drives avoidable support load.

## What
Let a customer cancel an order before shipment and receive a full refund.

## Scope
- Customer-initiated cancellation of unshipped orders
- Automatic full refund on cancellation

## Out of Scope
- Partial cancellation
- Admin-initiated cancellation
- Post-shipment returns

## Definition of Done
- [ ] Eligible cancellation is available and leaves the order cancelled.
- [ ] Cancellation refunds the complete order payment.
- [ ] Ineligible cancellation is rejected without changing the order.

## Manual verification
- [ ] Confirm the initiated refund in the Stripe test dashboard.
```
