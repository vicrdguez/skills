# Inspect Private Reports: Behavior

## B1: A Work Item is a sufficient starting point

Through `skl`, a caller can locate and retrieve the current committed implementation and Watchdog reports for a selected Work Item. The CLI supplies known exact ledger references rather than requiring the caller to discover its storage layout or remember a previous command's SHA. Ordinary status/output makes the available private report access discoverable. Exact command names, flags, and textual layout are delegated within existing Markdown-default/explicit-JSON conventions.

Current means the report file at the selected committed ledger HEAD, not an uncommitted working-tree edit, the last GitHub body, or a document chosen by its prose. There may be a current file for each phase; exposing one does not imply that it describes the latest overall lifecycle outcome. Existing structured metadata and workflow records retain their meanings.

Access is on-demand and local. It requires neither a worker Claim nor Needs Human, and must also work for Ready for Merge. It does not pause work, acquire/release Claims, initiate review, or require publication success or forge credentials. Missing reports are reported as absent, not synthesized.

### Scenario: Inspect findings before deciding whether to intervene
- Given a Work Item has completed multiple local phases and has implementation/review reports
- And no human inspection was required for those phases to complete
- When the human selects it through `skl` and requests its current review
- Then the original current report, including authored findings and their reasoning, is accessible with its exact ledger reference
- And the workflow is neither paused nor advanced by this inspection.

### Scenario: Current committed evidence excludes working-tree edits
- Given the latest committed report exists and its working-tree file contains uncommitted changes
- When current report content is retrieved
- Then the returned document is the committed version identified by its ledger reference
- And no working-tree content is substituted or committed.

## B2: Git supplies history and exact versions

Let callers reach earlier report rounds and retrieve a chosen exact historical version using existing path-scoped Git history or recorded report/input references exposed through `skl`. Git is the database; do not create a report-history index, per-round copy store, new finding collection, or parallel decision log. Git commit/path identity refers to the ledger document and is distinct from source-code revisions recorded inside it.

An explicit historical reference returns that version even if the current report, lifecycle, or Claim has changed. Historical retrieval uses the path valid at the named revision. An unavailable commit, path, or report is an actionable error rather than permission to return a newer version, inspect GitHub as a substitute, invent missing reasoning, or fetch unrelated private/project history. This feature does not promise source-code object retention or reconstruct undocumented agent decisions.

This is report browsing, not changing the rule that workflow decisions use current records and explicit inputs rather than reconstructing lifecycle from arbitrary Git history or commit subjects.

### Scenario: Compare an earlier W3 with its later disposition
- Given an earlier committed Watchdog report records W3 and a later report records the agent's subsequent disposition
- When the caller retrieves the earlier version and the current version through the available Git-backed route
- Then each returns its own authored evidence and disposition unchanged, with its exact ledger identity
- And no new engine verdict or automatically merged findings ledger is produced.

### Scenario: Historical evidence is unavailable
- Given a requested exact report commit or path is not available in the configured clone
- When retrieval is requested
- Then the operation identifies the unavailable reference and a concrete correction/access action
- And it does not silently return the current report or change ledger/source history.

## B3: Preserve evidence, privacy, and workflow authority

Return original report contents without rewriting findings, interpreting Markdown as a disposition, classifying architectural decisions, or generating a summary in place of the source. Callers can inspect recorded W identities and reasons as authored. Existing schema-aware operations retain schema-1 semantics and distinguish structured facts from opaque report prose; raw evidence access is not a new schema or authority.

Read-only inspection does not mutate reports, Contracts, Decisions, Claims, Review Count, lifecycle, forge attachments, or publication metadata; it creates no commits, workflow checkpoints, or durable inspection state. It neither publishes private evidence nor calls GitHub to retrieve a replacement. Displaying evidence to the authorized local caller is distinct from automatically adding it to a public PR.

No notification system, guided interview/viewer, automatic Human Decision, new approval gate, report registry, or finding database is introduced. An optional later viewer may navigate this evidence but is not part of this slice.

### Scenario: Inspect offline without affecting a later worker
- Given a newer worker Claim exists and the forge is unavailable
- When a caller retrieves current and historical reports
- Then the local evidence remains readable without contacting the forge
- And the ledger revision, dirty files, Claim, reports, and completed review count remain unchanged.
