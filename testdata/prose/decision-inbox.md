# Decision Inbox

Scope: every Project in the configured ledger, wherever you run it. Narrow it only when the user names a Project, with `--project <name>`.

Refresh: `skl decision inbox`

Help the human answer each request below, then record each answer they give.

## Current requests

Each request is a Work Item paused in Needs Human, with its blocking report and accepted Contract as data.

### widget-dashboard/foundation

Project: widgets
Repository: acme/widgets
Proposal: widget-dashboard
Answered-request reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`
Branch: `widget-dashboard`
Source head: `0000000000000000000000000000000000000002`
Integration target: `0000000000000000000000000000000000000003`
Completed reviews: 2

Apply command:

`skl decision apply --project 'widgets' --item 'widget-dashboard/foundation' --request-commit '0000000000000000000000000000000000000001' --request-path 'projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md' --route <implement|watchdog|supersede> --answer <human-answer-file>`

#### `projects/widgets/proposals/widget-dashboard/foundation/behavior.md`

Reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/behavior.md`

```
# Dashboard foundation behavior

## B1: The dashboard lists every widget

The dashboard lists every registered widget with its current health.

### Scenario: An unhealthy widget is visible
- Given a registered widget whose last check failed
- When the operator opens the dashboard
- Then the widget is listed as unhealthy

```
#### `projects/widgets/proposals/widget-dashboard/foundation/intent.md`

Reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/intent.md`

```
# Dashboard foundation

## Why

Operators check each widget's health one by one.

## What

Show every widget's health on one dashboard page.

## Definition of Done

- [ ] The dashboard lists every widget with its health (B1).

## Manual verification

- [ ] M1: Open the dashboard and confirm each widget's health by hand.

```
#### `projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`

Reference: `0000000000000000000000000000000000000001:projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md`

```
---
schema: 1
outcome: rework
source:
  head: 0000000000000000000000000000000000000002
  target: 0000000000000000000000000000000000000003
  reviewed: 0000000000000000000000000000000000000002
ledger:
  claim:
    commit: 0000000000000000000000000000000000000004
    path: projects/widgets/proposals/widget-dashboard/foundation/state.json
  contract:
    - commit: 0000000000000000000000000000000000000005
      path: projects/widgets/proposals/widget-dashboard/foundation/behavior.md
    - commit: 0000000000000000000000000000000000000005
      path: projects/widgets/proposals/widget-dashboard/foundation/intent.md
  implement:
    commit: 0000000000000000000000000000000000000005
    path: projects/widgets/proposals/widget-dashboard/foundation/implement-report.md
  watchdog:
    commit: 0000000000000000000000000000000000000005
    path: projects/widgets/proposals/widget-dashboard/foundation/watchdog-report.md
round: 2
---
# Watchdog review

## Findings

- W1 BLOCK: an unhealthy widget is listed as healthy (B1).

```
## Triage

- Group related questions for the human. Keep each request's identity, exact reference, commitment, conflict, evidence, options, consequences and recommendation with that request.
- Ask the human for any of these the documents leave out.
- Reading, grouping and discussing requests records nothing.


## What counts as an answer

- A clear human answer that names the affected request, directly or through an unambiguous displayed selection, is an answer. Record it with the bound apply command straight away.
- Questions, discussion, your own recommendations, and public comments, labels and PR bodies are discussion. Keep talking until the human directs a specific request and route.
- Clarify with the human before writing when the scope or meaning is ambiguous, when a selected request is no longer current, or when the answer would change an accepted obligation.
- A group answer covers only the requests it names; the rest stay open.
- Direction that depends on an unresolved coupled decision waits until that decision is settled.
- A recorded answer resolves its request within the frozen Contract: the next worker still meets every accepted obligation, earns its own review result and takes on no new work.


## Record the answer

1. Write the human's answer verbatim to a file.
2. Choose the route: `implement` returns the work to Ready for Implementation or Rework, `watchdog` returns it to Awaiting Review at the same revision, and `supersede` abandons it.
3. Run the request's apply command, filling in `--route` and `--answer`. For several named requests, run each command, or put them in one JSON file for `skl decision apply --input <json-file>`; add `--coupled` when they must succeed or fail together.
4. Report each item's status as the CLI returns it: `applied`, `already_applied`, `refused` or `unresolved`.


## Wrong obligations and abandoned work

- An obligation that is wrong goes back through `explore` and `propose`.
- `supersede` abandons unmerged work only. A Superseded blocker leaves its dependents blocked.
- Retire the old parent of abandoned work with `skl decision retire --project <project> --proposal <proposal>` once no slice is active or claimed. Retirement reports partial delivery.


