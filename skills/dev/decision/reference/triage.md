# Triage Rules for the Decision Inbox

Reference for resolving current Needs Human requests. The bound inbox and result instructions are authoritative for one invocation; this resource expands the rules they apply. Reading it records nothing.

## Read ledger-wide, filter explicitly

The inbox is every Project's current Needs Human requests. It is resolved from the configured Workflow Ledger, so it works from a non-repository directory and from inside any source checkout without discovering or registering workspaces. `--project <name>` is the only narrowing; the working directory never narrows scope. A configured ledger that cannot be resolved or read is an access problem, reported as unavailable and never as an empty inbox. An empty inbox creates no work.

## Present each request without merging obligations

Each request is one Work Item paused on one blocking Phase Report at an exact ledger commit and path. Present, separately for every request:

- the accepted commitment the question concerns;
- the conflict or decision the human must settle;
- the recorded evidence;
- the available options and each option's consequences;
- the worker's recommendation, labeled as a recommendation.

Requests may be grouped when they concern the same rule, but grouping is display only. Keep each request's identity, exact references, and item-specific differences intact. Never let one request's evidence, options, or recommendation stand in for another's, and never summarize away a difference that could change an answer. When a needed detail is missing from the supplied documents, ask for it; do not invent it.

## Distinguish an answer from discussion

Only an explicitly scoped human answer authorizes a change. A clear answer naming the affected request, directly or by an unambiguous displayed selection, is sufficient: record it and its route together and do not demand a ceremonial second confirmation.

These are not authorization:

- a question about an option or its consequences;
- a discussion of trade-offs without a directed continuation;
- an agent suggestion or recommendation, however confident;
- a GitHub comment, label, PR body, or issue edit.

Continue the conversation until the human directs a specific request and continuation. Ask for clarification before writing when scope or meaning is ambiguous, when a selected request is no longer the current blocking request, or when the direction tries to change an accepted obligation. Submitting a decision without an identified request and continuation route is refused without mutation.

## Keep multi-item direction explicit

Direction may name several requests, including across Projects, but grouping is not blanket authorization. Validate every selected request, commit each answer with its route atomically, and report each item as `applied`, `already_applied`, `refused`, or `unresolved`. A mixed result is described as mixed: never claim the whole group succeeded. When one request's answer depends on another unresolved coupled decision, clarify before writing rather than silently enacting part of it. An exact repeated operation recognizes the recorded result; it never records a second decision, releases a later Claim, or overwrites later work.

## Continuation routes

- `implement` returns the item to Ready for Implementation when no Submission exists, or to Rework after review; existing progress, the Submission, fixed references, and completed-review history are preserved.
- `watchdog` returns the item to Awaiting Review at the same code revision, including an unchanged revision; the completed-review count is retained and the two-review automatic-rework limit is not reset.
- `supersede` abandons unmerged work. It preserves frozen Contracts, reports, exact references, Merged slices, and unmerged source work.

A recorded answer informs the next worker's independent judgment within the frozen Contract. It never manufactures a pass, waives an obligation, resets the review count, or adds new work.

## Wrong obligations and retirement

An accepted Contract is not amended by a decision. When the human wants different obligations, the path is renewed proposal and re-slicing through `explore` and `propose`; replacement work belongs to a new Proposal, and no replacement is invented here.

Explicit abandonment may mark affected unmerged slices Superseded. Retiring the old parent is allowed only when no active work or Claim remains, and it reports partial delivery rather than all-delivered completion. Merged slices stay Merged, Superseded slices stay Superseded, dependents stay blocked until their blockers are Merged, and no archive move, dependency remapping, forge completion observation, or source deletion happens in this path. A Superseded blocker is not Merged.
