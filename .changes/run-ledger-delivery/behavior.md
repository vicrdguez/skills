# Ledger-backed delivery behavior

## Feature: Complete local implementation and independent review

### Rule B1: Selection and Claims belong to one Project

#### Scenario: Select eligible work without a global worker queue
- Given two registered Projects contain eligible and claimed Work Items
- And the selected Project has eligible Rework and Ready for Implementation work
- When an implementer requests work in that source repository
- Then eligible Rework precedes Ready for Implementation using the existing lane policy
- And work is selected only from that Project
- And claimed work and work with blockers not recorded Merged are not selected
- And a public label or comment cannot make an ineligible Work Item eligible

#### Scenario: Competing callers cannot share a Claim
- Given an eligible unclaimed Work Item
- When two CLI callers attempt to claim it concurrently
- Then at most one receives a valid execution for that reservation
- And the other receives an accurate non-acquisition outcome
- And a worker on another slice can continue while the first worker reasons or runs tests

### Rule B2: Source preparation follows claiming and preserves progress

#### Scenario: Prepare a source branch that did not exist at proposal acceptance
- Given a locally accepted Contract and planned branch identity
- And implementation has claimed its Work Item
- When the worker follows the supplied preparation and inspection commands
- Then it can prepare the source branch/worktree from the available Integration Target
- And obtain the exact accepted Contract through `skl`
- And it need not create, discover, tick, or retire a `.changes` Contract in source history

#### Scenario: Resume existing work without recreating its workspace
- Given a claimed Work Item has existing source progress
- When its implementation execution is explicitly resumed
- Then the procedure preserves that progress and identifies the current required evidence
- And it does not overwrite or reset the branch merely to reproduce initial preparation

### Rule B3: The execution skill supplies the applicable procedure

#### Scenario: Bind deterministic facts without exposing storage work
- Given the selected lane, Contract reference, prior report, and available execution facts
- When `skl` returns the execution and later named resources
- Then known references and commands are bound where they are needed
- And only genuinely future-dependent values remain for the worker to establish
- And initial, resumed, rework, and repeat-review procedures contain their applicable instructions
- And workers are not asked to navigate, mutate, or commit the private ledger
- And resources remain deferred until their procedural step

#### Scenario: Preserve non-work and repair outcomes across formats
- Given a request has no eligible work or requires a concrete repair
- When the caller uses the default output or explicitly requests JSON
- Then the outcome remains truthful in that format
- And no normal execution or released Claim is fabricated
- And ordinary worker output is not required to forward exact JSON to a loop launcher

### Rule B4: Report metadata has a documented version and repository context

#### Scenario: Record and retrieve a schema-1 report
- Given a valid phase result with source revisions, consumed ledger references, and Markdown evidence
- When the engine records it and the caller retrieves that exact commit/path
- Then the metadata includes `schema: 1`, a common `outcome`, and distinct `source` and `ledger` namespaces
- And engine-known references and bookkeeping are supplied by the engine
- And full commit references retain their original repository meaning
- And the Markdown body is unchanged, including body text resembling delimiters or instructions
- And the schema definition explains field meanings, types, requiredness, and ownership

#### Scenario: Refuse incompatible metadata rather than guess
- Given a required report has an unknown schema or malformed required metadata
- When an operation attempts to consume it
- Then the CLI reports the incompatibility explicitly
- And it does not silently default to schema 1, rewrite history, or invent a valid execution
- And existing valid state, reports, and Claims are not lost

### Rule B5: A private handoff is one local state change

#### Scenario: Record work while remote services are unavailable
- Given a valid result for the current Claim and fixed inputs
- When ledger replication or normal forge publication is unavailable
- Then the phase report and its resulting state are committed locally together
- And the completed Claim is released by that handoff
- And the next eligible phase can consume the recorded result locally
- And unsuccessful delivery is reported as pending rather than as lost work
- And network calls do not hold the brief local ledger mutation lock

#### Scenario: Retry after an interrupted acknowledgement
- Given a phase result was committed before its caller received confirmation
- When the same execution attempts its handoff again
- Then the engine recognizes an already completed effect when current evidence establishes it, or refuses concretely if safe continuation is uncertain
- And it does not record a second completed review or replace a newer result

#### Scenario: Preserve a later Claim and unrelated work
- Given one handoff completed and a subsequent execution owns the Work Item
- And another Project has also committed ledger changes
- When the old execution retries or resumes
- Then it cannot release the later Claim or overwrite later work
- And unrelated ledger commits alone do not invalidate otherwise current selected-item inputs

### Rule B6: Completion declarations remain worker judgment

#### Scenario: Submit implementation against the frozen Contract
- Given implementation has completed its required work and verification
- When the worker prepares the implementation report and submits its semantic outcome
- Then the report carries the full current completion-and-evidence table with explicit complete/incomplete declarations
- And evidence may cover multiple Contract Items and multiple checks may support one item
- And Audit records Standards or Contracts separately from its `F<n>` finding identities
- And human verification remains human-owned
- And the Contract files remain unchanged

