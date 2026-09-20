# Verify contracts without prescribing construction order

## Why

Mandatory test-first cycles and scenario-to-test bookkeeping can produce extensive tests without establishing the promised behavior or architecture. Implementors and fresh reviewers need one acceptance contract that permits valid implementation choices while requiring credible evidence.

## What

Deliver the contract-grounded testing, implementation, and review policy accepted in ADR 0005 through the post-#44 rendering architecture. Replace the tdd skill with one testing skill and make its consumers, shared acceptance criteria, and Submission evidence agree.

## Scope

- Implement after #48, #49, and #50 are Merged, using their delivered Skill Modules, typed contexts, continuations, and deferred resources.
- Deliver testing guidance on observable behavior, appropriate verification boundaries, independent expectations, regression sensitivity, mocking, and retained test value without a prescribed construction sequence.
- Align Implement, included guidance, Audit, Watchdog, and their relevant resources on behavioral conformance, architectural conformance, and local implementation quality.
- Permit contract-preserving implementation choices, existing verification boundaries, and in-scope refactoring; escalate consequential unresolved decisions.
- Account for all obligations with grouped evidence and assess the changed test set as a whole.
- Transition current skill/resource references, inclusion metadata, and owned installed stubs coherently while preserving explicit published obligations and user-owned content.

## Out of Scope

- The new Explore recap, Propose fidelity review, and substantive artifact-authoring conventions, delivered by materialize-approved-contracts; only live testing-name pointers in Propose change here.
- Optional implementation delegation, pre-Audit target integration, queue redesign, or post-approval conflict-resolution assistance.
- Engine interpretation of requirement prose, an evidence schema, a compatibility framework, repository-wide test pruning, or agent evaluations and benchmarks.

## Definition of Done

- [x] Standalone and included testing guidance, public resources, manifests, and supported harness discovery consistently use the testing skill, with a safe owned-stub upgrade and preservation of user-owned content.
- [x] Applicable complete Execution Skills and continuations permit contract-preserving construction and verification choices without global red-green, scenario/test cardinality, or Audit-only refactoring mandates, while honoring explicit frozen obligations.
- [x] Testing guidance requires independent expectations and meaningful regression sensitivity, admits faithful controlled alternatives with disclosed limits, and directs material unresolved uncertainty to the human.
- [x] Implement and Audit assess changed tests for preserved behavioral and failure-mode protection without per-test ledgers, unique-bug quotas, or mandatory replacement suites.
- [x] Audit and Watchdog use consistent criteria that distinguish contractual violations and specific evidence gaps from acceptable preferences and concrete nonblocking debt.
- [x] Deferred Submission guidance accounts for all rules, scenarios, and architectural obligations through grouped concrete evidence, results, and material limitations while preserving existing handoff obligations.

## Manual verification

None.
