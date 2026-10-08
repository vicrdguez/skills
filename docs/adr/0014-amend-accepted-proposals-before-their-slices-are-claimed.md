# Amend an accepted Proposal before its Slices are claimed

An accepted Proposal may be amended in place: Slices added, the Contracts of never-claimed Slices replaced, the Dependencies of unclaimed Slices changed, and unclaimed Slices superseded, as one recorded ledger revision through the same intake format acceptance takes. The freeze that ADR 0006 bound to acceptance now binds to the first Claim: a Contract is frozen once a worker has read it under a Claim, never before. This replaces ADR 0006's rule that wrong Contracts require a replacement Proposal and that no dependency-remapping machinery is added; renewed exploration and proposal still precede every amendment.

## Status

Accepted after Explore confirmation; implementation is pending.

## Consequences

- A Slice's Contract, title and planned branch change only while current records show it was never claimed: Ready for Implementation, no Claim, no report, no Submission, no Decision. Ledger history is not consulted, so a Claim released without a handoff leaves the Slice amendable.
- Dependencies are human direction, not Contract. An Amendment may change them on any unclaimed Slice that is not Merged, including Slices in Rework, Needs Human, Awaiting Review and Ready for Merge; the meaning of an edge, eligibility only once every blocker is Merged, does not change. A Slice added to an old Proposal takes that Proposal's place in selection order.
- Supersession is likewise human direction on any unclaimed, unmerged Slice. Superseding a Needs Human Slice through an Amendment is the decision on its request; no separate answer is recorded. The Decision route `supersede` remains for answering a request. A superseded Slice keeps its records in place; a replacement is a new Slice under a new name, and a superseded name is never reused.
- The amended Proposal's graph must be consistent: acyclic across recorded cross-Proposal edges as well as declared ones, every edge naming a recorded Slice, and no active Slice depending on a Superseded one. Membership changes are explicit; a recorded Slice missing from the declaration is a refusal, never a supersession.
- Claimed and Merged Slices are never written. Work in progress finishes before any decision to supersede it. The Proposal Branch name of ADR 0013 is fixed at acceptance and not amendable; an added Slice takes its Integration Target at its first Claim like any other.
- The Amendment is its ledger commit; no amendment record, timestamp or reason field is kept beyond the commit message. Publication of changed, added and parent issues follows the existing best-effort path; issues and pull requests of superseded Slices are left as the Decision route leaves them.
- The propose Procedure remains the only author of Contracts and gains the amendment path: export the accepted Proposal as an intake directory, apply the confirmed changes, review fidelity over the changed Slices and their dependents, then amend. Explore precedes an amendment as it precedes a Proposal.
- Not in scope: waiving a blocking finding or an obligation without changing the Contract, renaming a Slice, and closing the forge artifacts of superseded work.

## Considered Options

- **Replacement Proposal with cross-Proposal edges** (ADR 0006): cannot insert a blocker before a Slice of the original Proposal, and splits one design across two parent issues.
- **Retire and re-accept**: impossible for a never-started Proposal, since retirement needs a superseded Slice and supersession needed a Needs Human request; it also discards the published parent issue.
- **Amendable Contracts on paused Slices** (Needs Human, Rework) with work retained: would move the freeze from the first Claim to review and let a reviewer judge a Contract the implementer never read. Rejected to keep one freeze rule; the minor-finding case it would serve is deferred as a waiver.
