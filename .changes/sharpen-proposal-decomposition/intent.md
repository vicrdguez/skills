# Sharpen Proposal decomposition

## Why

Complete vertical slices can still bundle independently useful behaviors with different correctness and failure concerns. PR #11 combined publication and destructive cleanup: much of its size was justified safety coverage, while separating those deliveries could have reduced simultaneous review burden. The goal is coherent, reviewable Work Items, not smaller line counts or reduced verification.

## What

Sharpen Propose's existing decomposition and breakdown-approval guidance so Agent Workers prefer independent deliveries, explain review burden, and reconsider materially changed boundaries before publication.

## Scope

- Prefer separate Work Items for independently useful behavior. Judge independence after declared Dependencies are Merged, without relying on later Work Items, and retain vertical completeness.
- Combine independently useful behaviors only for a concrete reduction in overall implementation or review burden. Shared files or a shared Workflow stage alone are insufficient.
- Assess the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together, using agreed requirements and focused repository inspection.
- Briefly expose those concerns for each Work Item in the existing approval list, including reasons for combining independently useful behaviors.
- Revisit the existing approval loop before publication when artifact elaboration materially changes proposed boundaries or Dependencies; ordinary elaboration does not require renewed approval.
- Make surgical edits to the existing Propose Skill Definition, verify delivered instructions through the existing public seam, and update the existing capability documentation.

## Out of Scope

- Implementation before #8 (`review-and-complete-work-items`) is Merged, completing the dependency-ordered Work Items of Coordination Item #3. Ready for Merge or issue closure alone does not satisfy this Dependency. There is no Dependency on #12.
- Rephrasing unrelated skill prose, changing other Skill Definitions or their resources, or consolidating overlapping guidance.
- Line limits, scoring, exhaustive failure-case design, separate Work Items for every error case, or horizontal slices of types, plumbing, and tests.
- A new skill, artifact type, approval stage, evaluator, testing framework, or Workflow Engine enforcement of agent judgment.
- Reopening or splitting frozen Work Items during implementation; changing accepted behavior, required verification, Dependency mechanics, or publication mechanics.
- The separate Consumer Repository simplicity standard and changes to existing PRs, including PR #11.

## Definition of Done

- [ ] Retrieving Propose instructions supplies the independent-delivery preference, dependency-aware meaning of independence, vertical completeness, and the concrete justification required for combining behaviors.
- [ ] Retrieving Propose instructions supplies review-burden assessment and a brief explanation in the existing approval list, without line-count targets or exhaustive implementation planning.
- [ ] Retrieving Propose instructions distinguishes material boundary or Dependency changes that revisit approval before publication from ordinary elaboration, without authorizing changes to frozen Work Items.
- [ ] Propose prose changes are surgical: surrounding wording, stage order, existing approval questions, seam guidance, and unrelated behavior are preserved without superfluous rephrasing.
- [ ] Existing public instruction-retrieval tests cover the three delivery scenarios; complete-prose review covers the agreed examples and exceptions. Automated delivery checks are not reported as proof of agent judgment. The current repository Full Gate passes.
- [ ] `docs/capabilities/work-item-lifecycle.md` describes the delivered decomposition behavior rather than a planned extension, without changing unrelated capability content.

## Manual verification

None.
