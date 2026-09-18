# Resolve Human Decisions: Architecture

ADR0006 supplies authority, layout, reference, and Human Decision semantics. ADR0005 preserves many-to-many verification, independent judgment, and implementor freedom over construction order and test organization. The commitments below pin responsibility boundaries, not new command spellings, private function signatures, or a file-per-concept structure.

## A1: Derive the Inbox From the Existing Ledger Boundary

**Module:** `cmd/skl` owns CLI parsing and output; `workflow` owns deterministic inbox scope and item identity. Reuse the machine-config and private-ledger access delivered through `run-ledger-delivery` from `record-private-proposals`.

**Interface:** Read current Needs Human requests across Projects with an optional explicit Project filter. Return Project/Proposal/Work Item identity and exact report, Contract, and relevant source references. Resolve the ledger independently of the current source directory; do not route this operation through mandatory source-repository resolution in the current `setup/binding.go` seam.

**Seam:** Public `skl` invocation with isolated machine configuration and real local Git ledger repositories, both outside a repository and inside one of several source repositories. Inspect read-only outcomes and unchanged Claims/state. No source-checkout registry, copied inbox, or private navigation responsibility for agents.

## A2: Keep Decision Validation and Atomic Persistence Together

**Module:** Extend `workflow` at the local state-mutation boundary supplied by `run-ledger-delivery`. The CLI transports an explicit scoped answer; it does not infer authorization by parsing natural-language approval or forge comments.

**Interface:** An application identifies the Work Item, exact current request, human answer, and permitted continuation. Validate selected-item request, state, references, and Claim preconditions in the same brief serialized ledger mutation that records `decision.md` and its state change. Store full commit/path references; never compare only the whole-ledger tip or question text. Reuse local transaction, replication, and competing-history safeguards instead of introducing a decision journal or database. Network calls and human discussion do not hold the mutation lock.

An exact repeated operation recognizes the committed answer and route without overwriting later work. Per-item outcomes expose unsafe multi-item members; no global group transaction is required, but coupled direction must not be silently split. Reports contain the agent's request; `decision.md` contains the human's answer. Do not retain the baseline's `workflow/handoff.go` interpretation of an agent-authored pause document as the Human Decision record.

**Seam:** Public CLI decisions over real ledger commits, exercised with unrelated commits, a changed request, a later Claim, failure before commit, and lost-response retry. Verify the committed tree and CLI readback establish answer-plus-route together, with existing Contract/report bytes and later work preserved. Reuse existing dependency fault controls or real Git failure conditions, not internal mock transaction fleets.

## A3: Specialize the Conversation and Worker Handoffs Through Existing Distribution

**Module:** The existing `catalog.go`, `resource.go`, `distribution.go`, and authored `skills/dev/` definitions own Skill composition, deferred resources, embedding, and portable stubs. `workflow/implement.go` and `workflow/watchdog.go`, as evolved by the dependency, own worker input selection.

**Interface:** Add the conversational human-decision behavior through this distribution path, not a harness-specific worker or a second template engine. Supply task facts and concrete `skl` operations with known request arguments bound. Cover inbox, filtered/empty, changed-request, partial-application, and successful-decision outcomes in specialized Markdown and explicit JSON. Leave only genuinely unknown human choices to the conversation. Resources teach triage, related grouping, commitment/conflict/options/recommendation display, explicit scope, and the distinction between discussion and authorization; a clear answer needs no extra confirmation.

The next selected worker receives the exact stored decision and its ledger reference through `skl`, including same-code Watchdog continuation. It records the consumed decision using the dependency's schema-1 report interface, retains Review Count, and exercises independent judgment within the frozen Contract. Do not reimplement the loop, schema reader, Claim machinery, or review-budget policy. Remove conflicting direct-ledger or raw-GitHub direction instructions only where this decision path needs alignment. All production public writes, if any, remain through `skl`; no direct `gh` mutation instructions.

**Seam:** Rendered execution outputs and named resources through public `skl` retrieval, plus worker startup and handoff against real source/ledger Git fixtures. Verify exact consumed references, unchanged count before the next review, and the existing round-limit behavior after continuation. Instruction checks prove delivered guidance, not that arbitrary models correctly interpret every conversation; no conversational evaluation framework is required.

## A4: Limit Supersession to Human Disposition of Existing Work

**Module:** `workflow` owns the selected-item Superseded transition and guard for explicit retirement. Reuse current Proposal membership and state from the ledger. Existing completion and cleanup responsibilities are visible in `workflow/status.go` and `workflow/proposal.go`; their ledger adoption remains with slices 5 and 6.

**Interface:** Preserve frozen Contracts, reports, references, Merged slices, and unmerged source work. Wrong obligations require renewed Propose and a new Proposal rather than in-place amendment. Guard explicit parent retirement by absence of active work and Claims, and report partial delivery accurately. Do not add automatic terminal aggregate observation, forge closure reconciliation, archive movement, replacement graphs, or dependency remapping. A Superseded blocker is not Merged. Existing normal publication may remain pending without requiring `recover-forge-publication` for a local decision.

**Seam:** Public decision and private-read outputs over a real ledger with mixed Merged, Superseded, paused, Ready for Merge, and claimed children. Check retirement refusal/success, preservation of merged history and source branches, unchanged dependencies, and no archive movement. No sibling recovery or completion slice is needed to demonstrate this behavior.

## A5: Verify at Observable Boundaries Without Prescribing Construction

Use existing Go tests and helpers in `cmd/skl`, with real temporary source/ledger Git repositories and isolated config. Reuse the existing controlled forge HTTP seam only for negative authority or publication-independence checks that need it; do not require a live account or replace Git with internal mock fleets. `cmd/skl/decision_authorization_test.go`, `review_authorization_test.go`, and `completed_review_test.go` expose useful trust/retry regressions, but their current public-comment authority and checkpoint mechanics are not the target design.

Focused coverage should group B1-B2 with A1/A3 at inbox and rendered-output boundaries; B3-B5/B7 with A2/A3 at decision application and retry boundaries; B6 with A3 at worker startup and report handoff; and B8 with A4 at explicit disposition. These are many-to-many evidence groupings, not a required test inventory or one-test-per-scenario prescription. Account for every rule, scenario, and architectural commitment in the Submission's Verification section, with concrete results and limitations. Demonstrate regression sensitivity for changed failure handling. Retain the Full Gate and independent Watchdog judgment; normal Go verification includes `go test ./...` and `go vet ./...` under the repository's checks at implementation time.

No `tasks.md`, new testing framework, mandatory test-first sequence, agent benchmark, speculative migration layer, or separate inbox service is warranted. The sole direct prerequisite is `run-ledger-delivery`; exact mechanical names and code organization are delegated within these boundaries.
