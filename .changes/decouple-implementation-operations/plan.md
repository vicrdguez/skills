# Decouple Implementation Operations Plan

## Approach
Extend the repository-bound seam from decouple-setup-and-publication across complete implementation command paths. Use ADR 0002 as the ownership contract and the merged #3 behavior as the compatibility reference. Preserve the predecessor's working Setup/publication paths.

## Implementation decisions
- Use the predecessor's opaque values and repository-bound Backend rather than a parallel implementation-only representation or compatibility wrapper.
- The engine owns which item is eligible, what state permits a handoff, which Git evidence is required, and whether an interrupted transition can proceed. The adapter supplies normalized observations and performs requested materialization, including transport-level retry observation.
- Decode GitHub labels, issue/PR attachments, native IDs, and trusted machine metadata in integration code. Keep canonical contradiction handling, transition permission, and Claim/recovery decisions in Mechanics. Opaque agent prose remains opaque.
- Keep semantic source-Work-Item linkage in the engine request; render the existing GitHub closing footer outside Mechanics. Preserve native issue/Submission references and numeric public CLI/JSON representations through integration-level conversion.
- Preserve established age/tie-break ordering without parsing opaque identities in the engine or delegating selection to the Backend.
- Reuse the concrete Repository for fixed snapshots, ancestry, local and remote heads, and ledger history. Pass the selected remote from integration; retain supported remote layouts and existing snapshot freshness guarantees.
- Preserve previously persisted metadata, Claims, native attachments, and operation records. Existing shipped representations are a concrete compatibility requirement, not a reason to add speculative compatibility layers.
- Do not move agent-owned Audit, TDD, prose judgment, or Full Gate execution into either engine or adapter.

### Module shapes & seams

#### [MODIFIED] Implementation Workflow and CLI integration
**Interface:** existing next/start, resume, inspect, submit, and needs-human command inputs and observable outcomes; the Workflow interface receives semantic intent, a bound Backend, and concrete Git context.

**Test strategy:** B1-B7 materialize at the existing CLI application seam with the existing in-memory Backend and temporary real repositories. Reuse the merged implementation scenarios and interruption fixtures instead of duplicating a new suite. Where a scenario has cases, use idiomatic table-driven checks. Do not mock private orchestration or assert internal call sequences.

#### [MODIFIED] GitHub adapter and Catalog inputs
**Interface:** normalized records and requested semantic mutations, plus integration-supplied native presentation for existing packets and published records.

**Test strategy:** retain the GitHub HTTP adapter checks for adoption, trusted metadata, opaque prose, and ambiguous writes. Extend only to verify moved representation responsibilities. Preserve golden/decoded packet assertions at the existing Catalog seam; no new rendering interface on identities.

#### [UNCHANGED CONTRACT] Repository and ledger history
**Interface:** concrete Git evidence and existing ledger inspection results, independent of backend selection.

**Test strategy:** retain real-Git lifecycle and fixed-head checks. Wiring changes may adjust inputs, not weaken invariants or introduce a second Git abstraction.

## Sequence
1. Trace every merged implementation caller and adapter decision before editing shared records.
2. Carry Work Start/resume and then inspection/handoffs through the established seam with focused behavioral checks.
3. Preserve native output and persisted recovery compatibility through adapter checks.
4. Audit complete ownership and retained regression coverage, then update lifecycle documentation.
