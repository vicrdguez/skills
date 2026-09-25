---
name: decision
description: Resolve current Needs Human requests from the ledger-wide Decision Inbox with explicitly scoped human direction.
disable-model-invocation: true
---

# Human Decision Result

The selected direction was recorded. Every selected item below is resolved by one committed answer and route, so no separate confirmation is required.

## Item outcomes

Every outcome below is the CLI's exact per-item result. Do not redraft it, retry a stale answer hoping for a different result, or read a recorded route as approval to merge.

### widget-dashboard/foundation

Project: widgets
Status: `applied`
Route: `implement`
Recorded decision reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/decision.md`
Answered request: `0000000000000000000000000000000000000002:projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`

## Only the human answer authorizes

- Only an explicitly scoped human answer authorizes a change. A clear answer that names the affected request, directly or by an unambiguous displayed selection, is sufficient; record it without a ceremonial second confirmation.
- A question about an option, a discussion of trade-offs, an agent suggestion or recommendation, and a public GitHub comment, label, or PR body are not authorization. Keep the conversation open until the human directs a specific request and continuation.
- Clarify before any write when the scope or meaning is ambiguous, when a selected request is no longer current, or when the direction would change an accepted obligation. Never silently apply a group answer to an unmentioned request, and never partially enact direction whose meaning depends on an unresolved coupled decision.
- A recorded answer resolves the current request within the frozen Contract. It never amends the Contract, manufactures a pass, waives an obligation, or authorizes new work.


## Changed obligations and abandonment

- Wrong obligations require renewed proposal and re-slicing through `explore` and `propose`. Do not edit the accepted Contract or invent replacement work here.
- `supersede` abandons unmerged work only. It preserves frozen Contracts, reports, exact references, Merged slices, and unmerged source work; a Superseded blocker is not Merged, so its dependents stay blocked.
- An old parent with abandoned slices is retired only when no active work or Claim remains. Retirement reports partial delivery, never all-delivered completion, and performs no archive move, dependency remapping, forge completion observation, or source deletion.


## After a recorded decision

A committed decision reaches the next selected worker through `skl` with its exact reference, within the frozen Contract. It informs that worker's independent judgment; it never manufactures a pass or resets completed-review history. Only a human merges.


## Ledger replication

- widgets/widget-dashboard/foundation: pushed — the local result is authoritative; no further detail was reported
