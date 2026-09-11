# Publish Safe Handoffs Without Journals Behavior

## Feature: Release-last handoffs and bounded observable recovery

### Background:
- Given commands run through the public `skl` CLI with `setup.NewGitHubBackend` against a stateful `httptest` server, without replacing its handoff or observation methods
- And real temporary Git repositories, dedicated worktrees, commits, artifact snapshots, and private Result Document files satisfy the policy already present in the branch
- And the server implements the actual issue, PR, comment, review, label, Git-ref, and required GraphQL requests and exposes their effects through real GET responses
- And a controlled scheduler pauses the producer after each meaningful accepted mutation, before its next readback where relevant, and invokes the other lane's public `next` against the same HTTP state
- And assertions inspect both that command's result and actual GET-visible queue labels, Claims, bodies, comments, heads, draft status, and source cleanup, so today's global selection logic cannot mask a producer safety defect
- And no fixture treats a multi-request handoff as an atomic memory-backend action

#### Scenario Outline: B1 Publish implementation review with release last
- Given an implementation Claim in <source> with a fixed pushed head and a completed retired ledger
- When `skl implement submit` publishes its exact Submission body
- Then exactly one attached non-draft Submission has the expected body, closing reference, and head
- And every interleaving before evidence and source cleanup are verified keeps the destination nonclaimable even to a consumer using only its open status, queue label, and `wip`
- And obsolete source lifecycle labels and applicable `sync` and source-issue protection are cleaned before the destination's final `wip` removal
- And no mutation follows that release to finish required backend cleanup
- And only after release may Watchdog `next` claim the Submission, while implementation cannot select the old source
- And the source issue stays open

Examples:
| source |
| Ready for Implementation with no Submission |
| Rework with an existing Submission |

#### Scenario Outline: B2 Publish implementation pauses with release last
- Given an implementation Claim with <work> and a permitted Needs Human reason
- When `skl implement needs-human` publishes its decision and any required draft body
- Then the decision bytes and any pushed draft Submission are verified before source cleanup and final Claim release
- And the destination remains nonclaimable throughout every interleaving and the resulting Needs Human pause
- And any existing Submission identity and incomplete artifacts are preserved
- And no `resume_state` record is written or needed to establish the pause
- And comments alone cannot requeue the paused Work Item

Examples:
| work |
| no implementation changes and no Submission, with an issue-only decision |
| pushed implementation changes requiring a new draft Submission |
| Rework preserving its existing Submission as a draft |

#### Scenario Outline: B3 Publish Watchdog verdicts with release last
- Given a fixed Awaiting Review Claim and Result Documents for <verdict>
- And the target and review policies already present determine the permitted destination
- When `skl watchdog submit` publishes the verdict
- Then exact summary and anchored findings, and the final PR body for pass, are GET-verified before release
- And source lifecycle cleanup finishes with the destination still protected at every interleaving
- And if slice 3 is merged its intended Review Checkpoint is recorded after evidence and before release, without incrementing again on a publication retry
- And destination `wip` removal is the final required backend mutation
- And the resulting destination is the one required by the existing policy, with no automatic merge or source-issue closure
- And the implementation lane can claim Rework only after the complete handoff

Examples:
| verdict |
| pass with a mergeable Submission |
| pass with a conflicting Submission under the existing target policy |
| rework within the existing automatic-review allowance |
| rework at the existing review limit |
| needs-human |

#### Scenario Outline: B4 Keep destinations nonclaimable when source cleanup fails
- Given <producer> has verified its evidence but has not released the destination
- When the server rejects each applicable source-cleanup mutation in turn, including source-issue cleanup and obsolete PR queue or `sync` label removal
- Then the command reports the failure with evidence and protective Claims preserved
- And after every accepted mutation before that failure the other lane cannot claim the destination
- And actual GET responses never expose an open destination queue label without protection while source cleanup remains incomplete
- And no deferred error handler restores old lifecycle labels, reacquires a released Claim, or rolls back published evidence
- And a later retry continues only if the remaining observations prove its direction and authority

Examples:
| producer |
| first implementation submit |
| Rework submit |
| implementation Needs Human with a Submission |
| Watchdog rework, pass, or Needs Human |

