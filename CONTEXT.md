# Agent Workflow

This context defines the repository-owned workflow that coordinates software-change work across interchangeable agent runtimes while keeping deterministic rules separate from agent judgment.

## Language

**Workflow**:
The repository-owned lifecycle that carries a proposed software change through implementation, review, and human completion.
_Avoid_: Pipeline, process

**Workflow Mechanics**:
The deterministic rules, invariants, and state meanings of the **Workflow**. The same observable state must yield the same valid decisions regardless of which **Agent Harness** participates.
_Avoid_: Scaffolding, prompt logic

**Workflow State**:
The backend-neutral lifecycle position of a **Work Item**. Backend labels and fields are representations of this state, not the state model itself.
_Avoid_: Label, status text

Canonical states are **Ready for Implementation**, **Awaiting Review**, **Rework**, **Needs Human**, **Ready for Merge**, **Merged**, and **Superseded**. A **Claim** is orthogonal to these states.

**Workflow Engine**:
The authoritative interpreter of **Workflow Mechanics**. It resolves eligible work and permits only valid workflow transitions.
_Avoid_: Agent runtime, control plane, orchestrator

**Workflow Backend**:
A provider of durable workflow state and permitted state mutations used by the **Workflow Engine**. One backend is active for a workflow operation.
_Avoid_: Harness, agent

**Workflow Projection**:
A backend-specific representation of canonical **Workflow State**, such as labels and linked records. It may be validated or rebuilt without changing workflow meaning.
_Avoid_: Workflow state, source of workflow semantics

**Setup**:
The deterministic operation that associates a **Consumer Repository** with its **Project** and prepares the guidance needed to participate in the **Workflow**.
_Avoid_: Agent skill, workflow definition

**Adoption**:
Explicit acceptance of an existing **Work Item** into the current **Workflow** without rewriting its implementation history. Missing or ambiguous contract evidence requires human resolution.
_Avoid_: Bulk migration, new proposal

**Workflow Definition Repository**:
The repository that owns and versions the single supported **Workflow**, its mechanics, and its agent behavior. It may distribute that workflow to many **Consumer Repositories**.
_Avoid_: Consumer repository

**Consumer Repository**:
A code repository in which the distributed **Workflow** coordinates software changes. It consumes one workflow rather than defining a custom one.
_Avoid_: Workflow definition repository

**Project**:
The grouping of **Proposals** and **Work Items** for exactly one **Consumer Repository**, named after that repository. Separate checkouts and worktrees of that repository belong to the same Project.
_Avoid_: Checkout, worktree, cross-repository initiative

**Workflow Ledger**:
The private, durable record of accepted work, phase results, human decisions, and **Workflow State** across **Consumer Repositories**. It is distinct from the **Workflow Definition Repository** and an individual Work Item's **Contract**.
_Avoid_: Implementation ledger, public collaboration history

**Agent Worker**:
The nondeterministic participant that owns reasoning, judgment, and source-code changes while following decisions produced by the **Workflow Engine**.
_Avoid_: Workflow engine, orchestrator

**Agent Harness**:
A runtime such as Pi, Codex, or Claude Code that hosts an **Agent Worker**. Changing harnesses must not change **Workflow Mechanics**.
_Avoid_: Workflow backend

**Harness Adapter**:
A thin, harness-specific entry point that connects an **Agent Harness** to the **Workflow Engine** without defining workflow semantics.
_Avoid_: Workflow implementation

**Skill Definition**:
The authoritative, harness-independent description of an **Agent Worker** behavior. It may be specialized only with facts and deterministic conditions.
_Avoid_: Harness skill, generated prompt

**Skill Module**:
A coherent authored unit of instructions used to compose a **Skill Definition**. It is source material, not a separately activated skill.
_Avoid_: Execution skill, skill stub

**Execution Skill**:
A **Skill Definition** specialized for one invocation and its known situation, supplying task facts and actions without requiring the **Agent Worker** to understand workflow storage or bookkeeping. It is the delivered instruction content, distinct from its authored **Skill Modules** and any transport metadata.
_Avoid_: Rendered skill, instruction packet

