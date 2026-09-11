# Leave Integration Into main to Humans Behavior

## Feature: Review candidates independently of human integration

### Background:
- Given commands are exercised through newApp(...).Run with real temporary Git repositories and the existing public backend binding
- And the selected Work Item has valid ownership, branch, artifact, and Claim evidence unless a scenario explicitly varies it
- And GitHub-specific observations and writes use the actual GitHubBackend against controlled HTTP responses
- And assertions observe CLI results, rendered packets, Git state, and externally visible backend effects rather than private helpers

#### Scenario: B1 Start and resume after main advances without a target pin
- Given a Ready Work Item has an available published branch and valid Artifact Baseline
- And main has advanced independently to a commit absent from the worker repository
- And target-selection and main-head lookup requests are unavailable
- When the worker runs implement next and later implement resume for the same Claim after further main movement and valid implementation progress
- Then both commands return the selected Work Item's work_available packet without requesting a target branch or target commit
- And neither command writes target_snapshot, target_branch, or synchronization_target metadata or returns a target pin
- And resume preserves branch progress, existing files, and the same Claim instead of requesting an old pin or merging main
- And branch and artifact preparation checks unrelated to target pins still apply
- And this satisfies intent.md D1

#### Scenario: B2 Submit a candidate without the target commit
- Given a claimed implementation branch has a pushed fixed head and valid completed, retired artifacts
- And neither today's main commit nor an obsolete target_snapshot commit is locally available or contained in that head
- And any obsolete target metadata is otherwise valid JSON
- When the worker runs implement submit with its Result Document
- Then the command reports awaiting_review and publishes one Submission against main
- And publication does not request a target commit, require target ancestry, or write target metadata
- And the source issue remains open with its merge-time closing reference on the Submission
- And this satisfies intent.md D2

#### Scenario Outline: B3 Pass review regardless of mergeability
- Given an Awaiting Review Claim has a valid fixed reviewed head, retired artifacts, summary, and complete final PR body
- And GitHub reports <mergeability> for its main-based Submission
- When the watchdog runs watchdog submit with verdict pass
- Then the command reports ready_for_merge and publishes done with the Claim released
- And it publishes the supplied review evidence and final PR body with the source issue's closing reference
- And it neither adds sync or rework nor writes synchronization metadata nor looks up a target head
- And it neither merges the PR nor closes the source issue
- And this satisfies intent.md D3

Examples:
| mergeability |
| mergeable |
| conflicting |
| unknown |

#### Scenario Outline: B4 Retry a pass without conflict rerouting
- Given a valid pass has reached <publication_state> with the same reviewed revision, summary, findings, and final body
- And GitHub changes mergeability from mergeable to conflicting or unknown before a later guard or retry
- When the watchdog retries the same semantic pass and Result Documents
- Then the command completes or confirms ready_for_merge without treating mergeability as a failed guard
- And existing exact-content and anchor rereads prevent duplicate review evidence
- And it does not create Rework, add sync, persist a target, or request the target head
- And the source issue remains open and no merge is performed
- And this satisfies intent.md D4

Examples:
| publication_state |
| review evidence published with an incomplete claimed done handoff |
| completed unclaimed done handoff |

#### Scenario Outline: B5 Observe and reconcile approval without integration rework
- Given a main-based Submission has <projection> with valid unchanged-head evidence
- And GitHub reports conflicting or unknown mergeability and target-head lookup is unavailable
- When the operator runs status
- Then the command reports ready_for_merge and completes only any already evidenced partial pass
- And a completed done projection is preserved without lifecycle writes caused by mergeability
- And no sync label, target metadata, target-head request, or automatic Rework is created
- And the source issue stays open until actual human merge evidence establishes Merged
- And this satisfies intent.md D5

Examples:
| projection |
| completed done |
| unambiguous partial done handoff retaining its Claim |

