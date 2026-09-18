# Recover Forge Publication: Behavior

These rules extend ADR0006's separation of local authority from Forge Publication. ADR0005 governs verification and construction freedom. Operation descriptions are semantic interfaces, not new command spellings. Publication references identify the selected local view and forge attachment; they are not permission to infer Workflow State from public prose.

## B1: Recover Pending Publication Without Repeating Work

An operator or agent can inspect pending publication and explicitly recover it through `skl`. This includes initial descriptive parent/child issues from slice 1 and PR creation or phase updates from slice 2. Recovery changes only publication bookkeeping and justified forge attachments, not accepted Contracts, source code, Phase Reports, Workflow State, completed Review Count, or Claims. Publication failure is not a worker-selection or handoff gate.

```gherkin
Scenario: Recover acceptance whose first issue attempt failed
  Given a multi-slice Proposal was accepted locally with frozen Contracts
  And initial publication left the parent and one child pending while another child issue exists
  And current temporary descriptive bodies are available
  When the operator requests publication recovery through skl
  Then the pending issue presentation and associations are completed where safely observable
  And the already-published child is reused rather than recreated
  And the original acceptance, planned branches, and Contracts remain unchanged
  And no implementation Claim, source preparation, or new acceptance is required

Scenario: Recover phase publication after local delivery continued
  Given implementation and Watchdog reports were committed locally during a forge outage
  And their latest PR publication remains pending
  When an agent requests recovery through skl
  Then recovery uses those recorded results without rerunning implementation, Audit, tests, or Watchdog
  And the completed Review Count, Workflow State, and Claims are unchanged
  And the same recovery is available without a Human Decision inbox operation

Scenario: Preserve pending work on another outage
  Given a valid pending publication and its current temporary body
  When the forge is still unavailable during explicit recovery
  Then skl reports the publication remains pending and preserves the usable temporary body
  And the local completed result remains authoritative and available to ordinary delivery
  And recovery does not enter a wait loop or create a publication-waiting Workflow State
```

## B2: Publish the Latest View, Not Missed History

Recovery uses the latest relevant locally recorded acceptance and phase results. Pending intermediate publications are not an event queue. Reuse an available temporary body only when its publication inputs still identify that view; stale prose requires a fresh agent-authored presentation, not automatic editing by the CLI. Unrelated ledger commits do not invalidate the selected publication inputs.

```gherkin
Scenario: Skip missed implementation and rejection updates
  Given implementation, review round 1 rejection, rework, and round 2 approval completed locally
  And none of their public updates succeeded
  When recovery publishes a body authored for the current approved view
  Then the PR presents that latest outcome and verification for its recorded source revisions
  And recovery does not replay each missed progress update or rejection comment
  And Review Count remains 2

Scenario: Reuse a current body without asking for new prose
  Given a pending issue or PR publication still matches its available temporary body inputs
  And another Project has since committed an unrelated ledger change
  When recovery is requested
  Then skl reuses the supplied human-facing prose without generating or summarizing it
  And the unrelated ledger commit does not force reauthoring
  And only applicable adapter-owned presentation details are added

Scenario: Do not reuse a superseded body
  Given the only available body describes review round 1 rejection
  And the current local result is round 2 approval
  When recovery is requested with that old publication context
  Then the stale body is not published as the current view
  And skl returns current references and authoring guidance instead of rewriting the body itself
```

## B3: Recover Lost Prose Through Specialized Instructions

Public bodies are ephemeral Result Documents, not ledger records. When applicable prose is absent, `skl` supplies specialized instructions, exact evidence references and private retrieval operations, and a concrete publication continuation with known arguments bound. The agent authors a new descriptive human-facing version; the CLI neither summarizes reports nor generates publication prose. No `issue.md`, `pr.md`, or equivalent outbox prose snapshot is stored in the ledger.