**Skill Stub**:
A discoverable filesystem representative of one **Skill Definition**, installed in each supported harness's user skill directory. It delegates instruction retrieval without duplicating behavior or requiring a harness plugin package.
_Avoid_: Skill definition, harness adapter

**Skill Resource**:
Named supporting material belonging to a **Skill Definition** and retrieved only when that behavior requires it.
_Avoid_: Inline prompt context

**Instruction Packet**:
The structured delivery representation of a skill invocation's instructions, facts, and inclusion manifest, distinct from the **Execution Skill** it carries.
_Avoid_: Execution skill, skill definition, model-generated prompt

**Result Document**:
Ephemeral, agent-authored material prepared for a phase handoff or human-facing publication. It is distinct from a durable **Phase Report**; a public body may be reconstructed from recorded evidence rather than retained as workflow history.
_Avoid_: Phase report, implementation ledger, durable project knowledge

**Phase Report**:
An **Agent Worker**'s recorded outcome, evidence, and input references for one implementation or **Watchdog Review** phase. Its outcome is distinct from the resulting **Workflow State**.
_Avoid_: Agent transcript, workflow state

**Completion Declaration**:
An **Agent Worker**'s explicit statement that an identified **Contract Item** is complete or incomplete at the reported source revision. Complete means the required outcome is satisfied and its applicable verification has succeeded; **Manual Verification** remains human-owned.
_Avoid_: Implied completion, contract checkbox mutation, engine judgment

**Proposal**:
An approved definition of one change materialized as one or more **Work Items**, with a **Coordination Item** for multi-slice work and explicit Dependencies between its Work Items. Changed contractual obligations require a replacement Proposal rather than rewriting the existing contracts.
_Avoid_: Work item, implementation ledger

**Work Item**:
One implementation slice whose identity remains stable from acceptance through merge or supersession. Issues, submissions, branches, worktrees, and contract documents are attachments or projections of it.
_Avoid_: Agent session, issue, pull request

**Contract**:
The frozen obligations accepted for one **Work Item**, covering its agreed behavior, architecture, delivery requirements, and **Manual Verification**. Changed obligations require renewed proposal rather than amendment of the existing Contract.
_Avoid_: Mutable plan, implementation ledger, incidental implementation detail

**Contract Item**:
An individually identified obligation or task within a **Contract**, carrying a local label and descriptive title. Labels distinguish behavior (`B<n>`), architecture (`A<n>`), tasks (`T<n>`), and **Manual Verification** (`M<n>`).
_Avoid_: Review finding, test case, document row number

**Submission**:
The proposed code changes explicitly attached to exactly one **Work Item** for independent review and human merge. A Work Item has at most one Submission; ownership is independent of names.
_Avoid_: Work item, claim

**Integration Target**:
The destination branch in a **Consumer Repository** into which the **Merge Authority** intends to merge a **Submission**. It is distinct from any particular observed revision of that branch.
_Avoid_: Target snapshot, submitted revision

**Dependency**:
A relationship in which one **Work Item** cannot become eligible until every blocking Work Item is **Merged**.
_Avoid_: Ready-for-merge prerequisite

**Claim**:
The exclusive reservation of a **Work Item** by one **Agent Worker**, orthogonal to **Workflow State**. Claims do not expire automatically; ordinary selection skips reserved work, and interrupted work requires explicit resume or release.
_Avoid_: Workflow state, issue label, time-limited lease

**Transition Operation**:
An attempt to move a **Work Item** toward a valid target **Workflow State**. Recovery observes already completed effects and stops for explicit inspection when safe continuation cannot be established.
_Avoid_: Shell command, rollback transaction

**Work Start**:
A project-scoped **Transition Operation** that selects and claims one eligible **Work Item** and returns its **Execution Skill**.
_Avoid_: Read-only queue lookup, instruction rendering

**Ready for Merge**:
The endpoint of autonomous delivery, where **Watchdog Review** has passed and final integration and merge remain human-owned. It is distinct from **Merged** and does not assert that integration is conflict-free.
_Avoid_: Done, merged

