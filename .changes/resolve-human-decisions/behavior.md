# Resolve Human Decisions: Behavior

These rules implement ADR0006's Human Decision boundary while preserving ADR0005's independent judgment and construction freedom. Operation descriptions below are semantic interfaces, not new command spellings. Example request names stand for full ledger commit/path references, with source revisions identified separately.

## B1: Discover Current Requests Without Source Checkouts

The Decision Inbox is derived from current Needs Human Work Items in the machine-configured Workflow Ledger. Its default scope is every Project, including from inside one Project's checkout; an explicit Project filter narrows it. Use the existing `$XDG_CONFIG_HOME/skl/config.json` configuration, falling back to `~/.config/skl/config.json`, without discovering or registering source workspaces. Inbox access neither claims work nor changes state.

```gherkin
Scenario: Read across Projects from a non-repository directory
  Given the configured ledger contains atlas/repair and beacon/upgrade in Needs Human
  And atlas/ready is Ready for Implementation
  And neither source checkout is available
  When the operator reads the Decision Inbox through skl from a non-repository directory
  Then atlas/repair and beacon/upgrade are listed with their Project and Proposal identities
  And atlas/ready is not listed
  And no source checkout or forge authentication is required
  And no Claim or Workflow State changes

Scenario: Filter explicitly rather than by working directory
  Given the same two Needs Human requests
  When the operator reads the inbox from atlas's source checkout without a filter
  Then both requests are listed
  When the operator selects Project beacon
  Then only beacon/upgrade is listed
  And an unknown Project is reported rather than silently broadening the scope

Scenario: Distinguish an empty inbox from unavailable configuration
  Given a readable configured ledger has no current Needs Human requests
  When the inbox is read
  Then skl reports an empty inbox without creating work
  When the configured ledger cannot be resolved or read
  Then skl reports the configuration or access problem instead of an empty inbox
```

## B2: Present Questions for Human Triage

Each request identifies its Work Item and blocking Phase Report at an exact commit/path, along with the accepted commitment, conflict or decision needed, relevant evidence, options and consequences, and recommendation. The conversational Skill may triage and group related questions but preserves each request's identity and differences. Blocking questions remain in Phase Reports; a question is not a Human Decision.

```gherkin
Scenario: Group related questions without hiding different obligations
  Given atlas/repair and beacon/upgrade ask about the same mandatory rule
  And their blocking reports cite different Contract Items and offer different options
  When the conversational Skill's invocation retrieves both through skl
  Then its output supplies both exact request references and their separate commitments and conflicts
  And it includes the recorded evidence, options, consequences, and recommendations
  And its instructions permit related-question grouping but require preserving item-specific differences
  And any missing request detail is identified for clarification rather than invented
  And no decision.md is recorded by reading or grouping the requests
```

## B3: Human Direction, Not Conversation, Authorizes Resolution

A clear human answer that names the affected requests, directly or by an unambiguous displayed selection, is sufficient authorization. The conversational Skill turns that answer into the semantic decision input; the CLI validates its explicit scope, request references, and permitted route, not whether prose sounds approving. Ask for clarification when scope or meaning is ambiguous, the request changed, or the direction attempts to change the Contract. Do not require a ceremonial second approval after a clear answer.

```gherkin
Scenario: A named answer is enough
  Given the current request for atlas/repair offers an in-contract implementation option
  When the human says "For atlas/repair, choose that option and send it to Implement"
  Then the Skill instructions authorize submitting that answer for that exact request through skl
  And skl can record the decision and route without a separate confirmation step
  And decision.md records the human's answer and answered request, not the agent's question

Scenario: Discussion and recommendations are not answers
  Given a current request and an agent recommendation
  When the human asks "Would that option work?" or discusses its trade-offs without directing continuation
  Then the rendered Skill instructions require continued discussion without a decision write
  And the same instructions forbid treating the agent recommendation as authorization
  And submitting a decision without an identified request and continuation route is refused without mutation

Scenario: A forge comment is not a ledger decision
  Given a Needs Human item has no recorded answer
  And a GitHub comment or label says to continue or waive a finding
  When the inbox and worker selection are read through skl
  Then the item remains paused
  And any such feedback is not presented as an authorized Human Decision
```

## B4: Match the Answered Request, Not the Ledger Tip

Apply direction only while the selected Work Item is still paused on the identified request and its relevant input references remain current. An unrelated ledger commit does not make that request stale. A replaced request remains different even if its prose is identical. Missing or ambiguous references and conflicting selected-item state produce a concrete refusal without overwriting newer work.

```gherkin
Scenario: Another Project commits during discussion
  Given atlas/repair is paused on request R1 with exact report and Contract references
  And the human answers R1
  And beacon/upgrade records a new result before the answer is applied
  When skl applies the answer to atlas/repair
  Then the unrelated ledger commit does not invalidate R1
  And the decision can be recorded against R1

Scenario: The selected request is replaced during discussion
  Given the human viewed request R1 for atlas/repair
  And its current blocking request is now R2, even if R2 repeats R1's question text
  When an answer naming R1 is submitted
  Then skl refuses to apply it and identifies the changed request for renewed human direction
  And R2, the current state, and any current Claim are preserved

Scenario: Another resolver already acted
  Given R1 was answered and the next worker has acquired a Claim
  When a different answer to R1 is submitted
  Then skl refuses to replace that decision or reroute the claimed work
  And the later worker's Claim and inputs are unchanged
```

## B5: Commit the Answer and Route as One Local Result

