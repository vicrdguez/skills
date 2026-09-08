# Wait for Claimable Work

## Why
Independent implementation and Watchdog supervisors need to wait for each other to produce work without spending model calls on polling or pretending that an empty queue means the project is finished.

## What
Add optional bounded waiting to both stage-oriented `next` commands. The Workflow Engine polls existing eligibility and Claim operations; callers receive one claimed item, a queue-local idle timeout, or an actionable failure.

## Scope
- `skl implement next` and `skl watchdog next`, including existing repository and remote selection.
- Immediate behavior without `--wait`; bare `--wait` defaults to 15 minutes; explicit positive duration overrides it.
- Configurable `--poll <duration>` with a 30-second default, positive duration validation, cancellation, and safe deadline behavior.
- Existing CLI and Backend test seams, help, and lifecycle capability documentation.

## Out of Scope
- Supervisor skills, worker launch, dispatch continuation, global completion detection, and a shared dashboard.
- Claim expiry, competing same-lane consumers, automatic recovery, new forge retry policy, and a persistent scheduler.
- Changes to ordering, dependencies, review-bounce limits, or human merge authority.

## Definition of Done
- [ ] D1 Both lanes retain immediate selection and no-work behavior without waiting, including explicit repository/remote handling. (B1)
- [ ] D2 Waiting checks immediately and returns as soon as existing selection claims work, with the requested idle window and poll interval. (B2-B3)
- [ ] D3 Empty waiting expires as a queue-local idle timeout without claiming global completion or creating workflow state. (B4)
- [ ] D4 Invalid options fail before backend effects; cancellation and idle deadlines prevent further polling without abandoning a possibly acquired Claim. (B5-B7)
- [ ] D5 Operational failures and deterministic refusals stop waiting rather than masquerading as empty observations. (B8)
- [ ] D6 Every poll reuses canonical ordering and eligibility, including human-merge Dependencies and held Claims. (B9)

## Manual verification
None. Deterministic CLI checks use controlled time; no live harness smoke test is required.
