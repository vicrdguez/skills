# Design through annotated checkpoints and ledger experiments

A design today lives in a conversation: challenges, revisions, prototypes and comparisons vanish with the session, and the human stays synchronous throughout. Replace explore and propose with one design playbook that runs through three checkpoints, seams, modules and proposal, each a document the human annotates in an editor and approves or sends back through the engine. A challenger and, for judgment nodes, a competing candidate run before the human sees a document. Observable questions become experiments in engine-owned worktrees that leave one report in the ledger. Product questions go to the human with a recommended default. The ledger holds every revision, annotation and report, and a session can end at any checkpoint and resume from the ledger alone. Per-project trust settings decide which checkpoints need a human. This supersedes the explore and propose Procedures and refines ADR 0005's fidelity review, which the proposal phase keeps.

## Status

Accepted after the proposal checkpoint of the design-workflow Proposal; implementation is pending.

## Consequences

- A Proposal may exist in design before it has Slices. Its checkpoints are Seams Review, Modules Review and Proposal Review, each awaiting, rework or approved. Approval and rework are engine transitions; approving the proposal checkpoint accepts the recorded Slices through the one acceptance path and lint.
- Checkpoint documents live in a flat `review/` directory at the ledger root, every attribute in frontmatter, with a shipped base file so the ledger opens as an Obsidian vault. Annotations are the diff between the recorded emission and the edited file; no syntax is required, and the next revision answers every annotation as acted on, considered or dismissed with reason.
- Every node carries decision, rationale, alternatives, evidence, class and default. A contested list carries the challenger's objections and their dispositions. Every document states what the chosen candidate decided that the question did not ask.
- The seams checkpoint never auto-approves. Modules and proposal may, per project; an auto proposal checkpoint accepts the Slices at emission after lint. Judgment nodes get a panel when the project asks for it; product nodes always stop for the human.
- Experiments are evidence for decisions, never commitments. The engine creates one worktree per candidate and one for the parent, snapshots the candidates read-only for comparison, commits only `experiments/<name>.md` and removes everything else. A Contract cites the report and the engine renders it to the implementer; an ADR carries a two-line evidence summary and cites nothing outside the repository. Nothing from an experiment is copied into the Consumer Repository.
- The `work` session is a coordinator and never does challenger, candidate or experiment work itself. Launch roles candidate, challenger, experiment-candidate and experiment-parent join the harness-scoped configuration of ADR 0015; each harness launches a role as it can, as a subagent or through dispatch. Candidate and challenger have no default model: the engine knows no model families (ADR 0015), so a second family is configured deliberately; the experiment roles inherit. Launching a role through engine dispatch with wait is a later Proposal. One phase per session by default; an attended flag keeps a session waiting for the human.
- A durable design document in the repository is a product node defaulting to no; ADRs are always written in the record phase.
- Trust settings live with the project: which checkpoints need a human, whether judgment nodes get a panel, the minimum evidence rank, and whether the Watchdog must run on a different model string than the implementer.

## Evidence

The Minos layout experiment ran two candidates on a second model family under a parent that corrected both and measured; the decouple-native-construction session settled materialization and Region lifecycle in two prototype rounds, and the agent, not the human, surfaced the second axis. The human's checkpoint job is evaluating surfaced opportunities, which is why the incidental-decisions line is mandatory.
