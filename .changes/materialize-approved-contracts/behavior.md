# Materialize Approved Contracts Behavior

These rules govern delivered authoring guidance for future Proposals under
[ADR 0005](../../docs/adr/0005-verify-contracts-instead-of-prescribing-test-order.md).
Scenarios bind consequential distinctions, not a test inventory. Their seam is
inspection of actual publicly retrieved instructions and resources, not simulated
agent decisions.

## Rule: Approval and Fidelity Have Different Jobs

Explore ends with one human-confirmed recap of consequential rules, architectural
commitments, and delegated choices. Before publication, Propose supplies that
approved recap and referenced decisions explicitly to one bounded fresh-context
fidelity review across the proposed slices. The reviewer identifies omissions,
weakening, strengthening, contradictions, and invented obligations without
redesigning or reopening accepted choices. Demonstrable transcription errors may
be corrected; decision gaps and semantic changes return to the human. Routine
rereading or reapproval of every artifact is not required.

### Scenario: One Approved Recap Supports the Whole Proposal

- Given an approved design is materialized into several proposed slices.
- When the delivered Explore and Propose instructions are inspected together.
- Then Explore requires the final consequential-rule, architecture, and delegated-choice recap to be confirmed before handoff.
- And Propose requires one fresh-context fidelity review across those slices with the approved recap and referenced decisions explicitly supplied, not assumed from conversation history.
- And faithful materialization does not trigger routine human rereading or a second semantic approval ceremony.

### Scenario: Correct a Lost Decision but Do Not Make a Missing One

- Given a draft omits an explicitly approved restriction while another consequential choice was never settled.
- When Propose's delivered fidelity-review guidance is inspected.
- Then it permits correction of the demonstrable omission against the approved source.
- But the unsettled choice or a proposed semantic change requires human resolution, not reviewer redesign or invented obligations.

## Rule: Compact Artifacts Preserve the Accepted Contract

Intent states the desired result, scope, exclusions, Definition of Done, and
human-owned Manual Verification without duplicating detailed behavior. Behavior
uses scoped named rules where consequential ambiguity needs resolution and binding
scenarios that discriminate interpretations, not redundant rules for every simple
case. Rules govern the relevant class of situations, beyond their examples.

Where relevant, precision considers preconditions, observable interfaces and
outcomes, state and side effects, prohibited effects or unchanged state, and
failure, cancellation, retry, or partial completion. These are reasoning aids,
not compulsory headings. Artifact headings are readable and uniquely referenceable,
without mandatory IDs or a requirement database.

A plan is required for pinned architecture: responsibility ownership, boundary
assumptions, deliberately agreed interfaces, and verification strategy, referencing
existing ADRs. Incidental sketches do not become obligations. Tasks exist only
for useful sequencing, dependencies, or coordination; neither scenario count nor
test count requires them. Scenarios do not prescribe one test each.

Implementation may add tests and behavior unambiguously implied by accepted rules
without editing the frozen ledger. Unresolved consequential behavior or architecture
returns to the human; silence does not delegate it. Explicit published obligations
remain binding, and existing endpoint integrity is unchanged.

### Scenario: Pin Responsibility Without Freezing an Illustrative Helper

- Given accepted architecture assigns responsibilities and an interface, an accompanying helper sketch is illustrative, and no separate coordination ledger is useful.
- When Propose's delivered artifact instructions and four templates are inspected.
- Then they require intent and behavior plus a plan preserving the agreed responsibilities, interface, and verification strategy from their approved source, referencing an existing relevant ADR where applicable.
- And they do not treat the illustrative helper as contractual and permit omission of tasks rather than manufacturing one task or test per scenario.
- And intent carries completion criteria and Manual Verification rather than duplicating the detailed contract.

### Scenario: An Implied Case Does Not Require a Ledger Addition

- Given a frozen rule unambiguously covers an unlisted case, while a different case needs an unresolved consequential decision.
- When the delivered behavior-template and artifact-freeze guidance are inspected.
- Then the implied case may be implemented and tested without extending the frozen scenario list.
- But the unresolved decision requires human disposition rather than a guessed requirement or ledger rewrite.
- And no new convention relaxes an explicit obligation in an already-published ledger.

## Rule: Authoring Guidance Stays Context-Free

Explore and Propose remain reasoning skills retrieved through the existing public
skill interface. Propose's four named templates remain context-free, deferred,
owner-relative resources. Approved context is supplied to the fidelity reviewer,
not introduced as renderer inputs, a new Propose `--input` API, or an engine-parsed
contract. Use the delivered post-#44 distribution framework rather than a second
renderer or the former generic monolithic output.

### Scenario: Retrieve Guidance Without a Proposal-Specific Rendering Session

- Given the foundation deliveries and `verify-contract-conformance` are Merged and present in the work branch.
- When Explore, Propose, and each existing Propose template are retrieved through their public skill/resource commands without proposal-specific inputs.
- Then the delivered content includes this authoring guidance, and the four template resources remain individually available even when a particular Proposal needs no tasks file.
- And the Markdown and explicit JSON skill forms agree, included definitions appear once, and private modules are not exposed as resources.
