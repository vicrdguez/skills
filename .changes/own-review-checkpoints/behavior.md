# Make Review Checkpoints CLI-Owned Behavior

## Feature: CLI-owned review scope and completion

### Background

- Given a real Git repository with a dedicated linked worktree for each Work Item and valid artifact history under the currently shipped protocol
- And CLI commands run through `newApp(...).Run(...)` using the actual `setup.NewGitHubBackend` against controlled HTTP observations and mutations
- And H is the full SHA examined by this invocation, F is its optional final Debt Marker head, C is the retained completed count, and N is the intended review number
- And the default review limit is two and unrelated existing Workflow policies remain enabled
- And Result Documents are opaque; the original packet or caller-retained context preserves the exact submission command for retry, while unavailable or ambiguous invocation inputs require inspection

#### Scenario Outline: B1 - Select checkpoint storage by Work Item worktree (D1)

- Given two Work Items have distinct worktrees and retained checkpoints with distinguishable counts
- When a review of the selected Work Item is started and completed with the CLI invoked from <caller>
- Then its packet and next disposition use only the selected worktree's count
- And only `.watchdog` inside `git -C <selected-worktree> rev-parse --absolute-git-dir` is replaced with `N:H`
- And neither the linked worktree's literal `.git` file, working files, other worktree checkpoint, nor a common-directory registry is used as checkpoint storage
- And packet facts and instructions expose review facts and commands, not checkpoint paths, format, or worker read/write instructions

Examples:
| caller |
| primary worktree |
| selected linked worktree |
| another linked worktree with explicit selected item |

#### Scenario Outline: B2 - Derive review scope without discarding a valid count (D2)

- Given the selected worktree has <checkpoint> and current PR head H
- When Watchdog next or resume prepares a review packet
- Then the completed count is <count>, scope is <scope>, and intended number is <number>
- And an incremental packet supplies the usable previous SHA and concrete comparison against H
- And a full packet explains the fallback without requiring previous-head extraction or fetching that previous revision
- And the Full Gate, active-finding verification, artifact checks, and critical-class scan remain required
- And startup does not create or modify the checkpoint

Examples:
| checkpoint | count | scope | number |
| absent | 0 | full | 1 |
| valid zero count and full SHA | 0 | full | 1 |
| 1:P with P available and ancestor of H | 1 | incremental | 2 |
| 1:H | 1 | incremental with empty code diff allowed | 2 |
| 2:P with P syntactically valid but unavailable locally | 2 | full | 3 |
| 2:P with P available but not an ancestor of H | 2 | full | 3 |

#### Scenario: B3 - Resume interrupted review using fresh invocation facts (D2)

- Given next supplied H and N from a retained completed checkpoint
- And the worker was interrupted before submitting any review evidence
- And the PR has since moved to available head H2 while its review Claim remains
- And old issue comments contain stale `watchdog_head`, `reviewed_head`, and `review_round_head` fields
- When Watchdog resume is invoked for that Work Item
- Then it supplies H2, the unchanged completed count, scope computed against H2, and a concrete submit command carrying the unchanged N and `--reviewed-head H2`
- And the earlier packet remains an immutable description of H rather than being silently reinterpreted as a review of H2
- And the worker is instructed to inspect current progress and review H2, not reuse checks of H as proof of H2
- And no checkpoint or review-head issue metadata is written and no other Work Item is selected

#### Scenario Outline: B4 - Refuse invalid checkpoint data and submit inputs (D3)

- Given <invalid condition> for the selected review
- When <command> is invoked
- Then it returns an actionable error identifying the invalid input or inaccessible checkpoint and the required repair
- And it does not silently substitute count zero, overwrite the checkpoint, publish review evidence, or release an existing Claim

Examples:
| invalid condition | command |
| empty existing checkpoint or missing SHA field | Watchdog next/resume |
| negative, nondecimal, or overflowing stored count | Watchdog next/resume |
| abbreviated or nonhex stored SHA, extra field, or trailing garbage | Watchdog next/resume |
| checkpoint read denied or selected Git directory cannot be resolved | Watchdog next/resume |
| missing, zero, negative, or overflowing review-number | Watchdog submit |
| abbreviated, nonhex, or option-like reviewed-head or final head | Watchdog submit |
| corrupt or unreadable retained checkpoint | Watchdog submit |

#### Scenario Outline: B5 - Count completed verdicts and cap only automatic failure-driven Rework (D4)

- Given C is <before> and N is <number> for a newly completed review
- And all non-count preconditions permit publication, including a mergeable pass
- When the worker submits <verdict> with the concrete fixed-number command
- Then the review policy chooses <outcome> and the completion records N:H before releasing the Claim
- And Rework and Needs Human retain that checkpoint, while verified `done` permits its subsequent removal
- And Needs Human still requires explicit human requeue rather than comments alone

