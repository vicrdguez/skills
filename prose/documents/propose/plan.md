# {Change title} Plan

<!--
Record the architecture the approved design pins for an implementer who was
not in the conversation. Point to existing ADRs instead of restating them.
Mark a sketch that only explains a decision **Illustrative**. Delete this
comment in the real file.
-->

## Sources and assumptions
<!-- The approved recap, relevant ADRs, landed prerequisites and boundary assumptions this plan rests on. -->

## Approach
<!--
How the pinned responsibilities fit together, the operational consequences and
limits the approved design accepts, and which owner handles recovery when it is
agreed.
-->

## Delegated
<!-- The boundary of choices left to the implementer, such as private paths, helpers and test layout. -->

## Forbidden
<!--
Decision classes the implementer may not take even where the design is silent,
one per line, such as a new public seam, a compatibility layer or a second
representation of the same data. Review blocks on any of them.
-->

## Responsibility ownership

| Owner | Responsibility |
| --- | --- |
| {module or authored guidance} | {behavior and decisions it owns} |

## Architectural commitments
<!-- Each agreed interface, ordering constraint, error mode or other decision the implementer keeps, labelled A<n>, with a Source: line naming the ADR under docs/adr/, the experiments/<name>.md report or proposal.md it comes from. -->

### Module shapes and seams

#### A1: [NEW/MODIFIED] {Module}
Source: {docs/adr/<adr>.md, experiments/<name>.md or proposal.md}

- **Interface:** {what callers must know}
- **Responsibilities:** {behavior hidden behind the interface}
- **Dependencies and assumptions:** {relevant facts}
- **Verification seam:** {the interface where its promised consequences are observed}

## Verification strategy
<!--
Map rules, scenarios and commitments to existing checks or inspection evidence,
one row per check with the rank it reaches: tests, build or inspection,
strongest first. Every B<n> and A<n> appears in a row's Covers, and one check
may cover several of them. Note material limitations.
-->

| Covers | Check | Rank |
| --- | --- | --- |
| {B1, A1} | {check or inspection} | {rank} |

## Sequence
<!-- Only the ordering that implementation, Dependencies or coordination need. -->