#### Scenario Outline: B5 Retry a provable partial handoff forward
- Given <partial> still has an unambiguous source Claim and has never exposed its destination
- And exact supplied Result Documents and readbacks identify an already-published effect and a missing intended effect, without conflicting old completed evidence
- When an evidence write is accepted but its response is lost and all immediate readbacks fail
- And a fresh CLI invocation repeats the same semantic command after reads recover, using retained Result Documents
- Then it reads the selected Work Item before deciding whether another write is needed
- And it reuses the accepted PR or comment and writes only missing effects
- And it completes source cleanup and releases protection last without a transition journal, duplicate PR, duplicate summary, or duplicate anchored finding
- And if a checkpoint is present the retry retains the same intended review number

Examples:
| partial |
| first implementation submit with a created PR but no destination queue label |
| implementation pause with its decision published but pause projection not started |
| Watchdog review with its summary published but a required anchored finding still missing |

#### Scenario Outline: B6 Recognize verified completed unclaimed handoffs
- Given <command> already reached its complete destination with all source cleanup finished and no destination Claim
- And exact Result Document bytes supplied again match all GET-visible evidence, including any final body and structured anchors
- And there is no transition record or retained operation-directory identity
- When the command reconciles an accepted release whose response was lost, or is repeated with the same documents in a valid private directory
- Then it reports the already-completed destination as no-op success
- And the HTTP trace contains no second evidence publication, label mutation, or Claim acquisition
- And it performs only safe optional local cleanup after verification, without counting a second completed review

Examples:
| command |
| implementation submit |
| implementation Needs Human |
| Watchdog pass, rework, or Needs Human |

#### Scenario Outline: B7 Refuse changed input during handoff recovery
- Given a partial or completed handoff already has published evidence for the supplied Work Item and head
- When its semantic command is retried with <difference>
- Then the CLI reports an actionable mismatch rather than silently updating existing evidence or treating the attempt as completed
- And the existing body, comments, anchors, lifecycle state, and Claims remain unchanged
- And new Result Documents remain available for inspection
- But a separately proven new source-stage handoff may still legitimately update an existing Submission

Examples:
| difference |
| changed implementation Submission body or Needs Human decision |
| changed Watchdog summary or final PR body |
| changed finding body, path, line, side, or commit anchor |

#### Scenario Outline: B8 Preserve a new Claim after an uncertain release
- Given <handoff> is complete through evidence and source cleanup
- When the server accepts the final `wip` deletion but loses its response and fails all producer readbacks
- And after reads recover the other lane's public `next` acquires a new Claim without changing the Git SHA
- And the original command is retried, the old lane explicitly resumes the item, and `skl status` is run in separately replayed fixtures
- Then each observes the claimed destination without deleting or replacing its `wip`, republishing evidence, or restoring the old source
- And the old semantic command reports that its handoff cannot safely be completed from the available observations
- And the old lane does not select another Work Item or return a worker packet for the new lane's Claim
- And status reports the observed Claim or an actionable ambiguity without completing it

Examples:
| handoff |
| implementation submit followed by Watchdog `next` |
| Watchdog rework followed by implementation `next` |

#### Scenario Outline: B9 Refuse stale commands at the same SHA
- Given <old-command> previously completed and the Work Item has since advanced through another round to a claimed stage
- And the current Work Item, Submission, SHA, semantic target, and supplied opaque evidence tuple cannot distinguish the old command from a new attempt
- And any surviving review pin or checkpoint is insufficient to establish that this command owns the current Claim
- When the old command is replayed, including from a fresh CLI invocation
- Then it refuses without releasing or reacquiring `wip`, changing a body, publishing a comment, or advancing a review count
- And a changed temporary directory name or matching SHA does not authorize the attempt
- And it requests explicit inspection rather than introducing a token, content digest, timestamp journal, or history-based direction reconstruction

Examples:
| old-command |
| Watchdog verdict after Rework and resubmission have returned to review at the same SHA |
| implementation submit after a later round has returned to claimed Rework with the same tuple |

