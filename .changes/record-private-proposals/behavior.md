# Record Private Proposals Behavior

The agreed seam is the public `skl` CLI with real local source and ledger Git repositories; control forge responses through the existing HTTP adapter. The contract pins capabilities and output meanings, not new command spellings or internal helpers. Scenarios identify required observations, not a mandatory test count or construction order.

## Rule B1: One Machine Configuration Selects the Ledger

Read `$XDG_CONFIG_HOME/skl/config.json`, or `~/.config/skl/config.json` when `XDG_CONFIG_HOME` is unset or empty. Its only setting is `ledger`, an absolute path to an existing local Git clone. Use that clone's Git remote/upstream configuration; do not provision hosting or invent configuration defaults.

```gherkin
Scenario: Resolve the configured clone without source-local settings
  Given two valid ledger clones and a Consumer Repository
  And the XDG config selects one clone while the home config selects the other
  When I accept a proposal through skl with XDG_CONFIG_HOME set
  Then its local acceptance is recorded only in the XDG-selected clone
  When I accept another proposal with XDG_CONFIG_HOME unset
  Then its local acceptance is recorded in the home-configured clone
  And neither operation writes ledger configuration into the source repository

Scenario: Refuse unusable local configuration
  Given the selected config is missing or malformed, its ledger path is relative, or that path is not a usable local Git clone
  When I request proposal acceptance through skl
  Then I receive a concrete configuration repair without acceptance or issue publication
  And skl neither falls back to forge authority nor silently chooses another ledger
```

## Rule B2: Project Identity Belongs to the Source Repository

One Project represents one source repository and is named after its remote repository name, not a checkout directory. Reuse it across checkouts/worktrees of the same repository. Reject a different repository with the same name instead of inventing an alias; require repair when source identity is unresolved or ambiguous.

```gherkin
Scenario: Reuse a Project across renamed checkouts and worktrees
  Given a checkout named local-copy resolves to the remote repository acme/widgets
  And another checkout and a worktree resolve to that same repository
  When I use skl to accept distinct proposals from those locations
  Then all proposals belong to Project widgets with the same source identity

Scenario: Refuse a repository-name collision
  Given Project widgets already belongs to acme/widgets
  When I request acceptance from other/widgets
  Then skl identifies the conflicting source identities and refuses
  And it leaves existing Contracts, source files, and forge records unchanged
```

## Rule B3: Acceptance Freezes a Complete Proposal Locally

The authoritative acceptance is a local ledger commit containing the Proposal and all its slices under the layout in A2. Record declared dependencies, planned source branch identities, and initial Ready for Implementation state without Claims; a dependency remains blocking until its Work Item is Merged. Preserve the approved Contract bytes. Validate the complete declaration before acceptance, including safe record paths, required files, distinct slice identities, and resolvable, non-self-referencing, acyclic dependencies. Do not infer accepted obligations or dependency satisfaction from forge prose or labels.

```gherkin
Scenario: Accept a single slice before a source branch exists
  Given approved intent.md and behavior.md with no warranted plan.md or tasks.md
  And a planned source branch that does not exist
  When I accept the proposal through skl
  Then the ledger commit contains the proposal records, slice state, and exact supplied Contract files
  And no optional Contract, report, decision, or archive file is scaffolded
  And source refs, source files, and registered worktrees are unchanged

Scenario: Accept a dependent multi-slice proposal
  Given approved slices foundation and feature with feature blocked by foundation
  And foundation also names an existing ledger Work Item as a blocker
  When I accept the proposal through skl
  Then both slices, their warranted Contract files, planned branches, and dependencies are accepted together
  And membership follows their shared proposal directory without a duplicate child inventory
  And the human-facing parent is a non-claimable Coordination Item with no source branch or Submission

Scenario: Refuse an invalid declaration without partial acceptance
  Given a proposal has a missing required Contract, duplicate slice, escaping record path, unknown dependency, self-dependency, or cycle
  When I request acceptance through skl
  Then skl identifies the invalid input without accepting any slice or publishing issues
  And existing source and ledger work is preserved

Scenario: Repeating acceptance cannot amend a frozen Contract
  Given a proposal has already been accepted
  When I repeat its unchanged acceptance
  Then skl identifies the existing accepted work without duplicating it
  When I reuse that identity with changed Contract content or declared relationships
  Then skl refuses replacement in place and directs renewed proposal
  And the accepted content remains unchanged
```

## Rule B4: Readback Supplies Exact Contracts Without Discovery Work

Acceptance returns usable identities and concrete public CLI readback commands. Readback provides the accepted documents, including all Manual Verification obligations, with full ledger commit IDs and the paths valid at those revisions. It works from local records without source markers, commit-subject searches, forge access, or worker navigation of the ledger. Missing exact inputs produce a diagnostic, not guessed replacement content. Default Markdown and explicit JSON convey equivalent facts, including pending and refusal outcomes.

```gherkin
Scenario: Retrieve the accepted content after other activity
  Given acceptance returned a proposal/slice identity and exact Contract references
  And another proposal was committed later and the original temporary inputs were removed
  And the forge is unavailable and the source has no artifact markers
  When I use the supplied public readback command
  Then I receive the exact accepted document contents and full commit/path references
  And human-only obligations omitted from public issue prose are still present
  And explicit JSON readback preserves the same content and facts as default Markdown

Scenario: Do not substitute for an unavailable exact reference
  Given an explicit Contract reference names a missing commit or path
  When I request that content through skl
  Then skl identifies the unavailable reference and needed repair
  And it does not return another revision, a forge body, or a source artifact as that Contract
```