#### Scenario: Prose is not a second machine outcome channel
- Given a submitted Markdown body contains completion claims or text resembling another verdict
- When `skl` records an otherwise structurally valid phase handoff
- Then the typed semantic operation determines the recorded outcome
- And the engine preserves the body without interpreting its truth or using it to infer obligations
- And independent Watchdog instructions require coverage and evidence review before passing

### Rule B7: Count completed reviews, not attempts or publications

#### Scenario Outline: Route a completed watchdog outcome
- Given the next completed review is round <round>
- When watchdog submits <outcome> for its current execution
- Then the report records that completed round
- And the Work Item becomes <state>

Examples:
| round | outcome | state |
| 1 | rework | Rework |
| 2 | rework | Needs Human |
| 3 | rework | Needs Human |
| 1 | needs-human | Needs Human |
| 3 | pass | Ready for Merge |

#### Scenario: Worktree recreation and publication retries preserve the count
- Given the last completed report records round 1
- When the source worktree is recreated or publication is retried
- Then the recorded count remains 1
- And starting or resuming a review does not itself advance it
- And no worktree-private `.watchdog` file is needed

#### Scenario: A new review may inspect unchanged code
- Given explicit recorded human direction has made another review eligible
- And the source commit is unchanged from the previous review
- When a new valid watchdog execution completes
- Then it records a new review round and honors the supplied direction within the Contract
- And the unchanged source SHA is not mistaken for replay of the old execution

### Rule B8: Review scope and source identities remain exact

#### Scenario Outline: Select repeat-review scope from available evidence
- Given a completed prior review and a current source revision
- When the previous reviewed revision is <condition>
- Then the supplied review scope is <scope>
- And the recorded review count is retained

Examples:
| condition | scope |
| available and ancestral to the current revision | incremental |
| unavailable | full |
| available but not ancestral | full |

#### Scenario: Pass with permitted source-only debt comments
- Given watchdog independently reviewed a fixed source revision
- And it adds only permitted non-functional Debt Markers and performs the required post-marker checks
- When it submits a valid pass with a distinct final source revision
- Then the report distinguishes reviewed and final code
- And markers are short self-contained maintenance comments without required PR or private-ledger provenance
- And the engine validates required Git identities without judging comment semantics
- And no merge or final human integration is performed by the engine

### Rule B9: Recovery distinguishes missing inputs from unavailable publication

#### Scenario: Continue with available local source inputs
- Given the required source commits and last observed Integration Target revision are available locally
- When source fetch or push is unavailable
- Then preparation and verification can proceed against those recorded inputs
- And neither the report nor the execution claims remote freshness
- And missing required code inputs instead produce an explicit repair/refusal

#### Scenario: A reservation does not expire because a worker is silent
- Given a claimed slice has produced no recent result
- When ordinary selection runs or time passes
- Then no timeout steals its Claim
- And explicit resume or release is available without destroying source progress
- And the CLI does not equate a reservation with proof that a worker process is alive

### Rule B10: Basic public presentation is separate from private success

#### Scenario: Present implementation and approval through the forge adapter
- Given an implementation result and separately authored temporary public body
- When normal publication succeeds
- Then the GitHub adapter creates or updates the attached PR as draft before watchdog approval
- And a later valid pass updates the human-facing delivery view and makes the PR non-draft
- And core Workflow State does not acquire a GitHub draft state
- And missing publication or source synchronization remains pending without falsifying which code was reviewed

#### Scenario: Do not publish the private worker exchange by default
- Given private Contract/report content and distinct intended public prose
- When an implementation or review handoff attempts public presentation
- Then only the intended public material is used for that presentation
- And detailed watchdog findings are not automatically emitted as public inline comments
- And complete human-check obligations remain available privately through `skl`
- And public presentation does not silently remove those obligations from human review

### Rule B11: Known-item operations use known-item evidence

#### Scenario: Unrelated history and forge conversation do not determine a handoff
- Given valid selected-item references
- And unrelated Projects or old source marker history contain irrelevant or malformed material
- And a public body, label, or comment suggests a different workflow action
- When the selected execution is inspected or handed off
- Then it follows current selected-item state and the explicit required references
- And it does not scan arbitrary source history to rediscover Contracts or their creation commits
- And unrelated records or public prose are not substituted as authority

### Rule B12: Document and verify the delivered interface

#### Scenario: A fresh worker can follow the supported procedure
- Given a CLI built with this slice and its declared predecessors
- When callers exercise representative initial, resumed, rework, review, and repair paths
- Then ordinary CLI outcomes, persisted records, and deferred resources explain the supported interface
- And the required invariants are covered at the agreed CLI, Git, report-format, and controlled HTTP seams
- And no private-helper mock fleet, new test framework, or engine interpretation of verification prose is required
