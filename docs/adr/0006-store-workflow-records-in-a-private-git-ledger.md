# Store workflow records in a private Git ledger

Use one private Git Workflow Ledger as the authoritative record of Contracts, Workflow State, Phase Reports, and Human Decisions across projects. The local committed record supports autonomous implementation and review; forge issues, PR bodies, and comments become human collaboration surfaces rather than the worker-sharing layer. This separates successful work from forge availability and keeps intermediate worker material out of public project histories. Reuse Git's revision identities, history, and replication instead of adding SQLite, Jujutsu, a separate event journal, or a result-ID allocation mechanism. Source code stays in its Consumer Repository rather than being mirrored into the ledger.

## Status

Accepted after Explore confirmation, including the requested report-metadata schema versioning. Implementation is pending. This record does not change active Skill Definitions, running workflow operations, or the obligations of previously published Contracts.

On adoption, this supersedes ADR 0002's forge-authoritative records, best-effort Claim and worktree-private checkpoint choices, and ADR 0003's source-tree contract retirement and endpoint machinery. It refines ADR 0005's artifact naming and completion representation while preserving contract-grounded verification and independent judgment. ADR 0001's accepted #44 execution-specific instruction and output boundaries remain in force.

## Authority and setup

- One Project corresponds to one source repository and uses its repository name, not the local checkout name. Distinct repositories with the same name are rejected rather than assigned generated aliases. Separate checkouts and worktrees of one repository belong to the same Project.
- Configure one shared local ledger clone in `$XDG_CONFIG_HOME/skl/config.json`, falling back to `~/.config/skl/config.json`. Initially the only setting is `ledger`, an absolute local path. Reuse the clone's Git remote and upstream configuration; hosting is provisioned outside `skl`. Do not add settings solely for possible future needs.
- Normal workers access ledger content only through `skl`. They receive task facts, content, and concrete commands, not responsibility for ledger navigation, persistence, or bookkeeping. The CLI supplies whatever it can determine and binds known arguments; execution instructions explain only the values workers must establish later.
- Git history is append-only by operating convention, not an enforced archival or anti-rewrite guarantee. Remote push is attempted after authoritative ledger writes but may remain pending during outages. Unexpected competing remote history is different from unavailability: preserve local results and require explicit reconciliation before granting new work, rather than automatically merging or rebasing competing workflow decisions.
- Coordination is single-machine for now. Git provides replication, not a distributed ownership protocol. Historical source-code revisions need not be retained after squash merges, rebases, or cleanup; reports keep the actual revisions used even when those objects are no longer available. This is a readable execution history, not a guaranteed reproducible code archive.

## Record layout

```text
projects/<repository-name>/
  project.json
  proposals/<proposal>/
    proposal.json
    proposal.md
    <slice>/
      state.json
      intent.md
      behavior.md
      plan.md                 # when warranted
      tasks.md                # when warranted
      implement-report.md     # once produced
      watchdog-report.md      # once produced
      decision.md             # when needed
  archive/<proposal>/
    ...
```

- Proposals are the grouping mechanism; a multi-slice proposal has a human-facing parent issue. Slices stay with their proposal. Do not add milestones or duplicate child inventories when the layout already determines membership. Files are created when needed rather than scaffolded empty.
- `state.json` holds current lifecycle, Claim, dependency, source/forge attachment, and pending-publication information. Git history preserves state changes. Phase reports hold outcomes and evidence; no `results.jsonl`, per-result directory, or separate transition stream is required.
- Current report files contain the latest result, with prior versions preserved by Git. A reference identifies a document at an exact commit and path, not necessarily the commit that introduced it. Store full commit IDs and abbreviate them for display. Ordinary operations use current records and explicit references rather than discovering identity through Git history or commit-subject searches.
- Ledger references retain the path valid at the referenced revision, so archiving does not invalidate historical inputs. Source references identify the relevant code revisions separately from ledger revisions. Fixed-item validation remains local to the selected work and required evidence, without rebuilding unrelated project history.

## Report metadata

