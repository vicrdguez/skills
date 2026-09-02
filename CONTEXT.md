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
The deterministic operation that validates a Consumer Repository, installs the minimal agent-facing guidance, and prepares its inferred Workflow Backend.
_Avoid_: Agent skill, workflow definition

**Adoption**:
Recognition of an existing Work Item created by the former prose-driven workflow without rewriting it first. Unambiguous projections continue normally; contradictions enter **Needs Human**.
_Avoid_: Bulk migration, new proposal

**Workflow Definition Repository**:
The repository that owns and versions the single supported **Workflow**, its mechanics, and its agent behavior. It may distribute that workflow to many **Consumer Repositories**.
_Avoid_: Consumer repository

**Consumer Repository**:
A code repository in which the distributed **Workflow** coordinates software changes. It consumes one workflow rather than defining a custom one.
_Avoid_: Workflow definition repository

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

**Skill Stub**:
A discoverable filesystem representative of one **Skill Definition**, installed in each supported harness's user skill directory. It delegates instruction retrieval without duplicating behavior or requiring a harness plugin package.
_Avoid_: Skill definition, harness adapter

**Skill Resource**:
Named supporting material belonging to a **Skill Definition** and retrieved only when that behavior requires it.
_Avoid_: Inline prompt context

**Instruction Packet**:
The concrete instructions and structured facts produced for one skill invocation. Its machine-readable and rendered forms express the same information, and its manifest ensures every bundled Skill Definition is loaded at most once.
_Avoid_: Skill definition, model-generated prompt

**Result Document**:
Ephemeral, template-guided Markdown written by an Agent Worker for the Workflow Engine to publish as an opaque backend projection. It lives in a private temporary directory rather than the Consumer Repository; the engine does not parse or judge its prose.
_Avoid_: Implementation ledger, durable project knowledge

**Proposal**:
An approved definition of one change materialized as one or more **Work Items**. A multi-slice Proposal also has one **Coordination Item** and explicit Dependencies between its Work Items.
_Avoid_: Work item, implementation ledger

**Work Item**:
One implementation slice whose identity remains stable from publication through merge or supersession. Backend issues, submissions, branches, worktrees, and active artifacts are attachments or projections of it.
_Avoid_: Agent session, issue, pull request

**Submission**:
The proposed code changes attached to one **Work Item** for independent review and human merge.
_Avoid_: Work item, claim

**Dependency**:
A relationship in which one **Work Item** cannot become eligible until every blocking Work Item is **Merged**.
_Avoid_: Ready-for-merge prerequisite

**Claim**:
The V1 best-effort reservation of a **Work Item** by one **Agent Worker**, projected as `wip` under a single-operator assumption. Interrupted work resumes by Work Item identity, while ordinary queue selection skips it until successful handoff, explicit abandon, or administrative release.
_Avoid_: Atomic lease, ownership guarantee

**Claim Token**:
The optional durable identity of a future exclusive **Claim**. V1 does not require one because concurrent claim attempts are outside its supported operating model.
_Avoid_: V1 `wip` projection, operation identifier

**Transition Operation**:
A durable attempt to move workflow state toward one valid target state. Repeating the same operation reconciles completed steps and safely resumes any remaining work.
_Avoid_: Shell command, rollback transaction

**Work Start**:
A **Transition Operation** that selects and claims one eligible **Work Item** and returns an **Instruction Packet** describing the existing Git preparation the Agent Worker must perform.
_Avoid_: Read-only queue lookup, instruction rendering

**Target Snapshot**:
The immutable target-branch commit observed when implementation or Synchronization Rework starts. The resulting Submission must contain it; later movement of the target branch does not silently change the active obligation.
_Avoid_: Latest target branch, artifact baseline

**Ready for Merge**:
The **Workflow State** in which review has passed and only the human-owned merge remains. It is distinct from **Merged**.
_Avoid_: Done, merged

**Needs Human**:
The paused **Workflow State** for a decision automation cannot make. It retains the state to resume and any existing **Submission** so a human resolution returns it to the correct queue.
_Avoid_: Failed, abandoned

**Synchronization Rework**:
Repair required when a Submission no longer merges cleanly into its target branch. It requires fresh validation and Watchdog Review but does not consume the review-failure bounce allowance.
_Avoid_: Review failure, human merge

**Merged**:
The terminal **Workflow State** reached when the accepted code change has actually entered its target branch.
_Avoid_: Ready for merge, approved

**Superseded**:
The terminal **Workflow State** of an abandoned Work Item whose replacement requires renewed exploration and proposal.
_Avoid_: Needs human, merged

**Merge Authority**:
The human who decides and performs the merge outside the **Workflow Engine** after a change reaches **Ready for Merge**.
_Avoid_: Reviewer, agent worker