## Rule B5: Initial Publication Follows Local Acceptance

Attempt ledger push and descriptive human-facing issue publication after local acceptance. A single-slice Proposal has its slice issue; a multi-slice Proposal also has a parent issue grouping its children. Issue bodies are agent-authored temporary transport, not persisted `issue.md`/`pr.md` files or CLI-generated prose. Record successful attachments and pending publication facts without treating public bodies, labels, or comments as canonical state. An ordinary push or forge failure leaves accepted work readable and the unmet effect visibly pending; it does not undo acceptance or prevent attempting the other publication surface.

```gherkin
Scenario: Publish descriptions without publishing the private worker record
  Given accepted proposal content and separate temporary descriptive issue bodies
  When the initial ledger push and issue publication succeed
  Then the configured ledger upstream receives the accepted record
  And the forge receives the supplied slice descriptions and a parent for multi-slice work
  And the ledger records their attachments but does not persist public-body files
  When someone edits those issue bodies, labels, or comments
  Then skl readback still reports the locally accepted Contracts and state

Scenario: Keep local acceptance through ordinary publication failures
  Given local acceptance succeeds
  And the ledger push is unavailable, forge authorization fails, or one child publication fails after another succeeds
  When the initial publication attempts finish
  Then skl reports accepted work separately from the pending effects and their causes
  And successful issue attachments remain recorded and the exact Contracts remain readable
  And another publication surface is still attempted when its inputs are available
  And repeating acceptance does not recreate known attached issues

Scenario: Do not guess after uncertain issue creation
  Given issue creation may have succeeded but no reliable attachment was recorded
  When skl cannot safely establish whether publication completed
  Then it retains the local acceptance and reports the unresolved publication for repair
  And it does not claim success or blindly create a duplicate
```

## Rule B6: Competing Ledger History Is Not an Outage

Unexpected competing upstream history requires explicit reconciliation, not ordinary pending-outage treatment. Preserve local accepted results and readback; never automatically merge/rebase workflow decisions or overwrite either history. Diagnose observed competition before any new work grant. This slice grants no work, as specified in B8.

```gherkin
Scenario: Detect competing history during replication
  Given a proposal has been accepted locally
  And another clone has pushed competing workflow history to the configured upstream
  When skl attempts ledger replication
  Then it reports reconciliation required distinctly from an unavailable remote
  And it preserves the local acceptance and remote history without merging, rebasing, or force-pushing
  And readback remains available while no new work grant is authorized

Scenario: Refuse unsafe local mutation without absorbing unrelated edits
  Given ledger edits or an interrupted local write prevent safe acceptance
  When I request acceptance through skl
  Then skl gives a concrete repair/refusal without claiming acceptance succeeded
  And it neither discards those edits nor includes unrelated work in an acceptance commit
```

## Rule B7: Propose Authors Read-Only Private Contracts

Propose prepares the accepted proposal content and read-only Contract files for CLI intake, not source `.changes` edits, branch/worktree preparation, or baseline/completion commits. Give independently tracked commitments descriptive local labels B<n>, A<n>, warranted T<n>, and human-owned M<n>; do not number every paragraph or duplicate an identity across documents. Tasks remain optional. Durable project knowledge remains in project documentation. Preserve #53's approved-contract fidelity behavior and ADR 0005's many-to-many verification policy.

```gherkin
Scenario: Retrieve coherent target intake guidance
  Given the distributed Propose definition and its artifact resources
  When I retrieve them through the public skl instruction/resource interface
  Then they explain preparing frozen Contracts, planned branches, temporary issue descriptions, and local acceptance/readback
  And they keep accepted files read-only rather than ticking completion boxes
  And they use descriptive local Contract Item labels without a numbering service or mandatory tasks file
  And they do not require source branches, source artifact commits, or one test per scenario
  And intake does not depend on invoking deferred cleanup or execution operations
```

## Rule B8: Intake Is Useful Before Execution Is Supported

Until `run-ledger-delivery` supplies the complete execution path, accepted ledger work is readable but not executable through legacy Implement/Watchdog paths. Return an explicit unsupported-operation explanation rather than a Claim, execution instructions, or a misleading empty queue. Missing configuration or records must not trigger forge-authoritative fallback. Preserve existing non-adopted work without importing, reinterpreting, or cleaning it up; do not ship a permanent legacy mode.

```gherkin
Scenario: Use intake without entering unsupported delivery
  Given a newly accepted ledger slice and no run-ledger-delivery support
  When I request its Contract through skl
  Then readback succeeds with its recorded dependencies and planned source branch
  When I request implementation or Watchdog work through the active CLI
  Then skl explains that ledger delivery is not supported yet
  And it grants no Claim, returns no worker execution packet, and creates no source branch or worktree

Scenario: Preserve non-adopted work without forge fallback
  Given existing source artifacts, unmerged branches, dirty worktrees, and forge issues absent from the ledger
  When I use the new intake path or request unsupported workflow mutation or cleanup
  Then unrelated work is left intact and unsafe or unsupported mutation is refused
  And forge records are not selected as authoritative substitutes or automatically adopted
  And any adoption guidance requires a human-directed administrative cutover with normal workers stopped
```
