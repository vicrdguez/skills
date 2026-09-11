# Publish Safe Handoffs Without Journals Plan

## Approach

This is approved slice 4, with no blocking Dependencies. It provides the publication invariant needed by slice 5 without implementing that consumer. The review burden is one shared cross-lane invariant and its failure/recovery limits across the existing producers, not a queue redesign.

Baseline inspection: `2f68e43` includes the candidate-first design and `68e9bb9` (#32). Publication already uses opaque backend identities; preserve the existing public numeric GitHub CLI representation. The planned correction in `docs/capabilities/work-item-lifecycle.md` and `docs/adr/0002-use-a-backend-neutral-state-model-without-a-cli-database.md` is design intent, not proof that every neighboring slice has landed.

Current implementation hazards are localized. `workflow/handoff.go` records `ImplementationTransition`, relies on directory and content digests, and can reacquire a Claim on errors after publication. `setup/implementation.go` releases PR `wip` before source-issue cleanup and currently uses the decision digest to exclude opaque prose from metadata parsing. `workflow/review.go` can complete a target-state Claim merely because head and evidence match, and local cleanup can turn success into an error. `setup/watchdog.go` infers pending direction from timeline events; `workflow/status.go` acts on that inference. Fix these paths together rather than adding a second recovery mechanism.

### Release-last protocol

1. Observe the selected Work Item, its actual source and Submission projections, fixed Git head, applicable artifact/policy evidence, and supplied Result Documents. Establish whether this is a new source-stage handoff, a provable partial retry, a verified completed-unclaimed target, or an ambiguity. Do not let normalized state erase which record holds each queue label and Claim. Unknown identity, direction, evidence, or head stops before mutation.
2. Keep the destination nonclaimable throughout publication. On a first implementation Submission, a newly created PR has no destination queue label; establish its protective `wip` before adding one. For an existing PR retain its source-stage `wip`. Never acquire or reuse a target-stage Claim merely because an old command's SHA matches. An issue-only pause keeps the issue's Claim until completion. No new label, token, or durable operation marker is needed.
3. Publish required evidence and read it back through the backend: one attached PR with its fixed head, base, body, closing reference, and draft state; the opaque decision or summary; and each exact structured finding anchor and body. Read before retrying an uncertain write. Reuse only observed matching effects. An ordinary newly proven Rework handoff may update the existing PR; a partial/completed replay with changed input may not silently overwrite it.
4. With destination protection intact, project the intended destination and finish all source cleanup: obsolete source queue labels, applicable `sync`, source-issue Ready/Needs Human projections, and obsolete source-issue Claims. A cross-record handoff may remove the old issue Claim only after destination protection is established. Read back evidence and the complete source-cleanup result. Preserve unrelated labels. Needs Human remains nonclaimable, with its decision visible and any work preserved.
5. If slice 3 is merged, retain its evidence -> intended checkpoint -> release ordering; failure to record the required checkpoint is a pre-release failure. Do not redesign its count or retry identity. Otherwise keep the existing review/bounce policy. Recheck selected state and head before releasing the destination's protective `wip`, which is the last required backend mutation. No source cleanup or required evidence write belongs after it.
6. Reobserve the selected destination. Verified final unclaimed state with matching evidence is success, including an accepted write whose response was lost. If observations are unavailable or now show a possibly newer Claim, stop without another release, Claim acquisition, or rollback. Once publication has been verified, optional private Result Document cleanup and any already-landed done-checkpoint cleanup can only add a cleanup warning, not change the verdict outcome.

The implementation may arrange evidence and protected target writes within these constraints; no particular helper hierarchy or new backend interface is mandated. Protection must be observable from the destination record itself, not from hidden source metadata or today's repository-wide queue filtering.

### Recovery limits

| Observation | Permitted action |
|---|---|
| Unambiguous source Claim with no prior completed matching handoff and required effects missing | Publish or resume forward after validating supplied evidence and policy. |
| Exact accepted evidence with missing effects and visible state proving this partial source handoff never exposed its target | Reuse the accepted effects and finish the missing ones, releasing last. |
| Final target unclaimed, source cleanup complete, all supplied evidence verified | No-op success; no second publication, release, or review increment. |
| Final target claimed, or release acceptance/readback uncertain and a later Claim is possible | Preserve the Claim and stop; matching SHA or evidence is not ownership. |
| Both review and rework with wip and no independent proof of direction | Stop. This overlap can arise in either lane; label ordering and timeline history do not disambiguate it. |
| Old command returns to the same source/target/head/evidence tuple after later-stage work | Refuse when current observations cannot distinguish it; do not add storage to make every historical replay provable. |
| Mismatched supplied evidence, missing proof, unknown state, conflicting attachment, moved head, or failed readback | Preserve observed state and files; report the specific inspection/repair needed. |

A live invocation can retain its observations in memory while it executes. That is not durable Agent Worker state and does not survive a new invocation or override an uncertain release. A genuine partial retry is required, but automatic recovery at every interruption point is not. Losing the proof after source labels are removed is an accepted stop point. Reconstructing old direction from a journal or PR timeline is not an alternative.

`status` has no Result Document argument and therefore often lacks the proof a semantic retry has. It may report an observed final state and retain existing unrelated status duties, but it must not complete an ambiguous Claim or invent expected evidence. Any unambiguous reconciliation it does perform follows the same release-last rule. If slice 2 is unmerged, a newly justified conflict-to-Synchronization-Rework action from an unclaimed Ready for Merge source also establishes protection before exposing Rework; this is not permission to adopt an existing ambiguous target Claim. Preserve ordinary merge observation and Coordination Item completion.

## Implementation decisions

- V1 remains a single-operator, best-effort `wip` reservation. The tests interleave the two lanes around handoff visibility, not competing same-lane atomic Claims. No lease, lock, expiry, Claim Token, new journal, private authoritative database, or compensating rollback is introduced.
- Remove transition production, consumption, and authoritative output, including `ImplementationTransition` and its `from`, `target`, `head`, `body_digest`, `decision_digest`, `directory`, and `completed` fields. Remove `resume_state` as a stored workflow requirement. A Git head, Result Document, or packet directory remains useful in its ordinary role, never as a persisted operation identity.
- Historical comments need not be deleted. Existing feedback reads can return them, but retired fields cannot drive selection, normalize contradictory labels, resume execution, or authorize release. Remove only journal-dependent queue exclusions now invalidated; no wholesale query/filter redesign.
- Keep Target Snapshot/synchronization pins owned by slice 2 and review pins/timeline bounce counting owned by slice 3 if those slices are unmerged. Stop using the timeline to infer transition direction even while its independent bounce-count use remains. Do not claim that removal of this journal removes all issue metadata or all timeline reads.
- Use the smallest correct transport gate during that coexistence: frame newly published opaque decisions with a fixed non-metadata prefix, preserving the Result Document bytes inside it. Metadata recognition applies only to the top-level engine envelope, not substrings inside opaque transport. Exact comparison uses the expected transported bytes. Keep the existing authorization gate for legitimate remaining pin metadata; do not replace the decision digest with another hash, identifier, or persisted classifier. Unclassifiable historical evidence is not permission to guess a transition.
- Worker resume means inspect current branch code, artifacts, and visible feedback for the same Work Item. A new private output directory is allowed; a stored prior directory, execution cursor, or `resume_state` is not required. Existing independently owned target and fixed-review policy checks still apply. Explicit human requeue chooses the next stage; prose does not.
- A cleanup warning must be observable in public structured output and consistent with rendered worker guidance. Keep the successful status and supply the remaining local path/reason. Retain safe directory validation, ownership markers, and symlink/unexpected-file protections. Do not relax file safety merely to make cleanup succeed.
- Reuse any landed claim-safety protection that satisfies these observations. The separate pending claim-safety task is not absorbed or required to land first. Shared files are not a reason to broaden into selection, supervisors, or runner decomposition.

### Module shapes & seams

#### [MODIFIED] Workflow Engine

Interface: the existing semantic submit/pause, Watchdog submit, explicit resume, and status operations reached through `cmd/skl/implement.go`, `cmd/skl/watchdog.go`, and `cmd/skl/status.go`. Dependencies are the existing Workflow Backend for durable observations/mutations, the concrete Repository/Git operations for fixed evidence, and the filesystem for ephemeral Result Documents and any already-landed checkpoint.

Keep policy and safe-continuation decisions here, behind the same public interface. Concentrate proof and release invariants so implementation, Watchdog, and status cannot disagree. `workflow/handoff.go`, `workflow/review.go`, `workflow/implement.go`, `workflow/watchdog.go`, and `workflow/status.go` are the existing locality; introduce no standalone recovery framework or Repository abstraction. Public outcomes distinguish success, cleanup warning, repairable refusal, and operational failure.

#### [MODIFIED] GitHubBackend and presentation

Interface: existing implementation/review observation and publication operations in `setup/implementation.go` and `setup/watchdog.go`, and public output in `setup/presentation.go`. External dependencies are GitHub HTTP and its native issue/PR/label/comment representations. The adapter supplies faithful record-level observations and performs guarded, read-back mutations; it does not decide ambiguous transition direction from history. Narrowly adjust the interface only if the engine otherwise cannot observe required source/destination protection.

Use existing exact comment and anchor reconciliation rather than a content registry. Keep existing opaque backend IDs and provider-specific presentation ownership from #32. Remove journal presentation without migrating unrelated numeric public identifiers or policy fields. Update `skills/dev/implement/SKILL.md` and `skills/dev/watchdog/SKILL.md` only for same-item evidence inspection, bounded retry, and cleanup-warning guidance.

#### Test strategy

The pinned seam is the public CLI via `newApp` and the actual `setup.NewGitHubBackend`, not direct private functions or a memory-only backend. A test HTTP server is the external dependency adapter. It must accept actual POST/PATCH/DELETE requests and return stateful GET observations; GraphQL draft conversion must be real adapter traffic where exercised. Use real Git fixture helpers already in `cmd/skl` for heads, artifact retirement, worktrees, and files.

`cmd/skl/watchdog_requeue_test.go` demonstrates HTTP setup but currently switches to an in-memory handoff backend; that hybrid is not sufficient proof here. Existing adapter tests and helpers may be reused for setup, but each B scenario leaves one public CLI test, with table-driven variants as specified. No new test framework or internal observer interface is required.

Use channels or equivalent deterministic HTTP checkpoints, not sleeps. Pause after each meaningful accepted evidence, label, draft, source-cleanup, and release mutation. Run the other lane before letting the producer continue, without holding the server-state mutex across the nested command. Separately assert actual GET-visible eligibility so a source-level aggregate Claim cannot hide an unprotected PR. After final release, allow the other lane to claim and ensure the old producer never mutates its Claim.

Lost-response tests must apply the write, fail its response, exhaust/fail immediate readbacks, and then run a fresh command after the other lane claims. Include a fresh, provable partial retry as the positive control. Record accepted writes and request payloads to prove no double publication or hidden-journal replacement. Vary old journal content and count-equivalent timeline direction hints independently of visible state; do not assert zero metadata/timeline reads while independently owned policy still needs them.

Exercise cleanup warnings using real unexpected entries or a controlled real filesystem failure after publication verification; do not rely on permissions that pass when tests run with elevated privileges. Keep before/after file assertions for uncommitted work and private-directory safety. The tests must run locally without GitHub credentials, network access to GitHub, or a harness session.

## Sequence

1. Reinspect the integration head for landed slices 2/3 and the separate claim-safety task; retain their owned behavior and reuse compatible protections. This is integration context, not a new blocking Dependency or permission to modify sibling artifacts.
2. Materialize B1 through B14 as individual red-green cycles at the pinned public CLI seam, building only the fixture support needed by the current behavior. Remove replaced journal/timeline-direction expectations instead of keeping a parallel old recovery path.
3. Run focused public CLI tests, then `go test ./...`, `go vet ./...`, and `git diff --check`; verify every B has exactly one matching task and DoD criterion and that retired records have no authoritative readers/writers.
4. Finish the documentation task: update the lifecycle capability, ADR consequences, and affected worker/CLI guidance for the delivered slice only, retaining clearly marked ownership of unmerged candidate-first corrections.
