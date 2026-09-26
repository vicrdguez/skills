# {title}

<!--
State the desired result compactly; the rules and scenarios live in
behavior.md. Delete this comment in the real file.
-->

## Why
<!-- The problem and motivation from the user's perspective. -->

## What
<!-- The result this change introduces. -->

## Scope
<!-- What this change includes. -->

## Out of Scope
<!-- What this change deliberately excludes. -->

## Definition of Done
<!--
Observable completion outcomes as `- [ ]` bullets, each citing the labels it
covers: B<n> rules, A<n> commitments, warranted T<n> tasks. An outcome may cover
several rules, and a rule several outcomes.
-->

## Manual verification
<!--
Checks only a human can run, such as visual checks, third-party dashboards,
unavailable credentials or production-like data, as `- [ ]` bullets labelled
M<n>. "None" is a valid answer.
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
- [ ] Eligible cancellation is available and leaves the order cancelled (B1).
- [ ] Cancellation refunds the complete order payment (B1).
- [ ] Ineligible cancellation is rejected without changing the order (B1).

## Manual verification
- [ ] M1: Confirm the initiated refund in the Stripe test dashboard.
```