- Phase reports use YAML frontmatter and a common `outcome` field. The `source` namespace identifies source-repository revisions; `ledger` identifies consumed Contracts, implementation/watchdog reports, and Human Decisions using explicit commit/path references. Watchdog records its review `round`; reviewed code and any permitted post-review final code remain distinguishable.
- Use `go.yaml.in/yaml/v3`, initially pinned to `v3.0.5`, for YAML encoding and decoding. This is the YAML organization's stable, security-maintained branch, with no third-party module dependencies; at selection, v4 remains a release candidate. Extract the leading frontmatter with small standard-library delimiter handling, decode into schema-specific types, and preserve the Markdown body unchanged. No Markdown parser, general frontmatter framework, or custom YAML parser is needed.
- Persisted phase reports carry a small integer `schema`, initially `1`. It identifies the frontmatter format, not the report's Git revision, the review round, or the CLI release. A versioned format definition supplied by `skl` documents each field's meaning, type, requiredness, ownership, and repository context.
- Preserve the meanings and definitions of stored schema versions as the format evolves; introducing a new schema must retain reading of previously emitted schemas. `skl` writes the current schema and reads historical schemas into the facts used by current execution instructions; agents should not guess how to interpret old metadata. Unknown versions, such as a schema newer than the installed CLI understands, produce an explicit refusal rather than silent reinterpretation. Historical reports are not rewritten to upgrade their format.
- Implement schema 1 only initially. Add concrete readers/adaptation when another schema is introduced, not a speculative compatibility framework, migration service, or schema registry. Change the schema version when metadata structure or interpretation changes, not merely when report prose changes. Decoding structured metadata does not authorize interpreting prose as a completion verdict.
- The engine supplies known references and bookkeeping fields. Workers supply their judgments and evidence through the semantic operation and temporary result documents. Precise command spellings and remaining mechanical field details are implementation choices within these boundaries.

## Contracts and verification

- Contracts remain frozen after acceptance. Accepted files are read-only; reports carry progress and completion evidence. Retire completion-tick edits, separate Artifact Completion snapshots, baseline/completion marker discovery, and removal of Contracts from source trees before review. Durable project knowledge still belongs in project documentation rather than only in the private ledger.
- Contract Items use descriptive titles and locally authored labels: `B<n>` for behavior, `A<n>` for architecture, `T<n>` for warranted tasks, and `M<n>` for human verification. Number independently tracked commitments, not every paragraph or organizational heading. Do not duplicate a commitment's identity merely because several documents discuss it. No global counter, requirements database, or CLI numbering service is needed.
- The implementation report's Verification section provides the full current completion-and-evidence table, not just a delta. Agent-owned items use `complete` or `incomplete`; complete means their required outcome and applicable verification succeeded for the reported code revision. Missing entries never imply completion. Grouped references and many-to-many evidence are permitted, without prescribing one test per item. Human checks remain separate and human-owned.
- Completion declarations, coverage, conformance, and evidence quality are worker judgment, independently checked by Watchdog. `skl` validates deterministic state, Claim, reference, and outcome preconditions; it does not interpret the table or infer obligations from prose. Preserve the Full Gate, Audit, independent Watchdog, and the verification policy established by ADR 0005.
- Rename Audit's Artifacts axis to Contracts. Audit findings use `F<n>` with Standards or Contracts recorded separately as their axis; Watchdog findings retain Work-Item-local `W<n>` identities. Contract items and findings remain distinct. Historical findings keep their original identifiers.
- Debt Markers are brief, self-contained source comments explaining nonblocking debt or a potential issue. They need no PR number, finding ID, or private-ledger provenance. Useful existing public issue links are optional, not a prerequisite for recording debt.

## Execution and recovery

