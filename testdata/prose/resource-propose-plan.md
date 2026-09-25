# {Change title} Plan

<!--
Create plan.md when the approved design pins architecture. Preserve those accepted
decisions for a fresh implementer without inventing new ones. Reference existing
ADRs instead of restating them, and label sketches that explain a decision as
illustrative so they do not become accidental obligations.

Give each architectural commitment a descriptive local label A<n> so intent.md
and behavior.md can reference it; number independently tracked commitments only.

Frozen when `skl ledger accept` records the proposal. A material architectural
change requires human resolution and a renewed Proposal, not an edit to accepted
files. Delete this comment in the real file.
-->

## Authority and assumptions
<!--
Name the approved recap, relevant ADRs, landed prerequisites, and boundary
assumptions on which this plan depends.
-->

## Approach
<!--
Explain how the pinned responsibilities fit together. Keep private paths, helper
choices, and other delegated implementation details open unless they were
explicitly agreed.
-->

## Responsibility ownership
<!-- State which module owns each contractual responsibility. -->

| Owner | Contractual responsibility |
| --- | --- |
| {module or authored guidance} | {behavior and decisions it owns} |

## Architectural commitments
<!--
Record only deliberately agreed interfaces, ordering constraints, error modes,
and other decisions the implementer must preserve. Label each A<n>.
-->

### Module shapes & seams
<!--
Name modified modules, their observable interfaces, dependencies and boundary
assumptions, and the seams at which accepted outcomes can be verified. Label any
helper or code sketch **Illustrative** unless its exact shape was approved.
-->

#### [NEW/MODIFIED] {Module}
- **Interface:** {what callers must know}
- **Responsibilities:** {behavior hidden behind the interface}
- **Dependencies and assumptions:** {relevant facts}
- **Verification seam:** {observable interface and promised consequences}

## Verification strategy
<!--
Map rules, scenarios, and architectural obligations to suitable existing checks or
inspection evidence. Evidence may be grouped many-to-many; scenarios do not
prescribe one test each. Record material limitations.
-->

## Sequence
<!-- Include only ordering needed for implementation, dependencies, or coordination. -->
