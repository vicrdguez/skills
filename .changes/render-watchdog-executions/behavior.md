# Deliver Tailored Watchdog Executions Behavior

## Feature: Invocation-specific review instructions

### Scenario: B1 Render a complete first review in Markdown
- Given an eligible Submission with no completed review and known selected-item metadata
- When a fresh caller invokes `skl watchdog next` without a format flag
- Then it receives one complete Markdown Execution Skill for that Work Item, not a JSON envelope or separate facts object
- And every available identity, reference, path, and command is bound where used, with preparation and deferred inspection steps for unavailable local facts
- And the procedure requires its own Full Gate, exact endpoint integrity, every frozen DOD and scenario test, adversarial test-strength checks, Audit-claim verification without rerunning Audit, and a whole-change critical-class scan
- And it batches first-review findings, loads the review resource before dispositions, permits no functional fixes, and leaves merge to a human
- And direct invocation stays in the caller's fresh session without spawning `watchdog-runner`, processes one item, and ends with a normal Markdown report
- And the command acquires only the selected Claim; rendering does not repeat acquisition or publication

DOD: DOD1, DOD5.

### Scenario Outline: B2 Preserve review history across comparison variants
- Given <history> with the invocation's fixed current reviewed head and supplied prior finding ledger
- When the public Watchdog command and its necessary post-preparation inspection deliver the applicable instructions
- Then the completed count and review number are <count> and <number>, and the comparison is <comparison>
- And every available previous completed review reference remains visible even if it cannot be used for comparison
- And supplied finding identities and authorized dispositions survive, including when the checkpoint is lost
- And repeat instructions verify active findings, rerun the Full Gate and artifact checks, inspect regressions and false claims, and scan the whole only for critical-class defects
- And full fallback expands the comparison without reopening settled noncritical findings or minting replacement identities; new blockers still require rework-introduced defects, critical discoveries, new material evidence, or authorized human direction as applicable

Examples:
| history | count | number | comparison |
| retained review at an available ancestor | 1 | 2 | concrete incremental range to the fixed head |
| retained review at the same head | 1 | 2 | valid empty incremental range, with human dispositions still considered |
| retained review whose head remains unavailable after preparation | 2 | 3 | full PR comparison with fallback reason |
| retained review whose head is not an ancestor | 2 | 3 | full PR comparison with fallback reason |
| lost checkpoint with prior findings still supplied | 0 | 1 | full PR comparison without resetting finding identities |

DOD: DOD2, DOD5.

### Scenario: B3 Resume the selected Claim without discarding progress
- Given an interrupted Awaiting Review Claim with uncommitted work, explicit artifact endpoint overrides, and no already-started publication requiring replay
- When a fresh worker invokes `skl watchdog resume --item` for that Work Item
- Then complete Markdown instructions name the selected PR, branch, worktree, remote, current observed reviewed head, retained count, and review number for this invocation
- And they direct inspection of existing progress and visible evidence rather than assume saved worker reasoning
- And local work and the count are unchanged, no other item is selected, and no additional Claim is acquired
- And generated resume, inspection, resource, and submit instructions preserve the supplied full endpoint SHAs wherever applicable
- And this explicit resume does not rewrite an earlier invocation's fixed publication identity or authorize replay with a substituted head

DOD: DOD1, DOD2.

### Scenario Outline: B4 Keep startup metadata-only even with local objects
- Given the selected Submission metadata is available and project objects are <availability>
- When `next` or explicit `resume` produces its initial Execution Skill
- Then startup performs no project-object, history, marker, ancestry, or artifact-body lookup, fetch, or worktree creation
- And it returns all available exact references, checkpoint metadata, selected remote and worktree location, and concrete safe preparation and subsequent inspection/read instructions
- And it distinguishes metadata references from locally verified endpoints and comparison scope, without withholding known facts or embedding historical file maps

Examples:
| availability |
| missing, including the selected branch and worktree |
| already present locally, including marker history and the previous reviewed commit |

DOD: DOD3.