- Worker selection remains scoped to the current source repository or an explicit repository argument. Preserve existing lane priorities rather than adding scheduling policy. Propose records accepted work and planned branch identity; source branch/worktree preparation happens after implementation claims a slice. Resume and rework preserve existing progress. No cross-project checkout registry or automatic global source-workspace discovery is required.
- Different slices may run concurrently, with at most one active worker per slice. Serialize only brief ledger mutations, not agent reasoning, tests, or network publication. Preconditions concern the selected work, so unrelated ledger commits do not invalidate its execution.
- A local handoff records its Phase Report and state change together. Interrupted or repeated operations must not duplicate a completed review, release a later Claim, or overwrite later work. Preserve results and return a concrete repair/refusal when safe recovery cannot be established, rather than scanning arbitrary history to reconstruct every possible operation.
- Claims have no automatic expiry. Ordinary selection skips claimed slices; interrupted work requires explicit resume or release. A recorded reservation alone does not establish whether its worker is still running.
- Remove the separate `.watchdog` file. The committed watchdog report supplies the completed-review count and last reviewed code revision. Completed reviews, including Needs Human, advance the count; interrupted attempts and repeated publication do not. Keep the existing two-review automatic-rework limit and permit a passing review at any round. Human continuation does not reset the recorded count, and worktree recreation does not lose it.
- Incremental review still needs an available previous reviewed revision ancestral to the current code; otherwise review is full without resetting the count. Active operations must have their required code inputs. Source fetch/push failures alone do not prevent local work when the necessary inputs exist: use and record the last observed Integration Target revision without claiming remote freshness.
- Ready for Merge ends autonomous delivery. Human merge and final integration remain outside the engine. Observed merge and unmerged PR closure retain their Merged/Superseded bookkeeping meanings for completion and dependency eligibility. Public bodies, labels, and comments are not authoritative worker instructions or state edits.

## Human decisions and publication

- The human decision skill uses a ledger-wide inbox derived from Needs Human slices, with project filtering, triage, related-question grouping, and conversational resolution. Blocking Phase Reports contain the question, evidence, options, and recommendation; `decision.md` records the human's direction against the exact request answered. A separate decision database or persistent chat cursor is unnecessary.
- Explicitly scoped human direction is enough to record a decision and requeue the affected slice together; no mandatory second confirmation is required. Ask again for ambiguity, a changed request, or an attempted contract change. Discussion and agent recommendations are not authorization. Human directives may resolve findings within the frozen Contract, not amend its obligations.
- Wrong Contracts require renewed proposal and re-slicing. Replacement work belongs to a new Proposal; preserve merged slices, supersede abandoned slices, and retire the old Proposal once no active work remains. Retirement does not claim full delivery. No replacement-specific dependency-remapping or validation machinery is added; the ordinary requirement that blockers be Merged remains.
- Publish descriptive human-facing issues when accepted work is recorded, without making publication a local-work prerequisite. Attempt PR publication after implementation and refresh human-facing progress and delivery content thereafter. GitHub maps pre-approval work to a draft PR and Watchdog approval to ready-for-review presentation; draft handling belongs to the forge adapter, not the core state model.
- Public bodies remain temporary and reconstructible from recorded evidence. Do not add `issue.md` or `pr.md` to the ledger. If temporary content is lost, an agent prepares a new human-facing version; the CLI does not become a prose generator. Track pending publication, then publish the latest relevant view rather than replaying every missed intermediate update.
- Keep detailed worker reviews private by default. Selected actionable inline findings can be published explicitly. The complete Manual Verification obligations remain privately accessible through `skl`; public presentations may omit private operational details but cannot remove obligations from the human's review.

## Cleanup, adoption, and scope

- Archive whole Proposals through explicit cleanup when all slices are terminal and unclaimed. Preserve the distinction between fully delivered and superseded work. The existing Propose skill invokes cleanup before preparing new slices; it may also be invoked independently, but is not an automatic merge hook. Archiving does not authorize deletion of unmerged source work; retain existing safe merged-workspace cleanup separately.
- Existing work may be copied into the new ledger by a human-directed, ad-hoc administrative agent outside the normal Workflow, with normal workers stopped. This is the narrow direct-write exception. Preserve accepted obligations, known reports/state, issue/PR references, and actual code revisions; do not invent missing evidence or historical executions. Do not ship an importer or a permanent dual-authority mode for this bootstrap.
- Follow #44's specialized Markdown defaults and explicit JSON option for programmatic callers, including non-work and repair outcomes and deferred resources. The CLI is the interface. The human decision skill consumes it, while unattended loop launching and scheduling remain separate work.
- Do not add SQLite, Jujutsu, distributed coordination, an HTTP service, a daemon, a loop launcher, milestones, source-code mirroring or permanent source retention, enforced append-only history, a separate results log, persisted public bodies, or speculative migration/compatibility infrastructure.
