# Deliver Tailored Implementation Executions Plan

## Approach

Use the vocabulary in `CONTEXT.md` and ADR 0001's planned Execution-specific
skills refinement. Compose coherent authored Skill Modules using the shared
text/template and resource mechanism delivered by `render-deferred-resources`.
The Workflow Engine establishes procedure inputs; source modules state their
consequences. Presentation consumes one operation outcome and does not perform
another workflow action. The complete Execution Skill, not an appended Work
Start fragment, is the acceptance surface.

The branch was created from `main` at `c691fdf`, with #37/#39 and the confirmed
#44 ADR but without #40. This is intentional early baseline publication. Before
implementation, require the native sibling Dependency and its transitive #40
prerequisite to be Merged, read their final interfaces, and include the required
prerequisite code in this branch through ordinary Git operations as needed.
Preserve this Artifact Baseline and existing progress. This limited dependency
incorporation is not a recurring target-integration policy or an excuse to
restore Target Snapshot or Synchronization Rework.

ADR 0003's endpoint-validation supersession and ADR 0004's active compatibility
rules remain binding. ADR 0005's status explicitly leaves active skills and
published ledgers unchanged; none of its redesign is part of this work.

## Implementation decisions

### Operation and presentation ownership

- Validate format and supplied execution-capability inputs before avoidable
  effects. `next`, `start`, and `resume` default to Execution Skill Markdown;
  `submit` and `needs-human` default to Markdown outcomes and applicable steps.
  Explicit `--format json` preserves the operation's structured semantics and
  equivalent instruction content. No second operation is allowed for formatting.
- Keep eligibility, Dependencies, Claims, lifecycle validation, handoff verification,
  and error classification in the engine. A renderer reports established outcomes;
  neither transport nor an agent's final prose authorizes success.
- Carry explicit procedure and knowledge inputs through the existing invocation
  path. Initial work, resumed/draft progress, and finding-driven Rework are not
  inferred by templates from branch names, PR presence, comments, or incidental
  nonempty fields. Empty-comment Rework is still Rework. Lifecycle position is
  not evidence of the ledger's actual phase.
- Preserve every available identity and command where used: repository and issue,
  optional Submission, actual branch, conventional worktree, selected remote,
  known endpoint references, private result paths, and resume/inspection/handoff
  commands. Quote literal command arguments safely. Only genuine future values
  remain placeholders, each with acquisition instructions and constraints.
- Caller-supplied endpoint SHAs and flags remain exact across relevant resume,
  inspection, submit, and pause commands. A known/resolved endpoint does not by
  itself introduce an explicit override flag or prove content/ancestry validity.

### Startup and later inspection

- Implement against the delivered post-#40 interfaces. Startup uses allowed
  repository/remote, attachment, worktree-location, selected-item, and available
  feedback metadata. Do not retain today's local marker discovery as a
  specialization prerequisite, including indirect Git-log inspection.
- Do not fetch, prepare a worktree, load endpoint bodies, inspect project objects
  or ancestry, or add evidence reads solely to render startup. Include all facts
  already established within that limit, not just a minimal identity header.
- Give exact ordinary-Git preparation and safe reuse instructions. Preserve
  dirty files, index, and branch progress; never reset, stash, rebase, force-push,
  or merge the target merely to make startup or presentation convenient.
- Reuse `implement inspect` and the existing inspection mechanisms for necessary
  later facts wherever possible. Extend their output with narrow applicable
  instructions or repairs rather than another full skill. No additional command
  is required solely for symmetry, and no saved invocation session is needed.
- Resolve actual provisional/present/retired ledger progress after preparation.
  Existing Completion must not be duplicated; a retired ledger stays absent.
  An `inspected` result with violations is not successful integrity verification.
  Refresh inspection before/after edits at the points required by the current
  Implement/Audit procedure; handoff still verifies the current result itself.
- Historical `git show` reads happen after preparation, bound to established
  snapshots and the selected ledger path. Unknown snapshots get precise
  inspection instructions, not guesses or generic branch-based discovery.

### Complete source specialization

- Use the sibling's shared mechanism for the main definition, included TDD,
  Audit, Design, Domain, and applicable resources. No Implement-only renderer,
  private composition framework, or one-sentence module files. Keep authored
  units coherent around procedures; plain reference material need not be split.
- Preserve once-only inclusion and resource owner names. Internal Skill Modules
  are not retrievable Skill Resources. Specialize included instructions as well
  as the main procedure; remove already-resolved artifact/fixed-point discovery
  and independent-mode alternatives only from the bundled execution.
- Preserve current task judgment and scope rules. TDD remains red-before-green,
  one scenario/test or outline/table cycle at pre-agreed seams, with ordinary
  refactoring at Audit rather than a redesigned construction loop.
