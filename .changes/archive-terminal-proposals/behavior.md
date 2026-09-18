# Archive Terminal Proposals Behavior

These rules apply to private-ledger cleanup under ADR 0006, not the current source-tree `.changes` carrier. Named Gherkin scenarios specify observable outcomes; they do not require one test per scenario or a construction order.

## B1. Archive The Whole Proposal Only When Terminal And Unclaimed

Explicit cleanup may move a proposal only when every slice is Merged or Superseded and no slice holds a Claim. Preserve the complete directory, including proposal records, Contracts, current reports, decisions when present, attachments, and pending-publication information. Do not archive individual children of an active proposal. An unreadable or unknown required state is not terminal evidence. All-Merged means fully delivered; mixed terminal or wholly Superseded means retirement without full delivery.

```gherkin
Scenario: Archive a fully delivered proposal
  Given projects/widgets/proposals/widget has exactly two slices
  And both are recorded Merged and unclaimed
  When explicit cleanup runs for widgets
  Then the entire widget directory is at projects/widgets/archive/widget
  And it is absent from projects/widgets/proposals/widget
  And all proposal and slice documents are preserved
  And the proposal remains fully delivered

Scenario: Retire mixed terminal work without claiming full delivery
  Given widget contains one Merged slice and one Superseded slice
  And neither holds a Claim
  When explicit cleanup archives widget
  Then both slices remain together with their original terminal states
  And the proposal is not presented as fully delivered
  And no replacement is created or dependency remapped

Scenario Outline: Ineligible membership preserves the whole active proposal
  Given widget contains a Merged unclaimed slice and <other>
  When cleanup checks widget
  Then the entire proposal remains under projects/widgets/proposals/widget
  And no child files are moved into the archive
  And the reason for preservation is observable

  Examples:
    | other                                 |
    | a Ready for Merge unclaimed slice     |
    | a Needs Human unclaimed slice         |
    | a Rework unclaimed slice              |
    | a Ready for Implementation slice      |
    | an Awaiting Review slice              |
    | a terminal slice with a Claim         |
    | a slice whose required state is unknown |
```

## B2. A Path Move Changes Neither Lifecycle Nor Historical Inputs

Archiving is ledger organization, not a new lifecycle transition or a result rewrite. Stable proposal and Work Item identities remain usable at the archive location. Earlier report/Contract references retain the exact full ledger commit and the path valid at that revision; current location does not replace a historical path. Normal dependency reads can still use confirmed Merged blockers after archival; Superseded blockers remain unsatisfied. Preserve Git history rather than build a second history index or source-code archive.

```gherkin
Scenario: An earlier report reference still retrieves its original document
  Given a Watchdog report names an implementation input by a full ledger commit and projects/widgets/proposals/widget/core/implement-report.md
  And that referenced version differs from the current implementation report
  When widget is archived
  Then retrieving the stored commit/path through the existing record interface returns the original referenced bytes
  And reading the current report at the archive location returns its current bytes
  And neither reference nor report metadata has been rewritten to the archive path

Scenario: Archived blockers retain their original dependency meaning
  Given an active slice names a Work Item in widget as a blocker
  When widget is archived
  Then the same Work Item identity remains resolvable
  And a confirmed Merged blocker still satisfies that Dependency
  And a Superseded blocker still does not satisfy it
  And no history or unrelated proposal inventory scan is needed to discover identity

Scenario: Source objects are not an archive prerequisite
  Given widget's terminal facts and ledger reports are committed
  And a historical source revision named by a report is no longer available
  When widget is archived
  Then its ledger documents and exact references are preserved
  And no source mirror, source-history reconstruction, or revision substitution is attempted
```

## B3. Source Cleanup Retains Its Stronger Merge Safety Boundary

Archival alone never permits source deletion. Retain the existing merged-workspace cleanup path only for actually Merged, unclaimed work with explicit owned source attachments, a clean registered worktree at its expected owned location, and HEAD equal to the exact confirmed accepted Submission source head. A target squash SHA, similar branch name, review approval, or ancestral/partially merged code is not a substitute. Preserve source work if any required fact or Git safety check is unavailable. Never delete remote branches.

```gherkin
Scenario: Safe merged source work is cleaned separately
  Given a slice is confirmed Merged and unclaimed
  And its explicitly owned local branch is registered at its expected worktree location
  And that worktree is clean with HEAD equal to the confirmed accepted source head
  When explicit cleanup evaluates its source work
  Then the existing safe cleanup path removes the local worktree and local branch
  And its ledger reports and attachments remain available
  And no remote branch is deleted

Scenario Outline: Unsafe source work is preserved even if archival is eligible
  Given the ledger proposal is terminal and unclaimed
  And its source work has <condition>
  When explicit cleanup runs
  Then that source worktree, local branch, and contents are preserved
  And source preservation is reported separately from the archive result

  Examples:
    | condition                                                     |
    | tracked modifications or untracked files                      |
    | HEAD different from the confirmed accepted source head       |
    | unknown accepted head or unavailable required Git observation |
    | an unexpected registered worktree location                   |
    | an absent or contradictory explicit ownership attachment     |
    | a Superseded unmerged Submission                              |

Scenario: A squash merge does not require source-head ancestry
  Given a confirmed merge records accepted source head slice-head and target squash head squash-head
  And the owned clean worktree HEAD is slice-head at its expected location
  And slice-head is not an ancestor of squash-head and its upstream branch has been pruned
  When safe source cleanup runs
  Then it removes the matching local worktree and local branch
  And it does not replace the accepted source head with squash-head or require an upstream branch

Scenario: Nonterminal source work is not removed
  Given a clean owned worktree belongs to a Ready for Merge or otherwise unmerged slice
  When explicit cleanup runs
  Then that worktree and branch remain even if the code resembles changes in the Integration Target
  And no archived directory elsewhere supplies deletion authority
```