For each affected Work Item, commit `decision.md` and the associated state change together. Store the human's direction, exact answered request, and chosen continuation, leaving prior decisions available in Git history. An answer cannot exist authoritatively without its route, nor can a requeue exist without its answer. Local success does not wait for forge publication or ledger replication; preserve the existing dependency's pending-replication and competing-history rules.

```gherkin
Scenario: Record a local decision while the network is unavailable
  Given atlas/repair is paused on R1 with no active Claim
  And a clear answer directs Implement within the frozen Contract
  And forge publication and ledger remote push are unavailable
  When skl applies the decision
  Then one authoritative local commit contains decision.md and the Implement routing state
  And it references R1 without changing the accepted Contract or completed reports
  And the local result is usable with network work reported separately as pending

Scenario: Interrupt and repeat a decision operation
  Given a valid answer to R1 and its chosen route
  When the local write fails before its authoritative commit
  Then neither an answer-only nor a route-only result becomes authoritative
  When the commit succeeds but the caller loses the response and repeats the same operation
  Then skl recognizes the recorded result without recording a second decision or requeue
  And a later Claim or later result is never released, reset, or overwritten
```

## B6: Continue the Selected Phase Without Resetting Review History

Human direction may return work to Implement, return it to Watchdog including at unchanged code, or supersede it. Initial implementation continuation uses Ready for Implementation; finding-driven implementation continuation uses Rework; review continuation uses Awaiting Review. Preserve branch, Submission, fixed source/Contract/report references, and completed Review Count. The next worker obtains the exact decision through `skl` and records its consumed reference under the existing report contract. Human direction informs judgment; it does not manufacture a pass, waive obligations, or reset the two-review automatic-rework limit.

```gherkin
Scenario: Resume implementation within the Contract
  Given an initial implementation paused before a Submission exists
  When the human resolves its current question and directs Implement
  Then it becomes Ready for Implementation with its existing progress preserved
  And its next project-scoped worker receives the exact recorded answer and reference through skl
  Given instead a finding-driven pause after review
  When the human directs implementation changes
  Then it becomes Rework without losing its Submission or completed-review history

Scenario: Reconsider findings at the same code revision
  Given Watchdog round 2 paused atlas/repair at source revision S
  When the human answers its current request with an in-contract finding disposition and directs Watchdog
  Then the item becomes Awaiting Review without requiring a new source commit
  And Review Count remains 2
  And the next Watchdog invocation is round 3 at S with the exact Human Decision supplied through skl
  And a passing review can reach Ready for Merge
  But another failing review cannot regain automatic Rework by resetting the count

Scenario: Workers consume decisions through the CLI boundary
  Given a worker is selected after a Human Decision
  When its specialized instructions and report inputs are retrieved
  Then they supply the exact decision content and full ledger reference consumed
  And they direct recording that reference in the phase handoff
  And they do not instruct the worker to read ledger files or GitHub directives as authority
```

## B7: Keep Multi-Item Authorization Explicit

Direction may cover several named requests, including across Projects. Grouping alone is not blanket authorization. Validate each selected request, keep each decision/state write atomic, and report each applied, already-applied, refused, or unresolved item. Do not silently apply a group answer to unmentioned requests or partially enact direction whose meaning depends on an unresolved coupled decision.

```gherkin
Scenario: Apply independently scoped answers and expose a stale member
  Given the human explicitly directs Implement for atlas/repair at R1 and beacon/upgrade at R2 independently
  And R1 is still current but R2 has been replaced
  When the Skill submits the scoped decisions through skl
  Then atlas/repair can be applied atomically
  And beacon/upgrade is reported as not applied with its stale-request reason
  And no other inbox member is changed
  And the result does not claim the whole group succeeded

Scenario: An ambiguous or coupled group answer needs clarification
  Given a displayed group contains three requests
  When the human says "do the suggested thing" without an unambiguous selection or common direction
  Then the Skill instructions require clarifying the affected requests and direction before writing
  Given instead the human directs two changes only if both are safe together
  When one request no longer matches
  Then the remaining answer is not silently treated as independent authorization
```

## B8: Preserve Contracts and Distinguish Retirement From Delivery

Human Decisions resolve work within accepted obligations. Wrong obligations require renewed proposal and re-slicing; replacement work belongs to a new Proposal. Explicit abandonment may mark affected unmerged slices Superseded, preserving already Merged slices and all historical evidence. Human-directed retirement of the old parent is allowed only when no active work or Claim remains, and must not mean all slices were delivered. Dependency satisfaction still requires Merged. This is not terminal aggregate observation or archive cleanup.

```gherkin
Scenario: Do not amend a wrong Contract through a decision
  Given atlas/repair's accepted Contract requires an outcome the human now wants to remove
  When direction attempts to drop that obligation and resume the same item
  Then the Skill calls for renewed proposal and re-slicing rather than editing the Contract
  And no contract-changing continuation is authorized
  When the human instead explicitly abandons the current item
  Then skl may record the scoped supersession without creating replacement work
  And accepted files and prior reports remain unchanged

Scenario: Retire abandoned work without claiming complete delivery
  Given an old Proposal has one Merged slice and two current Needs Human slices
  When the human explicitly supersedes both paused slices and directs retirement of the old parent
  Then the Merged slice remains Merged and the abandoned slices become Superseded
  And the parent can be retired only after no active or claimed slice remains
  And the result describes partial delivery rather than all-delivered completion
  And dependents of the Superseded slices remain blocked
  And no dependency remapping, archive move, forge completion observation, or source deletion occurs

Scenario: Active work prevents parent retirement
  Given another child is Rework, Ready for Merge, or claimed
  When retirement of its parent is requested
  Then skl reports that retirement was not applied because active work remains
  And it does not release the Claim, infer a merge, or silently abandon that child
```
