# Contract-grounded verification plan

## Approach

Implement ADR 0005's common testing and acceptance contract as one coherent change to its producers and consumers. This is a later policy redesign, not permission to change #44's frozen obligations. Implement only after #48, #49, and #50 are Merged; use ordinary Git preparation as needed to make their delivered code available in this work branch before adapting it.

The foundation is defined by ADR 0001 and the published render-deferred-resources, render-implementation-executions, and render-watchdog-executions contracts. Use their actual landed interfaces and module organization rather than inventing future private paths, typed fields, or another renderer.

## Pinned decisions

- testing owns good-test guidance. Audit owns acceptance and finding criteria and exposes the shared criteria as an Audit-owned resource that Watchdog can retrieve without executing Audit. The exact resource filename is an implementation choice.
- Behavioral and architectural obligations are contractual; local quality is judged against mandatory standards and concrete consequences. These distinctions do not require another review stage or another Audit reviewer.
- Implementor discretion covers choices that preserve the contract, not missing consequential decisions. Specific frozen obligations outrank general freedom; no ledger migration or automatic reinterpretation is introduced.
- Architecture commitments, required observations, and trustworthy expected outcomes remain binding. A test may use another suitable existing interface without requiring architectural redesign or automatically adding another test layer. Relevant Design and mocking guidance must not contradict this policy; unrelated cleanup is excluded.
- Evidence grouping is worker judgment expressed in existing opaque Markdown. It does not become a CLI input schema, a requirement parser, an attestation engine, or an extra document.
- Test-first order and scenario/test/task cardinality are not global acceptance criteria. TDD need not be mentioned in testing guidance; explicit user or frozen-contract requirements still apply.

## Module shapes and seams

### Testing definition, resources, and discovery

Replace the existing tdd definition with one testing definition and its owned resources. Its interface is standalone reasoning-skill retrieval, once-only inclusion where applicable, public resource retrieval, and supported harness discovery. Update current callers, embedding, catalog registration, metadata, documentation pointers, and installation coherently. Propose's live testing-name pointers change here, but its substantive artifact conventions remain the sibling slice's responsibility.

Use existing marker-owned installation rules for the known retired stub locations. Preserve user-owned files and extra unrelated content; do not add a general uninstall framework or speculative permanent alias. Existing explicit published obligations remain binding. A concrete immutable invocation conflict that cannot be satisfied requires human disposition rather than silently dropping its requirement.

### Execution and review instruction composition

Adapt the foundation's authored Skill Modules, shared renderer, typed procedure contexts, and applicable narrow inspection continuations. Keep deterministic procedure selection and established facts in the engine, policy judgment in the workers, and private module layout an implementation choice.

The applicable initial, resumed, Rework, full-review, and repeat-review instructions must agree with their included guidance and deferred resources. Known references stay bound; genuinely later evidence and decisions are not guessed at startup. Retain metadata-only startup, established comparison and endpoint identities, specialized capability recipes, fixed review heads, and verified/refused handoff behavior. Do not revive JSON passthrough loops, generic workflow-definition retrieval, or hidden mode inference from incidental fields.

### Acceptance resources and Submission Verification

Share the acceptance criteria through the Audit-owned resource interface, preserving its actual owner in retrieval commands. Watchdog still obtains its own disposition/transport guidance at the existing deferred step. Include only the needed criteria, not another invocation of Audit or its complete execution procedure.

Adapt Implement's deferred submission resource for grouped evidence, retaining its first-implementation/Rework specialization, concrete private destination, Summary, current Audit ledger, and applicable finding-resolution mapping. Keep resource discovery, typed validation, lossless command arguments, and progressive disclosure from #48. Findings, results, actual evidence references, and dispositions remain worker-authored content.

## Verification strategy

Use existing public CLI, renderer, installation, and resource-retrieval seams. Extend relevant foundation fixtures rather than rebuild their framework: representative complete Execution Skills, applicable inspection continuations, deferred first/Rework submission output, standalone and included testing, and shared Audit criteria. Check parent-generated resource commands through the real argument/retrieval path, correct ownership, once-only inclusion, and Markdown/explicit-JSON agreement.

Exercise fresh installation and owned tdd-to-testing upgrade while preserving user-owned content and stable repeat installation. Expected names, outputs, and effects must be independent of the implementation, not copies read from the template under test.

For semantic prose, inspect the complete delivered instructions and resources against the rules in behavior.md, including all applicable consumers and future-dependent branches. These scenarios specify the guidance the Workflow delivers; they do not require a live agent evaluation, a substring assertion per policy sentence, an exhaustive combination framework, or one executable test per scenario. Retain distinct existing regression protection and run the repository's documented checks as they exist after the prerequisites land.

## Sequence

1. Inspect the landed #44 delivery and existing callers and tests; adapt the canonical testing definition and owned distribution.
2. Align construction, scope, regression, retention, and acceptance guidance across complete executions, continuations, and relevant resources.
3. Update grouped Submission evidence and public pointers, verify delivery and upgrade behavior, and inspect policy consistency across consumers.
