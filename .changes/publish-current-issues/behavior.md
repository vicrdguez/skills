# Publish Current Issue Presentations: Behavior

## B1: Publish the current accepted view, not an unfinished attempt

After acceptance commits locally, `skl` attempts descriptive issue/parent publication. An explicit operation can also publish the selected current accepted view whether or not an earlier attempt failed. Explicit publication neither repeats acceptance nor requires an implementation Claim, source branch preparation, or PR publication. It uses committed local records and deliberately supplied current public prose, not an inventory of pending attempts. Exact command spelling is delegated.

Publication is local-first and bounded: the command may wait for an HTTP response or timeout, but forge failure cannot undo or invalidate local acceptance, change frozen Contracts or planned branches, or gate subsequent local work. Missing intermediate presentations need not be replayed.

### Scenario: Publish after an interrupted attempt and further local progress
- Given acceptance committed and its issue publisher exited before recording an attachment
- And local work has subsequently advanced without waiting for publication
- When current issue publication is explicitly invoked with current agent-authored prose
- Then it can attempt the current presentation without reconciling an old reservation or replaying work
- And local reports, lifecycle, Claims, and Contracts are unchanged by publication.

## B2: Persist identity, not publication coordination

For this slice, the only durable forge-publication facts are established child issue associations in `state.json` and the parent issue association in `proposal.json`, using their existing repository/number fields. Existing accepted titles, Project/Proposal identities, Contracts, and ordinary workflow state remain authoritative inputs, not new publication metadata.

Normal and explicit issue/parent publication must not create or depend on durable reservations, leases, publication-specific lock files, pending-attempt notes, retry journals, body paths/digests, reuse tokens, or alternative publication-state stores. Obsolete issue/parent fields in existing records must not become a precondition requiring operator reconciliation. Stop carrying these in-scope fields in affected current-record writes without rewriting historical commits or accepted Contracts. Brief ordinary ledger-mutation locks remain permitted; do not hold them over forge I/O.

The independent PR slice owns PR-specific fields; their removal is not a prerequisite here. Existing private-ledger Git replication bookkeeping is not forge issue publication and remains outside this slice's redesign.

### Scenario: An old issue reservation has no continuing authority
- Given a committed accepted record contains an old issue or parent publication reservation
- And there is no usable temporary public body
- When the caller supplies fresh current prose and invokes issue/parent publication
- Then the old reservation does not require release, liveness confirmation, or a special reconciliation operation
- And the operation retains only established in-scope forge identities, not replacement attempt tracking.

## B3: Use established objects and preserve associations

Use a recorded issue/parent repository and number as the established attachment rather than repeatedly creating or guessing ownership from a similar title or arbitrary public prose. Inspect and update the selected attached object as applicable. With no established attachment, an initial create may be attempted; an unambiguously observed association may be reused, but guessing an attachment is not authorized.

Record a successfully established association using brief local mutation, preserving concurrent local work and existing different associations. Parent publication and grouping use the proposal's accepted membership and established child attachments; unavailable children or partial linking do not undo acceptance or become a durable publication gate. A later invocation reevaluates current records and observable grouping, not a saved retry plan. Conflicting or unavailable attachment evidence is reported for the selected invocation rather than silently retargeting or replacing an established association.

### Scenario: A later invocation completes current parent grouping
- Given a multi-slice Proposal has an established parent and one child, while another child is not yet attached
- When a publication attempt cannot complete all currently requested effects
- Then the existing associations and local acceptance remain intact without a persisted grouping-retry record
- When the missing child is subsequently attached and current parent publication is invoked
- Then existing objects are reused and the current grouping can be applied without replaying earlier publication attempts.

## B4: Bounded retries without a recovery obligation

Allow bounded immediate retries of reads and safely repeatable updates. Before further updates, stop if their selected local inputs are now superseded. Retry counts and timing are delegated within that bound. Never automatically retry a create whose effect is uncertain after a timeout or lost response. Report success, failure, missing input, or uncertainty honestly in the immediate command result; exhausting requests creates no durable attempt record, recovery obligation, or publication-waiting state.

A later explicit invocation is a new best-effort publication of the then-current view. Duplicate creation is not guaranteed impossible across separate invocations when an earlier response was lost. Temporary stale public content from concurrent movement is acceptable; this is not permission to knowingly submit superseded inputs, to overwrite newer local work, or to introduce a distributed transaction, restorative rollback protocol, or exactly-once delivery system.

### Scenario: An issue create succeeds but its response is lost
- Given GitHub may have created the issue but the response is unavailable
- When the current invocation handles the uncertain outcome
- Then it does not automatically send another create to resolve that uncertainty
- And it reports uncertainty without altering local completion or persisting a publication attempt
- And a later explicit invocation uses current inputs without a promise of cross-invocation deduplication.

## B5: Agent-authored prose remains separate from private evidence

Each invocation receives current agent-authored public prose, supplied directly or reauthored from private evidence. A caller may supply an existing temporary file, but the ledger does not maintain a registry of that file or authorize its later reuse. If suitable prose is absent, expose current accepted references, private retrieval operations, and specialized authoring guidance with known continuation arguments bound through the existing instruction/resource delivery mechanisms. Do not reauthor, summarize, or classify arbitrary prose in the CLI.

Public descriptions communicate commitments, current context, useful evidence, and appropriate human obligations; they do not automatically include frozen Contracts, full reports, operational details, or private decision history. Complete Manual Verification remains privately accessible and human-owned. No public text is workflow authority. Markdown defaults and explicit JSON expose equivalent operation facts under existing CLI conventions. Instructions direct forge execution through `skl`, not direct `gh` commands or ledger edits.

### Scenario: Temporary prose is lost
- Given accepted records remain available but a prior public body file is gone
- When current publication is requested without suitable prose
- Then the caller receives enough current private evidence references and guidance to author a new description
- When the caller supplies it
- Then publication can proceed without recovering the old bytes or creating durable prose metadata.
