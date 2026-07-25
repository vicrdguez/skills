# Tasks — {change title}

<!--
This the coordination ledger for implementation. Write it when there's more than a couple of scenarios or any non-behavioral chores. Rules:

- One behavioral task per Gherkin scenario → each is one red-green TDD cycle.
- Stable ids (B1, C1, D1…) so an orchestrator can dispatch and track.
- When relevant include the capability-doc update as the final doc task. 
-->

## Behavioral  (one per scenario → a red-green cycle)
- [ ] B1  {Scenario name}            → behavior.md §1
- [ ] B2  {Scenario name}            → behavior.md §2

## Chores  (non-behavioral work: migrations, wiring, config)
- [ ] C1  {chore}

## Docs
- [ ] D1  Update docs/capabilities/{name}.md

---

### Example

```md
# Tasks — add-order-cancellation

## Behavioral
- [ ] B1  Cancel an unshipped order            → behavior.md §1
- [ ] B2  Reject cancelling a shipped order     → behavior.md §2
- [ ] B3  Refund amount matches the order total → behavior.md §3

## Chores
- [ ] C1  Migration: add orders.cancelled_at
- [ ] C2  Wire OrderCancelled → refund handler

## Docs
- [ ] D1  Update docs/capabilities/orders.md
```
