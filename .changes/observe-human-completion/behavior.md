# Observe Human Completion Behavior

The rules use ADR 0006's private ledger authority and CONTEXT.md's state meanings. Scenario names distinguish binding observations, not required test functions or implementation order. Example PR numbers and revision names are fixture identities, not prescribed storage fields or command syntax.

## B1. Record Actual Completion Of The Explicitly Owned Submission

The ledger's explicit Work Item, Submission, source repository, and Integration Target attachments determine what may be observed. A successful forge observation of that owned Submission merged into that target records Merged; confirmed closure without merge records Superseded. Missing, inconsistent, or inaccessible identity evidence is not terminal evidence. Titles, body footers, issue closure, labels, comments, and other PRs cannot establish or replace ownership.

```gherkin
Scenario: Human merge becomes durable bookkeeping
  Given slice delivery owns PR 21 in acme/widgets targeting acme/widgets main
  And its ledger state is Ready for Merge
  When status observes PR 21 merged into that Integration Target
  Then the ledger records the confirmed merge and delivery is Merged
  And a subsequent local status read reports Merged
  And the engine has not requested a forge merge or changed source code

Scenario: Unmerged closure is abandonment rather than delivery
  Given slice delivery owns PR 21 in acme/widgets targeting acme/widgets main
  When the forge confirms that PR 21 is closed and not merged
  Then the ledger records delivery as Superseded
  And its Contract, phase results, and source attachments remain available
  And no dependency is satisfied by that closure

Scenario Outline: Unrelated or incomplete evidence cannot terminate the selected slice
  Given the ledger attaches delivery to PR 21 in acme/widgets targeting acme/widgets main
  When the available evidence is <evidence>
  Then that evidence does not record delivery as Merged or Superseded
  And an attachment mismatch or incomplete required observation is reported for inspection
  And the engine does not search for or attach a substitute Submission

  Examples:
    | evidence                                                        |
    | a merged PR 22 with the same title and a body mentioning delivery |
    | PR 21 returned with a different or unknown source repository     |
    | PR 21 merged into a different or unknown target repository       |
    | PR 21 merged into release instead of the attached main target    |
    | an issue marked closed with a done label but no PR merge fact    |
```

## B2. Ready For Merge Ends Engine Delivery Ownership

Approval is not merge. Mergeability, target movement, public instructions, and failed publication do not revoke recorded approval, trigger integration, or create conflict-driven Rework. This observation path does not decide or carry out human integration and does not recover phase handoffs from public content.

```gherkin
Scenario: A conflicting open PR remains ready for human integration
  Given delivery has a committed passing Watchdog report and is Ready for Merge
  And its owned PR is open
  When status observes a moved target and conflicting or unknown mergeability
  And a public comment asks the engine to rebase and restart review
  Then delivery remains Ready for Merge
  And the report and completed-review count are unchanged
  And no integration, merge, new Claim, or Rework operation is initiated

Scenario: No published Submission is not a terminal result
  Given local delivery reached Ready for Merge while normal PR publication failed
  And no exact owned Submission attachment was recorded
  When status is requested
  Then it presents the stored Ready for Merge state and the missing observation capability
  And it does not infer merge or abandonment from missing public records
  And it does not require publication recovery or a human decision UI to present local facts
```

## B3. Confirmed Facts Survive Unavailability Without Inventing Certainty

Previously committed terminal evidence remains available without a fresh forge or source fetch. A timeout, access denial, missing remote record, malformed response, or other unavailable observation is neither merge nor unmerged closure. Distinguish a stored fact from an unavailable refresh, and an unknown blocker from a satisfied one. Local recording and replication use the predecessor's ledger mutation and pending-push semantics.

```gherkin
Scenario: Confirmed merge is usable while offline
  Given the ledger already records delivery Merged with confirmed owned-Submission evidence
  And the forge and ledger remote are unavailable
  When status and normal dependency selection read that record locally
  Then the confirmed Merged fact remains visible and can satisfy that blocker
  And no fresh remote confirmation is required to use it
  And no remote freshness is claimed

Scenario: An unavailable unknown blocker remains unsatisfied
  Given dependent names blocker as a Dependency
  And blocker has no confirmed Merged evidence in the ledger
  When required observation of blocker's owned Submission fails or is unavailable
  Then blocker does not become Merged or Superseded
  And dependent is not claimed or started on the strength of that observation
  And the unresolved observation is reported rather than presented as known completion

Scenario: Ledger replication failure does not erase a recorded merge
  Given the forge confirms the exact owned Submission merged
  When the terminal ledger mutation commits locally but its remote push is unavailable
  Then the local merge fact remains authoritative and readable
  And pending replication is reported under the existing ledger policy
  And repeating observation does not manufacture another phase result
```

## B4. Observation Cannot Clobber Claims, Results, Or Terminal Work

Terminal bookkeeping uses the predecessor's brief serialized, selected-record mutation boundary. Revalidate the selected identity, attachments, lifecycle, Claim, and relevant result references before committing an observation. Changes to those preconditions require safe refusal or re-observation, not a stale overwrite. An unrelated ledger commit alone is not a conflict. Observation is neither a phase handoff nor Claim release; retain Claims for explicit existing recovery. A retry must recognize an already recorded fact and must not replay earlier workflow states over it.

