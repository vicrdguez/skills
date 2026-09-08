# Sharpen Proposal decomposition Plan

## Approach

Amend the existing Propose decomposition and approval instructions after #8 is Merged. This is one complete behavior: choosing boundaries, explaining them, and revisiting them before publication. No Workflow Engine behavior changes are required.

The durable decision is recorded in `docs/capabilities/work-item-lifecycle.md`, under the planned Proposal decomposition extension. Respect ADR 0004's separation of Workflow Mechanics from agent judgment and its surgical-edit constraint. Use `writing-for-agents` when editing the Skill Definition.

## Implementation decisions

- Edit `skills/dev/propose/SKILL.md` at its existing slice-drafting and breakdown-approval instructions. Preserve surrounding wording and order; no superfluous rephrasing, unrelated cleanup, new step, or duplicated reference policy.
- Keep current vertical completeness, Worker Session sizing, seam selection, approval questions, and frozen-artifact rules. Add the approved decision criteria rather than replacing these constraints with size heuristics.
- Apply the change to the post-#8 definition, not a copied pre-CLI version. Do not restore legacy publication mechanics or alter any other Skill Definition or resource.
- Promote the existing planned capability entry to delivered behavior as the final documentation change; keep unrelated capability content intact.

### Modified module and seam

The public seam is `skl skill propose`, which delivers an Instruction Packet. Reuse the existing in-process CLI tests in `cmd/skl/main_test.go`, including `TestRetrieveConcreteProposeInstructions`, adapting to the equivalent existing test if #8 has renamed it. Add focused cases or subtests for the three scenarios; no separate harness or test framework.

Check the primary Propose instructions independently of bundled skills. Keep expectations independent and limited to required guidance; avoid whole-document snapshots and gratuitous sentence-by-sentence assertions. Confirm red before the corresponding prose edit, then green. Preserve existing instruction-distribution checks rather than duplicating them.

Text-presence assertions establish delivery, not semantic consistency or correct agent decisions. Review the complete delivered prose separately against these cases and record concise evidence in the existing Audit/handoff, not a new artifact:

| Case | Required interpretation |
| --- | --- |
| Publication and safe cleanup can deliver separately, but share a Workflow stage | Prefer separate Work Items; the shared stage alone does not justify combining them. |
| A complete later capability depends on an earlier Work Item | Evaluate it after that Dependency is Merged; the Dependency does not disqualify its own Work Item. |
| Separating two useful behaviors would increase overall implementation or review burden | Combining them is permitted with a concrete explanation, not a categorical requirement to split. |
| One coherent delivery has multiple relevant error cases | Include its failure and recovery concerns in the review-burden assessment; do not create a Work Item per error case. |
| Artifact elaboration exposes a material change to boundaries or Dependencies | Explain and revise the breakdown through existing approval before publication. |
| Artifact elaboration only adds detail within an unchanged coherent delivery | Continue without renewed approval; frozen Work Items remain protected afterward. |

## Sequence

1. After the Dependency is Merged, synchronize with the target branch using the normal Workflow and inspect the current Propose definition and retrieval tests.
2. Implement each delivery scenario through the existing red-green loop, keeping prose changes surgical. Review the full resulting instructions against the cases above.
3. Update the capability doc and run the repository's current Full Gate and `git diff --check`. The focused retrieval checks do not replace the Full Gate, Audit, or Watchdog Review.
