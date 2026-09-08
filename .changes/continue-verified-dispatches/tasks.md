# Tasks - Continue Verified Dispatches

Blocked by: `wait-for-claimable-work` (slice 1). This is slice 2 of five approved slices; it delivers the complete CLI path independently of later harness work.

## Behavioral
Each task is one scenario's red-green cycle; Scenario Outline rows are table-driven cases at the same approved seam. CLI means the existing fake Backend plus real temporary Git; HTTP means the concrete GitHub adapter's public interface, with fresh adapter instances reading persisted fixtures.

- [ ] B1 Selection supplies commands and startup resumes the Claim -> behavior.md B1; DOD1; CLI, both lanes, execute root-bound resume from another directory with non-default remote and quoted paths; absent first implementation worktree, existing worktrees, target advancement, and watched-head drift refusal. Assert original pins, worktree-bound handoff commands, and reuse or safe marker-only cleanup of the discarded next packet directory.
- [ ] B2 Completed stage handoff authorizes continuation -> behavior.md B2; DOD2; CLI, every handoff row through next -> returned startup -> existing semantic submit/Needs Human commands. Assert the separate previous_handoff progress facts. In both lanes assert successful active-directory cleanup, no orphaned discarded marker-only directory, and preservation of superseded directories containing result/repair documents.
- [ ] B3 Worker exit status does not establish completion -> behavior.md B3; DOD2; CLI, success-without-proof and error-after-proof controls.
- [ ] B4 Unproven or wrong-stage handoff halts -> behavior.md B4; DOD3; CLI, no selection/polling/repair or destructive mutation; otherwise-valid held-Claim and wrong-stage fixtures isolate their intended guards.
- [ ] B5 Earlier completion cannot authorize an unfinished later round -> behavior.md B5; DOD3; CLI, both lanes and equal-head rounds.
- [ ] B6 Other-lane advancement does not invalidate historical completion -> behavior.md B6; DOD4; CLI, every advancement row with preservation of later obligations.
- [ ] B7 Continuation reuses the waiting contract without replay semantics -> behavior.md B7; DOD5; CLI using slice 1 timing seam, both lanes and all wait/poll forms including poll without wait, explicit `no_work`/`idle_timeout`, fresh windows and deliberate reuse after a received no-work response.
- [ ] B8 Waiting validation, cancellation, and operational failures stop safely -> behavior.md B8; DOD5; CLI, every condition and no false idle outcome, replacement, or loop retry.
- [ ] B9 Invalid continuation references fail safely -> behavior.md B9; DOD6; CLI, all reference rows through `next --after` only.
- [ ] B10 Uncertain selection requires explicit recovery rather than replay -> behavior.md B10; DOD6; CLI, failures after Claim and during metadata/packet work, discarded output, retained Claims, and command help/diagnostics stating the caller's stop-and-inspect obligation; no harness tests.
- [ ] B11 GitHub reconstructs round evidence across later activity -> behavior.md B11; DOD7; HTTP observations into CLI, both lanes, pagination and later equal-head rounds.
- [ ] B12 GitHub evidence must be observed and trustworthy -> behavior.md B12; DOD7; HTTP, existing applied-but-response-lost readback, unapplied/readback-failed writes, and trust/binding/conflict controls with otherwise-valid attachments.

## Chores
- [ ] C1 Wire `next --after`, help, JSON command fields, and minimal internal round facts end to end; preserve existing item-based resume/handoff commands and legacy persisted metadata without inventing historical evidence. Check invalid continuation flag combinations before mutations.
- [ ] C2 Run the repository Full Gate: `go test -count=1 ./...`, `go vet ./...`, `go build ./...`, `node --test prompts/queue-next.test.mjs`, existing Go formatting checks, and `git diff --check`. Record B1-B12 evidence; existing regressions cover DOD8 and PR #19 W1-W4 without a new repeated scenario. Confirm no private helper tests or harness integration were added.

## Docs
- [ ] D1 Update `docs/capabilities/work-item-lifecycle.md` using the `domain` skill's capability format: document shipped command fields, root-bound startup and pinned-obligation guards, local packet-directory reuse/cleanup with document preservation, round-specific proof, non-idempotent continuation with caller single-use convention and stop-on-ambiguity manual recovery, shared waiting/options and `idle_timeout`, and stage success rules. Move only this slice's delivered CLI behaviors out of the planned extension; leave harness work planned and retain the human merge boundary. This is the final task.