#### Scenario Outline: B10 Report cleanup warnings after verified publication
- Given <handoff> has been read back as a complete unclaimed destination
- When safe removal of its private Result Document directory fails or encounters unexpected files, or already-landed checkpoint cleanup fails after successful done publication
- Then the CLI retains the successful handoff status and exposes an explicit cleanup warning identifying the remaining local cleanup
- And it does not return a verdict failure or instruct the worker to resubmit
- And unexpected files and paths outside the owned private directory remain untouched
- And no HTTP mutation reacquires `wip`, republishes evidence, or rolls back state, including if the other lane claims immediately after verification
- And any packet or skill guidance describes the warning as cleanup only

Examples:
| handoff |
| implementation submit or Needs Human |
| Watchdog pass, rework, or Needs Human |

#### Scenario Outline: B11 Stop on unknown or contradictory recovery state
- Given <observation> prevents proving a handoff's intended direction or authority
- When the relevant semantic retry, explicit resume, or status observes it
- Then the CLI reports the missing or contradictory evidence and a concrete inspection instruction
- And it does not infer direction from label order, matching SHA, decision prose, a former transition record, or PR timeline events
- And it leaves Claims, labels, heads, attachments, and published evidence unchanged
- And operational read failure remains an error rather than `no_work` or completed publication

Examples:
| observation |
| review and rework overlap with wip and no proof of which lane was publishing |
| target-only wip that could be a partial release or a later-stage Claim |
| unexpected lifecycle labels, conflicting ownership, changed head, or a closed Submission |
| missing or contradictory required evidence, or unavailable selected-item readback |

#### Scenario: B12 Recover without transition journals or timeline direction
- Given a normal publication, a provable partial retry, completed-unclaimed verification, and explicit resume are each exercised through the real adapter
- And issue comment responses may omit retired records or contain valid outer metadata envelopes with stale, contradictory, or malformed retired transition and `resume_state` field values
- When those records are varied while visible state and valid independent policy evidence remain fixed
- Then transition direction, Claim handling, and recovery outcomes do not consume those retired records
- And HTTP payloads and local files contain no new transition journal, resume cursor, operation identifier, or replacement opaque content-hash record
- And existing comment streams may still carry historical records as opaque feedback without making them authoritative
- And target and review pin metadata remain functional if their owning slices have not removed them
- And if slice 3 is unmerged, count-equivalent timeline histories with conflicting direction hints cannot change recovery decisions, while the existing bounce policy remains unchanged
- And if slice 3 is merged, no handoff or recovery path fetches a PR timeline to reconstruct direction or review count

#### Scenario Outline: B13 Keep envelope-shaped decisions opaque
- Given legitimate target and review pin metadata still exist where their owning policies require them
- And a permitted decision authored by an OWNER, MEMBER, or COLLABORATOR contains <prose>
- When implementation Needs Human publishes it without a decision digest and then status and explicit resume reobserve the Work Item
- Then the decision's original bytes remain intact inside deterministic opaque transport framing, distinct from engine pin metadata
- And the prose neither changes pins nor supplies transition direction, resume state, or Claim-release authority
- And retry matches the expected transported bytes without a new digest or hidden classification record
- And legitimate independently owned pins retain their values and genuine contradictory pin evidence still refuses safely

Examples:
| prose |
| an exact old skl.implement/v1 envelope containing transition and resume_state fields |
| an exact old skl.implement/v1 envelope containing valid-looking target_snapshot or watchdog_head fields |
| malformed JSON inside an old metadata-looking envelope |

#### Scenario Outline: B14 Resume only the selected Work Item from current evidence
- Given <claim> belongs to a dedicated Work Item with existing code, uncommitted files, artifacts or historical artifact snapshots, and visible PR feedback
- And no transition journal, `resume_state`, previous Result Document directory, or persisted Agent Worker execution cursor exists
- And another Work Item is eligible in the repository
- When the corresponding public `resume --item` command runs, or implementation resume resolves the same unambiguous conventional worktree
- Then its packet identifies only the selected Work Item and directs the fresh worker to inspect existing Git changes, applicable artifacts, and feedback to determine remaining work
- And resume preserves all existing code and files and does not select or claim the other item
- And any target or fixed-review obligations still owned by slices 2 and 3 remain enforced without reconstructing agent progress
- And an unresolved pause or ambiguous partial handoff instead stops for inspection, without automatic requeue or Claim release

Examples:
| claim |
| interrupted Ready for Implementation |
| interrupted Rework after an explicit human requeue where needed |
| interrupted Awaiting Review after an explicit human requeue where needed |