**Needs Human**:
The paused **Workflow State** for a decision automation cannot make. Any existing **Submission** remains attached while the human decides how the Workflow should continue.
_Avoid_: Failed, abandoned

**Human Decision**:
Explicitly authorized human direction answering an identified request for judgment and specifying how the **Workflow** should continue within the accepted contract. Changed contractual obligations require renewed proposal rather than amendment of the existing Work Item's contract.
_Avoid_: Inferred approval, agent recommendation

**Decision Inbox**:
The unresolved requests attached to **Work Items** in **Needs Human** across **Projects**, considered together for human triage and resolution. Its scope is distinct from project-scoped worker selection.
_Avoid_: Worker queue, final review queue

**Merged**:
The observed terminal **Workflow State** in which the accepted code change has entered its **Integration Target**. It closes the Work Item and satisfies Dependencies without making merge an engine-owned phase.
_Avoid_: Ready for merge, approved, engine-owned merge phase

**Superseded**:
The terminal **Workflow State** of an abandoned Work Item whose replacement requires renewed exploration and proposal.
_Avoid_: Needs human, merged

**Merge Authority**:
The human who decides how to integrate an approved **Submission** after review, resolves conflicts caused by later **Integration Target** movement, and performs the final merge outside the **Workflow Engine**. This is distinct from an Agent Worker's required pre-Audit merge of one observed target revision into the work branch.
_Avoid_: Reviewer, agent worker

**Manual Verification**:
A human-owned check required by the accepted contract, whose satisfaction is decided by the **Merge Authority**. Its complete obligations remain available privately even when public presentation omits operational details.
_Avoid_: Validation gate, agent-verifiable scenario

**Final Review Package**:
The human-facing account of a **Submission**'s commitments, delivered outcome, verification evidence, deviations, and remaining risks. It supports the **Merge Authority** without exposing the full worker history or replacing the underlying evidence.
_Avoid_: Agent handoff, worker transcript

**Forge Publication**:
Delivery of human-facing work descriptions, progress, or outcomes to an external collaboration service. Publication is distinct from recording a **Phase Report** or advancing **Workflow State** in the **Workflow Ledger**.
_Avoid_: Worker handoff, authoritative phase result

**Human Finding Directive**:
An explicitly authorized human disposition for an existing **Review Finding** within the accepted contract. It informs Watchdog Worker judgment without replacing it.
_Avoid_: Validation result, inferred intent

**Coordination Item**:
A non-claimable parent that groups multiple child Work Items, with no branch or Submission of its own. It completes when every child is **Merged**; retiring a partially delivered Proposal does not assert completion.
_Avoid_: Work item, implementation slice

**Full Gate**:
The Consumer Repository's complete test, typecheck, and lint verification run by implementers and watchdog reviewers. It remains Agent Worker behavior rather than a Workflow Engine operation.
_Avoid_: Audit, workflow invariant

**Post-Marker Check**:
The narrow check Watchdog performs after adding Debt Markers: record the final head, run `git diff --check`, and run the existing formatter or parser check for touched files. It does not rerun the Full Gate or Audit and does not parse markers with regular expressions.
_Avoid_: Full gate, audit

**Audit**:
Implementation-phase agent judgment against repository **Standards** and applicable **Contracts** before submission. It has no independent Workflow State or Claim.
_Avoid_: Validation gate, watchdog review

**Audit Finding**:
An observation from **Audit** with an `F<n>` identity, an axis of Standards or Contracts, severity, evidence, and disposition. It describes a problem with conformance rather than defining a new **Contract Item**.
_Avoid_: Architectural commitment, contract item, watchdog finding

**Watchdog Review**:
Independent agent judgment performed in a fresh Worker Session after submission. It may change workflow disposition and may add only permitted **Debt Markers** to the Submission.
_Avoid_: Audit, validation gate

