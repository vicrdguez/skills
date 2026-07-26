# {change title} Acceptance

<!--
Written by `watchdog` on a pass, as a new artifact in `.changes/<slug>/`. This is
the acceptance gap: everything the human must validate manually because an agent
cannot run or observe it. Keep every item concrete and checkable in minutes.
Delete this comment in the real file.
-->

## Verification the agent could not run
<!--
Steps that need a human or a real environment: visual checks, third-party
dashboards/sandboxes, credentials the agent lacks, production-like data.
-->

- [ ] {step}

## Human acceptance checklist
<!--
One line per intent.md "Definition of Done" item, phrased as something the human
can demo directly.
-->

- [ ] {check}

------
## Example

```md
# Add order cancellation — Acceptance

## Verification the agent could not run
- [ ] Cancel an order and confirm the refund appears as initiated in the Stripe test dashboard

## Human acceptance checklist
- [ ] Cancel an unshipped order from the UI; it shows "cancelled" and a refund is initiated
- [ ] Attempt to cancel a shipped order; the action is rejected and the order is unchanged
```