#### Scenario: B6 Continue existing sync-labeled Rework without reviving synchronization
- Given an existing Submission carries rework and stale sync with valid artifacts and the review evidence required by the current Rework flow
- And trusted historical metadata contains unavailable and conflicting target_snapshot values, target_branch, and synchronization_target
- When the operator observes status and the worker starts and resumes that Rework
- Then its existing rework state and Claim rules determine queueability, not sync or the obsolete target fields
- And the commands neither revive an old target pin nor request target commits nor write target or synchronization metadata
- And startup and observation leave stale sync and unrelated labels alone
- When the worker completes the ordinary implementation-to-review handoff
- Then it updates the same Submission and removes stale sync through the existing authorized handoff cleanup
- And it does not recreate the ledger, erase feedback, or reset or reinterpret retained review-count evidence
- And a sync label alone does not make an item queueable or resolve contradictory lifecycle labels
- And this satisfies intent.md D6

#### Scenario Outline: B7 Publish only to main
- Given the selected Work Item is eligible for <operation> with pushed fixed-head evidence
- And its existing Submission, if any, has base main
- And no implementation target-selection lookup is available
- When the worker runs <operation> with the required Result Documents
- Then any newly created PR uses base main and any existing PR retains base main and its identity
- And the command follows its ordinary review or draft-preservation handoff without choosing another destination or looking up main's commit
- And the source issue remains open
- And this satisfies intent.md D7

Examples:
| operation |
| implement submit creating a Submission |
| implement submit updating a Submission |
| implement needs-human preserving work in a draft Submission |
| watchdog submit publishing a passing final body |

#### Scenario Outline: B8 Refuse an existing non-main PR without retargeting it
- Given the selected Work Item's existing PR has base release rather than main
- And its other evidence permits <operation>
- When the operator runs <operation>
- Then the affected command reports an actionable refusal identifying the PR and the required main destination
- And it directs the human to inspect and explicitly repair the PR base before retrying
- And it does not retarget the PR, replace its body, change its labels, release an existing Claim, or publish the affected handoff
- And the refusal is scoped to this item rather than a repository-wide repair or label sweep
- And this satisfies intent.md D8

Examples:
| operation |
| implement submit updating an existing PR |
| implement needs-human preserving a draft PR |
| watchdog submit publishing a pass |
| status reconciling a pending pass |

#### Scenario Outline: B9 Preserve review and publication validity checks
- Given a main-based Submission reports conflicting or unknown mergeability
- And <invalid_evidence> contradicts an otherwise permitted implementation or Watchdog handoff
- When the worker submits or retries that handoff through the CLI
- Then the command refuses for the actual invalid evidence rather than mergeability or a missing target pin
- And it does not falsely report a completed handoff, discard repair evidence, or release the held Claim
- And ordinary failing-review and Needs Human routing retain the existing review-counter policy, including historical synchronization exclusions where still used
- And this satisfies intent.md D9

Examples:
| invalid_evidence |
| local and published implementation heads differ |
| the PR head moves during verdict publication |
| the supplied reviewed head differs from the claimed reviewed revision |
| a post-marker head does not descend from the reviewed head |
| required retired artifact evidence is invalid |
| the required review Claim is absent |
| retry Result Documents disagree with published evidence |

#### Scenario: B10 Render instructions without forced integration
- Given the public CLI can render Implement, Watchdog, and Audit skills and selected-item packets
- When the operator requests implementation command help, skill instructions, and start or resume packets
- Then no supported implementation command or generated retry command advertises --target-snapshot
- And an invocation supplying that removed flag is rejected by CLI argument parsing rather than restoring legacy behavior
- And packet JSON and Markdown carry no target pin or forced merge instruction
- And the guidance names main as the workflow PR destination and the main merge-base as the worker's first-review comparison, distinct from Artifact Baseline and any retained repeat-review evidence
- And ordinary Git inspection, fetching, and safe worktree use remain worker responsibilities without a new CLI preparation step
- And Watchdog guidance says a valid pass reaches done regardless of conflicts or unknown mergeability, with conflicts informational in the session summary and integration and merge left to the human
- And independent Audit still honors an explicitly supplied fixed point
- And this satisfies intent.md D10