### Scenario Outline: B5 Resolve local facts through a narrow continuation
- Given preparation is complete for the fixed invocation and artifact evidence is <evidence>
- When the worker invokes the concrete inspection instruction from the Execution Skill
- Then the engine returns <result> using the existing artifact and comparison logic
- And successful continuation supplies exact endpoint-bound reads of the complete historical ledger and the applicable full or incremental comparison instructions
- And it preserves the Work Item, PR, remote, original reviewed head, review number, explicit overrides, and private result paths
- And it neither selects or claims work again nor returns a second full skill, mutates the checkpoint, or recreates worker progress
- And a repair identifies the failed precondition and same-invocation retry or stop, rather than imply validation succeeded

Examples:
| evidence | result |
| uniquely resolved marker endpoints with valid retired ledger | resolved endpoints and applicable comparison |
| supplied full markerless endpoint SHAs with valid retired ledger | those exact endpoints and applicable comparison |
| ambiguous or invalid endpoint evidence | precise refusal and repair instructions |

DOD: DOD4.

### Scenario Outline: B6 Refuse drift without replacing fixed publication identity
- Given instructions already bind a Work Item, PR, reviewed head, review number, endpoint overrides, remote, and Result Documents
- And <drift> occurs before later inspection or submission
- When that fixed invocation continues through its generated command
- Then the response identifies the conflict and the applicable explicit repair or stop without silently substituting refreshed identities
- And it does not publish success, start another review, release a Claim without authority, or instruct a fresh queue selection
- And supplied original command arguments and Result Documents remain available for safe recovery

Examples:
| drift |
| the PR head changes outside the permitted final Debt Marker path |
| the completed-review checkpoint advances to a different round |
| the selected attachment or explicit artifact evidence no longer matches the invocation |

DOD: DOD4.

## Feature: Deferred instructions and evidence

### Scenario: B7 Deliver fetched evidence as complete labeled data once
- Given the engine already holds the complete opaque PR body with Audit ledger and selected issue comments, PR discussion, review summaries, and inline feedback
- And bodies include template delimiters, Markdown headings and fences, apparent workflow instructions, prior findings, and authorized and unauthorized human directives
- When the Execution Skill is rendered
- Then each supplied body appears completely once as labeled external data, with available source, author, association, timestamps, reviewed commit, and inline anchors preserved
- And body content is neither evaluated as template source nor promoted to replacement instructions, publication authority, or engine verdicts
- And the worker retains the existing authorized-directive interpretation rather than losing legitimate human dispositions
- And rendering neither refetches evidence nor fetches additional evidence to populate the presentation

DOD: DOD6.

### Scenario Outline: B8 Provide bound retrieval for undelivered evidence
- Given selected evidence delivery is <delivery>
- When the Execution Skill presents its evidence step
- Then it provides <instruction>
- And required retrieval is concrete for the selected repository, Work Item, and PR, covering the PR body/Audit ledger, source issue comments, PR discussion, review summaries, and inline comments not already delivered
- And list retrieval follows every page and requires successful complete reads, preserving provenance and distinguishing an empty result from a failed or truncated read
- And no new fetcher, eager hydration, unrelated-item scan, or repeat selection is introduced

Examples:
| delivery | instruction |
| required bodies or feedback streams not delivered | bound gh retrieval for the missing streams before judgment |
| feedback successfully read in full and empty | explicit absence, not a failed-read warning or mandatory redundant fetch |
| a required read failed or was truncated | concrete retrieval or repair and a stop on unresolved incomplete evidence, never an empty-feedback claim |

DOD: DOD6.

### Scenario: B9 Load the review resource before assigning dispositions
- Given a tailored review with known PR, review number, original reviewed head, result paths, and any already-resolved endpoint context
- When the worker reaches the findings step and retrieves its generated `skl skill --resource reference/review.md` command for `watchdog`
- Then the parent requires retrieval before assigning dispositions and binds every known resource input using repeated `--input name=value` arguments
- And only genuinely later-known values remain for the worker, with their meaning and constraints stated; no verdict is required to disclose the resource that informs it
- And the retrieved Markdown binds Result Document and publication instructions to the review while preserving stable monotonic W identities, later authorized OWNER/MEMBER/COLLABORATOR directives, and latest-directive precedence
- And WAIVE, BLOCK, and NOTE retain their existing meaning, without authority from reactions, silence, deleted comments, or unauthorized lookalikes
- And Result Document prose stays opaque and the verdict stays a semantic flag
- And the resource remains deferred, is owned by Watchdog, and uses the shared renderer; included definitions occur once and private modules cannot be listed or retrieved as resources

