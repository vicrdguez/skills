# Publish Current Phase PR Presentations: Behavior

## B1: Local completion precedes current-view publication

Normal implementation and Watchdog handoffs commit their authoritative local result before attempting public presentation. An explicit `skl` operation can publish the selected Work Item's current relevant committed result whether or not an earlier attempt failed. It does not rerun implementation, tests, Audit, or Watchdog, repeat a handoff, acquire/release a worker Claim, reset Review Count, or require successful issue publication. Exact command spelling is delegated.

Select from ordinary current workflow state and recorded reports with their source/input references, not a new persisted latest-publication selector or arbitrary Markdown verdict parsing. Superseded phase updates are not a queue. Preserve subsequent human direction/local results rather than treating an older report as permission to overwrite them. With unavailable or inconsistent required inputs, report the concrete limitation without manufacturing evidence or changing workflow authority.

Publication is bounded and local-first: a command may wait for an HTTP response/timeout, but publication failure cannot undo local completion or become a worker-selection/handoff gate.

### Scenario: Publish the latest review after interrupted implementation publication
- Given implementation completed locally and its public attempt stopped before settlement
- And Watchdog subsequently completed locally, recording a newer review result
- When current PR publication is invoked with prose for that result
- Then it attempts the latest relevant presentation without publishing the missed implementation update or reconciling its old reservation
- And reports, Review Count, lifecycle, and any later Claim remain unchanged.

## B2: Keep only the established PR association as publication state

The existing Submission repository/number association is the only durable PR-forge-publication fact. Ordinary Work Item state, branch, reports, source revisions, and exact consumed-input references remain; no second publication state model is added.

Normal and explicit PR publication must neither create nor depend on durable active reservations, leases, publication-specific lock files, pending-attempt notes, source-publication receipts, latest-publication phase selectors, retry journals, temporary-body paths/digests/view registries, or inline-publication receipts. Old PR reservation/pending fields in current records must not require liveness confirmation, manual ledger editing, or a special reconciliation command. Affected current-record writes stop carrying in-scope fields; historical Git records, Contracts, and reports remain untouched. Brief ordinary ledger mutations are permitted, never held across network publication.

Retain a successfully established PR association without reassigning a different known association or overwriting newer local results. Lack of a recorded response does not create a durable recovery obligation. Issue/parent fields are owned by the independent issue slice; private-ledger Git replication remains distinct.

### Scenario: A publisher exits and no cleanup operation runs
- Given a selected Work Item carries an old Active reference to an earlier implementation or review report
- And its current local result is valid and has independently authored public prose
- When current publication is invoked
- Then that old reference has no authority to block publication or require matching the old report
- And no replacement attempt record or persistent coordination mechanism is created.

## B3: Present actual recorded source without forcing history

Use the established repository, branch, target, PR association, and current report's recorded source revisions. If source publication is needed, reuse the ordinary non-force Git path for that branch/revision. Implementation normally pushes source; an outage may leave later sequential local phases ahead of the remote. Do not require a separately stored last-published-source receipt to attempt an otherwise safe ordinary push. Never change code, reconstruct a replacement review, force-push, overwrite conflicting remote history, retarget an attached PR, or create a replacement to evade an attachment conflict.

Keep draft/ready presentation in the GitHub adapter, not core lifecycle state. Pre-approval work is draft. A locally approved result may be presented as ready when the observed PR head matches its recorded final source revision. A known mismatch is not approval of the different code. Missing source objects, failed or unsafe pushes, changed attachment identity, or unavailable required observations produce an immediate limitation, not a local outcome change. Observing closed/merged objects is not permission for publication to record terminal workflow state, reopen objects, or unblock dependencies.

### Scenario: An outage spans sequential local phases
- Given the latest local review result references a final revision available in the source repository
- And the established remote branch lags that revision because source publication failed
- When explicit current publication can publish that exact revision through an ordinary non-force push
- Then it may observe the matching PR head and present the latest result
- But when the push is unavailable or conflicts with remote history, it reports the limitation without forcing source, falsely presenting approval, or changing the local review result.

## B4: Bound effects and report uncertainty without durable repair

Permit bounded immediate retries for reads and safely repeatable updates. Before additional updates, stop when their selected local inputs are superseded; unrelated ledger commits alone do not supersede those inputs. A later Claim or result must survive attachment bookkeeping. Retry parameters are delegated, but no wait loop, queue, background publisher, or persisted retry obligation is allowed.

Do not automatically retry an ambiguously completed PR create after a timeout/lost response. Report its uncertainty and finish. Later explicit invocations select the then-current result; duplicate creation is not guaranteed impossible across separate invocations. Use an established association, or unambiguous observable identity where available, rather than guessing ownership from arbitrary public prose. This is best-effort presentation, not globally at-most-once or exactly-once delivery.

Temporary stale public content caused by concurrent movement is accepted. Do not knowingly submit superseded inputs or represent observed different code as approved, but do not introduce transactional rollback, compensating publication workflows, or durable restoration records to eliminate races. Report observed limitations honestly; no successful GitHub acknowledgment is a condition of local progress.

### Scenario: Newer work is recorded while an HTTP request is in flight
- Given publication starts for one committed result
- And a newer result or Claim is recorded while the network request is in flight
- When the attempt returns
- Then local bookkeeping preserves the newer records and existing different associations
- And further updates stop if their inputs are superseded
- And any temporary stale public presentation does not trigger a durable repair protocol or a replay of the older result.

### Scenario: Create response loss
- Given GitHub may have created the PR but the response is lost
- When the invocation handles the failure
- Then it does not automatically send another uncertain create
- And it reports uncertainty without a durable attempt record, changed Review Count, or invalidated local outcome.

## B5: Current agent prose, private evidence, and native CLI guidance

Each invocation receives separately authored current public prose, supplied directly or reauthored from private evidence. Temporary files may be inputs, not registered ledger assets. If suitable prose is absent, provide current result references, recorded reviewed/final/source revisions as applicable, private retrieval operations, and specialized authoring instructions with known continuation arguments bound. Reuse the existing instruction/resource mechanism and deferred resources. Do not generate a public summary in the CLI, recover identical lost prose, persist a public-body snapshot, or classify arbitrary prose as a verdict.

Public presentation describes current progress/outcome, useful verification, risks, and appropriate human checks. Do not automatically append complete reports, Contracts, private operational details, or raw decision history. Complete Manual Verification remains privately accessible and human-owned. Detailed findings remain private; this slice adds no inline-publication interface or receipt store. Forge effects stay in `skl`, not agent-executed `gh` commands. Default Markdown and explicit JSON provide equivalent current facts and actionable limitations under established CLI conventions.

### Scenario: Reauthor after several missed phases
- Given local implementation, rejection, rework, and a subsequent review completed while public prose was lost
- When an agent asks to publish the current result
- Then it receives evidence and guidance for that result rather than a list of missed public updates
- And freshly authored prose can be supplied without registering its path or restoring old bodies
- And no private report or inline finding is automatically exported.
