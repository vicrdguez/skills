# Tasks - Render Typed Deferred Resources

## Behavioral

Each named scenario or outline is one red-green cycle at the seams pinned in `plan.md`; an outline uses focused table-driven cases.

- [ ] B1 Discover inputs without loading the procedure - `behavior.md` B1; DOD1.
- [ ] B2 Render the applicable Implement submission procedure - `behavior.md` B2; DOD2.
- [ ] B3 Render a decision with the applicable preservation instructions - `behavior.md` B3; DOD3, DOD5.
- [ ] B4 Retrieve review guidance before deciding dispositions - `behavior.md` B4; DOD4.
- [ ] B5 Reject invalid resource inputs before rendering - `behavior.md` B5; DOD5.
- [ ] B6 Preserve literal input data across stateless calls - `behavior.md` B6; DOD6.
- [ ] B7 Follow a concrete parent's deferred resource command - `behavior.md` B7; DOD6, DOD7.
- [ ] B8 Keep context-free retrieval and bundled ownership usable - `behavior.md` B8; DOD7, DOD8.
- [ ] B9 Expose only public resources from the embedded distribution - `behavior.md` B9; DOD9.

## Chores

- [ ] C1 Before implementation, verify #40 is Merged and incorporate its required code while preserving its metadata-only startup contract and the frozen ledger. The advisory `Blocked by: #40` prefix is not engine enforcement or a general target-sync policy.

## Docs

- [ ] D1 Update README resource retrieval guidance and converted-resource examples with valid discovery or invocation-bound commands, explaining only later-known values and retaining context-free no-input examples. Do not change later-slice lifecycle defaults or ADR 0005 behavior.