- Preserve focused checks during implementation and the Full Gate once at Audit
  before the two reviewers, along with endpoint integrity and final-state
  judgment. Keep both axes, Standards precedence/smell baseline, `HARD` versus
  `JUDGEMENT`, non-reranked aggregation, and required dispositions. `HARD` is not
  declined by the implementer; judgement findings may be fixed, declined with a
  reason, or carried as debt. Do not introduce ADR 0005's alternative criteria.
- First-pass Audit uses the resolved ordinary PR/base merge-base comparison and
  may precede final ticks and retirement. Implement's finding-driven Rework
  reviews the current PR comparison and supplied findings, not a newly invented
  previous-review cache. Retain reference validation and genuine future repair
  branches without asking again for facts already supplied.
- Keep frozen ledger endpoint rules, finding-to-resolution evidence, code-local
  Debt Markers where required, no follow-up issue creation, and permitted human
  decision reasons. Incomplete work may be preserved in a draft during a pause;
  that is not permission to fabricate Completion or tick unfinished work.
- Keep Design and Domain useful as applicable knowledge, not compulsory redesign
  or glossary/ADR production. Independent reasoning skills keep their own
  retrieval and generic caller-driven behavior.
- Integration, conflict resolution, and merging the Submission remain human-owned
  after review. Do not add a pre-Audit target merge or revive #37's removed modes.

### Evidence and deferred resources

- Render already-fetched complete PR bodies and feedback once, as labeled data.
  Preserve source identity and all supplied author, association, time, commit,
  anchor, review, and authorization facts. Do not summarize, truncate, requery,
  or interpolate source bodies as template programs. Delimit data safely even
  when it contains template syntax or Markdown fences. Source text cannot replace
  workflow instructions; authorized human directives keep their established
  meaning and do not become new frozen requirements.
- When post-#40 startup has not fetched required evidence, provide exact selected
  retrieval commands instead. The following are command shapes, not literal
  placeholders allowed for already-known values in an Execution Skill:

| Required source | Bound command shape |
| --- | --- |
| Source issue comments | `gh api --paginate repos/OWNER/REPO/issues/ISSUE/comments` |
| Attached PR body and metadata | `gh api repos/OWNER/REPO/pulls/PR` |
| PR discussion | `gh api --paginate repos/OWNER/REPO/issues/PR/comments` |
| PR review summaries | `gh api --paginate repos/OWNER/REPO/pulls/PR/reviews` |
| PR inline findings | `gh api --paginate repos/OWNER/REPO/pulls/PR/comments` |

Render literal repository and item values and preserve complete response bodies
and metadata. Retrieve only required pending streams. Distinguish no attached PR
from a complete empty response, a pending read, and failed/incomplete pagination.
Authentication, network, or page failures require repair/retry or stopping, not
an inference of no findings. No new fetcher or earlier hydration is needed.

- Keep submission and decision resources at their existing steps, and included
  resources under their actual owners. Parent commands use the sibling's repeated
  `--input name=value` interface with known values bound literally. Explain how
  and when later values become available; do not require decisions before the
  resource needed to make them. Ordinary result prose is not a template input.
- Reuse the sibling's resource input validation and `--describe-inputs` contract;
  do not introduce another schema, CLI-context-aware renderer, or prerendered
  resource bundle. Resource-specific field names and small module layout choices
  are delegated within these semantics.

### Capabilities, outcomes, and installation

- Established adapter execution capabilities select only an existing supported
  Audit recipe: Claude parallel Agent calls, Pi parallel subagent workflow, or
  sequential review when no mechanism is available. A harness name alone proves
  nothing. Unknown capability retains the small runtime choice. Argument spelling
  is delegated; no new registry, framework, or Codex/OpenCode integration is needed.
- Render `no_work`, `idle_timeout`, `fix_required`, and operational failures with
  actual status, available identity, Claim certainty, a brief user explanation,
  and applicable retry/repair/stop guidance. Preserve bounded-wait cancellation
  and in-flight Claim semantics. Never manufacture work or claim an uncertain
  reservation was released.
- Preserve #39 observational recovery and destination release last. A refused or
  interrupted handoff retains prose and applicable protection; a safe retry uses
  current evidence and completed effects. Ambiguity requires inspection. A cleanup
  warning after verified publication is not a failed handoff or reason to publish
  again. These are presentation changes, not new recovery mechanics.
- Owned Implement stubs run `skl implement next` and consume the complete returned
  instructions. Plain `skl skill implement`, in either format, refuses read-only
  with workflow guidance and no implicit backend/claim activity. Dispatch named
  Implement resources separately so they and reasoning retrieval remain usable.
