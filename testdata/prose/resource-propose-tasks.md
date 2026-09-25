# Tasks — {change title}

<!--
This optional coordination ledger exists only when useful sequencing,
dependencies, or coordination benefit from explicit tracking. Group work by a
coherent implementation outcome. Scenario count, test count, and construction
order never require matching tasks, and stable IDs are optional rather than a
requirement database. Label warranted tasks T<n>; do not number every step.

Frozen when `skl ledger accept` records the proposal. Accepted files are
read-only: completion evidence lives in reports, not in ticks here. Delete this
comment in the real file.
-->

## Behavioral contract work
- [ ] {coherent behavior group; reference relevant rules or scenarios when useful}

## Chores
- [ ] {non-behavioral migration, wiring, or configuration work}

## Docs
- [ ] {documentation work}

---

### Example

```md
# Tasks — add-order-cancellation

## Behavioral contract work
- [ ] Deliver cancellation eligibility, state change, and refund outcomes.

## Chores
- [ ] Add the cancellation timestamp migration before wiring the refund handler.
```
