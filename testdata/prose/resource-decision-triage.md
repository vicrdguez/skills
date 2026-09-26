# Decision Rules

These are the rules `skl decision inbox` renders with its requests. Take request facts and apply commands from the inbox.

The inbox spans every Project in the configured ledger, wherever you run it. Narrow it only when the user names a Project, with `--project <name>`.

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
- When the human directs retiring the old parent of abandoned work, run `skl decision retire --project <project> --proposal <proposal>` once no slice is active or claimed. Retirement reports partial delivery.