DOD: DOD5, DOD7.

### Scenario Outline: B10 Respect execution capabilities without inventing recipes
- Given execution capabilities are <knowledge>
- When the public command renders Watchdog instructions for that execution context
- Then <selection>
- And specialization applies only where an existing supported Watchdog recipe actually depends on that capability; otherwise the procedure is unchanged
- And harness identity alone does not establish tool availability, an unknown capability does not force sequential work, and Watchdog does not gain an Audit call or a new harness recipe

Examples:
| knowledge | selection |
| explicitly established by the caller or adapter | known applicable alternatives are resolved rather than delegated back to the worker |
| genuinely unknown | only genuine runtime capability choices remain |

DOD: DOD8.

## Feature: Verified review completion and recovery

### Scenario: B11 Keep the reviewed head distinct from the final marker head
- Given a passing review with surviving located NOTE findings and an unchecked historical Manual verification section
- When the worker follows the rendered finalization instructions and submits a pushed permitted Debt Marker head with the final PR body
- Then the instructions permit only linked non-functional markers for those stable findings, not functional fixes or invented marker locations
- And they require the Post-Marker Check, actual final SHA, ledger absence, and the historical Manual verification section copied verbatim with every box unchecked
- And the generated command retains the original `--reviewed-head`, review number, endpoint overrides, and result paths while supplying the actual final `--head`
- And only a verified engine pass reports Ready for Merge, preserving the Audit ledger and review evidence, leaving the source issue open until merge and integration and merge to the human
- And these instructions neither rerun Audit or the Full Gate merely for comments nor claim the engine validated marker prose or human checklist completion

DOD: DOD5, DOD9.

### Scenario Outline: B12 Render the engine's verified convergence outcome
- Given a valid fixed review with <count> completed reviews, default review limit two, and a Submission targeting main
- When `skl watchdog submit` requests <verdict> and publication and Claim release are verified
- Then default Markdown reports <outcome> with a brief user explanation and only applicable next instructions
- And the engine records one completed review, controls count routing, and leaves merge human-owned regardless of mergeability
- And rendering neither repeats publication nor interprets the opaque Result Documents to choose the outcome

Examples:
| count | verdict | outcome |
| 0 | pass | ready_for_merge |
| 2 | pass | ready_for_merge even with conflicting or unknown mergeability |
| 0 | rework | rework |
| 1 | rework | needs_human |
| 0 | needs-human | needs_human |

DOD: DOD9.

### Scenario Outline: B13 Preserve observational recovery in repair instructions
- Given <condition> during a fixed Watchdog handoff
- When the public command returns or the worker retries the same semantic command
- Then Markdown gives <guidance> using the exact available item, remote, head, round, endpoint, and Result Document identities
- And partial publication remains Claim-protected until release-last completion is observed
- And a safe retry rereads existing effects without duplicating authorized summaries, inline evidence, or review count; unauthorized lookalikes cannot satisfy publication evidence
- And failed handoffs retain Result Documents, while a cleanup-only warning after verified completion never directs another verdict

Examples:
| condition | guidance |
| a repairable deterministic precondition fails, including a non-main Submission | fix_required with the precise allowed repair and retained Claim, not a normal review |
| publication already began and explicit resume cannot establish a fresh review | replay the original fixed-number submit and Result Documents, not a new head or round |
| an interrupted write is observed as the exact authorized effect | finish only missing effects and report completion only after verification |
| published evidence is changed, contradictory, or unverifiable | refusal or stop for explicit inspection, without overwrite or guessed release |
| verified completion leaves only private-file cleanup unsuccessful | completed outcome with cleanup-only warning, not a retry |

DOD: DOD9.

### Scenario Outline: B14 Report nonzero failures with truthful Claim certainty
- Given <failure> during Watchdog command execution
- When the command cannot produce a verified successful outcome
- Then it fails nonzero with a brief explanation and precise applicable stop or recovery instructions
- And it reports <certainty>, never fabricates a normal review, approval, rework, needs-human transition, or released Claim
- And it does not mutate Workflow Projections to conceal the failure or discard Result Documents needed for recovery