## B4. Archival And Source Cleanup Have Independent Outcomes

The ledger move must be able to succeed while source cleanup preserves or cannot remove a worktree. Conversely, a failed or ineligible archive supplies no new authority for source deletion; existing safe merged cleanup remains separately governed. Do not roll back a committed archive, erase evidence, or claim source removal merely because the other part succeeded. No cross-repository transaction or source-work archive mirror is required.

```gherkin
Scenario: A dirty source worktree does not prevent archival
  Given widget is terminal and unclaimed
  And its merged slice's owned worktree contains uncommitted work
  When explicit cleanup runs
  Then widget is committed at its archive location
  And the dirty worktree and branch remain unchanged
  And the result distinguishes archived records from preserved source work

Scenario: Source removal failure does not undo a ledger archive
  Given widget is terminal and unclaimed and the ledger move commits
  When an otherwise safe source worktree removal fails
  Then the committed archive remains available
  And the source cleanup failure is reported without claiming that removal succeeded
  And retry does not move or discard the proposal again

Scenario: Safe merged cleanup need not retire active siblings
  Given widget has one safely cleanable Merged slice and one active slice
  When explicit cleanup runs
  Then widget remains in the active proposals directory
  And the already safe Merged source work is cleaned under the existing rules
  And the active slice's source work and records remain untouched
```

## B5. Retried Or Interrupted Moves Preserve One Complete Proposal

Use the predecessor's guarded, serialized ledger mutation boundary to recheck the selected proposal's membership, lifecycle, Claims, and destination before movement. A whole-directory move must not degrade into independently disappearing files. A matching already archived proposal is a harmless repeat, but a distinct destination or ambiguous partial move requires explicit repair without overwrite or deletion. Unrelated ledger commits alone do not invalidate eligible cleanup. Recognize completed local work before retrying; do not silently sweep an unexplained partial tree into success.

```gherkin
Scenario: Retry after a committed archive is harmless
  Given cleanup already committed widget at its archive location
  And the command response was interrupted
  When cleanup is repeated
  Then the same complete archived proposal remains
  And there is no second proposal copy, lifecycle change, or report rewrite
  And the absent original path is not treated as permission to delete unrelated work

Scenario: A distinct archive destination is never overwritten
  Given projects/widgets/proposals/widget is eligible
  And projects/widgets/archive/widget already contains a different record
  When cleanup attempts to archive widget
  Then it reports the collision for repair
  And both directories and all their documents remain intact

Scenario: A later Claim prevents a stale archive decision
  Given cleanup has read widget as terminal and unclaimed
  When a Claim or other relevant selected-proposal precondition changes before the guarded move
  Then cleanup does not archive from its stale snapshot
  And it preserves the later record and reports the changed precondition

Scenario: Move or commit failure does not silently lose files
  Given widget is eligible for archival
  When the directory move or ledger commit is interrupted or fails
  Then the complete proposal remains recoverable at the original or archive location
  And no successful committed archive is reported without a confirming ledger commit
  And retry either safely finishes the identified move or refuses with a concrete repair explanation
  And it does not discard a partial tree, overwrite a distinct archive, or rewrite unrelated ledger work
```

## B6. Cleanup Is Explicit And Uses Ordinary Ledger Semantics

The public cleanup operation is callable independently. Propose invokes it before preparing new slices, preserving reported refusal/repair conditions rather than bypassing them. Status, observed merge, and Watchdog approval do not automatically archive. Cleanup consumes already recorded terminal facts without requiring live forge confirmation or successful issue, PR, comment, or label publication. Reuse normal local commit and attempted ledger push behavior: unavailability leaves committed local work with pending replication; competing remote decisions require explicit reconciliation under predecessor policy, not an automatic merge/rebase.

```gherkin
Scenario: Cleanup works without a new proposal or sibling interfaces
  Given a proposal completed through normal local delivery and confirmed human completion
  And every slice is terminal and unclaimed
  And forge publication remains pending and decision UI and catch-up are not present
  When the user invokes the public cleanup operation without proposing new work
  Then the proposal is archived using its stored facts
  And pending-publication and attachment information are retained
  And no successful remote publication is required

Scenario: Propose calls cleanup before preparation
  Given the user is preparing a new proposal
  When Propose supplies the preparation workflow
  Then explicit cleanup is invoked before preparing the new slices
  And its archive, preserved-source, and repair outcomes are not bypassed
  And observing a merge or passing Watchdog alone never invokes archive movement

Scenario: An offline ledger push does not reverse archival
  Given widget's archive move commits successfully in the local ledger
  When the ordinary ledger push attempt is unavailable
  Then the archive remains authoritative locally and pending replication is reported
  And a retry does not restore widget to the active proposals directory
  And a later competing remote history is preserved for explicit reconciliation rather than automatically merged

Scenario Outline: Later sibling output uses ordinary records
  Given <later output> is recorded through an ordinary ledger mutation
  And the current proposal is terminal and unclaimed
  When explicit cleanup evaluates the current proposal records
  Then the same guarded archive rules apply
  And cleanup does not invoke either sibling interface or require replacement-specific validation

  Examples:
    | later output                                               |
    | authorized human direction leaving the proposal terminal    |
    | a recovered attachment or pending-publication update        |
```