Examples:
| before | number | verdict | outcome |
| 0 | 1 | rework | Rework |
| 1 | 2 | rework | Needs Human with Rework resume state |
| 2 | 3 | rework | Needs Human with Rework resume state |
| 0 | 1 | needs-human | Needs Human with Awaiting Review resume state |
| 1 | 2 | needs-human | Needs Human with Awaiting Review resume state |
| 0 | 1 | pass | Ready for Merge |
| 1 | 2 | pass | Ready for Merge |
| 2 | 3 | pass | Ready for Merge |

#### Scenario: B6 - Distinguish a new same-SHA review from retrying the previous round (D4)

- Given a Needs Human review at H completed as round 1 and retained `1:H`
- When its exact round-1 command and unchanged Result Documents are retried before any requeue
- Then the completed publication is a no-op success with count 1 and no duplicate evidence
- When a human explicitly requeues review at the same H and a fresh worker completes round 2 with its own round-2 summary and concrete command
- Then the review counts as round 2 even with no new commits
- And a rework verdict now enters Needs Human, not another automatic Rework round
- And replaying round 2 leaves `2:H` rather than incrementing again

#### Scenario Outline: B7 - Complete evidence before recording the round (D5)

- Given a valid intended round N and unchanged Result Documents
- When an HTTP fault occurs at <evidence phase> with <observation>
- Then <first outcome> and the checkpoint is unchanged until all required evidence is observed exactly
- And `wip` is not released before complete evidence and checkpoint replacement
- When the fault is removed and the same fixed-number command is retried if needed
- Then the CLI reuses exact published effects and records N:H once before completing the handoff
- And summary bytes, final PR body with closing footer, and inline body/commit/path/line/side anchors remain intact

Examples:
| evidence phase | observation | first outcome |
| summary publication | write unapplied and readback proves absence | actionable failure with Claim retained |
| inline publication after summary | write applied but response lost and exact readback succeeds | forward progress without duplicate inline comment |
| final body publication on pass | write applied but response lost and exact readback succeeds | forward progress without duplicate body publication |
| evidence readback | read fails so applied effect is unknown | stop with retained Result Documents and Claim |

#### Scenario Outline: B8 - Replace the single checkpoint atomically (D5)

- Given complete review evidence for N has been published and observed while `wip` remains
- And the prior checkpoint is <prior>
- When replacement fails at <phase>
- Then the only authoritative checkpoint is <retained> and no partial `count:sha` becomes readable
- And the CLI returns actionable repair instructions, retains Result Documents and the Claim, and does not release `wip`
- When the storage fault is repaired and the exact N command is retried
- Then evidence is not duplicated and the checkpoint becomes N:H before handoff
- And there is no second persistent metadata file, backup, or journal

Examples:
| prior | phase | retained |
| absent | temporary-file creation denied | absent |
| C:P | temporary-file creation denied | C:P |
| C:P | rename denied after temporary write | C:P |

#### Scenario Outline: B9 - Retry after checkpoint replacement using the original intended round (D5)

- Given round N's exact evidence is published and N:H has replaced the checkpoint
- And the command with `--review-number N`, original reviewed head, final head if any, verdict, and Result Document paths remains in existing packet/result context
- When execution is interrupted at <phase>
- And a fresh CLI invocation retries that exact command against matching published observations
- And current observations independently establish a still-protected unfinished source handoff or a verified completed unclaimed destination
- Then it treats N as already recorded, not as a request for N+1
- And it finishes only the outstanding effects, preserves the round's original review-policy disposition, and publishes no duplicate evidence
- And it deletes the checkpoint only if `done` is subsequently verified

Examples:
| phase |
| after checkpoint replacement before destination label publication |
| after destination label publication before source-label cleanup |
| wip release applied but response or final readback lost |
| verified nonterminal handoff before the caller received success |

#### Scenario Outline: B10 - Stop when a retained command cannot prove the intended completion (D5)

- Given a submitted or resumed round has <ambiguity>
- When the CLI observes the selected Work Item, checkpoint, and available exact published receipts
- Then it reports an actionable refusal directing inspection or replay of the original fixed-number command and Result Documents
- And it does not guess a review number, overwrite evidence, decrement or increment the checkpoint, republish a verdict, or release a later-stage Claim
- And it adds no persistent recovery record and does not parse summary prose or timeline events for a count