**Manual Verification**:
A human-owned check copied from the Artifact Baseline into the Submission at **Ready for Merge**. The Merge Authority decides whether it is satisfied.
_Avoid_: Validation gate, agent-verifiable scenario

**Human Finding Directive**:
An authorized human comment that gives a Watchdog Worker a disposition for an existing Review Finding. The Workflow Engine supplies these comments to the worker without replacing its judgment.
_Avoid_: Validation result, inferred intent

**Coordination Item**:
A non-claimable parent that groups multiple child Work Items. It has no branch or Submission and completes when every child is **Merged**.
_Avoid_: Work item, implementation slice

**Implementation Ledger**:
The ephemeral Markdown contract used while implementing a **Work Item**. It is removed before review and is not durable project knowledge.
_Avoid_: Capability documentation, archived plan

**Artifact Baseline**:
The immutable Git snapshot containing the initially accepted **Implementation Ledger**.
_Avoid_: Review head, mutable plan

**Artifact Completion**:
The immutable Git snapshot recording permitted completion ticks immediately before the **Implementation Ledger** is removed.
_Avoid_: Review head, artifact archive

**Full Gate**:
The Consumer Repository's complete test, typecheck, and lint verification run by implementers and watchdog reviewers. It remains Agent Worker behavior rather than a Workflow Engine operation.
_Avoid_: Audit, workflow invariant

**Post-Marker Check**:
The narrow check Watchdog performs after adding Debt Markers: record the final head, run `git diff --check`, and run the existing formatter or parser check for touched files. It does not rerun the Full Gate or Audit and does not parse markers with regular expressions.
_Avoid_: Full gate, audit

**Audit**:
Implementation-phase agent judgment invoked by the implementer to find standards and artifact-conformance problems before submission. It has no independent Workflow State or Claim.
_Avoid_: Validation gate, watchdog review

**Watchdog Review**:
Independent agent judgment performed in a fresh Worker Session after submission. It may change workflow disposition and may add only permitted **Debt Markers** to the Submission.
_Avoid_: Audit, validation gate

**Review Finding**:
A watchdog observation with a stable identity, disposition, evidence, review round, and reviewed commit. Findings form the human-readable ledger that drives rework and human decisions.
_Avoid_: Validation failure, free-form comment

**Debt Marker**:
A non-functional source comment linked to a nonblocking **Review Finding** so deferred work remains visible near the affected code.
_Avoid_: Functional code change, blocking finding

**Worker Session**:
One fresh **Agent Worker** context dedicated to one **Work Item**. A **Harness Adapter** may start successive sessions while the **Workflow Engine** continues to report eligible work.
_Avoid_: Queue, workflow

**Local Backend**:
A **Workflow Backend** that supports a complete workflow without a hosted forge.
_Avoid_: Local Git helper, GitHub cache

## Example dialogue

> **Developer:** Can the Codex harness choose a different ready issue than Pi?
>
> **Domain expert:** No. The Workflow Engine applies the same Workflow Mechanics to the same Workflow Backend state. The Harness Adapter only gives each Agent Worker access to that decision.
>
> **Developer:** Who decides whether the implementation is maintainable?
>
> **Domain expert:** The Agent Worker does; maintainability requires judgment. The Workflow Engine owns only deterministic eligibility, invariants, and transitions.
>
> **Developer:** What happens when two Worker Sessions request the same Work Item?
>
> **Domain expert:** V1 does not promise atomic exclusion between concurrent sessions. Under its single-operator model, the first observed `wip` projection causes ordinary queue selection to skip that Work Item; atomic Claim Tokens are a possible later extension.
>
> **Developer:** Does the Claude Skill Stub define different behavior from the Pi one?
>
> **Domain expert:** No. Both expose the same Skill Definition and retrieve equivalent Instruction Packets; only their activation metadata is harness-specific.
>
> **Developer:** GitHub shows the `done` label. Is the Work Item Merged?
>
> **Domain expert:** Not necessarily. That label is a Workflow Projection of Ready for Merge; Merged is a separate terminal Workflow State.
>
> **Developer:** Can I start a dependent Work Item because its blocker passed review?
>
> **Domain expert:** No. The Dependency remains unsatisfied until the blocking Work Item is Merged into the target branch.
>
> **Developer:** Where is the accepted contract after the Implementation Ledger is deleted?
>
> **Domain expert:** The Workflow Engine resolves the Artifact Baseline and Artifact Completion snapshots for review; durable project knowledge belongs in the glossary, ADRs, and capability documents.
>
> **Developer:** Does the Full Gate replace Audit or Watchdog Review?
>
> **Domain expert:** No. It proves project checks only. Audit and Watchdog Review still provide the judgment the Workflow Engine cannot.
>
> **Developer:** Who checks a Manual Verification box?
>
> **Domain expert:** The Merge Authority. The Workflow Engine carries it into the Submission but does not claim to verify it.
