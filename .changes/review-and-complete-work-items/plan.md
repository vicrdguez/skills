# Review and complete Work Items Plan

## Approach
Complete the Workflow module with Watchdog start/resume/verdict operations and normalized status observation. A fresh Agent Worker receives fixed facts and raw prior prose, performs all judgment and project checks, then returns a semantic verdict plus opaque publication files. Workflow owns convergence, mergeability routing, label/state transitions, merge observation, Dependency release, and Coordination Item completion.

Do not add a review-document parser, identity service, merge bot, or batch scheduler. GitHub's structured label history counts completed bounces; explicit GitHub merge state defines Merged. Existing Pi queue adapters consume structured outcomes but remain outside the Workflow Engine.

## Implementation decisions
- Expose stage-oriented `skl watchdog` next/resume/submit operations and read-oriented `skl status`. Do not expose generic comment, label, review, merge, or state mutation wrappers.
- Select the oldest unclaimed `review` PR with stable Work Item identity as tie-breaker. Preserve the best-effort `wip` Claim and explicit resume behavior.
- Pin the current PR head at Watchdog start and verify it remains fixed through verdict publication. Supply Artifact Baseline and Completion trees, the audit ledger, previous review summaries/inline comments, and later human comments as packet facts or opaque content.
- The Agent Worker, not `skl`, identifies stable `W<n>` findings, interprets human directives, evaluates test strength and requirements, runs the Full Gate, and performs Watchdog judgment.
- Preserve the fresh-context trust boundary as an operating instruction; add no harness identity or attestation mechanism.
- Never rerun Audit from Watchdog. The CLI also runs no project gate.
- Verdict is an explicit semantic command value. Summary bodies and any anchored inline bodies are opaque files; file path, reviewed commit, source path, line, and side are structured transport inputs where required.
- Reconcile publication idempotently by observing exact existing comment/review content and structured anchors before retrying an ambiguous write. Do not parse its prose or add a transaction protocol.
- Count completed finding-driven bounces from GitHub lifecycle label history, not finding Markdown. First failure transitions `review + wip` to `rework`; a later failure transitions to `needs-human`.
- Preserve full first review and incremental repeat review in the Watchdog Skill Definition. Previous reviewed heads and finding meaning remain agent-read from supplied prior content rather than CLI-parsed.
- A human directive changes eligibility only through an explicit backend requeue projection. The engine supplies authorized comments verbatim and infers nothing from text.
- Watchdog may add only Debt Marker source comments when a passing code-local Note needs one. The agent performs the current formatter/parser and `git diff --check`, commits, and pushes. The engine verifies the submitted fixed remote head but does not parse markers or rerun checks.
- On pass, the agent supplies the complete final PR body including Manual Verification recovered from Artifact Baseline. The CLI publishes it opaquely and appends the idempotent `Closes #N` footer.
- A mergeable pass swaps `review + wip` to `done` and leaves the source issue open. `done` means Ready for Merge, never Merged.
- Before accepting pass, query structured GitHub mergeability against the current target. A conflict creates Synchronization Rework, pins that current target for the next Implement packet, and does not increment finding-driven bounce history.
- Human merge remains outside the CLI. GitHub's merged PR state is authoritative evidence that code entered the target and its closing reference closes the source issue immediately.
- On later observations, derive Merged, unblock Dependencies, and close an open Coordination Item only when every child is Merged.
- Needs Human retains whether the correct resume projection is Rework or Awaiting Review and any existing Submission. Human relabeling makes the choice explicit.
- Superseded records remain terminal and retain a lightweight branch reference; do not delete that evidence during cleanup.
- `skl status` performs only safe forward reconciliation of semantically equivalent partial projections. Contradictions report Needs Human and are not overwritten.
- Update Pi queue prompts and runner assets to invoke the semantic stage commands and continue only from structured expected outcomes. Embed/install these Pi-only adapters through `skl`; add no Claude or Codex queue runner in V1.
- Revise Watchdog instructions surgically: preserve judgment language, ordering, Full Gate, first/repeat scope, finding rules, Debt Marker behavior, human boundary, and no-code rule except non-functional comments; remove only deterministic selection, GitHub mutation, archive, and handoff mechanics.

### Module shapes & seams

#### [MODIFIED] Workflow
**Interface:** request or resume Watchdog work, submit a typed verdict with opaque publication inputs, and observe normalized status.

Workflow hides queue order, Claim handling, bounce counting, mergeability routing, label transitions, Merged derivation, Dependency eligibility, parent completion, and safe forward reconciliation.

Dependencies: normalized Backend port, concrete Repository for fixed Git heads and ledger snapshots, and Instruction Catalog for Watchdog packets.

**Test strategy:** invoke all scenarios through `skl watchdog` and `skl status` using an in-memory Backend and temporary real Git repositories. Assert normalized states, published opaque bytes, fixed heads, and eligibility—not internal mutation order.

#### [MODIFIED] Backend port and GitHub adapter
**Interface:** query Awaiting Review candidates, Claims, PR heads/mergeability/merge state, label history, issue/PR comments, inline review anchors, human access metadata, Dependencies, and Coordination Items; project review outcomes and parent closure.

Keep GitHub-specific endpoints, pagination, timeline events, permission shapes, and label names inside the adapter. Add only calls required by Watchdog and status behavior.

**Test strategy:** Workflow behavior uses the in-memory adapter. HTTP adapter tests cover review-comment transport, label-history normalization, mergeability and merged state, issue-closing observations, pagination, and ambiguous-write rereads.

#### [MODIFIED] Repository
**Interface:** verify local/remote reviewed heads, expose historical Artifact snapshots, and confirm retired ledger absence without mutating product code.

The Agent Worker remains responsible for Debt Marker edits, post-marker checks, commits, and pushes.

**Test strategy:** temporary Git repositories provide literal reviewed-head and artifact histories through command-level scenarios.

#### [MODIFIED] Instruction Catalog
**Interface:** render a concrete Watchdog packet containing the fixed reviewed head, historical artifacts, audit content, earlier finding content, human comments, review-round facts, and exact semantic handoff commands.

The Catalog specializes instructions but does not interpret findings or decide verdicts.

**Test strategy:** golden Markdown plus decoded JSON assertions verify fixed facts, raw content delivery, no Audit invocation, and absence of duplicate supporting definitions.

#### [MODIFIED] Pi queue adapters
**Interface:** launch one fresh implementation or Watchdog Worker Session for one item and consume its structured stage outcome.

Adapters own Pi scheduling only. They neither select Work Items nor interpret board state or prose.

**Test strategy:** fixture-driven adapter checks cover continue and stop decisions from representative structured outcomes; manual verification proves fresh contexts in Pi.

## Sequence
1. Extend normalized Backend records with review candidates, lifecycle history, comments, mergeability, merge state, and Coordination Item completion.
2. Implement Watchdog next/no-work/resume and fixed Instruction Packets.
3. Implement opaque finding publication and first-bounce/second-failure transitions.
4. Implement pass, Debt Marker head verification, Manual Verification body transport, and Ready for Merge.
5. Add Synchronization Rework without consuming the finding bounce.
6. Implement status observation, Merged derivation, Dependency release, Coordination Item closure, Needs Human resume, and Superseded recognition.
7. Surgically migrate Watchdog instructions and install the updated Pi queue adapters.
8. Update lifecycle documentation and remove every stale archive-on-pass or issue-close-at-done instruction.
