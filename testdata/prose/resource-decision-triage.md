# Triage Rules for the Decision Inbox

Reference for resolving current Needs Human requests. Read `skl decision inbox` for current request facts and bound commands before applying these rules. Reading this resource records nothing.

## Read ledger-wide, filter explicitly

The inbox is every Project's current Needs Human requests. It is resolved from the configured Workflow Ledger, so it works from a non-repository directory and from inside any source checkout without discovering or registering workspaces. `--project <name>` is the only narrowing; the working directory never narrows scope. A configured ledger that cannot be resolved or read is an access problem, reported as unavailable and never as an empty inbox. An empty inbox creates no work.

## Triage without losing identity

- Group related questions freely for the human, but keep each request's identity, exact reference, commitment, conflict, evidence, options, consequences, and recommendation separate. A shared rule does not merge two obligations or erase their differences.
- Any missing commitment, conflict, evidence, option, consequence, or recommendation is clarified with the human; it is never invented, guessed, or filled in by inference.
- Reading the inbox, grouping requests, and discussing them record no decision and change no Workflow State. Retrieving the detailed rules with `skl skill --resource reference/triage.md decision` records nothing either.


## Only the human answer authorizes

- Only an explicitly scoped human answer authorizes a change. A clear answer that names the affected request, directly or by an unambiguous displayed selection, is sufficient; record it without a ceremonial second confirmation.
- A question about an option, a discussion of trade-offs, an agent suggestion or recommendation, and a public GitHub comment, label, or PR body are not authorization. Keep the conversation open until the human directs a specific request and continuation.
- Clarify before any write when the scope or meaning is ambiguous, when a selected request is no longer current, or when the direction would change an accepted obligation. Never silently apply a group answer to an unmentioned request, and never partially enact direction whose meaning depends on an unresolved coupled decision.
- A recorded answer resolves the current request within the frozen Contract. It never amends the Contract, manufactures a pass, waives an obligation, or authorizes new work.


## Record exactly what the human decided

Write the human's answer verbatim to a file and choose the continuation route. Submit it with the request's bound inbox command, replacing only the two unknown values: `--route <implement|watchdog|supersede>` and `--answer <human-answer-file>`. When the direction covers several independently named requests, submit each request's own command, or submit one explicitly scoped multi-input file with `skl decision apply --input <json-file>`. Report each item as `applied`, `already_applied`, `refused`, or `unresolved`; never claim the whole group succeeded when a member did not. An exact repeated operation recognizes the recorded result without a second decision or requeue and without releasing a later Claim or overwriting later work. For coupled direction, clarify unresolved conditions before writing; `--coupled` requires the entire selected group to validate together.

The continuation route is part of the answer: `implement` returns initial work to Ready for Implementation or finding-driven work to Rework, `watchdog` returns it to Awaiting Review at the same code revision, and `supersede` abandons unmerged work. Existing progress, Submission, fixed references, completed-review history, and the two-review automatic-rework limit remain intact; the next worker exercises independent judgment. Only the CLI records the answer and its route together and atomically. A submission without an identified request and continuation route is refused without mutation.

Explicitly retiring the old parent of abandoned work uses `skl decision retire --project <project> --proposal <proposal>`; a parent is retired only when no active or claimed slice remains.


## Changed obligations and abandonment

- Wrong obligations require renewed proposal and re-slicing through `explore` and `propose`. Do not edit the accepted Contract or invent replacement work here.
- `supersede` abandons unmerged work only. It preserves frozen Contracts, reports, exact references, Merged slices, and unmerged source work; a Superseded blocker is not Merged, so its dependents stay blocked.
- An old parent with abandoned slices is retired only when no active work or Claim remains. Retirement reports partial delivery, never all-delivered completion, and performs no archive move, dependency remapping, forge completion observation, or source deletion.

