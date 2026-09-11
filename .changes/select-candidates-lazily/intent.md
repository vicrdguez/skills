# Select candidates without reconstructing the queue

## Why

`skl implement next` and `skl watchdog next` currently perform broad issue/PR discovery, load unrelated comments and review history, and repeat that work around a Claim. Even `no_work` can be slow. Work Start also requires Git evidence before delivering the instructions that tell a worker how to obtain it.

This is the principal queue-latency delivery of the candidate-first Proposal. Make the cost follow the candidates actually considered and the one item selected, not the repository's historical Work Items or unrelated discussions.

## What

Select from open queue-labelled records, apply ordering and candidate-local Dependencies, claim with selected-item readback, and return one concrete Instruction Packet without requiring local project objects. Resolve the explicit owning issue/PR relationship rather than matching titles. Agent Workers retain Git preparation and artifact inspection.

## Scope

- Open `rework` PRs before open `ready` issues for implementation; open `review` PRs for Watchdog; discard `wip` locally.
- Order each queue by its own record creation time and use its stable numeric GitHub record identity for equal-time ordering in the integration, without parsing opaque identities in Workflow Mechanics.
- Read Ready Dependencies only for tentative candidates, before their discussions, and resolve only referenced blockers as actually Merged.
- Explicit one-to-one issue/PR ownership in first publication, subsequent updates, selection, resume, and attachment-sensitive status/cleanup paths.
- Fetch complete selected-item context without enriching unselected or blocked items.
- Selected-item Claim preflight and readback; reuse release-last handoffs from the blocking slice.
- Return startup/resume facts without project commit, tree, or blob prerequisites, without fetching or creating worktrees in `next`, and without embedding historical artifact files.
- Preserve structured numeric GitHub CLI outputs, selected remote, single-item worker behavior, and explicit same-item resume.
- Structural performance tests and repeatable before/after measurements at the approved CLI seam.

## Out of Scope

- Changing endpoint validation or markerless Adoption; `validate-artifact-endpoints` is independent, not a blocker.
- Reintroducing target pins, hidden review pins, transition journals, or historical name-ownership rules removed by prerequisites.
- Changing review judgments, the completed-review limit, private checkpoint representation, or human-only integration into `main`.
- Automatic Git merges, resets, stashes, rebases, force-pushes, worktree replacement, or project checks inside the selector.
- A persistent queue cache, database, background daemon, GitHub Search as authoritative discovery, or unbounded concurrent requests.
- Atomic competing-worker Claims, queue supervisors/waiting, wholesale backend-independence migration, or changing historical discovery legitimately used by proposal reconciliation and merged cleanup.

## Definition of Done

- [ ] D1 Implementation chooses eligible Rework before Ready using PR age, not source issue age. (B1)
- [ ] D2 Ready and Watchdog queues use their own record age, deterministic identity ties, and exclude Claims. (B2)
- [ ] D3 Empty or entirely claimed queues return `no_work` without historical discovery, discussions, or local project-object reads. (B3)
- [ ] D4 Blocked Ready candidates are skipped before detailed context retrieval and younger candidates are considered in order. (B4)
- [ ] D5 Only actually Merged referenced blockers satisfy Dependencies, including referenced closed records. (B5)
- [ ] D6 Complete candidate and relevant Dependency pagination is observed without speculative hydration of later candidates. (B6)
- [ ] D7 Publication establishes one explicit issue/PR association and renamed titles do not break later operations. (B7)
- [ ] D8 Missing, conflicting, or invalid explicit ownership causes a selected-item error without mutation or global name guessing. (B8)
- [ ] D9 A successful Claim is verified through only its selected records, without a second repository-wide discovery. (B9)
- [ ] D10 Drift during acquisition is handled without overwriting incompatible state, returning a wrong packet, or blindly claiming a replacement after uncertainty. (B10)
- [ ] D11 Both `next` commands return one usable packet when the selected branch objects and worktree are absent locally. (B11)
- [ ] D12 Explicit resume stays on its supplied Claim and supplies continuation instructions that preserve existing work. (B12)
- [ ] D13 The selected worker receives all relevant feedback with original content and provenance, but unselected discussions are not requested. (B13)
- [ ] D14 Forge failures and incomplete observations are errors, never false `no_work`; targeted dependency failures are not treated as satisfied blockers. (B14)
- [ ] D15 The selector preserves prerequisite handoff and CLI-owned checkpoint behavior while exposing no file-management burden to agents. (B15)
- [ ] D16 Measured request/object-read work is independent of unrelated historical records and discussions, with a reproducible before/after performance report. (B16)

## Manual verification

None. Required behavior is testable with controlled HTTP responses, temporary Git repositories/worktrees, and CLI packet/output inspection; live GitHub measurements are optional supporting evidence, not an acceptance gate.
