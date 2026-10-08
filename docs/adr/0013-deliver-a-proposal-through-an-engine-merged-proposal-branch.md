# Deliver a Proposal through an engine-merged Proposal Branch in Auto Mode

A Proposal's Slices may integrate into a Proposal Branch named at acceptance instead of `main`, with the Workflow Engine squash-merging each passing Submission into that branch as a Transition Operation and the human merging the whole Proposal Branch into `main` once. This moves the human gate from every Slice to the Proposal, so Dependencies clear without a human merge and one lane of each phase drains a Proposal's queue in order. It amends ADR 0004's `main`-only Submission destination and ADR 0006's statement that every merge stays outside the engine: merge into `main` remains human-owned; merge into an open Proposal Branch does not.

## Status

Accepted after Explore confirmation; implementation is pending.

## Consequences

- Auto Mode is a property of a lane invocation, not of the Proposal. The Proposal Branch name is fixed at acceptance and ignored until an Auto Mode implement lane opens the branch from the current `main` at its first Claim on the Proposal. A Proposal may enter Auto Mode with Slices already in `main`.
- While a Proposal Branch is open, every Slice of that Proposal that has not yet reported or submitted takes it as Integration Target at its implement Claim, whatever the lane's mode; a Slice that has already reported or submitted keeps its target. Otherwise a mode switch would build a Slice on a base missing its Dependencies. The branch is spent once it enters `main` and is never reopened; later work is a new Proposal.
- The engine merges only a reviewed head that descends from the current Proposal Branch head, so the squash commit carries exactly the reviewed and gated tree. To make that the common case, `watchdog prepare` performs an Integration Merge of the branch head into the work branch before review; a conflict refuses prepare and becomes a Rework finding for the implementer. Watchdog's writes to a Submission stay limited to Debt Markers, and no agent resolves conflicts on review.
- A refused merge leaves the Slice at Ready for Merge with the reason in the Outcome Instruction; the lane continues, and Auto Mode watchdog lanes retry pending merges on each poll. The human merges by hand or routes the Slice back through the existing `watchdog` decision route.
- A Dependency is satisfied only by a blocker Merged into `main` or into the dependent's own Proposal Branch.
- Proposal lifecycle stays derived from Slice states and the observed completion of the Proposal Submission; `proposal.json` gains the branch and, once open, target, submission and completion facts. The Proposal Submission is a draft from the first Slice merge with an engine-rendered body, and ready once every Slice is Merged.
- Not in scope: per-Slice human checkpoints, automatic sync of `main` into the Proposal Branch, enforcing one lane per phase, backends other than GitHub, and amending accepted Proposals.

## Considered Options

- **Auto Mode recorded on the Proposal at acceptance**: simpler to reason about, but it forbids moving a running Proposal into or out of Auto Mode, which is the case that motivated the change.
- **Watchdog performs the merge and resolves conflicts**: makes the reviewer an author of unreviewed lines and ties merge semantics to agent behavior rather than mechanics.
- **Merge after pass without integration** (clean merge or fast-forward only): either lands a tree nobody gated, or bounces nearly every Slice when implement runs one Slice ahead of watchdog.
