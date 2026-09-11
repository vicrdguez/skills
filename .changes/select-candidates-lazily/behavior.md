# Candidate-first Work Start Behavior

## Feature: Ordered, lazy candidate discovery

#### Scenario: B1 Rework selection uses PR age and precedes Ready
- Given two open unclaimed Rework PRs whose source issue ages have the opposite ordering
- And an older open unclaimed Ready issue
- When `skl implement next` selects work
- Then the oldest Rework PR is selected by PR creation time
- And no Ready Dependency or discussion is requested while that Rework candidate is eligible

#### Scenario Outline: B2 Queue-local age and identity determine selection
- Given open candidates in the requested queue, including an older candidate carrying `wip`
- And two unclaimed candidates with the same creation time and different record numbers
- When the command requests its next item
- Then the claimed record is skipped
- And the lower-numbered unclaimed record at the oldest eligible creation time is selected

Examples:
| Command | Queue | Age and tie record |
| implement next | ready, with no eligible rework | source issue |
| implement next | rework | PR |
| watchdog next | review | PR |

#### Scenario Outline: B3 No work does not trigger unrelated discovery
- Given the open queue-labelled records are empty or all carry `wip`
- And the repository has unrelated open records, historical Work Items, and large old PR discussions
- And the local selected-project branch objects are unavailable
- When the command requests its next item
- Then it returns `no_work` without claiming anything
- And it does not enumerate closed/merged candidates, fetch discussions or hidden metadata, or inspect project commit/tree/blob objects

Examples:
| Command |
| implement next |
| watchdog next |

#### Scenario: B4 Dependencies precede Ready context enrichment
- Given no eligible Rework PR and two unclaimed Ready issues ordered oldest first
- And the older issue has an unsatisfied Dependency
- And the younger issue has no unsatisfied Dependencies
- When `skl implement next` selects work
- Then it checks the older issue's Dependencies before considering the younger issue
- And it does not request the older issue's discussions or artifact contents
- And it claims the younger issue without checking later Ready items

#### Scenario Outline: B5 Blocker completion is established only for referenced Dependencies
- Given a tentative Ready issue with one explicitly referenced blocker
- And that blocker's observed completion evidence is the stated condition
- When `skl implement next` evaluates the candidate
- Then the Dependency has the stated result
- And any terminal Work Item reads are confined to that referenced blocker and its explicit Submission evidence
- And unrelated historical Work Items are not loaded

Examples:
| Evidence | Result |
| the owning Submission is actually merged | satisfied |
| the owning Submission is done but not merged | unsatisfied |
| the issue is closed without merged Submission evidence | unsatisfied |
| the referenced Work Item cannot be established as merged | unsatisfied |

#### Scenario: B6 Pagination preserves candidate and Dependency completeness
- Given the first page contains only claimed candidates and a later page contains the oldest available candidate
- And an unsatisfied Dependency of that candidate occurs after the first Dependency page
- And a younger candidate is unblocked
- When `skl implement next` selects work
- Then it follows the necessary page continuations and skips the blocked candidate
- And it selects the younger unblocked candidate
- And it never treats an unfinished page stream as complete or downloads unrelated candidate discussions

## Feature: Explicit ownership and selected-item acquisition

#### Scenario: B7 Publication and subsequent commands use the explicit owning link
- Given an implementation Work Item without a Submission
- When the worker publishes its first Submission and the owning issue is later renamed
- Then publication establishes and verifies one explicit issue/PR owning association
- And Rework or Watchdog selection resolves the same Work Item through that association
- And submission updates, status attachment reporting, and safe cleanup do not redirect ownership based on either title
- And Git preparation uses the PR's actual head branch

#### Scenario Outline: B8 Invalid ownership is a selected-item refusal
- Given an otherwise eligible open PR whose ownership has the stated defect
- When the command inspects it as the next candidate
- Then it returns an actionable selected-item problem without claiming or publishing over that defect
- And it does not search issue titles or historical PR branches for a replacement owner

Examples:
| Defect |
| no explicit owning issue |
| multiple conflicting owning issues |
| another active Submission already owns the same issue |
| an ownership reference outside the supported repository attachment |

