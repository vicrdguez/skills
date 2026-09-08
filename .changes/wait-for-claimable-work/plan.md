# Wait for Claimable Work Plan

## Approach
Extend the public stage commands through the existing Workflow Engine selection paths. Empty selection is the only condition that permits another poll. The same bounded waiting behavior serves both lanes; do not add a scheduler, backend-specific polling policy, or a second eligibility implementation.

## Implementation decisions
- Support `--wait`, `--wait=2m`, and `--wait 2m`. Bare `--wait` means 15m. Accept positive Go duration syntax with units; reject empty explicit values, unitless numbers, nonpositive values, and overflow before backend effects. A following flag ends a bare optional wait value; a following non-flag token is an explicit value and must validate.
- `--poll 5s` and `--poll=5s` accept positive durations and default to 30s. A valid poll option without `--wait` is accepted but introduces no waiting; it is still validated. A poll interval longer than the wait is valid and is capped by the remaining idle window.
- Preserve the existing `work_available`, `no_work`, packet, and refusal contracts. Only waiting exhaustion adds the successful structured status `idle_timeout`; it reports local inactivity, not global completion. Help and emitted diagnostics explain the distinction.
- Measure each idle window from entry to the selection/wait operation after input validation. Backend operation time counts toward the window. Check immediately, then sleep after each completed empty observation for the lesser of the poll interval and remaining time. Do not overlap requests, poll at or after expiry, or use progress output to reset the window.
- The idle deadline stops new selection attempts; it is not a cancellation deadline for an in-flight Claim. Finish that attempt under existing request limits, return its successful Claim or error even if late, and return idle_timeout if it finishes empty. External cancellation still uses existing cancellation/error semantics and never abandons uncertain Claims.
- Keep machine-readable stdout as one final structured outcome. No heartbeat protocol, streamed JSON outcome replacement, or model calls are required while waiting.
- Preserve selected repository and remote across every attempt. Use existing read-back Claim handling and stage-specific Git/ledger guards. No-work polls must not create private result directories or mutate projections.
- Operational errors, `fix_required`, and other nonempty refusal outcomes end the call. Existing bounded Backend retries remain; the wait loop adds none.
- This slice changes both CLI lanes but not Pi queue prompts, worker definitions, stage handoffs, canonical ordering, review judgment, or provider independence. PR #19 is merged; its shared Watchdog mechanics are the baseline.

### Module shapes & seams

#### Modified: Stage CLI and Workflow Engine
Interface: `skl implement next` and `skl watchdog next`, including help, options, structured results, and existing explicit repository/remote flags. Start with `cmd/skl/implement.go`, `cmd/skl/watchdog.go`, `workflow/implement.go`, and `workflow/watchdog.go`.

Dependencies: existing replaceable Backend and real Repository/Git operations; time and sleeping are the only newly variable implementation concern. Use one minimal internal controllable wait/clock dependency if necessary rather than a new exported scheduling framework.

Test through the existing CLI app seam with the existing in-memory Backend and temporary real Git repositories. Assert final outcomes and observable Backend effects using deterministic time; do not write tests against private polling helpers. For canonical eligibility, reuse existing fixtures and independently valid negative cases rather than recreating the whole lifecycle suite.

#### Existing: Concrete GitHub Backend
Interface: existing exported adapter operations over the existing HTTP test server. This slice should require no new persisted representation. Reuse existing transport/ambiguous-write coverage; add an HTTP regression only if the new behavior changes that adapter. Do not introduce a fake scheduler or harness runtime as a test seam.

## Sequence
1. Add immediate/default and newly available work checks at the CLI seam, then the minimum shared waiting path.
2. Add deadline, cancellation, invalid-input, and refusal scenarios one red-green cycle at a time; preserve existing immediate-selection tests.
3. Run the repository Full Gate. Update CLI help/README and move only the delivered waiting behavior from planned to current in `docs/capabilities/work-item-lifecycle.md`.
