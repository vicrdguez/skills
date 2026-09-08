# Decouple Setup and Proposal Publication

## Why
Workflow operations currently discover GitHub repositories and carry provider-shaped records despite the backend-neutral Workflow contract. Correcting the complete Workflow after #3 requires an actual repository-bound seam, established through working Setup and Proposal publication paths rather than an unused interface layer.

## What
Move provider selection and repository binding outside Workflow Mechanics, then run Setup and Proposal publication through that seam while preserving their observable behavior and existing GitHub records.

## Scope
- Start only after every Work Item under Coordination Item #3 is Merged; #8 is the direct blocker through its existing dependency chain.
- Cover Setup and Proposal publication as they exist after #3, including single-slice and coordinated publication, Dependencies, adoption, and interrupted publication.
- Supply a repository-bound Backend to Workflow operations; keep provider discovery, credentials, endpoints, owner/repository interpretation, and native representations in the integration.
- Introduce small opaque identity values for Work Items, Submissions, and related references where needed by shared records; preserve external identifiers and established ordering.
- Keep Git evidence in the concrete Repository module and pass the selected remote explicitly to Git operations instead of assuming `origin` inside Mechanics.
- Keep publication ordering, graph validation, eligibility for publication, and recovery decisions in the Workflow Engine; the Backend observes and materializes.
- Move Setup's label vocabulary behind semantic Backend preparation while retaining its fixed GitHub projections and owned-file behavior.
- Update shared callers mechanically when required for compilation; implementation and review decision migration belongs to the dependent slices.

## Out of Scope
- A second Backend, a Local Backend, provider plugins, new configuration, or a new Repository abstraction.
- New user-facing commands, identifier migrations, changed GitHub labels, or changed workflow policy.
- Implementation and review lifecycle decision migration, status, and cleanup beyond necessary shared-call-site adjustments.
- Unrelated refactoring, dependency changes, and a standalone portability test suite.

## Definition of Done
- [x] B1 Setup preserves its owned guidance and Workflow Projections through a repository-bound Backend.
- [x] B2 Repository selection preserves supported remote inference and refusal-before-mutation behavior without provider discovery inside Mechanics.
- [x] B3 A single-slice Proposal publishes through opaque identities while retaining the existing GitHub-facing record and artifact reference.
- [x] B4 Coordinated publication preserves engine-owned dependency ordering and graph rejection.
- [x] B5 Retrying interrupted publication adopts durable records without duplicating or prematurely readying Work Items.
- [x] B6 Publication still enforces pushed-head, target-ancestry, and Artifact Baseline evidence from Git before backend writes.
- [x] Setup and publication Mechanics contain no provider parsing, native identifier formatting, label definitions, or backend repository coordinates; shared identity and Backend wiring conform to ADR 0002 and every affected caller still builds.
- [x] Existing Setup and publication checks remain green; documentation describes the corrected ownership without advertising new capabilities.

## Manual verification
None.
