# Human Decision Result

Every selected answer was recorded with its route.

## Item outcomes

Report each outcome to the human as the CLI gives it.

### widget-dashboard/foundation

Project: widgets
Status: `applied`
Route: `implement`
Recorded decision reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/decision.md`
Answered request: `0000000000000000000000000000000000000002:projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`

For the next request, run `skl decision inbox`.

## What counts as an answer

- A clear human answer that names the affected request, directly or through an unambiguous displayed selection, is an answer. Record it with the bound apply command straight away.
- Questions, discussion, your own recommendations, and public comments, labels and PR bodies are discussion. Keep talking until the human directs a specific request and route.
- Clarify with the human before writing when the scope or meaning is ambiguous, when a selected request is no longer current, or when the answer would change an accepted obligation.
- A group answer covers only the requests it names; the rest stay open.
- Direction that depends on an unresolved coupled decision waits until that decision is settled.
- A recorded answer resolves its request within the frozen Contract: the next worker still meets every accepted obligation, earns its own review result and takes on no new work.

## Wrong obligations and abandoned work

- An obligation that is wrong goes back through `explore` and `propose`.
- `supersede` abandons unmerged work only. A Superseded blocker leaves its dependents blocked.
- When the human directs retiring the old parent of abandoned work, run `skl decision retire --project <project> --proposal <proposal>` once no slice is active or claimed. Retirement reports partial delivery.


## Ledger replication

- widgets/widget-dashboard/foundation: pushed — the local result is authoritative; no further detail was reported