#### Scenario: B9 Claim readback does not rediscover the queue
- Given a selected eligible item whose current state remains compatible
- When the command acquires its additive `wip` Claim
- Then it verifies the selected source/Submission records and returns their observed identity
- And it does not enumerate the candidate queue again or hydrate unrelated Work Items
- And unrelated labels and feedback remain unchanged

#### Scenario: B10 Acquisition drift cannot return an invalid handoff
- Given a candidate changes state or becomes claimed between discovery and acquisition
- When the CLI refreshes that candidate or observes an uncertain claim response
- Then it does not overwrite incompatible projections or report a packet for an unverified Claim
- And an observed Claim or uncertain acquisition is reported with explicit recovery information
- And uncertainty does not trigger a blind replacement `next` or release of another stage's Claim

## Feature: Deferred Git preparation and complete selected context

#### Scenario Outline: B11 Startup does not require project objects
- Given a Git repository with usable repository/remote configuration
- And the selected issue/PR is eligible on GitHub
- But its branch commits, artifact trees/blobs, and dedicated worktree do not exist locally
- When the command requests its next item
- Then it returns `work_available` with one concrete Instruction Packet for the observed Claim
- And the packet tells the worker how to fetch, create or safely reuse its worktree, and inspect artifacts afterward
- And the CLI neither fetches nor checks out the branch nor demands local endpoint inspection before returning
- And the packet does not require embedded historical artifact file bodies

Examples:
| Command |
| implement next |
| watchdog next |

#### Scenario: B12 Resume is explicit continuation rather than another selection
- Given an explicitly identified existing implementation or Watchdog Claim
- And its dedicated worktree contains unfinished local work
- When the worker requests resume for that identity
- Then the CLI returns instructions to inspect existing code, artifacts, and feedback and continue that same work
- And it does not query another queue candidate, reset, stash, merge, replace, or discard local work
- And an unavailable or ambiguous Claim yields explicit repair rather than falling back to `next`
- And worktree-only resume infers identity only from an unambiguous explicit attachment, otherwise requesting `--item`

#### Scenario: B13 Only selected feedback is hydrated and none is truncated
- Given the selected PR has source comments, PR discussion, inline findings, reviews, and human directives across multiple pages
- And an unrelated PR discussion endpoint would fail if requested
- When the CLI supplies worker-facing context for the selected work
- Then the relevant selected streams are complete with raw bodies, author associations, and commit/anchor facts preserved
- And it does not request the unrelated discussion
- And findings and directives remain agent judgment rather than queue-eligibility predicates

#### Scenario Outline: B14 Required observation failure is not an empty queue
- Given a required discovery, owning-link, Dependency, selected-feedback, or acquisition observation fails as stated
- When the command encounters that failure
- Then it reports the operational error or actionable selected-item refusal
- And it does not report `no_work` from incomplete evidence or treat unknown Dependencies as satisfied
- And it does not claim another item after uncertain acquisition

Examples:
| Failure |
| authentication or authorization failure |
| network or server failure |
| partial GraphQL result with an error |
| missing required continuation page |
| inaccessible Dependency relationship |

#### Scenario: B15 Startup composes with safe handoffs and private checkpoints
- Given the prerequisite handoff and checkpoint behaviors are installed
- When implementation publishes reviewable work and Watchdog later submits findings
- Then neither queue can acquire the destination before evidence, checkpoint work where applicable, and source cleanup complete
- And the next packet carries only the review facts and concrete commands the worker needs
- And the worker is not instructed to locate, parse, or edit `.watchdog`
- And no hidden issue metadata or PR-timeline state reconstruction is reintroduced

#### Scenario: B16 Unrelated history does not scale Work Start cost
- Given controlled repositories with the same eligible candidate prefix and selected feedback
- But substantially different numbers of historical Work Items and unrelated discussion pages
- When repeatable CLI measurements execute `no_work` and successful selection fixtures
- Then the candidate-discovery and claim-readback request work does not grow with that unrelated data
- And startup launches no Git object/history inspection for selected project commits
- And a before/after report records request counts and elapsed distributions without making a machine-specific millisecond threshold an acceptance condition
