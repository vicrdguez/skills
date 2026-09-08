# Decouple Review and Completion Operations Plan

## Approach
Complete ADR 0002's correction after both preceding decoupling slices. The authoritative command behavior is the merged #3 implementation, including its review/completion slice; this proposal does not invent replacement command names or lifecycle rules before that implementation lands.

## Implementation decisions
- Reuse the repository-bound Backend, opaque values, and separate Git context already established. Adapt every review/status/completion command delivered by #3; preserve earlier corrected paths.
- The Backend normalizes native observations and materializes requests. The engine owns oldest-eligible selection, bounce/convergence policy, permitted transitions, merge/completion eligibility, and workflow recovery. Human interpretation of findings remains Agent Worker behavior, not engine policy.
- Native labels, GitHub issue/PR numbers, review anchors, comment metadata, closing footers, and native packet references remain integration concerns. Preserve external output, trusted source facts, persisted metadata, and adopted records without a new identity scheme.
- Opaque identity equality is not queue ordering. Retain established age and tie-break behavior through semantic ordering facts without provider parsing in Mechanics or backend-side eligibility decisions.
- Use the concrete Repository for historical ledgers, reviewed-head ancestry, conflict evidence, and safe local cleanup. Keep the selected remote explicit. No Backend-owned Git evidence or speculative Repository interface.
- Completion is still human-merge observation, not reviewer approval. Retain engine-owned reconciliation of partial transitions and existing transport-level retry observation in the adapter.
- Finish by tracing all Workflow entry points delivered by #3, including Setup/publication and implementation from predecessors. Remove remaining provider-specific decisions/representations from Mechanics; constrain changes to this ownership correction and regression repair caused by it.
- Do not turn the final ownership check into unrelated cleanup, a framework, or new product behavior.

### Module shapes & seams

#### [MODIFIED] Review, status, and cleanup Workflow
**Interface:** the existing review/verdict, status, and cleanup CLI operations with their semantic inputs, outcomes, fixed evidence, and bound Backend.

**Test strategy:** B1-B7 use the existing CLI application seam, in-memory Backend, and temporary real repositories introduced by #3. Reuse existing review, bounce, human-merge, and interruption coverage. Compare literal semantic outcomes and observable projections, not private call order. No second Backend implementation or standalone conformance suite.

#### [MODIFIED] GitHub adapter and Catalog integration
**Interface:** normalized observations and semantic mutations with native reference/anchor conversion and unchanged user-facing presentation outside the engine.

**Test strategy:** existing HTTP adapter tests verify label/record/anchor conversion, opaque prose, source authorization, and ambiguous-write observation. Retain Catalog golden/decoded packet checks, extending only for changed inputs.

#### [UNCHANGED CONTRACT] Repository
**Interface:** concrete Git and filesystem evidence with existing safety/refusal behavior.

**Test strategy:** keep existing real-Git tests for fixed review heads, retired artifacts, conflict evidence, and safe cleanup. Architectural separation is verified by Audit across actual callers, not tests that inspect source strings or concrete private types.

## Sequence
1. Trace the merged review/completion/status/cleanup paths and their shared callers.
2. Correct review start and verdict paths through the established seam with focused behavioral checks.
3. Correct human requeue, synchronization, completion observation, and cleanup while preserving recovery.
4. Audit the complete Workflow for residual provider coupling, preserve the Full Gate, and update lifecycle documentation.
