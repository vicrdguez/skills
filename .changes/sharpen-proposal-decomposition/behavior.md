# Sharpen Proposal decomposition Behavior

## Feature: Deliver reviewable Proposal decomposition guidance

### Background

- Given #8 has reached Merged and the repository provides the existing `skl skill propose` Instruction Packet retrieval interface
- And decomposition remains Agent Worker judgment rather than a Workflow Engine decision

#### Scenario: Supply dependency-aware independent-delivery guidance

- When the caller retrieves the Propose Instruction Packet
- Then Propose's own instructions prefer separate Work Items for behaviors that deliver safe, useful results independently
- And independence is assessed after declared Dependencies are Merged, without requiring later Work Items
- And each Work Item remains a complete vertical delivery rather than an isolated layer
- And combining independently useful behaviors requires a concrete reduction in overall implementation or review burden
- And sharing files or a Workflow stage alone does not justify combining them

#### Scenario: Supply review-burden assessment and explanation guidance

- When the caller retrieves the Propose Instruction Packet
- Then Propose's own instructions assess the behavior and materially different correctness, failure, and recovery concerns a reviewer must understand together
- And that assessment uses agreed requirements and focused repository inspection, rather than line counts or exhaustive implementation planning
- And different error cases alone do not require separate Work Items
- And the existing breakdown approval briefly explains those concerns for each Work Item and the reason for combining independently useful behaviors
- And the existing title, blocking Dependencies, delivered behavior, and user approval questions remain available

#### Scenario: Supply bounded reconsideration of the approved breakdown

- When the caller retrieves the Propose Instruction Packet
- Then Propose's own instructions return to the existing approval loop before publication if artifact elaboration materially changes proposed boundaries or Dependencies
- And the worker explains the discovery and proposes the revised breakdown for approval before freezing the artifacts
- And ordinary elaboration of a coherent behavior does not require renewed approval
- And this guidance adds no approval stage and grants no permission to split frozen Work Items during implementation

## Verification boundary

These scenarios verify the instructions delivered through the public interface, not autonomous agent compliance. Tests must inspect Propose's own instructions, not satisfy assertions accidentally from bundled supporting skills. Complete-prose review separately checks the examples in `plan.md`; that review is agent-observable evidence, not human-only Manual Verification or an automated semantic guarantee.
