# Publish Current Issue Presentations

## Why

Accepted work must remain usable when GitHub is unavailable. An interrupted descriptive-issue attempt must not leave a durable publication protocol that an operator must recover before publishing the current view. This replaces the issue/parent part of superseded #63 and PR #78 under ADR0007.

## What

Publish current descriptive child and parent issues through `skl`, both after local acceptance and by explicit invocation, using agent-authored prose and established forge attachments without durable forge-publication tracking.

## Scope

- Simplify normal acceptance publication and provide explicit current-view issue/parent publication without repeating acceptance or preparing source work.
- Retain existing issue/parent associations and parent/child grouping; remove issue/parent reservations and forge-publication notes.
- Supply current private evidence and authoring guidance when fresh public prose is needed; do not register temporary bodies in the ledger.
- Bounded HTTP behavior, immediate outcome reporting, and local-result preservation at public CLI/Git/HTTP seams.

## Out of Scope

- PR/source/draft-ready publication, owned independently by `publish-current-pulls`; this slice does not depend on it.
- Inline findings, a guided decision viewer, notification infrastructure, a background publisher, durable retries, or stronger delivery guarantees.
- Changes to worker Claims, Contracts, local lifecycle, human-decision intake, completion observation, or private-ledger Git replication policy.
- Diagnosing or fixing workflow-design bug #80, or reopening the superseded PR.

## Definition of Done

This uses the current source Artifact Baseline carrier. These checkboxes are endpoint-verification markers, not permission to mutate accepted target Contracts or store publication progress in them.

- [x] Normal acceptance and explicit invocation publish current issue/parent presentations and associations without a publication-success gate (B1, B3, A1).
- [x] Established issue/parent identities are retained without in-scope durable publication coordination, pending-attempt records, or prose registration (B2, A2).
- [x] Bounded requests, safe retries, uncertain creates, and later explicit attempts obey the accepted best-effort semantics while preserving authoritative local records (B3-B4, A1-A2).
- [x] Agent-authored prose and useful private evidence/guidance are available through existing CLI/resource seams without automatically exporting private reports or generating prose (B5, A3).
- [x] Existing Go tooling, real temporary Git, controlled HTTP, and rendered guidance provide credible grouped evidence, including regression sensitivity and the Full Gate (A4).

## Manual verification

None. Controlled local Git/HTTP and instruction-rendering checks can establish the required behavior; live GitHub delivery guarantees and arbitrary agent prose quality are not claimed.