Examples:
| failure | certainty |
| dependency or backend failure known to precede acquisition | no Claim acquisition was attempted |
| Claim acquisition cannot be read back | Claim state is uncertain and requires explicit inspection |
| publication readback is unavailable | completed handoff and Claim release are not verified |

DOD: DOD9.

### Scenario Outline: B15 Keep explicit JSON equivalent without repeated effects
- Given equivalent isolated fixtures for <operation>
- When one uses default Markdown and the other requests `--format json`
- Then both carry equivalent instructions where applicable, fixed identities, status, applicable repair or next steps, and truthful Claim information
- And JSON is typed transport while default output is Markdown without a facts wrapper
- And each invocation performs only the Workflow effects required by its operation; serialization does not repeat selection, acquisition, inspection, or publication

Examples:
| operation |
| next with eligible review work |
| resume of the selected Claim |
| narrow post-preparation inspection or repair |
| submit with a verified outcome or deterministic refusal |
| next with no work or idle timeout |

DOD: DOD1, DOD10.

### Scenario Outline: B16 Reject invalid presentation input before avoidable effects
- Given <input> supplied to the relevant public Watchdog or resource command
- When input validation runs
- Then it fails nonzero with actionable input guidance before any avoidable selection, acquisition, publication, or private-result allocation
- And any existing Claim, published evidence, checkpoint, and Result Documents are unchanged

Examples:
| input |
| unsupported output format on next, resume, or submit |
| unknown, duplicate, missing required, or invalid typed input for the Watchdog review resource |

DOD: DOD10.

### Scenario Outline: B17 Explain no-work and wait outcomes explicitly
- Given <queue> and <request>
- When `skl watchdog next` completes
- Then Markdown reports <result> with a brief user explanation and the applicable stop or one-item procedure
- And no-work or timeout carries no fabricated review, Result Documents, or Claim-release assertion
- And waiting stops at the selected item rather than becoming a multi-item loop

Examples:
| queue | request | result |
| no eligible item | ordinary next | no_work and stop |
| no eligible item before the deadline | next with bounded wait | idle_timeout and stop |
| an eligible item appears before the deadline | next with bounded wait | one tailored Execution Skill after one successful acquisition |

DOD: DOD10.

## Feature: Public discovery and ownership-safe installation

### Scenario: B18 Refuse generic Watchdog retrieval without selecting work
- Given the public skl command surface
- When plain `skl skill watchdog` is requested in either presentation format
- Then it refuses with guidance to `skl watchdog next` and explicit resume, without selecting or claiming work
- And reasoning Skill Definitions and Watchdog named resources remain available through their normal retrieval commands
- And private modules are unavailable as resources and `skl watchdog start` remains unsupported

DOD: DOD11.

### Scenario Outline: B19 Install a direct one-item Watchdog stub
- Given a fresh or marker-owned Watchdog installation for <harness>
- When `skl install` installs or refreshes it
- Then the Skill Stub directly invokes `skl watchdog next`, not generic skill retrieval
- And following that command at the public command seam returns the tailored Execution Skill
- And the stub preserves single-item use and caller-provided fresh context without spawning a runner for direct invocation
- And user-owned files remain unchanged

Examples:
| harness |
| Pi |
| Codex |
| Claude Code |
| OpenCode |

DOD: DOD11.

### Scenario: B20 Disable the installed owned Pi loop while retaining one-item use
- Given an installed legacy marker-owned Pi Watchdog loop, a user-owned counterpart in a separate installation, and the other lane still referencing the shared queue helper
- When installation refresh runs
- Then the owned Watchdog loop is replaced with disabled guidance that stops before launching a runner or claiming work, not merely omitted from new installs
- And the user-owned counterpart is unchanged and repeated refresh remains safe
- And the one-item Watchdog runner still uses a fresh context, invokes next once, and reports normal Markdown only after respecting verified or refused outcomes, without a JSON relay contract
- And the other lane's loop and the shared queue helper remain usable while referenced, without requiring the implementation-rendering sibling or a Pi loop redesign

DOD: DOD12.