```gherkin
Scenario: Reauthor a lost initial issue body
  Given accepted work has pending issue publication and its temporary body was deleted
  When recovery is requested
  Then the output identifies the selected Project, Proposal, and Work Item
  And it provides exact accepted-Contract references and private access through skl
  And it gives instructions to author a descriptive issue body and a bound continuation
  And it does not synthesize an issue body or mutate local acceptance
  When an agent supplies a newly authored current body through that continuation
  Then publication can proceed without restoring the lost bytes

Scenario: Reauthor a lost final PR presentation
  Given Ready for Merge is recorded locally and the final temporary PR body is missing
  When recovery is requested in Markdown or explicit JSON form
  Then the output carries current result references, reviewed and final source revisions, and deferred authoring resources
  And it makes complete Manual Verification obligations privately retrievable through skl
  And an agent can author a fresh Final Review Package without rerunning the review
  And the ledger contains pending metadata and references but no durable public-body prose snapshot
```

## B4: Observe Ambiguous Effects Before Repeating Writes

Use known forge attachments and sufficiently specific observable identity to recognize creates and updates whose responses were lost. Never identify ownership from a similar title or arbitrary public comment alone. Reuse established effects and avoid duplicate creates, repeated already-satisfied updates, and duplicate explicitly selected inline findings where their identity is observable. If observation cannot distinguish absence from an unconfirmed effect, preserve pending information and require explicit inspection rather than blindly repeat a write. This is not an exactly-once guarantee across an unobservable external service.

```gherkin
Scenario: Recover a successful create with a lost response
  Given an issue or PR create took effect but its response was lost before the attachment was recorded locally
  When recovery observes one unambiguous matching object for the selected publication
  Then it records or reuses that attachment and completes only remaining presentation work
  And it does not create a second object
  When the same recovery is repeated after successful readback
  Then it reports the latest presentation already satisfied without duplicate effects

Scenario: Lost prose does not authorize a second create
  Given an initial create may have succeeded and its original temporary body is gone
  When recovery can establish the unique matching forge object from the available identity evidence
  Then it reuses that object
  And any new agent-authored current prose updates that object rather than creating another
  But if identity cannot be established without guessing
  Then recovery preserves the ambiguity and reports explicit inspection is needed

Scenario: Unavailable or conflicting observations stop retries
  Given a publication write returned an ambiguous error
  When readback is unavailable or multiple objects match the claimed ownership
  Then recovery reports what remains unconfirmed and performs no blind duplicate write
  And it retains pending metadata and available temporary prose
  And a matching title or decision-shaped public comment does not resolve the ambiguity

Scenario: A partially applied presentation resumes only missing effects
  Given the current PR body was updated but the draft-to-ready response was lost
  When recovery observes that both effects already match the current approved view
  Then it records publication satisfaction without repeating either mutation
  Given instead draft-to-ready is observably still missing
  Then only that remaining presentation effect is attempted
```

## B5: Protect Current Requests and Later Work

Before effects and before recording satisfaction, validate the selected current publication inputs and forge attachment: Project/repository, Work Item/Submission ownership, target, relevant report and source revisions, and pending view. Expected source-publication lag is not itself a conflicting attachment: reuse slice 2's normal source-publication path when the exact inputs remain available and safe catch-up is established. Do not present approval of the intended revision against a different PR head, force an unexpected source update, or rerun work to manufacture replacement evidence. A stale invocation cannot publish an old body over a newer result, reassign an object, or clear newer pending work. Recheck after network effects; if local state changed meanwhile, preserve observable receipts without pretending the latest view was published. Network publication is not a transaction with the ledger and must not hold a worker Claim or a long-lived ledger mutation lock.

```gherkin
Scenario: Refuse a stale request before mutation
  Given an agent prepared publication for source S1 and report R1
  And the selected Work Item now has source S2 and report R2
  When the old continuation is submitted
  Then skl refuses the stale publication before an external mutation
  And it supplies the current context needed to recover
  And it neither overwrites R2 nor clears its pending publication

Scenario: Preserve newer pending work after an in-flight external effect
  Given recovery began for current view V1
  And V2 is committed locally while the forge request for V1 is in flight
  When the V1 effect is observed
  Then skl may retain its observable publication receipt or attachment
  But it does not mark V2 published, revert V2, or release a later worker Claim
  And the result reports that the newer view still needs recovery

Scenario: Catch up an expected lagging source publication
  Given the current local approved view references available source revision S2
  And its correctly owned PR still has the previously published head S1 because source publication failed
  And the normal source-publication path can safely publish S2 without changing its code or overwriting unexpected work
  When recovery is requested
  Then it can reuse that path and observe the intended source revision before presenting its approval
  And it does not reject expected publication lag as an ownership conflict
  And local reports, Workflow State, Review Count, and Claims remain unchanged
  But if safe catch-up cannot be established
  Then it preserves pending work and reports the necessary repair instead of forcing an update

Scenario: Do not retarget or reassign an attached PR
  Given pending publication is bound to one Project, owning Work Item, branch, target, and source revision
  When observation shows a different repository, owner, target, or an unexpected or unestablished head change
  Then recovery reports the conflicting attachment for repair without overwriting it
  And it does not create a replacement PR to evade the conflict
  And local Workflow State, Claims, and reports remain unchanged

Scenario: Observed external closure is not completion recording
  Given publication is pending and the attached PR is now merged or closed
  When recovery observes that the requested active presentation no longer applies
  Then it preserves the local result and reports the completion-observation boundary
  And it neither reopens or replaces the PR nor records Merged or Superseded
  And dependency unblocking and terminal aggregate observation remain with observe-human-completion
```

