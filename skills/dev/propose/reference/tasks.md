# Tasks — {change title}

<!--
This is the coordination ledger for implementation. Write it when sequencing, dependencies, or non-behavioral chores benefit from explicit tracking. Rules:

- A behavioral task may coordinate one or more related scenarios; task boundaries follow coherent implementation work rather than test cardinality or construction order.
- Stable ids (B1, C1, D1…) so an orchestrator can dispatch and track.

Frozen at the `[baseline] <slice-slug>` commit: the `[completion] <slice-slug>` endpoint may only change an existing non-manual `[ ]` to lowercase `[x]`. No task is added during implementation or rework.
-->

## Behavioral contract work
- [ ] B1  {coherent behavior group}  → behavior.md §1–2
- [ ] B2  {another behavior group}   → behavior.md §3

## Chores  (non-behavioral work: migrations, wiring, config)
- [ ] C1  {chore}

## Docs
- [ ] D1  {documentation task}

---

### Example

```md
# Tasks — add-order-cancellation

## Behavioral contract work
- [ ] B1  Order cancellation rules and outcomes → behavior.md §1–3

## Chores
- [ ] C1  Migration: add orders.cancelled_at
- [ ] C2  Wire OrderCancelled → refund handler
```