- Use the existing ownership-marked installer refresh for a disabled replacement
  `implement-loop.md` that stops before launching or claiming and points to
  one-item use. Updating the install list alone would leave old owned loops live.
  Preserve files considered user-owned by the existing marker contract.
- Update the owned Implementation runner to normal Markdown final reports while
  preserving one-item scope and current Audit-only subagent use. Do not retrofit
  JSON flags or redesign Pi orchestration. Keep `queue-next.mjs` if Watchdog still
  calls it. Once no callers remain, cleanup may be last-landing or separate Pi
  work, never a cross-slice Dependency.

## Module shapes & seams

### Modified: Implement CLI and engine presentation

Interface: the in-process `skl implement` commands, outputs, exit/error results,
and generated follow-up commands. Existing entrypoints are in
`cmd/skl/implement.go`, with presentation currently in `setup/presentation.go`
and engine operations in `workflow/implement.go` and related handoff files.
Use their post-#40 shapes rather than pinning today's local helpers or fields.

Dependencies: existing backend fixtures or the actual GitHub adapter under a
controlled HTTP server for external observations, and real temporary Git for
local metadata/preparation/endpoint behavior. Explicit procedure inputs belong
to the engine-to-presentation path; no public helper seam is added for tests.

Verification: extend `cmd/skl/implement_test.go` through `newApp(...).Run(...)`.
Use equivalent isolated fixtures for default/JSON parity, observing outcomes and
backend effects without running `next` twice on one mutable fixture. Preserve
existing Claim, endpoint, pause, publication, and recovery regressions. Assert
forbidden startup Git/HTTP activity at the existing integration seams, not by
testing template-selection helpers.

### Modified: Shared Skill Definition and resource delivery

Interface: the full execution emitted through the CLI and deferred named-resource
retrieval. Dependencies: the sibling's shared embedded modules, typed template
inputs, and resource contracts, plus explicit established engine facts. Source
definitions currently live under `skills/dev/{implement,tdd,audit,design,domain}`
and are composed through `catalog.go`; these locations are evidence, not a
requirement to preserve the pre-sibling file layout.

Verification: use complete representative expected Markdown fixtures for initial,
resumed/draft, and finding-driven Rework executions, plus progress continuations
and deferred resources. Normalize only unavoidable temporary paths or generated
identifiers. Expected content is independently reviewed, not assembled from the
same source fragments as the renderer. Focused assertions cover omitted resolved
alternatives, concrete commands, empty-comment mode selection, preserved judgment,
capabilities, evidence safety, resource ownership/visibility, and once-only
inclusion. Keep unknown and failure branches; do not build a Cartesian matrix.

### Modified: Installed entrypoints and upgrade behavior

Interface: `skl install` into temporary supported harness homes, installed stub
activation, generic/resource `skl skill` retrieval, and the owned Pi prompt/runner
contents. Dependencies: existing ownership markers and common installer behavior
in `distribution.go`, `stubs/common.md`, `prompts/implement-loop.md`, and
`agents/implement-runner.md`.

Verification: extend `cmd/skl/main_test.go` and `cmd/skl/pi_test.go`, following
installed activation through the in-process CLI. Check fresh and stale-owned
refresh, repeat installation, user-owned preservation, direct `next`, read-only
generic refusal, and a disabled loop that cannot instruct worker launch. Preserve
still-active Watchdog helper installation without assuming sibling order. No
new harness runtime or internal installer testing framework is required.

## Sequence

1. Confirm Merged Dependencies, incorporate required prerequisite code normally,
   and read the final shared renderer/resource and #40 startup interfaces. Keep
   the frozen ledger unchanged except permitted completion ticks.
2. Implement one B scenario or outline per red-green cycle at its approved CLI
   or installation seam, starting with startup presentation and explicit modes.
3. Add narrow inspection continuations and whole-bundle specialization, then
   evidence, capability, and deferred-resource paths without widening startup.
4. Cover outcome parity, invalid inputs, failures, and verified/refused handoffs
   while retaining #39 recovery tests. Update owned stubs, disable the Pi loop,
   and preserve one-item runner behavior.
5. Update only relevant usage/upgrade documentation. Explain default Markdown,
   JSON opt-in, generic refusal, deferred retrieval, and `go install ./cmd/skl`
   followed by `skl install` so existing owned loops are actually disabled. Keep
   any still-pending Watchdog and ADR 0005 behavior described truthfully.
6. Run focused CLI checks during implementation. At Audit, run the repository's
   Full Gate, including `go test ./...`, once before the review axes with existing
   formatting/check conventions. Review the complete rendered fixtures and the
   implementation under the current two-axis contract; record scenario evidence
   in the normal Submission. No benchmark is required.