```gherkin
Scenario: A later Claim and phase result beat a stale observation
  Given status has read delivery and begun observing its attached PR
  When delivery acquires a later Claim or records a later phase result before the observation write
  Then the stale write does not overwrite that Claim, result, or current state
  And the operation reports the changed preconditions instead of claiming reconciliation succeeded

Scenario: Confirming external completion is not Claim release
  Given delivery still has a recorded Claim
  And its exact owned PR has been merged outside the engine
  When a completion observation is recorded against unchanged selected-record preconditions
  Then the confirmed terminal fact is available
  And the existing Claim and phase reports are preserved
  And a stale worker handoff cannot move the terminal item back into a work queue

Scenario: Retry after an interrupted response preserves terminal work
  Given a completion observation committed delivery as Merged or Superseded
  And the command response was interrupted after that commit
  When observation is retried or older open-PR evidence arrives
  Then the recorded terminal outcome is not replaced by an earlier state
  And no Claim is released and no report or review count is rewritten
  And contradictory evidence requires explicit inspection rather than resurrection

Scenario: Unrelated work does not invalidate an observation
  Given delivery's selected-record preconditions remain unchanged
  When another slice commits its own result while delivery's forge read is in progress
  Then that unrelated ledger commit alone does not prevent recording delivery's confirmed completion
```

## B5. Only Actual Merge Satisfies Normal Dependencies

Selection consumes the ledger's confirmed Merged evidence for each explicitly named blocker. Ready for Merge, Superseded, issue closure, publication success, and an inaccessible blocker are insufficient. Preserve existing lane priority and Claim rules. Do not substitute replacement work or add replacement-specific validation.

```gherkin
Scenario Outline: The named blocker controls normal eligibility
  Given dependent is otherwise eligible and unclaimed
  And dependent explicitly names blocker as its only Dependency
  When normal selection sees blocker as <condition>
  Then dependent <eligibility>

  Examples:
    | condition                              | eligibility                      |
    | confirmed Merged                       | is eligible under existing rules |
    | Ready for Merge with a passing report  | remains blocked                  |
    | Superseded after unmerged PR closure   | remains blocked                  |
    | unconfirmed while observation is down  | remains blocked                  |
    | Superseded with different work Merged  | remains blocked                  |
```

## B6. Full Delivery And Terminal Retirement Are Different

A Coordination Item is fully delivered only when every child is Merged. Mixed Merged/Superseded outcomes, or all Superseded outcomes, may make the whole proposal retireable once no child is active or claimed, but never establish full delivery. This slice reports that distinction; moving a proposal is owned by #6. Public parent-issue closure is not evidence of completion.

```gherkin
Scenario Outline: Child facts determine the parent account
  Given a proposal has exactly children first and second
  And both children are unclaimed
  When status accounts for <first> and <second>
  Then the proposal is reported as <account>

  Examples:
    | first      | second          | account                              |
    | Merged     | Merged          | fully delivered                      |
    | Merged     | Superseded      | terminal but not fully delivered     |
    | Superseded | Superseded      | terminal but not fully delivered     |
    | Merged     | Ready for Merge | not yet terminal or fully delivered  |
    | Merged     | unknown         | not yet terminal or fully delivered  |

Scenario: Terminal facts do not hide a retained Claim
  Given every child is terminal but one child retains a Claim
  When status accounts for the proposal
  Then the Claim remains visible and the proposal is not reported as safe to retire
  And a closed public parent issue does not change that result
```

## B7. Fixed-Item Observation Stays Local To Required Evidence

Explicit item reads and blocker checks use the selected record, its exact attachments and references, and the required blocker evidence. They do not enumerate repository-wide issues, PRs, timelines, source history, ledger history, or unrelated proposal contents to rediscover identity. Project status may enumerate current project records for its requested summary; parent accounting uses that parent's children, not a forge-wide inventory. Later sibling-produced decisions or publication attachments use the same boundaries.

```gherkin
Scenario: One item's blocker check does not rebuild the project
  Given the selected item names only blocker as a Dependency
  And unrelated proposal records or forge inventory endpoints are inaccessible
  When the selected item's required local evidence and exact blocker evidence are available
  Then its fixed-item validation and blocker check can complete
  And no unrelated record hydration or repository-wide history or forge inventory request occurs

Scenario: A later publication attachment uses the same observation path
  Given delivery was locally Ready for Merge without a Submission attachment
  And a later ordinary ledger mutation records its exact owned Submission attachment
  When the forge confirms that attached Submission merged
  Then the normal observation path can record Merged using the current guarded record
  And no decision UI, catch-up interface, or special recovery-only observation path is required
```

## B8. Merge Evidence Does Not Rewrite Phase Evidence

Keep the confirmed accepted Submission source head distinct from the target's merge or squash revision and from revisions actually implemented or reviewed. Store available merge facts with their repository/target context without substituting them into schema-1 reports or historical ledger references. Human integration may change the merged source head; observing that merge neither claims it was the earlier reviewed result nor triggers engine rework. Terminal observation does not require historical source objects to remain available.

```gherkin
Scenario: Squash merge preserves actual execution inputs
  Given implementation and Watchdog reports record source head reviewed-head and exact ledger commit/path inputs
  And a human integrates further changes and the forge confirms accepted source head human-head merged as squash-head
  When completion is recorded
  Then Merged evidence distinguishes human-head from squash-head and the Integration Target
  And the reports still name reviewed-head and their original ledger references unchanged
  And no verification of the human integration result is invented

Scenario: Missing historical source objects do not erase completion
  Given the ledger contains confirmed merge evidence and reports for a no-longer-available source revision
  When status or dependency selection reads the stored facts
  Then the terminal fact and report references remain readable
  And the engine does not fetch arbitrary history, mirror source code, or rewrite the unavailable revision
```