Examples:
| ambiguity |
| requested N is older than C or skips beyond C+1 |
| requested N equals C but the checkpoint SHA differs from the supplied reviewed SHA |
| requested N equals C but supplied verdict conflicts with observed disposition, or summary/final body/inline anchors differ from published evidence |
| receipt read fails or matching evidence cannot unambiguously identify the intended round |
| resume encounters partial completion but the retained original command/result context is unavailable |
| an old command is replayed after another lane has claimed or advanced the Work Item |
| source-label cleanup has finished and target-only wip could be either unfinished release or a later-stage Claim |
| the protected source/destination overlap does not independently prove which handoff owns the Claim |

#### Scenario Outline: B11 - Preserve actual reviewed head separately from final Debt Marker head (D6)

- Given a worker reviewed H and retains that invocation's H and N in its command
- And no hidden issue metadata pins H
- When submit receives <inputs>
- Then <outcome>
- And only a completed review may record N:H; it never records F merely because F is the final PR head
- And instructions permit only nonfunctional Debt Marker comments, require commit/push and worker-owned Post-Marker Check, and do not ask the CLI to parse source or run the Full Gate

Examples:
| inputs | outcome |
| pass with pushed F descending from H and local/remote/PR agreement | accept existing Debt Marker finalization path |
| pass with F not pushed or local/remote/PR disagreement | refuse without consuming round or releasing Claim |
| pass with F not descending from H | refuse without consuming round or releasing Claim |
| rework or needs-human with a different F | refuse without consuming round or releasing Claim |
| no final-head override and PR moved away from H | refuse stale invocation without silently changing its reviewed head |

#### Scenario: B12 - Continue implementation and Audit without previous-review extraction (D7)

- Given finding-driven Rework has current code, retired historical artifacts, and visible review feedback
- And no usable previous-review SHA exists in metadata, summaries, or the checkpoint
- When implementation next and explicit resume are invoked
- Then they supply usable packets without asking for `--reviewed-head`, cache repair, or a required previous-review diff baseline
- And the worker continues from current code and feedback, keeps the ledger retired, and records finding resolutions
- And rendered Implement and included Audit guidance select a valid ordinary PR comparison when no explicit fixed point is supplied, rather than requiring a previous summary's head
- And Audit still honors an explicit user fixed point, both judgment axes, Full Gate, artifact integrity, and complete-final-implementation checks

#### Scenario Outline: B13 - Retain completed history during nonterminal work (D8)

- Given the selected worktree has a valid completed checkpoint C:P
- When <operation> occurs
- Then the checkpoint is not deleted or reset
- And the next review packet uses the retained C and derives scope from P's availability and ancestry

Examples:
| operation |
| completed Needs Human pause and explicit human requeue |
| finding-driven implementation and resubmission of the same PR |
| Implementation Ledger retirement |
| ordinary git clean in the selected working tree |

#### Scenario Outline: B14 - Delete only after verified done and report cleanup warnings (D8)

- Given pass evidence and N:H are recorded
- When the handoff reaches <publication> and local cleanup has <cleanup>
- Then <result>
- And no cleanup failure asks the worker to resubmit or causes duplicate review evidence
- And a verified completed unclaimed destination is not reopened merely because local cleanup remains

Examples:
| publication | cleanup | result |
| done not yet verified or Claim still held | no fault | retain checkpoint and retry context |
| verified done with Claim released | no fault | success and checkpoint removed |
| verified done with Claim released | checkpoint deletion denied | success with actionable cleanup warning and checkpoint retained |
| verified done with Claim released | existing private Result Document directory cleanup fails | success with actionable cleanup warning |

#### Scenario: B15 - Keep count policy independent of other completion callers and policies (D9)

- Given the retained completed count is two and timeline events suggest a different historical bounce count
- When a newly completed passing review is submitted as round 3
- Then review-count policy accepts it regardless of the timeline
- And while slice 2 is unmerged, an existing conflict diversion may still produce Synchronization Rework and retain `3:H`, without treating that pass as a count-limit failure
- When status observes or reconciles that already recorded review, or performs the existing independent conflict diversion
- Then it does not consume another round or rewrite the completed reviewed SHA
- And a caller lacking evidence that a pending review's round was recorded stops and directs the original submit retry instead of inventing a count or bypassing checkpoint-before-release
- And existing timeline-based partial-handoff reconstruction and unrelated target-policy decisions remain in place until their owning slices land

#### Scenario: B16 - Accept checkpoint loss when a dedicated worktree is recreated (D8)

- Given a Work Item previously retained count two and still has review history on GitHub
- When its dedicated worktree and private administrative directory are removed and the worktree is recreated without a checkpoint
- And Watchdog starts a new review
- Then it supplies count zero, full scope, and intended round 1 without restoring a count from timeline, summaries, a common registry, backup, or main-worktree fallback
- And a newly completed failing review can produce Rework as round 1
- And this accepted reset is described as a retained-worktree budget, not a durable lifetime cap