## B6: Let GitHub Own Draft Presentation

The GitHub adapter maps pre-approval work to draft presentation and Watchdog approval to ready-for-review presentation using the latest recorded outcome. Recovery catches up that presentation through the same adapter as normal publication. Draft is not a new core state, and changing it does not approve, complete, or rerun review.

```gherkin
Scenario: Publish unapproved work as draft
  Given the latest recorded result is Awaiting Review, Rework, or Needs Human with code to present
  And its initial PR publication is pending
  When recovery creates the PR through the GitHub adapter
  Then the PR is draft
  And the ledger keeps its existing Workflow State and Review Count

Scenario: Catch up approval without replaying a draft round
  Given the latest local Watchdog report passes at its recorded reviewed and final heads
  And the PR is missing or still draft because publication failed
  When recovery publishes the latest final presentation
  Then the GitHub adapter makes the PR ready for review
  And it need not replay earlier draft progress presentations
  And local Ready for Merge remains distinct from Merged
```

## B7: Keep Worker Evidence Private and Human Obligations Accessible

Publication instructions require descriptive issues and PR bodies: commitments, delivered outcome or current progress, useful verification, risks, and appropriate human checks. They do not dump frozen Contracts, complete Phase Reports, private operational details, or raw decision history. The complete Manual Verification obligations remain privately accessible through `skl`; public omissions do not remove them or mark them satisfied. The CLI transports agent-authored prose rather than interpreting arbitrary prose as a verdict or trying to classify all private content.

```gherkin
Scenario: Build a public presentation from private evidence without copying it
  Given private Contracts and reports contain operational details and a required manual check
  When recovery renders the authoring resources for a descriptive issue or Final Review Package
  Then they require appropriate public commitments, outcome, evidence, risks, and human checks
  And they instruct the author not to expose full Contracts, worker reports, or private details
  When the agent submits a suitable human-facing body through skl
  Then the HTTP publication contains that body and adapter presentation details, not automatically appended private evidence
  And the full manual obligation remains privately retrievable and unchanged
  And no public checkbox or comment is treated as completion authority
```

## B8: Publish Only Explicitly Selected Actionable Inline Findings

Detailed reviews remain private by default. An operator or agent may explicitly select actionable findings for publication, supplying human-facing prose and valid anchors at the reviewed source revision. Validate the path, line, side, reviewed revision, and current Submission association; do not guess a replacement anchor after code moves. Do not automatically publish every report finding or turn public finding comments into worker directives.

```gherkin
Scenario: No selection means no inline review dump
  Given a private Watchdog report contains W1, W2, and W3
  When recovery is requested only for the public PR body
  Then none of the three findings is automatically published inline
  When an operator explicitly selects actionable W2 with its human-facing text and reviewed-code anchor
  Then only W2 is eligible for inline publication
  And W1 and W3 remain private

Scenario: Preserve reviewed anchors and avoid duplicate findings
  Given W2 was explicitly selected at reviewed revision S with a valid path, line, and side
  And its inline write succeeded but the response was lost
  When recovery observes that exact authorized publication effect
  Then it does not post W2 again
  Given instead the supplied anchor is invalid, stale, or cannot be established on the attached review
  When recovery attempts that selected finding
  Then it reports the finding as not safely published without guessing a new location
  And its result distinguishes any independently completed body publication from the unresolved finding
  And the private report, Review Count, and Workflow State are unchanged
```