**Review Count**:
The number of completed **Watchdog Reviews** recorded for a **Work Item**, including reviews ending in **Needs Human** but excluding interrupted attempts and repeated recording or publication of the same review. Its limit restricts further automatic **Rework**, never approval of a passing review; recreating a worktree or receiving human direction does not reset it.
_Avoid_: Finding count, submission retry count, rework bounce count

**Review Finding**:
A **Watchdog Review** observation with a stable Work-Item-local `W<n>` identity, disposition, evidence, review round, and reviewed source revision. Findings drive rework and human decisions without defining new **Contract Items**.
_Avoid_: Validation failure, free-form comment

**Debt Marker**:
A succinct, self-contained, non-functional source comment explaining nonblocking technical debt or a potential issue near the affected code. It is a maintenance anchor, not a record of review provenance.
_Avoid_: Functional code change, blocking finding, review transcript

**Worker Session**:
One fresh **Agent Worker** context dedicated to one **Work Item**. A **Harness Adapter** may start successive sessions while the **Workflow Engine** continues to report eligible work.
_Avoid_: Queue, workflow

**Local Backend**:
A **Workflow Backend** that supports a complete workflow without a hosted forge.
_Avoid_: Local Git helper, GitHub cache

## Flagged ambiguities

- **Contract** names a Work Item's accepted obligations; **Workflow Ledger** names the private cross-project record. They are not interchangeable concepts.

## Example dialogue

> **Developer:** Can the Codex harness choose a different eligible Work Item than Pi?
>
> **Domain expert:** No. The Workflow Engine applies the same Workflow Mechanics to the same Workflow Backend state. The Harness Adapter only gives each Agent Worker access to that decision.
>
> **Developer:** Who decides whether the implementation is maintainable?
>
> **Domain expert:** The Agent Worker does; maintainability requires judgment. The Workflow Engine owns only deterministic eligibility, invariants, and transitions.
>
> **Developer:** What happens when two Worker Sessions request the same Work Item?
>
> **Domain expert:** Only one may hold its Claim. Workers may act concurrently on different Work Items, but interrupted reservations require explicit recovery rather than automatic expiry.
>
> **Developer:** Does the Claude Skill Stub define different behavior from the Pi one?
>
> **Domain expert:** No. Both expose the same Skill Definition. Their Execution Skills preserve the same Workflow Mechanics while reflecting the worker's known execution capabilities.
>
> **Developer:** Does the Agent Worker assemble the Skill Modules for its task?
>
> **Domain expert:** No. The Workflow Engine selects and composes them into the Execution Skill for that invocation.
>
> **Developer:** GitHub shows the `done` label. Is the Work Item Merged?
>
> **Domain expert:** Not necessarily. That label is a Workflow Projection of Ready for Merge; Merged is a separate terminal Workflow State.
>
> **Developer:** Does a merge conflict invalidate a successful Watchdog Review?
>
> **Domain expert:** No. The Agent Worker resolves conflicts from its bounded pre-Audit target merge before review. A later target advance does not invalidate that historical verdict; any resulting post-review integration conflict and the final merge belong to the Merge Authority.
>
> **Developer:** What does an unavailable previously reviewed revision mean for review scope?
>
> **Domain expert:** The worker performs a full review instead of focusing on changes since that revision and retains the recorded Review Count. Recreating its worktree does not erase completed reviews.
>
> **Developer:** Can I start a dependent Work Item because its blocker passed review?
>
> **Domain expert:** No. The Dependency remains unsatisfied until the blocking Work Item is Merged into the target branch.
>
> **Developer:** Where does a worker read the accepted Contract?
>
> **Domain expert:** Through the Workflow Engine, which supplies its frozen version from the Workflow Ledger. Completing work updates Phase Reports, not the Contract; durable project knowledge still belongs in the project's glossary and ADRs.
>
> **Developer:** Does the Full Gate replace Audit or Watchdog Review?
>
> **Domain expert:** No. It proves project checks only. Audit and Watchdog Review still provide the judgment the Workflow Engine cannot.
>
> **Developer:** Who completes Manual Verification?
>
> **Domain expert:** The Merge Authority. The human receives the complete obligations even when the public presentation omits private operational details; the engine does not claim to verify them.
